package tests

import (
	"context"
	"fmt"
	"time"

	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	metallbutils "github.com/metallb/metallb-operator/test/e2e/metallb"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CNF MetalLB", func() {

	var (
		nodeListString []string
		masterNode     k8sv1.Node
	)

	execute.BeforeAll(func() {

		masterNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(masterNodeList)).To(BeNumerically(">", 0))
		masterNode = masterNodeList[0]

		metallbIPList, err := helper.Config.GetMetallbVirtIP()
		Expect(err).ToNot(HaveOccurred())

		if len(metallbIPList) < 2 {
			Skip("The environment IP variable is not set or less than 2")
		}

		netmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
			helper.Config.General.CnfNodeLabel, "/")[1],
			metallbIPList[0])

		By(fmt.Sprintf("should select nodes by role %s ", parameters.RoleWorker))
		workerNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)

		Expect(len(workerNodeList)).To(BeNumerically(">", 1))

		Expect(err).ToNot(HaveOccurred())
		for _, node := range workerNodeList {
			nodeListString = append(nodeListString, node.Name)
		}
		if len(nodeListString) < 2 {
			Skip("Need at least 2 nodes to run MetalLB test")
		}
	})

	BeforeEach(func() {
		By("Setup Metallb")
		netmetallbhelper.SetupMetalLB()
	})

	AfterEach(func() {

		By("should delete AddressPool, Service, BGP Peers and test Pod after test")
		netmetallbhelper.DeleteAllAddressPools()
		err := netmetallbhelper.DeleteAllLBServices(netmlbparameters.TestNamespace)
		Expect(err).ToNot(HaveOccurred())
		netmetallbhelper.DeleteAllBGPPeers()
		Expect(err).ToNot(HaveOccurred())
		err = namespaces.CleanPods(netmlbparameters.TestNamespace, helper.Apiclient)
		Expect(err).ToNot(HaveOccurred())

		By("Should remove Metallb Configuration")
		metallb, err := metallbutils.Get(
			netmlbparameters.MetalLBOperatorNameSpace,
			netmlbparameters.UseMetallbResourcesFromFile,
		)
		Expect(err).ToNot(HaveOccurred())

		metallbutils.Delete(metallb)
	})

	// 47182
	It("MetalLB BGP Multi-Service Validation", func() {

		By("should create a BGP addresspool for service 1")
		addresspool := netmetallbhelper.DefineMetalLBAddressPool(netmlbparameters.AddressPoolS1v4,
			netmlbparameters.BGP,
			netmlbparameters.SingleIPv4Stack,
			netmlbparameters.AddressPoolS1v4Name)

		err := helper.Apiclient.Create(context.Background(), addresspool)
		Expect(err).ToNot(HaveOccurred())

		By("should create a BGP addresspool for service 2")
		addresspool = netmetallbhelper.DefineMetalLBAddressPool(netmlbparameters.AddressPoolS2v4,
			netmlbparameters.BGP,
			netmlbparameters.SingleIPv4Stack,
			netmlbparameters.AddressPoolS2v4Name)

		err = helper.Apiclient.Create(context.Background(), addresspool)
		Expect(err).ToNot(HaveOccurred())

		By("should create service 1 with 2 backend pods")
		err = netmetallbhelper.DefineAndCreateLBService(
			netmlbparameters.TestNamespace,
			netmlbparameters.SingleIPv4Stack,
			netmlbparameters.AddressPoolS1v4Name,
			netmlbparameters.AppLabel1,
			"Cluster")
		Expect(err).ToNot(HaveOccurred())

		netmetallbhelper.DefineAndRunMlbClientPod(nodeListString[0],
			helper.Config.Network.TestContainerImage,
			netmlbparameters.AppLabel1)

		netmetallbhelper.DefineAndRunMlbClientPod(nodeListString[1],
			helper.Config.Network.TestContainerImage,
			netmlbparameters.AppLabel1)

		By("should create service 2 with 2 backend pods")
		err = netmetallbhelper.DefineAndCreateLBService(
			netmlbparameters.TestNamespace,
			netmlbparameters.SingleIPv4Stack,
			netmlbparameters.AddressPoolS2v4Name,
			netmlbparameters.AppLabel2,
			"Cluster")
		Expect(err).ToNot(HaveOccurred())

		netmetallbhelper.DefineAndRunMlbClientPod(nodeListString[0],
			helper.Config.Network.TestContainerImage,
			netmlbparameters.AppLabel2)

		netmetallbhelper.DefineAndRunMlbClientPod(nodeListString[1],
			helper.Config.Network.TestContainerImage,
			netmlbparameters.AppLabel2)

		By("should create a IBGP Peer on Speakers")
		masterNodesIPv4List, err := helper.GetNodeIPListByLabel(parameters.RoleMaster)
		Expect(err).ToNot(HaveOccurred())
		err = netmetallbhelper.CreateSpeakerBGPPeer(masterNodesIPv4List[0], uint32(netmlbparameters.IBGPASN))
		Expect(err).ToNot(HaveOccurred())

		By("should create external FRR container")
		workerNodesAdresses, err := helper.GetNodeIPListByLabel(parameters.RoleWorker)
		Expect(err).ToNot(HaveOccurred())
		masterConfigMap := netmetallbhelper.DefineFRRConfigMap(workerNodesAdresses,
			netmlbparameters.MasterConfigMapName,
			netmlbparameters.IBGPASN,
			netmlbparameters.BGP)
		_, err = helper.Apiclient.ConfigMaps(netmlbparameters.TestNamespace).Create(
			context.TODO(),
			masterConfigMap,
			metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		frrPod := nethelper.DefineFRRPod(masterNode.Name, netmlbparameters.TestNamespace)
		masterNodePod := helper.WaitUntilPodCreatedAndRunning(frrPod, netmlbparameters.PodWaitingTime)

		By("Checking that BGP sessions are established")
		Eventually(func() bool {
			netmetallbhelper.CheckNeighborsStatus(masterNodePod, workerNodesAdresses)

			return netmetallbhelper.CheckNeighborsStatus(masterNodePod, workerNodesAdresses)
		}, 1*time.Minute, netmlbparameters.Interval).Should(BeTrue())

		By("should validate BGP routes to service")
		routesV4 := []string{netmlbparameters.AddressPoolS1v4[0], netmlbparameters.AddressPoolS2v4[0]}
		routeState := netmetallbhelper.CheckBGPRoutes(masterNodePod, workerNodesAdresses, routesV4)
		Expect(routeState).To(BeTrue())

		By("should validate curl to service 1")
		testPod := netmetallbhelper.DefineAndRunMlbPodMaster(masterNode.Name,
			netmlbparameters.TestNamespace,
			helper.Config.Network.TestContainerImage)

		netmetallbhelper.CurlMlbPod(testPod, netmlbparameters.AddressPoolS1v4[0])

		By("should validate curl to service 2")
		netmetallbhelper.CurlMlbPod(testPod, netmlbparameters.AddressPoolS2v4[0])
	})
})
