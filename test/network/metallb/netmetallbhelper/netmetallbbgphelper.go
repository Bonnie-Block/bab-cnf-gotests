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

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	metallbv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// CreateSpeakerBGPPeer creates BGP Peers on all worker nodes.
func CreateSpeakerBGPPeer(externalAddress string, bgpProtocol string, asn uint32) error {
	return helper.Apiclient.Create(context.Background(), defineSpeakerBGPPeer(externalAddress, asn, "", ""))
}

// DefineBGPFRRConfigMap returns configmap definition for the external FRR configuration.
func DefineBGPFRRConfigMap(ipAddresses []string, configMapName string) *k8sv1.ConfigMap {
	configMapData := make(map[string]string)

	configMapData["daemons"] = netmlbparameters.DaemonsFile
	configMapData["vtysh.conf"] = ""

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

// DefineFRRBGPConfigMap returns configmap definition for the external FRR BGP configuration.
func DefineFRRBGPConfigMap(ipAddresses []string, configMapName string, localAS int,
	protocol string, ipStack string) *k8sv1.ConfigMap {
	configMapData := make(map[string]string)

	var router netmlbparameters.NeighborConfig

	configMapData["daemons"] = netmlbparameters.DaemonsFile
	configMapData["vtysh.conf"] = ""

	if protocol == netmlbparameters.BGP {
		temp, err := template.New("bgp Config Template").Parse(netmlbparameters.BgpConfigTemplate)
		Expect(err).ToNot(HaveOccurred())

		switch ipStack {
		case netmlbparameters.SingleIPv4Stack:
			router = netmlbparameters.NeighborConfig{
				Addr1: ipAddresses[0],
				Addr2: ipAddresses[1]}

		case netmlbparameters.SingleIPv6Stack:
			router = netmlbparameters.NeighborConfig{
				Addr3: ipAddresses[2],
				Addr4: ipAddresses[3]}

		case netmlbparameters.DualIPStack:
			router = netmlbparameters.NeighborConfig{
				Addr1: ipAddresses[0],
				Addr2: ipAddresses[1],
				Addr3: ipAddresses[2],
				Addr4: ipAddresses[3]}

		default:
			Fail("Invalid or no IPStack is configured")
		}

		router.ASN = uint32(localAS)
		router.Password = netmlbparameters.BGPPassword

		var bfdConfig bytes.Buffer
		err = temp.Execute(&bfdConfig, router)
		Expect(err).ToNot(HaveOccurred())

		configMapData["frr.conf"] = bfdConfig.String()
	}

	bgpConfigMap := nethelper.DefineFRRBFDConfigMap(configMapName, netmlbparameters.TestNamespace, configMapData)

	return bgpConfigMap
}

// CheckNeighborsStatus returns informations for the all the neighbors in the given
// executor.
func CheckNeighborsStatus(frrPod *k8sv1.Pod, ipStack string, neighborsIPAddresses []string) bool {
	neighborState, err := pod.ExecCommand(helper.Apiclient, *frrPod,
		[]string{"vtysh", "-u", "-c", "show ip bgp neighbor json"})
	Expect(err).ToNot(HaveOccurred())

	parseNeigh := parseNeighbors(neighborState.String())

	sort.Slice(parseNeigh, func(i, j int) bool {
		return (bytes.Compare(parseNeigh[i].IP, parseNeigh[j].IP) < 0)
	})

	workerNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
	Expect(err).ToNot(HaveOccurred())

	switch ipStack {
	case netmlbparameters.SingleIPv4Stack:
		if len(parseNeigh) != len(workerNodeList) {
			fmt.Printf("Expected %d neighbours, got %d\n", len(workerNodeList), len(parseNeigh))

			return false
		}

		if !parseNeigh[0].IP.Equal(net.ParseIP(neighborsIPAddresses[0])) {
			fmt.Printf("neighbour %s ip not matching\n", neighborsIPAddresses[0])

			return false
		}

		if !parseNeigh[1].IP.Equal(net.ParseIP(neighborsIPAddresses[1])) {
			fmt.Printf("neighbour %s ip not matching\n", neighborsIPAddresses[1])

			return false
		}

	case netmlbparameters.SingleIPv6Stack:
		if len(parseNeigh) != len(workerNodeList) {
			fmt.Printf("Expected %d neighbours, got %d\n", len(workerNodeList), len(parseNeigh))

			return false
		}

		if !parseNeigh[0].IP.Equal(net.ParseIP(neighborsIPAddresses[2])) {
			fmt.Printf("neighbour %s ip not matching\n", neighborsIPAddresses[2])

			return false
		}

		if !parseNeigh[1].IP.Equal(net.ParseIP(neighborsIPAddresses[3])) {
			fmt.Printf("neighbour %s ip not matching\n", neighborsIPAddresses[3])

			return false
		}

	case netmlbparameters.DualIPStack:
		if len(parseNeigh) != len(workerNodeList)*2 {
			fmt.Printf("Expected 4 IPv64neighbours, got %d\n", len(parseNeigh))

			return false
		}

		if !parseNeigh[0].IP.Equal(net.ParseIP(neighborsIPAddresses[0])) {
			fmt.Printf("neighbour %s ip not matching\n", neighborsIPAddresses[0])

			return false
		}

		if !parseNeigh[1].IP.Equal(net.ParseIP(neighborsIPAddresses[1])) {
			fmt.Printf("neighbour %s ip not matching\n", neighborsIPAddresses[1])

			return false
		}

		if !parseNeigh[2].IP.Equal(net.ParseIP(neighborsIPAddresses[2])) {
			fmt.Printf("neighbour %s ip not matching\n", neighborsIPAddresses[2])

			return false
		}

		if !parseNeigh[3].IP.Equal(net.ParseIP(neighborsIPAddresses[3])) {
			fmt.Printf("neighbour %s ip not matching\n", neighborsIPAddresses[3])

			return false
		}
	default:
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

// CheckBGPRoutes returns informations about routes in the external frr container
// first for ipv4 routes and then for ipv6 routes.
func CheckBGPRoutes(frrPod *k8sv1.Pod, neighborsIPAddresses []string, prefixList []string, iPFamily string) error {
	command := []string{"vtysh", "-u", "-c", fmt.Sprintf("show bgp %s json", iPFamily)}

	// bgpStateOut example output after being parsed - map[4.4.4.100:{4.4.4.100/32 [10.46.55.116 10.46.55.115] 100}]
	bgpStateOut, err := pod.ExecCommand(helper.Apiclient, *frrPod, command)
	Expect(err).ToNot(HaveOccurred())

	routes, err := parseRoutes(bgpStateOut.String())
	Expect(err).ToNot(HaveOccurred(), "Failed to parse %s", err)
	fmt.Println(routes)

	for _, prefix := range prefixList {
		ipRoutes, routePrefix := routes[prefix]
		if !routePrefix {
			return fmt.Errorf("route %s not found", prefix)
		}

		ips := make([]net.IP, 0)
		ips = append(ips, ipRoutes.NextHops...)

		sort.Slice(ips, func(i, j int) bool {
			return (bytes.Compare(ips[i], ips[j]) < 0)
		})

		if iPFamily == netparameters.IPV4Family {
			if !ips[0].Equal(net.ParseIP(neighborsIPAddresses[0])) {
				return fmt.Errorf("neighbour %s ip not matching", neighborsIPAddresses[0])
			}

			if !ips[1].Equal(net.ParseIP(neighborsIPAddresses[1])) {
				return fmt.Errorf("neighbour %s ip not matching", neighborsIPAddresses[1])
			}
		}

		if iPFamily == netmlbparameters.IPV6Family {
			if !ips[0].Equal(net.ParseIP(neighborsIPAddresses[2])) {
				return fmt.Errorf("neighbour %s ip not matching", neighborsIPAddresses[2])
			}

			if !ips[1].Equal(net.ParseIP(neighborsIPAddresses[3])) {
				return fmt.Errorf("neighbour %s ip not matching", neighborsIPAddresses[3])
			}
		}
	}

	return err
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
func DeleteAllBGPPeers() error {
	bgpPeerList := metallbv1beta1.BGPPeerList{}

	err := helper.Apiclient.List(context.Background(), &bgpPeerList,
		runtimeclient.InNamespace(netmlbparameters.MetalLBOperatorNameSpace))
	if err != nil {
		return err
	}

	for _, bgpPeer := range bgpPeerList.Items {
		err = helper.Apiclient.Delete(context.Background(), &bgpPeer)
		if err != nil {
			return err
		}
	}

	Eventually(func() bool {
		return IsProtocolConfigured(netmlbparameters.BGPConfigPrefix)
	}, 1*time.Minute, 2*time.Second).Should(BeFalse(), "BGP configuration is not removed")

	return nil
}

// DefineFrrPodWithTestContainer creates an FRR Pod with a test container.
func DefineFrrPodWithTestContainer(masterNodeName string, namespace string) *k8sv1.Pod {
	frrPod := nethelper.DefineFRRPod(masterNodeName, namespace, false)

	frrPod.Spec.Containers = append(frrPod.Spec.Containers,
		k8sv1.Container{
			Name:  netmlbparameters.TestContainerName,
			Image: helper.Config.Network.TestContainerImage,
			SecurityContext: &k8sv1.SecurityContext{
				Capabilities: &k8sv1.Capabilities{
					Add: []k8sv1.Capability{
						"NET_ADMIN",
						"NET_RAW",
						"SYS_ADMIN",
					},
				},
			},
			Command: parameters.SleepCommand,
		},
	)

	return frrPod
}

// CreateParametersInJSON validates given parameters and returns json formatted string.
func CreateParametersInJSON(ipStack string, trafficPolicy string) string {
	BGPParameters, err := netmlbparameters.NewBGPTestParameters(ipStack, trafficPolicy)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error in parameters: ipStack=%s, "+
		"trafficPolicy=%s", ipStack, trafficPolicy))

	params, err := json.Marshal(BGPParameters)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error in parameters: ipStack=%s, "+
		"trafficPolicy=%s", ipStack, trafficPolicy))

	return string(params)
}
