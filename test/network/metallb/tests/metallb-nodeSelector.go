package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"go.universe.tf/metallb/api/v1beta1"
	k8sv1 "k8s.io/api/core/v1"
)

var _ = Describe("MetalLB NodeSelector", func() {
	var (
		ipv4metalLBIPList []string
		ipv6metalLBIPList []string
		workerNodeList    []k8sv1.Node
		masterNodeList    []k8sv1.Node
		clusterIPStack    string
		testSetupFail     = true
	)

	execute.BeforeAll(func() {

		ipv4metalLBIPList, ipv6metalLBIPList, err = netmetallbhelper.GetMetalLBIPByFamily()
		Expect(err).ToNot(HaveOccurred(), "Error retreiving METALLB_ADDR_LIST environment variable")

		By(fmt.Sprintf("should select nodes by role %s ", parameters.RoleWorker))
		workerNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
		Expect(len(workerNodeList)).To(BeNumerically(">", 1))
		Expect(err).ToNot(HaveOccurred())

		masterNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(masterNodeList)).To(BeNumerically(">", 0))

		clusterIPStack := netmetallbhelper.ValidateClusterIPStack()

		By(fmt.Sprintf("Running test on %s cluster", clusterIPStack))
		var ipV6Address string
		if clusterIPStack == netparameters.DualIPFamily {
			Expect(len(ipv6metalLBIPList)).To(BeNumerically(">", 0))
			ipV6Address = ipv6metalLBIPList[0]
		}

		netmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
			helper.Config.General.CnfNodeLabel, "/")[1],
			clusterIPStack,
			ipv4metalLBIPList[0],
			ipV6Address)
		testSetupFail = false
	})

	BeforeEach(func() {
		if testSetupFail {
			Fail("Test failed due to error in BeforeAll")
		}
		By("Setup Metallb")
		netmetallbhelper.SetupMetalLB()
	})

	AfterEach(func() {
		By("should delete AddressPool, Service, BGP Peers and test Pod after test")
		nadList := []string{netmlbparameters.ExternalNADName, netmlbparameters.External2NADName}
		configMapNameList := []string{netparameters.MasterConfigMapName, netparameters.Master2ConfigMapName}
		netmetallbhelper.RemoveMetallbBGPTestSetup(nadList, configMapNameList)
	})

	Context("Dual IPAddressPools", func() {
		var (
			masterNodeFRRPodOne *k8sv1.Pod
			masterNodeFRRPodTwo *k8sv1.Pod
		)

		BeforeEach(func() {

			By("should create two IPAddressPools")
			ipAddressPoolOne := netmlbparameters.AddressPoolS1
			ipAddressPoolTwo := netmlbparameters.AddressPoolS2

			if clusterIPStack == netparameters.IPV6Family {
				ipAddressPoolOne = netmlbparameters.AddressPool1V6
				ipAddressPoolTwo = netmlbparameters.AddressPool2V6
			}

			ipAddressPool1 := netmetallbhelper.DefineMetalLBIPAddressPool(
				ipAddressPoolOne, clusterIPStack, netmlbparameters.AddressPoolS1Name)

			ipAddressPool2 := netmetallbhelper.DefineMetalLBIPAddressPool(
				ipAddressPoolTwo, clusterIPStack, netmlbparameters.AddressPoolS2Name)

			for _, ipAddressPoolList := range []*v1beta1.IPAddressPool{ipAddressPool1, ipAddressPool2} {
				err := helper.Apiclient.Create(context.Background(), ipAddressPoolList)
				Expect(err).ToNot(HaveOccurred())
			}

			By("should create two Services")

			_, err = netmetallbhelper.DefineAndCreateLBService(netmlbparameters.TestNamespace, clusterIPStack,
				netmlbparameters.AddressPoolS1Name, netmlbparameters.AppLabel1, netmlbparameters.ProtocolTCP,
				netmlbparameters.ExtTrafPolCluster)
			Expect(err).ToNot(HaveOccurred())

			netmetallbhelper.DefineAndRunMlbServerPod(workerNodeList[0].Name,
				helper.Config.Network.TestContainerImage,
				netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandNGINX})

			_, err = netmetallbhelper.DefineAndCreateLBService(
				netmlbparameters.TestNamespace, clusterIPStack, netmlbparameters.AddressPoolS2Name,
				netmlbparameters.AppLabel2, netmlbparameters.ProtocolTCP, netmlbparameters.ExtTrafPolCluster)
			Expect(err).ToNot(HaveOccurred())

			netmetallbhelper.DefineAndRunMlbServerPod(workerNodeList[0].Name,
				helper.Config.Network.TestContainerImage,
				netmlbparameters.AppLabel2, []string{netmlbparameters.ArgCommandNGINX})

			By("should create external FRR containers")

			var ipv6Address string

			clusterIPStack := netmetallbhelper.ValidateClusterIPStack()

			if clusterIPStack != netparameters.IPV4Family {
				ipv6Address = ipv6metalLBIPList[0]
			}

			masterNodeFRRPodOne = netmetallbhelper.CreateFRRContainerOnMaster(workerNodeList, masterNodeList[0],
				ipv4metalLBIPList[0], ipv6Address, clusterIPStack, netmlbparameters.IBGPASN,
				netmlbparameters.ExternalNADName, netparameters.MasterConfigMapName, netmlbparameters.PropagateFalse)

			masterNodeFRRPodTwo = netmetallbhelper.CreateFRRContainerOnMaster(workerNodeList, masterNodeList[1],
				ipv4metalLBIPList[1], ipv6Address, clusterIPStack, netmlbparameters.IBGPASN,
				netmlbparameters.External2NADName, netparameters.Master2ConfigMapName, netmlbparameters.PropagateFalse)

			By("should create a BGP Peer on Speakers")

			ipFamily := netparameters.IPV4Family

			bgpPeerName1 := netmlbparameters.BGPPeerName1v4
			bgpPeerName2 := netmlbparameters.BGPPeerName2v4

			if clusterIPStack != netparameters.IPV4Family {
				ipFamily = netparameters.IPV6Family
				bgpPeerName1 = netmlbparameters.BGPPeerName1v6
				bgpPeerName2 = netmlbparameters.BGPPeerName2v6
			}

			err = netmetallbhelper.CreateSpeakerBGPPeerIPStack(ipFamily, ipv4metalLBIPList[0], ipv6Address,
				netmlbparameters.IBGPASN, bgpPeerName1)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create BGP Peer %s", bgpPeerName1))

			err = netmetallbhelper.CreateSpeakerBGPPeerIPStack(ipFamily, ipv4metalLBIPList[1], ipv6Address,
				netmlbparameters.IBGPASN, bgpPeerName2)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create BGP Peer %s", bgpPeerName2))

		})

		It("Advertise separate IPAddressPools using the node selector option", func() {
			By("should create BGPAdvertisement for external FRR1 container")
			err = helper.Apiclient.Create(
				context.Background(),
				netmetallbhelper.RedefineBGPAdvertisementWithNodeSelector(netmlbparameters.BGPAdvertisementName,
					workerNodeList[0].Name, clusterIPStack, []string{netmlbparameters.AddressPoolS1Name},
					[]string{netmlbparameters.BGPPeerName1v4}, netmlbparameters.PrefixLen32,
					netmlbparameters.LocalPref100),
			)
			Expect(err).ToNot(HaveOccurred())

			By("should create BGPAdvertisement for external FRR2 container")
			err = helper.Apiclient.Create(
				context.Background(),
				netmetallbhelper.RedefineBGPAdvertisementWithNodeSelector(netmlbparameters.BGPAdvertisement2Name,
					workerNodeList[1].Name, clusterIPStack, []string{netmlbparameters.AddressPoolS2Name},
					[]string{netmlbparameters.BGPPeerName2v4}, netmlbparameters.PrefixLen32,
					netmlbparameters.LocalPref100),
			)
			Expect(err).ToNot(HaveOccurred())
			By("should validate routes to external FRR1 container")
			workerNodesAdresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)

			Eventually(func() error {
				return netmetallbhelper.CheckBGPRoutesSingleNode(masterNodeFRRPodOne, []string{workerNodesAdresses[0]},
					[]string{netmlbparameters.AddressPoolS1[0]}, netparameters.IPV4Family,
					netmlbparameters.PrefixLen32)
			}, 30*time.Second, netmlbparameters.Interval).ShouldNot(HaveOccurred())

			Eventually(func() error {
				return netmetallbhelper.CheckBGPRoutesSingleNode(masterNodeFRRPodOne, []string{workerNodesAdresses[0]},
					[]string{netmlbparameters.AddressPoolS2[0]}, netparameters.IPV4Family,
					netmlbparameters.PrefixLen32)
			}, 30*time.Second, netmlbparameters.Interval).Should(HaveOccurred())

			By("should validate routes to external FRR2 container")

			Eventually(func() error {
				return netmetallbhelper.CheckBGPRoutesSingleNode(masterNodeFRRPodTwo, []string{workerNodesAdresses[1]},
					[]string{netmlbparameters.AddressPoolS2[0]}, netparameters.IPV4Family,
					netmlbparameters.PrefixLen32)
			}, 30*time.Second, netmlbparameters.Interval).ShouldNot(HaveOccurred())

			Eventually(func() error {
				return netmetallbhelper.CheckBGPRoutesSingleNode(masterNodeFRRPodTwo, []string{workerNodesAdresses[1]},
					[]string{netmlbparameters.AddressPoolS1[0]}, netparameters.IPV4Family,
					netmlbparameters.PrefixLen32)
			}, 30*time.Second, netmlbparameters.Interval).Should(HaveOccurred())
		})
	})
})
