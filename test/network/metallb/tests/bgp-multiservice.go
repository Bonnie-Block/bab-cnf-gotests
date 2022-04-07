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
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	metallbutils "github.com/metallb/metallb-operator/test/e2e/metallb"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CNF MetalLB", func() {

	var nodeListString []string

	execute.BeforeAll(func() {

		metalLBIPList, err := helper.Config.GetMetallbVirtIP()
		Expect(err).ToNot(HaveOccurred())

		if len(metalLBIPList) < 2 {
			Skip("The environment IP variable is not set or less than 2")
		}

		netmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
			helper.Config.General.CnfNodeLabel, "/")[1],
			netmlbparameters.SingleIPv4Stack,
			metalLBIPList[0],
			"")

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
		err = netmetallbhelper.DeleteAllBGPPeers()
		Expect(err).ToNot(HaveOccurred())
		err = namespaces.CleanPods(netmlbparameters.TestNamespace, helper.Apiclient)
		Expect(err).ToNot(HaveOccurred())
		err = netmetallbhelper.DeleteConfigMap(netparameters.MasterConfigMapName, netmlbparameters.TestNamespace)
		Expect(err).ToNot(HaveOccurred())
		err = nethelper.DeleteNADs([]string{netmlbparameters.ExternalNADName}, netmlbparameters.TestNamespace)
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
		addresspool := netmetallbhelper.DefineMetalLBAddressPool(netmlbparameters.AddressPoolS1,
			netmlbparameters.BGP,
			netmlbparameters.SingleIPv4Stack,
			netmlbparameters.AddressPoolS1Name)

		err := helper.Apiclient.Create(context.Background(), addresspool)
		Expect(err).ToNot(HaveOccurred())

		By("should create a BGP addresspool for service 2")
		addresspool = netmetallbhelper.DefineMetalLBAddressPool(netmlbparameters.AddressPoolS2,
			netmlbparameters.BGP,
			netmlbparameters.SingleIPv4Stack,
			netmlbparameters.AddressPoolS2Name)

		err = helper.Apiclient.Create(context.Background(), addresspool)
		Expect(err).ToNot(HaveOccurred())

		By("should create service 1 with 2 backend pods")
		err = netmetallbhelper.DefineAndCreateLBService(
			netmlbparameters.TestNamespace,
			netmlbparameters.SingleIPv4Stack,
			netmlbparameters.AddressPoolS1Name,
			netmlbparameters.AppLabel1,
			netmlbparameters.ProtocolTCP,
			netmlbparameters.ExtTrafPolCluster)
		Expect(err).ToNot(HaveOccurred())

		netmetallbhelper.DefineAndRunMlbClientPod(nodeListString[0],
			helper.Config.Network.TestContainerImage,
			netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandNGINX})

		netmetallbhelper.DefineAndRunMlbClientPod(nodeListString[1],
			helper.Config.Network.TestContainerImage,
			netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandNGINX})

		By("should create service 2 with 2 backend pods")
		err = netmetallbhelper.DefineAndCreateLBService(
			netmlbparameters.TestNamespace,
			netmlbparameters.SingleIPv4Stack,
			netmlbparameters.AddressPoolS2Name,
			netmlbparameters.AppLabel2,
			netmlbparameters.ProtocolTCP,
			netmlbparameters.ExtTrafPolCluster)
		Expect(err).ToNot(HaveOccurred())

		netmetallbhelper.DefineAndRunMlbClientPod(nodeListString[0],
			helper.Config.Network.TestContainerImage,
			netmlbparameters.AppLabel2, []string{netmlbparameters.ArgCommandNGINX})

		netmetallbhelper.DefineAndRunMlbClientPod(nodeListString[1],
			helper.Config.Network.TestContainerImage,
			netmlbparameters.AppLabel2, []string{netmlbparameters.ArgCommandNGINX})

		By("should create a IBGP Peer on Speakers")
		metalLBIPList, err := helper.Config.GetMetallbVirtIP()
		Expect(err).ToNot(HaveOccurred())

		masterNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(masterNodeList)).To(BeNumerically(">", 0))
		masterNode := masterNodeList[0]

		err = netmetallbhelper.CreateSpeakerBGPPeer(metalLBIPList[0],
			netmlbparameters.IBPGPProtocol, uint32(netmlbparameters.IBGPASN))
		Expect(err).ToNot(HaveOccurred())

		By("should create external FRR container")
		err = helper.Apiclient.Create(context.Background(), netmetallbhelper.DefineExternalNAD())
		Expect(err).ToNot(HaveOccurred())

		workerNodesAdresses, err := helper.GetNodeIPListByLabel(parameters.RoleWorker)
		Expect(err).ToNot(HaveOccurred())
		masterConfigMap := netmetallbhelper.DefineFRRBGPConfigMap(workerNodesAdresses,
			netparameters.MasterConfigMapName,
			netmlbparameters.IBGPASN,
			netmlbparameters.BGP,
			netmlbparameters.SingleIPv4Stack)
		_, err = helper.Apiclient.ConfigMaps(netmlbparameters.TestNamespace).Create(
			context.TODO(),
			masterConfigMap,
			metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		frrPod := netmetallbhelper.DefineFrrPodWithTestContainer(masterNode.Name, netmlbparameters.TestNamespace)
		frrPodWithNAD := pod.RedefinePodWithNetwork(frrPod,
			fmt.Sprintf(`[{"name": "%s", "ips": ["%s/%s"]}]`, netmlbparameters.ExternalNADName,
				metalLBIPList[0], netparameters.IPV4Subnet))
		masterNodeFRRPod := helper.WaitUntilPodCreatedAndRunning(frrPodWithNAD, netmlbparameters.PodWaitingTime)

		By("Checking that BGP sessions are established")
		Eventually(func() bool {
			netmetallbhelper.CheckNeighborsStatus(masterNodeFRRPod, netmlbparameters.SingleIPv4Stack,
				workerNodesAdresses)

			return netmetallbhelper.CheckNeighborsStatus(masterNodeFRRPod, netmlbparameters.SingleIPv4Stack,
				workerNodesAdresses)
		}, 1*time.Minute, netmlbparameters.Interval).Should(BeTrue())

		By("should validate BGP routes to service")
		routesV4 := []string{netmlbparameters.AddressPoolS1[0], netmlbparameters.AddressPoolS2[0]}
		err = netmetallbhelper.CheckBGPRoutes(masterNodeFRRPod, workerNodesAdresses,
			routesV4, netparameters.IPV4Family)
		Expect(err).ToNot(HaveOccurred())

		By("should validate curl to service 1")
		httpOutput, err := netmetallbhelper.HTTPMlbPod(frrPod, netmlbparameters.AddressPoolS1[0], netmlbparameters.Curl,
			netparameters.IPV4Family, netmlbparameters.TestContainerName)
		Expect(err).ToNot(HaveOccurred(), httpOutput)

		By("should validate curl to service 2")
		httpOutput, err = netmetallbhelper.HTTPMlbPod(frrPod, netmlbparameters.AddressPoolS2[0], netmlbparameters.Curl,
			netparameters.IPV4Family, netmlbparameters.TestContainerName)
		Expect(err).ToNot(HaveOccurred(), httpOutput)
	})
})
