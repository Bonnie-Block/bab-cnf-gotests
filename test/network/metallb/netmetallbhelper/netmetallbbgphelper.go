package netmetallbhelper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"text/template"
	"time"

	. "github.com/onsi/gomega"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	metallbv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateSpeakerBGPPeer creates BGP Peers on all worker nodes.
func CreateSpeakerBGPPeer(externalAddress string, asn uint32) error {
	bgppeer := defineSpeakerBGPPeer(externalAddress, asn)

	return helper.Apiclient.Create(context.Background(), bgppeer)
}

// defineSpeakerBGPPeer defines the bgppeer config.  Speakers are always AS 64500.  For EBGP
// connections the external FRR is changed.
func defineSpeakerBGPPeer(externalAddress string, asn uint32) *metallbv1beta1.BGPPeer {
	return &metallbv1beta1.BGPPeer{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "testpeer-",
			Namespace:    netmlbparameters.MetalLBOperatorNameSpace,
		},
		Spec: metallbv1beta1.BGPPeerSpec{
			MyASN:    netmlbparameters.IBGPASN,
			ASN:      asn,
			Address:  externalAddress,
			Port:     179,
			Password: netmlbparameters.BGPPassword,
		},
	}
}

// DefineFRRConfigMap returns configmap definition for the external FRR configuration.
func DefineFRRConfigMap(ipAddresses []string, configMapName string, asn int, protocol string) *k8sv1.ConfigMap {
	configMapData := make(map[string]string)

	configMapData["daemons"] = netmlbparameters.DaemonsFile
	configMapData["vtysh.conf"] = ""

	if protocol == netmlbparameters.BFDConfigPrefix {
		bfdConfig, err := defineBFDConfig(ipAddresses, asn)
		Expect(err).ToNot(HaveOccurred())

		configMapData["frr.conf"] = bfdConfig
	}

	if protocol == netmlbparameters.BGP {
		temp, err := template.New("bgp Config Template").Parse(netmlbparameters.BgpConfigTemplate)
		Expect(err).ToNot(HaveOccurred())

		router := netmlbparameters.NeighborConfig{
			Addr1:    ipAddresses[0],
			Addr2:    ipAddresses[1],
			ASN:      netmlbparameters.IBGPASN,
			Password: netmlbparameters.BGPPassword}

		var bfdConfig bytes.Buffer
		err = temp.Execute(&bfdConfig, router)
		Expect(err).ToNot(HaveOccurred())

		configMapData["frr.conf"] = bfdConfig.String()
	}

	return &k8sv1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: netmlbparameters.TestNamespace,
		},
		Data: configMapData,
	}
}

// defineBFDConfig returns string which represents BFD config file peering to all given IP addresses.
func defineBFDConfig(neighborsIPAddresses []string, asn int) (string, error) {
	if len(neighborsIPAddresses) < 1 {
		return "", fmt.Errorf("list of neigbors ip addresses is empty")
	}

	asnStr := strconv.Itoa(asn)
	bfdConfig := "bfd\n router bgp 64501\n"

	for _, ipAddress := range neighborsIPAddresses {
		bfdConfig += fmt.Sprintf(" neighbor %s remote-as %s\n neighbor %s bfd\n", ipAddress, asnStr, ipAddress)
	}

	bfdConfig += "!"

	return bfdConfig, nil
}

// CheckNeighborsStatus returns informations for the all the neighbors in the given
// executor.
func CheckNeighborsStatus(frrPod *k8sv1.Pod, neighborsIPAddresses []string) bool {
	neighborState, err := pod.ExecCommand(helper.Apiclient, *frrPod,
		[]string{"vtysh", "-u", "-c", "show bgp neighbor json"})
	Expect(err).ToNot(HaveOccurred())

	parseNeigh := parseNeighbors(neighborState.String())

	if len(parseNeigh) != 2 {
		fmt.Printf("Expected 2 neighbours, got %d\n", len(parseNeigh))

		return false
	}

	sort.Slice(parseNeigh, func(i, j int) bool {
		return (bytes.Compare(parseNeigh[i].IP, parseNeigh[j].IP) < 0)
	})

	if !parseNeigh[0].IP.Equal(net.ParseIP(neighborsIPAddresses[0])) {
		fmt.Printf("neighbour %s ip not matching\n", neighborsIPAddresses[0])

		return false
	}

	if !parseNeigh[1].IP.Equal(net.ParseIP(neighborsIPAddresses[1])) {
		fmt.Printf("neighbour %s ip not matching\n", neighborsIPAddresses[1])

		return false
	}

	return true
}

// parseNeighbour takes the result of a show bgp neighbor
// and parses the informations related to all the neighbours.
func parseNeighbors(vtyshRes string) []*netmlbparameters.Neighbor {
	neighborList := map[string]netmlbparameters.FRRNeighbor{}
	err := json.Unmarshal([]byte(vtyshRes), &neighborList)
	Expect(err).ToNot(HaveOccurred())

	res := make([]*netmlbparameters.Neighbor, 0)

	for k, neigh := range neighborList {
		ipAdd := net.ParseIP(k)
		if ipAdd == nil {
			return nil
		}

		connected := false

		if neigh.BgpState == netmlbparameters.BGPStateEstablished {
			connected = true
		}

		prefixSent := 0
		for _, s := range neigh.AddressFamilyInfo {
			prefixSent += s.SentPrefixCounter
		}
		res = append(res, &netmlbparameters.Neighbor{
			IP:          ipAdd,
			Connected:   connected,
			LocalAS:     strconv.Itoa(neigh.LocalAs),
			RemoteAS:    strconv.Itoa(neigh.RemoteAs),
			UpdatesSent: neigh.MessageStats.UpdatesSent,
			PrefixSent:  prefixSent,
			Port:        neigh.PortForeign,
		})
	}

	return res
}

// CheckBGPRoutes returns informations about routes in the external frr container
// first for ipv4 routes and then for ipv6 routes.
func CheckBGPRoutes(frrPod *k8sv1.Pod, neighborsIPAddresses []string, prefixList []string) bool {
	bgpStateOut, err := pod.ExecCommand(helper.Apiclient, *frrPod, []string{"vtysh", "-u", "-c", "show bgp ipv4 json"})
	Expect(err).ToNot(HaveOccurred())

	routes, err := parseRoutes(bgpStateOut.String())
	Expect(err).ToNot(HaveOccurred(), "Failed to parse %s", err)

	for _, prefix := range prefixList {
		ipRoutes, routePrefix := routes[prefix]
		if !routePrefix {
			fmt.Printf("Route %s not found\n", prefix)

			return false
		}

		ips := make([]net.IP, 0)
		ips = append(ips, ipRoutes.NextHops...)

		sort.Slice(ips, func(i, j int) bool {
			return (bytes.Compare(ips[i], ips[j]) < 0)
		})

		if !ips[0].Equal(net.ParseIP(neighborsIPAddresses[0])) {
			fmt.Printf("neighbour %s ip not matching\n", neighborsIPAddresses[0])

			return false
		}

		if !ips[1].Equal(net.ParseIP(neighborsIPAddresses[1])) {
			fmt.Printf("neighbour %s ip not matching\n", neighborsIPAddresses[1])

			return false
		}
	}

	return true
}

// parseRoute takes the result of a show bgp neighbor
// and parses the informations related to all the neighbours.
func parseRoutes(vtyshRes string) (map[string]netmlbparameters.Route, error) {
	toParse := netmlbparameters.IPInfo{}
	err := json.Unmarshal([]byte(vtyshRes), &toParse)

	if err != nil {
		return nil, err
	}

	res := make(map[string]netmlbparameters.Route)

	for k, frrRoutes := range toParse.Routes {
		destIP, dest, err := net.ParseCIDR(k)

		if err != nil {
			return nil, err
		}

		route := netmlbparameters.Route{
			Destination: dest,
			NextHops:    make([]net.IP, 0),
		}
		for _, n := range frrRoutes {
			route.LocalPref = n.LocalPref

			for _, nexthop := range n.Nexthops {
				ipAdd := net.ParseIP(nexthop.IP)
				if ipAdd == nil {
					return nil, fmt.Errorf("failed to parse ip %s", nexthop.IP)
				}

				if ipAdd.To4() == nil && nexthop.Scope == "link-local" {
					continue
				}
				route.NextHops = append(route.NextHops, ipAdd)
			}
		}

		res[destIP.String()] = route
	}

	return res, nil
}

// DeleteAllBGPPeers removes all BGPPeer CRs.
func DeleteAllBGPPeers() {
	bgpPeerList := metallbv1beta1.BGPPeerList{}

	err := helper.Apiclient.List(context.Background(), &bgpPeerList,
		runtimeclient.InNamespace(netmlbparameters.MetalLBOperatorNameSpace))
	Expect(err).ToNot(HaveOccurred())

	for _, bgpPeer := range bgpPeerList.Items {
		err = helper.Apiclient.Delete(context.Background(), &bgpPeer)
		Expect(err).ToNot(HaveOccurred())
	}

	Eventually(func() bool {
		return IsProtocolConfigured(netmlbparameters.BGPConfigPrefix)
	}, 1*time.Minute, 2*time.Second).Should(BeFalse(), "BGP configuration is not removed")
}
