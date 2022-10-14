package tests

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	metallboperatorv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	metallbutils "github.com/metallb/metallb-operator/test/e2e/metallb"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("CNF MetalLB", func() {

	var (
		nodeListString      []string
		metalLBIPList       []string
		masterNode          v1.Node
		workerNodesNameList []string
		err                 error
		testSetupFail       = true
	)

	execute.BeforeAll(func() {
		metalLBIPList, err = helper.Config.GetMetallbVirtIP()
		Expect(err).ToNot(HaveOccurred())

		netmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
			helper.Config.General.CnfNodeLabel, "/")[1],
			netparameters.IPV4Family,
			metalLBIPList[0],
			"")

		masterNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
		Expect(err).ToNot(HaveOccurred())

		Expect(len(masterNodeList)).To(BeNumerically(">", 0))
		masterNode = masterNodeList[0]

		workerNodesNameList = helper.GetNodeListStringByLabel(parameters.RoleWorker)

		By(fmt.Sprintf("should select nodes by role %s ", parameters.RoleWorker), func() {
			workerNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
			Expect(err).ToNot(HaveOccurred())
			for _, node := range workerNodeList {
				nodeListString = append(nodeListString, node.Name)
			}
			if len(nodeListString) < 2 {
				Skip("Need at least 2 nodes to run MetalLB test")
			}
		})
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

		By("should delete IPAddressPool, L2Advertisement and test Pod after test")
		netmetallbhelper.DeleteAllIPAddressPools()

		err := netmetallbhelper.DeleteAllL2Advertisements()
		Expect(err).ToNot(HaveOccurred())

		err = netmetallbhelper.DeleteAllLBServices(netmlbparameters.TestNamespace)
		Expect(err).ToNot(HaveOccurred())
		err = namespaces.CleanPods(netmlbparameters.TestNamespace, helper.Apiclient)
		Expect(err).ToNot(HaveOccurred())
		err = netmetallbhelper.DeleteLabelFromWorkers(netmlbparameters.SpeakerNodeTestLabel)
		Expect(err).ToNot(HaveOccurred())
		err = nethelper.DeleteNADs(netmlbparameters.TestNamespace, netmlbparameters.ExternalNADName)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to delete NADs.: %s", err))

		By("Should remove Metallb Configuration")
		metallb := &metallboperatorv1beta1.MetalLB{}
		err = helper.Apiclient.Get(context.Background(), types.NamespacedName{Name: "metallb",
			Namespace: netmlbparameters.MetalLBOperatorNameSpace}, metallb)
		Expect(err).ToNot(HaveOccurred())

		metallbutils.Delete(metallb)

	})

	// OCP-42936
	It("Validate MetalLB Layer 2 functionality", func() {
		By("should have valid environment IP variable for MetalLB address pool")
		netmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
			helper.Config.General.CnfNodeLabel, "/")[1],
			netparameters.IPV4Family,
			metalLBIPList[0],
			"")

		By("should create an IPAddressPool and L2Advertisement")
		ipAddressPool := netmetallbhelper.DefineMetalLBIPAddressPool(metalLBIPList, netparameters.IPV4Family,
			netmlbparameters.AddressPoolL2)

		err := helper.Apiclient.Create(context.Background(), ipAddressPool)
		Expect(err).ToNot(HaveOccurred())

		l2Advertisement := netmetallbhelper.DefineL2Advertisement(netmlbparameters.L2AdvertisementName,
			[]string{ipAddressPool.Name})
		err = helper.Apiclient.Create(context.Background(), l2Advertisement)
		Expect(err).ToNot(HaveOccurred())

		By("should create a MetalLB service")
		err = netmetallbhelper.DefineAndCreateLBService(
			netmlbparameters.TestNamespace,
			netparameters.IPV4Family,
			netmlbparameters.AddressPoolL2,
			netmlbparameters.AppLabel1,
			netmlbparameters.ProtocolTCP,
			"Cluster")
		Expect(err).ToNot(HaveOccurred())

		By("should create nginx test pods")
		netmetallbhelper.DefineAndRunMlbClientPod(nodeListString[0],
			helper.Config.Network.TestContainerImage,
			netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandNGINX})
		netmetallbhelper.DefineAndRunMlbClientPod(nodeListString[1],
			helper.Config.Network.TestContainerImage,
			netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandNGINX})

		By("should validate arping")
		announcingNodeName := netmetallbhelper.GetLBServiceAnnouncingNodeName()
		nodeIndex := netmetallbhelper.GetNodeIndex()
		nonAnnouncerNodeName := workerNodesNameList[nodeIndex["nonannouncerNodeIndex"]]

		Eventually(func() bool {
			announcingNodeName := netmetallbhelper.GetLBServiceAnnouncingNodeName()
			Expect(err).ToNot(HaveOccurred())

			Expect(nodeListString).To(ContainElement(announcingNodeName),
				"No announcing node found")

			return netmetallbhelper.GetLBServiceAnnouncingNodeName() != nonAnnouncerNodeName
		}, netmlbparameters.PodWaitingTime, netmlbparameters.Interval).Should(BeTrue())

		err = helper.Apiclient.Create(context.Background(),
			netmetallbhelper.DefineMacVlanNAD(netmlbparameters.ExternalNADName, netmlbparameters.BREXInterface))
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("An unexpected error occurred during br-ex NetworkAttachmentDefinition creation: %s", err))

		testPodDef, err := netmetallbhelper.DefineMlbPodWithNetwork(masterNode.Name,
			netmlbparameters.TestNamespace,
			helper.Config.Network.TestContainerImage, netmlbparameters.ExternalNADName, metalLBIPList[1])

		Expect(err).ToNot(HaveOccurred())
		testPod := helper.WaitUntilPodCreatedAndRunning(testPodDef, netmlbparameters.PodWaitingTime)

		err = netmetallbhelper.Arping(testPod, metalLBIPList[0], announcingNodeName)
		Expect(err).ToNot(HaveOccurred())

		By("should validate curl")
		Eventually(func() error {
			_, err := netmetallbhelper.HTTPMlbPod(testPod, metalLBIPList[1], metalLBIPList[0],
				netparameters.IPV4Family, parameters.MainContainerName, netmlbparameters.Layer2)

			return err
		}, 1*time.Minute, 2*time.Second).ShouldNot(HaveOccurred(), "unable to curl")
	})
	// OCP-42751
	It("Failure of MetalLB announcing speaker node", func() {
		By("should have valid environment IP variable for MetalLB address pool")
		netmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
			helper.Config.General.CnfNodeLabel, "/")[1],
			netparameters.IPV4Family,
			metalLBIPList[0],
			"")

		By("should create an Address Pool")
		ipAddressPool := netmetallbhelper.DefineMetalLBIPAddressPool(metalLBIPList,
			netparameters.IPV4Family,
			netmlbparameters.AddressPoolL2)

		err = helper.Apiclient.Create(context.Background(), ipAddressPool)
		Expect(err).ToNot(HaveOccurred())

		l2Advertisement := netmetallbhelper.DefineL2Advertisement(netmlbparameters.L2AdvertisementName,
			[]string{ipAddressPool.Name})
		err = helper.Apiclient.Create(context.Background(), l2Advertisement)
		Expect(err).ToNot(HaveOccurred())

		By("Changing the label selector for Metallb and adding a label for Workers")
		netmetallbhelper.UpdateSpeakerNodeLabel()

		By("should create a MetalLB service")
		err = netmetallbhelper.DefineAndCreateLBService(
			netmlbparameters.TestNamespace,
			netparameters.IPV4Family,
			netmlbparameters.AddressPoolL2,
			netmlbparameters.AppLabel1,
			netmlbparameters.ProtocolTCP,
			netmlbparameters.ExtTrafPolCluster)
		Expect(err).ToNot(HaveOccurred())

		By("should create nginx test pods")
		netmetallbhelper.DefineAndRunMlbClientPod(nodeListString[0],
			helper.Config.Network.TestContainerImage,
			netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandNGINX})
		netmetallbhelper.DefineAndRunMlbClientPod(nodeListString[1],
			helper.Config.Network.TestContainerImage,
			netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandNGINX})

		announcingNodeName := netmetallbhelper.GetLBServiceAnnouncingNodeName()
		log.Printf("Node %s is the MetalLB service announcer node", announcingNodeName)

		By("should cause failure of announcing speaker")
		announcerNodeIndex := netmetallbhelper.GetNodeIndex()
		index := announcerNodeIndex["announcerNodeIndex"]

		workerNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
		Expect(len(workerNodeList)).To(BeNumerically(">", 1))
		Expect(err).ToNot(HaveOccurred())

		delete(workerNodeList[index].Labels, netmlbparameters.SpeakerNodeTestLabel)
		_, err = helper.Apiclient.Nodes().Update(context.Background(),
			&workerNodeList[index],
			metav1.UpdateOptions{})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() bool {
			speakerPodList, _ := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).List(
				context.Background(),
				metav1.ListOptions{LabelSelector: netmlbparameters.SpeakersLabelSelector},
			)

			return len(speakerPodList.Items) == 1
		}, 1*time.Minute, 1*time.Second).Should(BeTrue())

		By("should have new MetalLB announcing node during failure of announcing speaker")
		var announcingNodeDuringFailure string

		Eventually(func() bool {
			netmetallbhelper.GetLBServiceAnnouncingNodeName()

			return netmetallbhelper.GetLBServiceAnnouncingNodeName() != announcingNodeName
		}, netmlbparameters.PodWaitingTime, netmlbparameters.Interval).Should(BeTrue(),
			fmt.Sprintf("Node %s is the new MetalLB service announcer node", announcingNodeDuringFailure))

		By("should validate arping")

		err = helper.Apiclient.Create(context.Background(),
			netmetallbhelper.DefineMacVlanNAD(netmlbparameters.ExternalNADName, netmlbparameters.BREXInterface))
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("An unexpected error occurred during br-ex NetworkAttachmentDefinition creation: %s", err))

		testPodDef, err := netmetallbhelper.DefineMlbPodWithNetwork(masterNode.Name,
			netmlbparameters.TestNamespace,
			helper.Config.Network.TestContainerImage, netmlbparameters.ExternalNADName, metalLBIPList[1])

		Expect(err).ToNot(HaveOccurred())
		testPod := helper.WaitUntilPodCreatedAndRunning(testPodDef, netmlbparameters.PodWaitingTime)

		announcingNodeDuringFailure = netmetallbhelper.GetLBServiceAnnouncingNodeName()
		log.Printf("Node %s is the new MetalLB service announcer node", announcingNodeDuringFailure)
		err = netmetallbhelper.Arping(testPod, metalLBIPList[0], announcingNodeDuringFailure)
		Expect(err).ToNot(HaveOccurred())

		By("should validate curl")
		Eventually(func() error {
			_, err := netmetallbhelper.HTTPMlbPod(testPod, metalLBIPList[1], metalLBIPList[0],
				netparameters.IPV4Family, parameters.MainContainerName, netmlbparameters.Layer2)

			return err
		}, 1*time.Minute, 2*time.Second).ShouldNot(HaveOccurred(), "unable to curl")

		By("After failure two Speaker pods are running")

		_, err = nodes.LabelNode(helper.Apiclient, workerNodeList[index].Name,
			netmlbparameters.SpeakerNodeTestLabel, "")
		Expect(err).ToNot(HaveOccurred())

		Eventually(netmetallbhelper.AreSpeakersReady, netmlbparameters.PodWaitingTime, netmlbparameters.Interval).
			Should(BeTrue(), "Speaker pods are not ready")

		By("should have node return to announcing node after failure")
		Eventually(func() string {
			announcingNodeAfterFailure := netmetallbhelper.GetLBServiceAnnouncingNodeName()

			return announcingNodeAfterFailure
		}, netmlbparameters.PodWaitingTime, netmlbparameters.Interval).Should(Equal(announcingNodeName))

		By("should validate arping")
		announcingNodeAfterFailure := netmetallbhelper.GetLBServiceAnnouncingNodeName()
		log.Printf("Node %s is the new MetalLB service announcer node", announcingNodeAfterFailure)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() error {
			return netmetallbhelper.Arping(testPod, metalLBIPList[0], announcingNodeAfterFailure)
		}, netmlbparameters.PodWaitingTime, netmlbparameters.Interval).Should(BeNil())

		By("should validate curl")

		Eventually(func() error {
			_, err := netmetallbhelper.HTTPMlbPod(testPod, metalLBIPList[1], metalLBIPList[0],
				netparameters.IPV4Family, parameters.MainContainerName, netmlbparameters.Layer2)

			return err
		}, 1*time.Minute, 2*time.Second).ShouldNot(HaveOccurred(), "unable to curl")
	})
})
