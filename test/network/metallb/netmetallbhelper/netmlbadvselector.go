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
	"go.universe.tf/metallb/api/v1beta1"
	k8sv1 "k8s.io/api/core/v1"
)

func TestBGPPeerSpecificIPAddressPools(
	ipStack string, workerNodeList []k8sv1.Node, masterNodeList []k8sv1.Node, bgpASN int, trafficPolicy string) {
	ipv4metalLBIPList, ipv6metalLBIPList, err := GetMetalLBIPByFamily()
	Expect(err).ToNot(HaveOccurred())

	clusterIPStack := ValidateClusterIPStack()

	switch ipStack {
	case netparameters.IPV4Family:
		if clusterIPStack == netparameters.IPV6Family {
			Skip("Cluster does not support IPv4")
		}
	case netparameters.IPV6Family:
		if clusterIPStack == netparameters.IPV4Family {
			Skip("Cluster does not support IPv6")
		}
	case netparameters.DualIPFamily:
		if clusterIPStack != netparameters.DualIPFamily {
			Skip("Cluster does not support both IPv4 and IPv6")
		}
	}

	By("should create two IPAddressPools")

	err = defineCreateIPAddressPools(ipStack, netmlbparameters.AddressPoolS1, netmlbparameters.AddressPoolS1Name)
	Expect(err).ToNot(HaveOccurred())

	err = defineCreateIPAddressPools(ipStack, netmlbparameters.AddressPoolS2, netmlbparameters.AddressPoolS2Name)
	Expect(err).ToNot(HaveOccurred())

	By("should create two BGP Advertisements with separate IPAddressPools and BGP Peer Spec")

	bgpAdvertisementDefinition1 := defineBGPAdvertisementWithPeer(netmlbparameters.BGPAdvertisementName,
		netmlbparameters.AddressPoolS1Name, netmlbparameters.BGPPeerName1v4, ipStack)

	err = helper.Apiclient.Create(context.Background(), bgpAdvertisementDefinition1)
	Expect(err).ToNot(HaveOccurred())

	bgpAdvertisementDefinition2 := defineBGPAdvertisementWithPeer(netmlbparameters.BGPAdvertisement2Name,
		netmlbparameters.AddressPoolS2Name, netmlbparameters.BGPPeerName2v4, ipStack)

	err = helper.Apiclient.Create(context.Background(), bgpAdvertisementDefinition2)
	Expect(err).ToNot(HaveOccurred())

	By("should create two service each with 2 backend pods")

	err = defineCreateServicesAndTestPods(ipStack, netmlbparameters.AddressPoolS1Name, netmlbparameters.AppLabel1,
		netmlbparameters.ProtocolTCP, trafficPolicy, workerNodeList)
	Expect(err).ToNot(HaveOccurred())

	err = defineCreateServicesAndTestPods(ipStack, netmlbparameters.AddressPoolS1Name, netmlbparameters.AppLabel1,
		netmlbparameters.ProtocolSCTP, trafficPolicy, workerNodeList)
	Expect(err).ToNot(HaveOccurred())

	err = defineCreateServicesAndTestPods(ipStack, netmlbparameters.AddressPoolS2Name, netmlbparameters.AppLabel2,
		netmlbparameters.ProtocolTCP, trafficPolicy, workerNodeList)
	Expect(err).ToNot(HaveOccurred())

	err = defineCreateServicesAndTestPods(ipStack, netmlbparameters.AddressPoolS2Name, netmlbparameters.AppLabel2,
		netmlbparameters.ProtocolSCTP, trafficPolicy, workerNodeList)
	Expect(err).ToNot(HaveOccurred())

	By("should create BGPPeers on Speakers")

	workerNodesAdresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)
	bgpPeerName := netmlbparameters.BGPPeerName1v4

	var ipv6Address string

	if ipStack != netparameters.IPV4Family {
		workerNodesV6Adresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV6Family)
		workerNodesAdresses = append(workerNodesAdresses, workerNodesV6Adresses...)
		bgpPeerName = netmlbparameters.BGPPeerName1v6
		ipv6Address = ipv6metalLBIPList[0]
	}

	err = CreateSpeakerBGPPeerIPStack(ipStack, ipv4metalLBIPList[0], ipv6Address, bgpASN, bgpPeerName)
	Expect(err).ToNot(HaveOccurred())

	bgpPeerName = netmlbparameters.BGPPeerName2v4

	if ipStack != netparameters.IPV4Family {
		bgpPeerName = netmlbparameters.BGPPeerName2v6
		ipv6Address = ipv6metalLBIPList[1]
	}

	err = CreateSpeakerBGPPeerIPStack(ipStack, ipv4metalLBIPList[1], ipv6Address, bgpASN, bgpPeerName)
	Expect(err).ToNot(HaveOccurred())

	By("should create 2 external FRR container")

	masterNodeFRRPod := createExternalFRRs(masterNodeList, workerNodeList, ipv4metalLBIPList,
		ipv6metalLBIPList, bgpASN, ipStack)

	By("should validate BGP specific route to external FRR from Master 0")

	validateBGPPeerSelectorRoutes(masterNodeFRRPod[0], workerNodesAdresses, ipStack,
		netmlbparameters.AddressPoolS1, true)

	By("should validate BGP specific route to external FRR from Master 1")

	validateBGPPeerSelectorRoutes(masterNodeFRRPod[1], workerNodesAdresses, ipStack,
		netmlbparameters.AddressPoolS2, true)

	By("should validate BGP specific routes were not propagated to external FRR on Master 0")

	validateBGPPeerSelectorRoutes(masterNodeFRRPod[0], workerNodesAdresses, ipStack,
		netmlbparameters.AddressPoolS2, false)

	By("should validate BGP specific routes were not propagated to external FRR on Master 1")

	validateBGPPeerSelectorRoutes(masterNodeFRRPod[1], workerNodesAdresses, ipStack,
		netmlbparameters.AddressPoolS1, false)

	By("should validate http and sctp traffic from external FFR on Master 0")

	validateTraffic(&masterNodeFRRPod[0], ipv4metalLBIPList[0], ipv6Address,
		ipStack, netmlbparameters.AddressPoolS1)

	By("should validate http and sctp traffic from external FFR on Master 1")

	validateTraffic(&masterNodeFRRPod[1], ipv4metalLBIPList[1], ipv6Address,
		ipStack, netmlbparameters.AddressPoolS2)
}

func defineCreateIPAddressPools(ipStack string, addressPoolList []string, addressPoolName string) error {
	defIPAddressPool := DefineMetalLBIPAddressPool(
		addressPoolList,
		ipStack,
		addressPoolName)

	err := helper.Apiclient.Create(context.Background(), defIPAddressPool)
	if err != nil {
		return err
	}

	return nil
}

func validateBGPPeerSelectorRoutes(masterNodeFRRPod k8sv1.Pod, workerNodesAdresses []string,
	ipStack string, addressPool []string, advertise bool) {
	route := []string{addressPool[0]}

	switch ipStack {
	case netparameters.IPV6Family:
		route = []string{addressPool[2]}
	case netparameters.DualIPFamily:
		route = []string{addressPool[0], addressPool[2]}
	}

	switch advertise {
	case false:
		Eventually(func() error {
			err := CheckBGPRoutes(&masterNodeFRRPod, workerNodesAdresses, route, ipStack, netmlbparameters.PrefixLen32)

			return err
		}, 1*time.Minute, 2*time.Second).Should(Equal(fmt.Errorf("route %s not found", route[0])))

	case true:
		Eventually(func() error {
			err := CheckBGPRoutes(&masterNodeFRRPod, workerNodesAdresses, route, ipStack, netmlbparameters.PrefixLen32)

			return err
		}, 1*time.Minute, 2*time.Second).ShouldNot(HaveOccurred(), "error checking BGP route")
	}
}

func defineCreateServicesAndTestPods(ipStack string, addressPoolName string, appLabelName string, protocolType string,
	trafficPolicy string, workerNodeList []k8sv1.Node) error {
	err := DefineAndCreateLBService(
		netmlbparameters.TestNamespace,
		ipStack,
		addressPoolName,
		appLabelName,
		protocolType,
		k8sv1.ServiceExternalTrafficPolicyType(trafficPolicy))

	if err != nil {
		return fmt.Errorf("error defining LB tcp service for %s - %w", addressPoolName, err)
	}

	DefineAndRunMlbServerPod(workerNodeList[0].Name,
		helper.Config.Network.TestContainerImage,
		appLabelName, []string{netmlbparameters.ArgCommandSCTPNGINX})

	DefineAndRunMlbServerPod(workerNodeList[1].Name,
		helper.Config.Network.TestContainerImage,
		appLabelName, []string{netmlbparameters.ArgCommandSCTPNGINX})

	return nil
}

func createExternalFRRs(masterNodeList []k8sv1.Node, workerNodeList []k8sv1.Node, ipv4metalLBIPList []string,
	ipv6metalLBIPList []string, bgpASN int, ipStack string) []k8sv1.Pod {
	var ipv6Address string

	if ipStack != netparameters.IPV4Family {
		ipv6Address = ipv6metalLBIPList[0]
	}

	masterNodeFRRPod1 := CreateFRRContainerOnMaster(workerNodeList, masterNodeList[0],
		ipv4metalLBIPList[0], ipv6Address, ipStack, bgpASN,
		netmlbparameters.ExternalNADName, netparameters.MasterConfigMapName, netmlbparameters.PropagateFalse)

	ipv4metalLBIPList = []string{ipv4metalLBIPList[1]}

	masterNodeFRRPod2 := CreateFRRContainerOnMaster(workerNodeList, masterNodeList[1], ipv4metalLBIPList[0],
		ipv6Address, ipStack, bgpASN, netmlbparameters.External2NADName,
		netparameters.Master2ConfigMapName, netmlbparameters.PropagateFalse)

	return []k8sv1.Pod{*masterNodeFRRPod1, *masterNodeFRRPod2}
}

func defineBGPAdvertisementWithPeer(bgpAdvertisementName string, addressPoolName string,
	bgpPeerName string, ipStack string) *v1beta1.BGPAdvertisement {
	bgpAdvertisementDefinition := DefineBGPAdvertisement(bgpAdvertisementName,
		[]string{addressPoolName},
		ipStack,
		netmlbparameters.PrefixLen32)
	bgpAdvertisementDefinition.Spec.Peers = []string{bgpPeerName}

	return bgpAdvertisementDefinition
}
