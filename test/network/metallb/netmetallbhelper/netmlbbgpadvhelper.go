package netmetallbhelper

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	k8sv1 "k8s.io/api/core/v1"
)

func TestBGPAdvertismentTable(ipStack string, metalLBIPList []string, workerNodeList []k8sv1.Node,
	masterNodeList []k8sv1.Node, prefixLenght int32) {
	By("should create external FRR container")

	masterNodeFRRPod := CreateFRRContainerOnMaster(workerNodeList, masterNodeList, metalLBIPList, ipStack,
		netmlbparameters.IBGPASN)

	By("should create a BGP addresspool")

	addresspoolIPList := netmlbparameters.AddressPoolV4Prefix32

	if ipStack == netmlbparameters.SingleIPv6Stack {
		addresspoolIPList = netmlbparameters.AddressPoolV6Prefix128
	}

	err := helper.Apiclient.Create(
		context.Background(),
		DefineMetalLBAddressPool(
			addresspoolIPList,
			netmlbparameters.BGP,
			ipStack,
			netmlbparameters.AddressPoolS1Name,
			prefixLenght),
	)
	Expect(err).ToNot(HaveOccurred())

	By("should create service with 1 backend pods")

	err = DefineAndCreateLBService(
		netmlbparameters.TestNamespace,
		ipStack,
		netmlbparameters.AddressPoolS1Name,
		netmlbparameters.AppLabel1,
		netmlbparameters.ProtocolTCP,
		netmlbparameters.ExtTrafPolCluster)
	Expect(err).ToNot(HaveOccurred())

	DefineAndRunMlbServerPod(workerNodeList[0].Name,
		helper.Config.Network.TestContainerImage,
		netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandNGINX})

	By("should create a BGP Peer on Speakers")

	ipFamily := netparameters.IPV4Family
	workerNodesAdresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)

	if ipStack != netmlbparameters.SingleIPv4Stack {
		workerNodesV6Adresses := nethelper.NodeIPsForFamily(workerNodeList, netmlbparameters.IPV6Family)
		workerNodesAdresses = append(workerNodesAdresses, workerNodesV6Adresses...)
		ipFamily = netmlbparameters.IPV6Family
	}

	err = CreateSpeakerBGPPeerIPStack(ipStack, metalLBIPList, netmlbparameters.IBGPASN)
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() bool {
		return CheckNeighborsStatus(masterNodeFRRPod, ipStack,
			workerNodesAdresses)
	}, 1*time.Minute, netmlbparameters.Interval).Should(BeTrue())

	By("should validate BGP route prefix")
	validatePrefix(masterNodeFRRPod, workerNodesAdresses, ipStack, prefixLenght)

	By("should validate BGP route community")

	err = RoutesForCommunity(masterNodeFRRPod, netmlbparameters.CommunityNoAdv, ipFamily)
	Expect(err).ToNot(HaveOccurred())
}

func validatePrefix(masterNodeFRRPod *k8sv1.Pod, workerNodesAdresses []string, ipStack string, prefixLenght int32) {
	var (
		routes   []string
		ipFamily string
	)

	switch ipStack {
	case netmlbparameters.SingleIPv4Stack:
		routes = []string{netmlbparameters.AddressPoolV4Prefix28[0]}
		if prefixLenght == netmlbparameters.PrefixLen32 {
			routes = []string{netmlbparameters.AddressPoolV4Prefix32[0]}
		}

		ipFamily = netparameters.IPV4Family

	case netmlbparameters.SingleIPv6Stack:
		routes = []string{netmlbparameters.AddressPoolV6Prefix126[0]}
		if prefixLenght != netmlbparameters.PrefixLen128 {
			routes = []string{netmlbparameters.AddressPoolV6Prefix128[0]}
		}

		ipFamily = netmlbparameters.IPV6Family
	}

	Eventually(func() error {
		return CheckBGPRoutes(
			masterNodeFRRPod,
			workerNodesAdresses,
			routes,
			ipFamily,
			prefixLenght)
	}, 1*time.Minute, netmlbparameters.TimeoutBFDBGP).ShouldNot(HaveOccurred())
}
