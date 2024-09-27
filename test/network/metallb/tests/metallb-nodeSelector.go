package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
	"go.universe.tf/metallb/api/v1beta1"
	k8sv1 "k8s.io/api/core/v1"
)

var _ = Describe("MetalLB NodeSelector", func() {
	var (
		ipv4metalLBIPList   []string
		ipv6metalLBIPList   []string
		workerNodeList      []k8sv1.Node
		masterNodeList      []k8sv1.Node
		workerNodesAdresses []string
		clusterIPStack      string
		testSetupFail       = true
	)

	execute.BeforeAll(func() {
		var err error

		ipv4metalLBIPList, ipv6metalLBIPList, err = netmetallbhelper.GetMetalLBIPByFamily()
		Expect(err).ToNot(HaveOccurred(), "Error retreiving METALLB_ADDR_LIST environment variable")

		By(fmt.Sprintf("should select nodes by role %s ", parameters.RoleWorker))
		workerNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
		Expect(len(workerNodeList)).To(BeNumerically(">", 1))
		Expect(err).ToNot(HaveOccurred())

		masterNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(masterNodeList)).To(BeNumerically(">", 0))

		workerNodesAdresses = nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)
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

			_, err := netmetallbhelper.DefineAndCreateLBService(netmlbparameters.TestNamespace, clusterIPStack,
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

		// OCP-53986
		It("Advertise separate IPAddressPools using the node selector option", polarion.ID("53986"), func() {
			By("should create BGPAdvertisement for external FRR1 container")
			err := createBGPAdvertisementWithNodeSelector(netmlbparameters.BGPAdvertisementName,
				netmlbparameters.CommunityNoAdv, clusterIPStack,
				[]string{netmlbparameters.AddressPoolS1Name}, []string{netmlbparameters.BGPPeerName1v4},
				map[string]string{parameters.LabelHostname: workerNodeList[0].Name})

			Expect(err).ToNot(HaveOccurred())

			By("should create BGPAdvertisement for external FRR2 container")

			err = createBGPAdvertisementWithNodeSelector(netmlbparameters.BGPAdvertisement2Name,
				netmlbparameters.CommunityNoAdv, clusterIPStack, []string{netmlbparameters.AddressPoolS2Name},
				[]string{netmlbparameters.BGPPeerName2v4},
				map[string]string{parameters.LabelHostname: workerNodeList[1].Name})

			Expect(err).ToNot(HaveOccurred())

			By("should validate routes to external FRR1 container")

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

		// OCP-53989
		It("Update the node selector option with a label that does not exist", polarion.ID("53989"), func() {
			By("should create BGPAdvertisement for external FRR1 container")

			err := createBGPAdvertisementWithNodeSelector(netmlbparameters.BGPAdvertisementName,
				netmlbparameters.CommunityNoAdv, clusterIPStack,
				[]string{netmlbparameters.AddressPoolS1Name}, []string{netmlbparameters.BGPPeerName1v4},
				map[string]string{parameters.LabelHostname: workerNodeList[0].Name})

			Expect(err).ToNot(HaveOccurred())

			By("should create BGPAdvertisement for external FRR2 container")

			err = createBGPAdvertisementWithNodeSelector(netmlbparameters.BGPAdvertisement2Name,
				netmlbparameters.CommunityNoAdv, clusterIPStack,
				[]string{netmlbparameters.AddressPoolS1Name}, []string{netmlbparameters.BGPPeerName2v4},
				map[string]string{parameters.LabelHostname: workerNodeList[1].Name})

			Expect(err).ToNot(HaveOccurred())

			By("should validate routes to external FRR1 container")

			Eventually(func() error {
				return netmetallbhelper.CheckBGPRoutesSingleNode(masterNodeFRRPodOne, []string{workerNodesAdresses[0]},
					[]string{netmlbparameters.AddressPoolS1[0]}, netparameters.IPV4Family,
					netmlbparameters.PrefixLen32)
			}, 30*time.Second, netmlbparameters.Interval).Should(HaveOccurred(),
				"Route is found on FRR1 container")

			By("should validate routes to external FRR2 container")

			Eventually(func() error {
				return netmetallbhelper.CheckBGPRoutesSingleNode(masterNodeFRRPodTwo, []string{workerNodesAdresses[1]},
					[]string{netmlbparameters.AddressPoolS1[0]}, netparameters.IPV4Family,
					netmlbparameters.PrefixLen32)
			}, 30*time.Second, netmlbparameters.Interval).ShouldNot(HaveOccurred(),
				"Failed to validate route on FRR2 container")

			By("should update BGPAdvertisement for external FRR1 container with non-existing label")
			err = netmetallbhelper.UpdateBGPAdvertisementNodeSelector("test-label")
			Expect(err).ToNot(HaveOccurred())

			By("should validate routes to external FRR1 container")

			Eventually(func() error {
				return netmetallbhelper.CheckBGPRoutesSingleNode(masterNodeFRRPodOne, []string{workerNodesAdresses[0]},
					[]string{netmlbparameters.AddressPoolS1[0]}, netparameters.IPV4Family,
					netmlbparameters.PrefixLen32)
			}, 30*time.Second, netmlbparameters.Interval).Should(HaveOccurred(),
				"Route is still present on FRR1 container")

			By("should validate routes to external FRR2 container")

			Eventually(func() error {
				return netmetallbhelper.CheckBGPRoutesSingleNode(masterNodeFRRPodTwo, []string{workerNodesAdresses[1]},
					[]string{netmlbparameters.AddressPoolS1[0]}, netparameters.IPV4Family,
					netmlbparameters.PrefixLen32)
			}, 30*time.Second, netmlbparameters.Interval).ShouldNot(HaveOccurred(),
				"Failed to validate route for FRR2 container")
		})
	})

	Context("Single IPAddressPool", func() {
		var (
			masterNodeFRRPod1 *k8sv1.Pod
			masterNodeFRRPod2 *k8sv1.Pod
		)

		BeforeEach(func() {

			By("should create one IPAddressPool")
			ipAddressPool1 := netmetallbhelper.DefineMetalLBIPAddressPool(netmlbparameters.AddressPoolS1,
				netparameters.IPV4Family, netmlbparameters.AddressPoolS1Name)

			err := helper.Apiclient.Create(context.Background(), ipAddressPool1)
			Expect(err).ToNot(HaveOccurred())

			By("should create two Services")

			_, err = netmetallbhelper.DefineAndCreateLBService(netmlbparameters.TestNamespace, clusterIPStack,
				netmlbparameters.AddressPoolS1Name, netmlbparameters.AppLabel1, netmlbparameters.ProtocolTCP,
				netmlbparameters.ExtTrafPolCluster)
			Expect(err).ToNot(HaveOccurred())

			netmetallbhelper.DefineAndRunMlbServerPod(workerNodeList[0].Name,
				helper.Config.Network.TestContainerImage,
				netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandNGINX})

			_, err = netmetallbhelper.DefineAndCreateLBService(netmlbparameters.TestNamespace, clusterIPStack,
				netmlbparameters.AddressPoolS1Name, netmlbparameters.AppLabel2, netmlbparameters.ProtocolTCP,
				netmlbparameters.ExtTrafPolCluster)
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

			masterNodeFRRPod1 = netmetallbhelper.CreateFRRContainerOnMaster(workerNodeList, masterNodeList[0],
				ipv4metalLBIPList[0], ipv6Address, clusterIPStack, netmlbparameters.IBGPASN,
				netmlbparameters.ExternalNADName, netparameters.MasterConfigMapName, netmlbparameters.PropagateFalse)

			masterNodeFRRPod2 = netmetallbhelper.CreateFRRContainerOnMaster(workerNodeList, masterNodeList[1],
				ipv4metalLBIPList[1], ipv6Address, clusterIPStack, netmlbparameters.IBGPASN,
				netmlbparameters.External2NADName, netparameters.Master2ConfigMapName, netmlbparameters.PropagateFalse)

			By("should create a BGP Peer on Speakers")

			ipFamily := netparameters.IPV4Family

			bgpPeerName1 := netmlbparameters.BGPPeerName1v4
			bgpPeerName2 := netmlbparameters.BGPPeerName2v4

			if clusterIPStack != netparameters.IPV4Family {
				ipFamily = netparameters.IPV6Family
				bgpPeerName1 = netmlbparameters.BGPPeerName1v6
			}

			err = netmetallbhelper.CreateSpeakerBGPPeerIPStack(ipFamily, ipv4metalLBIPList[0], ipv6Address,
				netmlbparameters.IBGPASN, bgpPeerName1)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create BGP Peer %s", bgpPeerName1))

			err = netmetallbhelper.CreateSpeakerBGPPeerIPStack(ipFamily, ipv4metalLBIPList[1], ipv6Address,
				netmlbparameters.IBGPASN, bgpPeerName2)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create BGP Peer %s", bgpPeerName2))
		})

		AfterEach(func() {
			By("should remove test label")
			err := netmetallbhelper.DeleteLabelFromWorkers(netmlbparameters.SpeakerNodeTestLabel)
			Expect(err).ToNot(HaveOccurred(), "failed to remove label")
		})

		// OCP-53987
		It("Advertise a single IPAddressPool with different attributes using the node selector option",
			polarion.ID("53987"), func() {
				By("should create BGPAdvertisement for external FRR1 container with the nodeSelector option")

				err := createBGPAdvertisementWithNodeSelector(netmlbparameters.BGPAdvertisementName,
					netmlbparameters.CommunityNoAdv, clusterIPStack,
					[]string{netmlbparameters.AddressPoolS1Name}, []string{netmlbparameters.BGPPeerName1v4},
					map[string]string{parameters.LabelHostname: workerNodeList[0].Name})

				Expect(err).ToNot(HaveOccurred(), "Error creating BGPAdvertisement for FRR1")

				By("should create BGPAdvertisement for external FRR2 container without the nodeSelector option")

				err = helper.Apiclient.Create(
					context.Background(),
					defineCreateBGPAdvertisementNoNodeSelector(netmlbparameters.BGPAdvertisement2Name,
						netmlbparameters.AddressPoolS1Name, netmlbparameters.BGPPeerName2v4, clusterIPStack,
						netmlbparameters.CustomCommunity, netmlbparameters.LocalPref500),
				)

				Expect(err).ToNot(HaveOccurred(), "Error creating BGPAdvertisement for FRR2")

				By("should validate route to the external FRR1 container with nodeSelector option")
				workerNodesAdressesList1 := []string{workerNodesAdresses[0]}

				Eventually(func() error {
					return netmetallbhelper.CheckBGPRoutesSingleNode(masterNodeFRRPod1, workerNodesAdressesList1,
						[]string{netmlbparameters.AddressPoolS1[0]}, netparameters.IPV4Family,
						netmlbparameters.PrefixLen32)
				}, 30*time.Second, netmlbparameters.Interval).ShouldNot(HaveOccurred(),
					"Failed to find route on FRR1")

				By("should validate route to the external FRR2 container without nodeSelector option")
				Eventually(func() error {
					return netmetallbhelper.CheckBGPRoutesSingleNode(masterNodeFRRPod2, workerNodesAdresses,
						[]string{netmlbparameters.AddressPoolS1[0]}, netparameters.IPV4Family,
						netmlbparameters.PrefixLen32)
				}, 30*time.Second, netmlbparameters.Interval).ShouldNot(HaveOccurred(),
					"Failed to find route on FRR2")

				By("should validate Local Preferences and Community settings to the external FRR containers")
				err = netmetallbhelper.ValidateLocalPref(masterNodeFRRPod1, netmlbparameters.LocalPref100,
					netparameters.IPV4Family)

				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
					"Local Pref is not as expected on FRR %s", masterNodeFRRPod1.Name))

				err = netmetallbhelper.ValidateLocalPref(masterNodeFRRPod2, netmlbparameters.LocalPref500,
					netparameters.IPV4Family)

				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
					"Local Pref is not as expected on FRR %s", masterNodeFRRPod2.Name))

				err = netmetallbhelper.ValidateRouteCommunity(masterNodeFRRPod1, netmlbparameters.CommunityNoAdv,
					netparameters.IPV4Family)

				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
					"Community is not as expected on FRR %s", masterNodeFRRPod1.Name))

				err = netmetallbhelper.ValidateRouteCommunity(masterNodeFRRPod2, netmlbparameters.CustomCommunity,
					netparameters.IPV4Family)

				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
					"Community is not as expected on FRR %s", masterNodeFRRPod2.Name))
			})

		// OCP-53988
		It("Advertise a single IPAddressPool with only one Speaker using the node selector option",
			polarion.ID("53988"), func() {
				By("should create BGPAdvertisement without the node selector option")
				bgpAdvertisement := netmetallbhelper.DefineBGPAdvertisement(netmlbparameters.BGPAdvertisement2Name,
					netmlbparameters.CommunityNoAdv, clusterIPStack, []string{netmlbparameters.AddressPoolS1Name},
					netmlbparameters.PrefixLen32, netmlbparameters.LocalPref100)
				err := helper.Apiclient.Create(context.Background(), bgpAdvertisement)
				Expect(err).ToNot(HaveOccurred())

				By("should create BGPAdvertisement for external FRR1 container with the node and peer selector option")
				err = createBGPAdvertisementWithNodeSelector(netmlbparameters.BGPAdvertisementName,
					netmlbparameters.CustomCommunity, clusterIPStack,
					[]string{netmlbparameters.AddressPoolS1Name}, []string{netmlbparameters.BGPPeerName1v4},
					map[string]string{parameters.LabelHostname: workerNodeList[0].Name})

				Expect(err).ToNot(HaveOccurred())

				By("should validate route to the external FRR1 container with nodeSelector and peer option")
				Eventually(func() error {
					return netmetallbhelper.CheckBGPRoutesSingleNode(masterNodeFRRPod1, []string{workerNodesAdresses[0]},
						[]string{netmlbparameters.AddressPoolS1[0]}, netparameters.IPV4Family,
						netmlbparameters.PrefixLen32)
				}, 30*time.Second, netmlbparameters.Interval).ShouldNot(HaveOccurred())

				By("should validate route to the external FRR2 container without nodeSelector option")
				Eventually(func() error {
					return netmetallbhelper.CheckBGPRoutesMultipleNodes(masterNodeFRRPod2, workerNodesAdresses,
						[]string{netmlbparameters.AddressPoolS1[0]}, netparameters.IPV4Family,
						netmlbparameters.PrefixLen32)
				}, 30*time.Second, netmlbparameters.Interval).ShouldNot(HaveOccurred())

				By("should validate Local Preferences for FRR1 container with nodeselector and peer option")
				err = validateLocalPref(masterNodeFRRPod1, netmlbparameters.LocalPref100, netmlbparameters.LocalPref100)

				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
					"Local Pref is not as expected on FRR %s", masterNodeFRRPod1.Name))

				By("should validate Community for FRR1 container with nodeselector and peer option")

				err = netmetallbhelper.ValidateRouteCommunity(masterNodeFRRPod1, netmlbparameters.CommunityNoAdv,
					netparameters.IPV4Family)

				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
					"Community %s  is not as expected on FRR1", netmlbparameters.CommunityNoAdv))

				err = netmetallbhelper.ValidateRouteCommunity(masterNodeFRRPod1, netmlbparameters.CustomCommunity,
					netparameters.IPV4Family)

				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
					"Community %s  is not as expected on FRR1", netmlbparameters.CustomCommunity))

				By("should validate Local Preferences for FRR2 container without node selector")

				err = validateLocalPref(masterNodeFRRPod2, netmlbparameters.LocalPref100, netmlbparameters.LocalPref100)

				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
					"Local Pref is incorrect on FRR %s", masterNodeFRRPod2.Name))

				By("should validate Community for FRR2 container without node selector")

				err = netmetallbhelper.ValidateRouteCommunity(masterNodeFRRPod2, netmlbparameters.CommunityNoAdv,
					netparameters.IPV4Family)

				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
					"Community %s  is not as expected on FRR2", netmlbparameters.CommunityNoAdv))

				err = netmetallbhelper.ValidateRouteCommunity(masterNodeFRRPod2, netmlbparameters.CustomCommunity,
					netparameters.IPV4Family)

				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
					"Community %s  is not as expected on FRR2", netmlbparameters.CustomCommunity))
			})

		// OCP-61336
		It("Update the node selector option with a label that does not exist", polarion.ID("61336"), func() {
			By("should create BGPAdvertisement for external FRR1 container")

			err := createBGPAdvertisementWithNodeSelector(netmlbparameters.BGPAdvertisementName,
				netmlbparameters.CommunityNoAdv, clusterIPStack,
				[]string{netmlbparameters.AddressPoolS1Name}, []string{netmlbparameters.BGPPeerName1v4},
				map[string]string{netmlbparameters.SpeakerNodeTestLabel: ""})

			Expect(err).ToNot(HaveOccurred())

			By("should validate routes to external FRR1 container")

			Eventually(func() error {
				return netmetallbhelper.CheckBGPRoutesSingleNode(masterNodeFRRPod1, []string{workerNodesAdresses[0]},
					[]string{netmlbparameters.AddressPoolS1[0]}, netparameters.IPV4Family,
					netmlbparameters.PrefixLen32)
			}, 30*time.Second, netmlbparameters.Interval).Should(HaveOccurred(),
				"Failed to validate route for FRR1 container")

			By("should update Node Label for FRR1 Container")

			_, err = nodes.LabelNode(helper.Apiclient, workerNodeList[0].Name, netmlbparameters.SpeakerNodeTestLabel,
				"")
			Expect(err).ToNot(HaveOccurred())

			Eventually(netmetallbhelper.AreSpeakersReady, netmlbparameters.Timeout, netmlbparameters.Interval).
				Should(BeTrue(), "Speaker pods are not ready")

			By("should validate routes to external FRR1 container")

			Eventually(func() error {
				return netmetallbhelper.CheckBGPRoutesSingleNode(masterNodeFRRPod1, []string{workerNodesAdresses[0]},
					[]string{netmlbparameters.AddressPoolS1[0]}, netparameters.IPV4Family,
					netmlbparameters.PrefixLen32)
			}, 30*time.Second, netmlbparameters.Interval).ShouldNot(HaveOccurred(),
				"Failed to validate route for FRR2 container")
		})

		// OCP-53991
		It("Remove from node label used in the node selector option", polarion.ID("53991"), func() {

			By("label the test node with test label")
			_, err := nodes.LabelNode(helper.Apiclient, workerNodeList[0].Name, netmlbparameters.SpeakerNodeTestLabel, "")
			Expect(err).ToNot(HaveOccurred())

			By("should create BGPAdvertisement for external FRR1 container")

			err = createBGPAdvertisementWithNodeSelector(netmlbparameters.BGPAdvertisementName,
				netmlbparameters.CommunityNoAdv, clusterIPStack,
				[]string{netmlbparameters.AddressPoolS1Name}, []string{netmlbparameters.BGPPeerName1v4},
				map[string]string{netmlbparameters.SpeakerNodeTestLabel: ""})

			Expect(err).ToNot(HaveOccurred())

			By("should validate routes to external FRR1 container")

			Eventually(func() error {
				return netmetallbhelper.CheckBGPRoutesSingleNode(masterNodeFRRPod1, []string{workerNodesAdresses[0]},
					[]string{netmlbparameters.AddressPoolS1[0]}, netparameters.IPV4Family,
					netmlbparameters.PrefixLen32)
			}, 30*time.Second, netmlbparameters.Interval).ShouldNot(HaveOccurred())

			By("should update Node Label for FRR1 Container")

			_, err = nodes.LabelNode(helper.Apiclient, workerNodeList[0].Name, netmlbparameters.SpeakerNodeTestLabel, "test")
			Expect(err).ToNot(HaveOccurred())

			Eventually(netmetallbhelper.AreSpeakersReady, netmlbparameters.Timeout, netmlbparameters.Interval).
				Should(BeTrue(), "Speaker pods are not ready")

			By("should validate routes to external FRR1 container")

			Eventually(func() error {
				return netmetallbhelper.CheckBGPRoutesSingleNode(masterNodeFRRPod1, []string{workerNodesAdresses[0]},
					[]string{netmlbparameters.AddressPoolS1[0]}, netparameters.IPV4Family,
					netmlbparameters.PrefixLen32)
			}, 30*time.Second, netmlbparameters.Interval).Should(HaveOccurred(),
				"Route to FRR2 container is still validate - expected to fail")
		})
	})
})

func createBGPAdvertisementWithNodeSelector(bgpadvertisementName, ipFamily, community string,
	ipAddressPoolName, bgpPeerName []string, label map[string]string) error {
	err := helper.Apiclient.Create(
		context.Background(),
		netmetallbhelper.RedefineBGPAdvertisementWithNodeSelector(bgpadvertisementName, ipFamily, community,
			ipAddressPoolName, bgpPeerName, netmlbparameters.PrefixLen32, netmlbparameters.LocalPref100, label))

	return err
}

func defineCreateBGPAdvertisementNoNodeSelector(bgpAdvertisementName, addressPoolName, bgpPeerName,
	ipStack, community string, localPref uint32) *v1beta1.BGPAdvertisement {
	bgpAdvertisement := netmetallbhelper.DefineBGPAdvertisementWithPeer(bgpAdvertisementName,
		addressPoolName, bgpPeerName, ipStack, community, localPref)

	bgpAdvertisement.Spec.Communities = []string{community}

	return bgpAdvertisement
}

// validateLocalPref verifies local pref from FRR is equal to configured Local Pref.
func validateLocalPref(frrPod *k8sv1.Pod, localPref1, localPref2 uint32) error {
	res, err := pod.ExecCommand(helper.Apiclient, *frrPod,
		append(netmlbparameters.VtyshFRRCmdPrefix, "show ip bgp json"))

	if err != nil {
		return errors.Wrap(err, "Failed to query routes")
	}

	toParse := netmlbparameters.IPInfo{}
	err = json.Unmarshal(res.Bytes(), &toParse)

	if err != nil {
		return errors.Wrapf(err, "Failed to parse routes %s", res.String())
	}

	for _, frrRoutes := range toParse.Routes {
		if frrRoutes[0].LocalPref != localPref1 {
			return errors.New("Local Preferenece %s is not present")
		}

		if frrRoutes[1].LocalPref != localPref2 {
			return errors.New("Local Preferenece %s is not present")
		}
	}

	return nil
}
