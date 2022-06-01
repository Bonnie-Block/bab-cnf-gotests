package netmetallbhelper

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
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
	clusterIPStack := ValidateClusterIPStack()
	if clusterIPStack != netparameters.IPV4Family {
		if ipStack != netparameters.IPV4Family &&
			bgpASN == netmlbparameters.EBGPASN {
			Fail("Test Skipped - BZ - https://bugzilla.redhat.com/show_bug.cgi?id=2063720")
		}
	}

	ipv4metalLBIPList, ipv6metalLBIPList, err := GetMetalLBIPByFamily()
	Expect(err).ToNot(HaveOccurred())

	By("should create external FRR container")

	masterNodeFRRPod := CreateFRRContainerOnMaster(workerNodeList, masterNodeList,
		ipv4metalLBIPList, ipv6metalLBIPList,
		ipStack, bgpASN,
		netmlbparameters.PropagateFalse)

	By("should create an IPAddressPool and BGPAdvertisement for service")

	ipAddressPoolIPList := netmlbparameters.AddressPoolS1

	if ipStack == netparameters.IPV6Family {
		ipAddressPoolIPList = netmlbparameters.AddressPoolV6
	}

	ipAddressPool := DefineMetalLBIPAddressPool(
		ipAddressPoolIPList,
		ipStack,
		netmlbparameters.AddressPoolS1Name)

	err = helper.Apiclient.Create(context.Background(), ipAddressPool)
	Expect(err).ToNot(HaveOccurred())

	bgpAdvertisement := DefineBGPAdvertisement(netmlbparameters.BGPAdvertisementName, []string{ipAddressPool.Name},
		ipStack, netmlbparameters.PrefixLen32)
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

	workerNodesAdresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)

	if ipStack != netparameters.IPV4Family {
		workerNodesV6Adresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV6Family)
		workerNodesAdresses = append(workerNodesAdresses, workerNodesV6Adresses...)
	}

	err = CreateSpeakerBGPPeerIPStack(ipStack, ipv4metalLBIPList, ipv6metalLBIPList, bgpASN)
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() bool {
		return CheckNeighborsStatus(masterNodeFRRPod, ipStack,
			workerNodesAdresses)
	}, 1*time.Minute, netmlbparameters.Interval).Should(BeTrue())

	By("should validate Traffic")

	validateTraffic(masterNodeFRRPod, workerNodesAdresses, ipv4metalLBIPList, ipv6metalLBIPList, ipStack)
}

func validateTraffic(masterFRRPod *k8sv1.Pod, nodeIPAdresses []string,
	ipv4metalLBIPList []string, ipv6metalLBIPList []string,
	ipStack string) {
	By("should validate BGP routes to service")

	var routes []string

	routes, ipFamily := defineIPRouteFamily(ipStack)

	if ipStack == netparameters.DualIPFamily {
		Eventually(func() error {
			return CheckBGPRoutes(
				masterFRRPod,
				nodeIPAdresses,
				routes,
				netparameters.IPV4Family,
				netmlbparameters.PrefixLen32)
		}, 2*time.Minute, netmlbparameters.TimeoutBFDBGP).ShouldNot(HaveOccurred())
	}

	Eventually(func() error {
		return CheckBGPRoutes(
			masterFRRPod,
			nodeIPAdresses,
			routes,
			ipFamily,
			netmlbparameters.PrefixLen32)
	}, 2*time.Minute, netmlbparameters.TimeoutBFDBGP).ShouldNot(HaveOccurred())

	By("should validate curl to service")

	_, err := curlService(masterFRRPod, ipStack, ipv4metalLBIPList, ipv6metalLBIPList)
	Expect(err).ToNot(HaveOccurred())

	By("should validate SCTP to service")

	err = sctpToService(masterFRRPod, ipStack)
	Expect(err).ToNot(HaveOccurred())
}

func curlService(frrPod *k8sv1.Pod, ipStack string,
	ipv4metalLBIPList []string, ipv6metalLBIPList []string) (string, error) {
	switch ipStack {
	case netparameters.IPV4Family:
		return HTTPMlbPod(frrPod, ipv4metalLBIPList[0], netmlbparameters.AddressPoolS1[0],
			netparameters.IPV4Family, netmlbparameters.TestContainerName, netmlbparameters.BGP)

	case netparameters.IPV6Family:
		return HTTPMlbPod(frrPod, ipv6metalLBIPList[0], netmlbparameters.AddressPoolS1[2],
			netparameters.IPV6Family, netmlbparameters.TestContainerName, netmlbparameters.BGP)
	}

	httpOutput, err := HTTPMlbPod(frrPod, ipv6metalLBIPList[0], netmlbparameters.AddressPoolS1[2],
		netparameters.IPV6Family, netmlbparameters.TestContainerName, netmlbparameters.BGP)
	if err != nil {
		return httpOutput, err
	}

	return HTTPMlbPod(frrPod, ipv6metalLBIPList[0], netmlbparameters.AddressPoolS1[2],
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
func CreateSpeakerBGPPeerIPStack(ipStack string,
	ipv4metalLBIPList []string, ipv6metalLBIPList []string,
	bgpASN int) error {
	if ipStack == netparameters.IPV4Family {
		return CreateSpeakerBGPPeer(ipv4metalLBIPList[0], uint32(bgpASN))
	}

	if ipStack == netparameters.IPV6Family {
		return CreateSpeakerBGPPeer(ipv6metalLBIPList[0], uint32(bgpASN))
	}

	err := CreateSpeakerBGPPeer(ipv4metalLBIPList[0], uint32(bgpASN))
	if err == nil {
		return err
	}

	return CreateSpeakerBGPPeer(ipv6metalLBIPList[0], uint32(bgpASN))
}

// DefineAnnotationWithIPStack creates an IP annotation for the pod network.
func DefineAnnotationWithIPStack(ipStack string,
	ipv4metalLBIPList []string, ipv6metalLBIPList []string,
	clusterIPStack string) string {
	annotation := `[{"name": "external", "ips": `
	if ipStack == netparameters.IPV4Family {
		return annotation + fmt.Sprintf(`["%s/%s"]}]`, ipv4metalLBIPList[0], netparameters.IPV4Subnet)
	}

	if clusterIPStack == netparameters.IPV4Family {
		Skip("Cluster does not support IPv6")
	}

	if ipStack == netparameters.IPV6Subnet {
		return annotation + fmt.Sprintf(`["%s/%s"]}]`, ipv6metalLBIPList[0], netparameters.IPV6Subnet)
	}

	return annotation + fmt.Sprintf(`["%s/%s","%s/%s"]}]`, ipv4metalLBIPList[0],
		netparameters.IPV4Subnet, ipv6metalLBIPList[0], netparameters.IPV6Subnet)
}

func sctpToService(masterFRRPod *k8sv1.Pod, ipStack string) error {
	var (
		externalLBIPList []string
	)

	switch ipStack {
	case netparameters.DualIPFamily:
		externalLBIPList = append(externalLBIPList, netmlbparameters.AddressPoolS1[1], netmlbparameters.AddressPoolS1[3])
	case netparameters.IPV4Family:
		externalLBIPList = append(externalLBIPList, netmlbparameters.AddressPoolS1[1])
	case netparameters.IPV6Family:
		externalLBIPList = append(externalLBIPList, netmlbparameters.AddressPoolS1[3])
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
