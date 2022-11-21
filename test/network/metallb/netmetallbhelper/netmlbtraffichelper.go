package netmetallbhelper

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
)

func TestBGPTable(ipStack string, workerNodeList []k8sv1.Node, masterNodeList []k8sv1.Node,
	trafficPolicyName string, bgpASN int) {
	ipv4metalLBIPList, ipv6metalLBIPList, err := GetMetalLBIPByFamily()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("An unexpected error occurred while"+
		" determining the IP addresses from the METALLB_ADDR_LIST environment variable.: %s", err))

	By("should create external FRR container")

	var ipv6Address string

	clusterIPStack := ValidateClusterIPStack()

	if clusterIPStack != netparameters.IPV4Family {
		ipv6Address = ipv6metalLBIPList[0]
	}

	masterNodeFRRPod := CreateFRRContainerOnMaster(workerNodeList, masterNodeList[0],
		ipv4metalLBIPList[0], ipv6Address,
		ipStack, bgpASN, netmlbparameters.ExternalNADName, netparameters.MasterConfigMapName,
		netmlbparameters.PropagateFalse)

	By("should create an IPAddressPool")

	ipAddressPoolIPList := netmlbparameters.AddressPoolS1

	if ipStack == netparameters.IPV6Family {
		ipAddressPoolIPList = netmlbparameters.AddressPool1V6
	}

	ipAddressPool := DefineMetalLBIPAddressPool(
		ipAddressPoolIPList,
		ipStack,
		netmlbparameters.AddressPoolS1Name)

	err = helper.Apiclient.Create(context.Background(), ipAddressPool)
	Expect(err).ToNot(HaveOccurred())

	By("should create a BGPAdvertisement with BGP")

	bgpAdvertisement := DefineBGPAdvertisement(netmlbparameters.BGPAdvertisementName, []string{ipAddressPool.Name},
		ipStack, netmlbparameters.PrefixLen32, netmlbparameters.LocalPref100)
	err = helper.Apiclient.Create(context.Background(), bgpAdvertisement)
	Expect(err).ToNot(HaveOccurred())

	By("should create service with 2 backend pods")

	err = DefineAndCreateLBService(
		netmlbparameters.TestNamespace,
		ipStack,
		netmlbparameters.AddressPoolS1Name,
		netmlbparameters.AppLabel1,
		netmlbparameters.ProtocolTCP,
		k8sv1.ServiceExternalTrafficPolicyType(trafficPolicyName))
	Expect(err).ToNot(HaveOccurred())

	err = DefineAndCreateLBService(
		netmlbparameters.TestNamespace,
		ipStack,
		netmlbparameters.AddressPoolS1Name,
		netmlbparameters.AppLabel1,
		netmlbparameters.ProtocolSCTP,
		k8sv1.ServiceExternalTrafficPolicyType(trafficPolicyName))
	Expect(err).ToNot(HaveOccurred())

	DefineAndRunMlbServerPod(workerNodeList[0].Name,
		helper.Config.Network.TestContainerImage,
		netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandSCTPNGINX})

	DefineAndRunMlbServerPod(workerNodeList[1].Name,
		helper.Config.Network.TestContainerImage,
		netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandSCTPNGINX})

	By("should create a BGP Peer on Speakers")

	bgpPeerName := netmlbparameters.BGPPeerName1v4
	workerNodesAdresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)

	if ipStack != netparameters.IPV4Family {
		workerNodesV6Adresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV6Family)
		workerNodesAdresses = append(workerNodesAdresses, workerNodesV6Adresses...)
		bgpPeerName = netmlbparameters.BGPPeerName1v6
		ipv6Address = ipv6metalLBIPList[0]
	}

	err = CreateSpeakerBGPPeerIPStack(ipStack, ipv4metalLBIPList[0], ipv6Address, bgpASN, bgpPeerName)
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() bool {
		return CheckNeighborsStatus(masterNodeFRRPod, ipStack,
			workerNodesAdresses)
	}, 1*time.Minute, netmlbparameters.Interval).Should(BeTrue())

	By("should validate BGP routes to service")

	validateRoute(masterNodeFRRPod, workerNodesAdresses, ipStack)

	By("should validate Traffic")

	validateTraffic(masterNodeFRRPod, ipv4metalLBIPList[0], ipv6Address, ipStack, ipAddressPoolIPList)
}

// validateTraffic run both curl and sctp end to end traffic.
func validateTraffic(masterFRRPod *k8sv1.Pod,
	ipv4metalLBIP string, ipv6metalLBIP string, ipStack string, addressPool []string) {
	_, err := curlService(masterFRRPod, ipStack, ipv4metalLBIP, ipv6metalLBIP, addressPool)
	Expect(err).ToNot(HaveOccurred(), "error attempting to curl server test pod")

	err = sctpToService(masterFRRPod, ipStack, addressPool)
	Expect(err).ToNot(HaveOccurred(), "error attempting to sctp server test pod")
}

func validateRoute(masterFRRPod *k8sv1.Pod, nodeIPAdresses []string, ipStack string) {
	var routes []string

	routes, ipFamily := defineIPRouteFamily(ipStack)

	if ipStack == netparameters.DualIPFamily {
		Eventually(func() error {
			return CheckBGPRoutesMultipleNodes(
				masterFRRPod,
				nodeIPAdresses,
				routes,
				netparameters.IPV4Family,
				netmlbparameters.PrefixLen32)
		}, 2*time.Minute, netmlbparameters.TimeoutBFDBGP).ShouldNot(HaveOccurred())
	}

	Eventually(func() error {
		return CheckBGPRoutesMultipleNodes(
			masterFRRPod,
			nodeIPAdresses,
			routes,
			ipFamily,
			netmlbparameters.PrefixLen32)
	}, 2*time.Minute, netmlbparameters.TimeoutBFDBGP).ShouldNot(HaveOccurred())
}

func curlService(frrPod *k8sv1.Pod, ipStack string, ipv4metalLBIP string,
	ipv6metalLBIP string, addressPoolIPs []string) (string, error) {
	switch ipStack {
	case netparameters.IPV4Family:
		return HTTPMlbPod(frrPod, ipv4metalLBIP, addressPoolIPs[0],
			netparameters.IPV4Family, netmlbparameters.TestContainerName, netmlbparameters.BGP)

	case netparameters.IPV6Family:
		return HTTPMlbPod(frrPod, ipv6metalLBIP, addressPoolIPs[2],
			netparameters.IPV6Family, netmlbparameters.TestContainerName, netmlbparameters.BGP)
	}

	httpOutput, err := HTTPMlbPod(frrPod, ipv6metalLBIP, addressPoolIPs[2],
		netparameters.IPV6Family, netmlbparameters.TestContainerName, netmlbparameters.BGP)
	if err != nil {
		return httpOutput, err
	}

	return HTTPMlbPod(frrPod, ipv6metalLBIP, addressPoolIPs[2],
		netparameters.IPV6Family, netmlbparameters.TestContainerName, netmlbparameters.BGP)
}

func defineIPRouteFamily(ipStack string) ([]string, string) {
	routesV4 := []string{netmlbparameters.AddressPoolS1[0]}
	routesV6 := []string{netmlbparameters.AddressPoolS1[2]}

	if ipStack == netparameters.IPV4Family {
		return routesV4, netparameters.IPV4Family
	}

	return routesV6, netparameters.IPV6Family
}

// CreateSpeakerBGPPeerIPStack creates a BGP Peer CRD.
func CreateSpeakerBGPPeerIPStack(ipStack string, ipv4metalLBIP string, ipv6metalLBIP string,
	bgpASN int, bgpPeerName string) error {
	if ipStack == netparameters.IPV4Family {
		return CreateSpeakerBGPPeer(ipv4metalLBIP, uint32(bgpASN), bgpPeerName)
	}

	if ipStack == netparameters.IPV6Family {
		return CreateSpeakerBGPPeer(ipv6metalLBIP, uint32(bgpASN), bgpPeerName)
	}

	err := CreateSpeakerBGPPeer(ipv4metalLBIP, uint32(bgpASN), bgpPeerName)
	if err == nil {
		return err
	}

	return nil
}

// DefineAnnotationWithIPStack creates an IP annotation for the pod network.
func DefineAnnotationWithIPStack(ipStack string,
	ipv4metalLBIP string, ipv6metalLBIP string,
	clusterIPStack string, nadName string) string {
	if ipStack == netparameters.IPV4Family {
		return fmt.Sprintf(`[{"name": "%s", "ips": ["%s/%s"]}]`, nadName, ipv4metalLBIP, netparameters.IPSubnet24)
	}

	if clusterIPStack == netparameters.IPV4Family {
		Skip("Cluster does not support IPv6")
	}

	if ipStack == netparameters.IPSubnet64 {
		return fmt.Sprintf(`["%s/%s"]}]`, ipv6metalLBIP, netparameters.IPSubnet64)
	}

	return fmt.Sprintf(`["%s/%s","%s/%s"]}]`, ipv4metalLBIP,
		netparameters.IPSubnet24, ipv6metalLBIP, netparameters.IPSubnet64)
}

func sctpToService(masterFRRPod *k8sv1.Pod, ipStack string, addressPool []string) error {
	var (
		externalLBIPList []string
	)

	switch ipStack {
	case netparameters.DualIPFamily:
		externalLBIPList = append(externalLBIPList, addressPool[1], addressPool[3])
	case netparameters.IPV4Family:
		externalLBIPList = append(externalLBIPList, addressPool[1])
	case netparameters.IPV6Family:
		externalLBIPList = append(externalLBIPList, addressPool[3])
	}

	for _, exteranlLBIP := range externalLBIPList {
		_, err := pod.ExecCommand(helper.Apiclient, *masterFRRPod, []string{"/bin/bash", "-c",
			fmt.Sprintf(netmlbparameters.ArgCommandServerSCTP,
				exteranlLBIP)},
			netmlbparameters.TestContainerName)

		if err != nil {
			return err
		}
	}

	return nil
}
