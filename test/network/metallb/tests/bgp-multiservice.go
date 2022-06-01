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
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CNF MetalLB", func() {

	var (
		nodeListString    []string
		ipv4metalLBIPList []string
		err               error
	)

	execute.BeforeAll(func() {

		ipv4metalLBIPList, _, err = netmetallbhelper.GetMetalLBIPByFamily()
		Expect(err).ToNot(HaveOccurred())
		if len(ipv4metalLBIPList) < 2 {
			Skip("there are not enough environment IPv4 addresses")
		}

		netmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
			helper.Config.General.CnfNodeLabel, "/")[1],
			netparameters.IPV4Family,
			ipv4metalLBIPList[0],
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
		netmetallbhelper.RemoveMetallbBGPTestSetup()
	})

	// 47182
	It("MetalLB BGP Multi-Service Validation", func() {
		By("should create an IPAddressPool and BGPAdvertisement for service 1")
		ipAddressPool := netmetallbhelper.DefineMetalLBIPAddressPool(netmlbparameters.AddressPoolS1,
			netparameters.IPV4Family,
			netmlbparameters.AddressPoolS1Name)

		err := helper.Apiclient.Create(context.Background(), ipAddressPool)
		Expect(err).ToNot(HaveOccurred())

		By("should create an IPAddressPool and BGPAdvertisement for service 2")
		ipAddressPool = netmetallbhelper.DefineMetalLBIPAddressPool(netmlbparameters.AddressPoolS2,
			netparameters.IPV4Family,
			netmlbparameters.AddressPoolS2Name)

		bgpAdvertisement := netmetallbhelper.DefineBGPAdvertisement(
			netmlbparameters.BGPAdvertisementName,
			[]string{netmlbparameters.AddressPoolS1Name, netmlbparameters.AddressPoolS2Name},
			netparameters.IPV4Family, netmlbparameters.PrefixLen32)
		err = helper.Apiclient.Create(context.Background(), bgpAdvertisement)
		Expect(err).ToNot(HaveOccurred())

		err = helper.Apiclient.Create(context.Background(), ipAddressPool)
		Expect(err).ToNot(HaveOccurred())

		By("should create service 1 with 2 backend pods")
		err = netmetallbhelper.DefineAndCreateLBService(
			netmlbparameters.TestNamespace,
			netparameters.IPV4Family,
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
			netparameters.IPV4Family,
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

		masterNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(masterNodeList)).To(BeNumerically(">", 0))
		masterNode := masterNodeList[0]

		err = netmetallbhelper.CreateSpeakerBGPPeer(ipv4metalLBIPList[0], uint32(netmlbparameters.IBGPASN))
		Expect(err).ToNot(HaveOccurred())

		By("should create external FRR container")
		err = helper.Apiclient.Create(context.Background(), netmetallbhelper.DefineExternalNAD())
		Expect(err).ToNot(HaveOccurred())

		workerNodesAdresses, err := helper.GetNodeIPListByLabel(parameters.RoleWorker)
		Expect(err).ToNot(HaveOccurred())
		masterConfigMap := netmetallbhelper.DefineFRRBGPConfigMap(workerNodesAdresses,
			netparameters.MasterConfigMapName,
			netmlbparameters.IBGPASN,
			netparameters.IPV4Family,
			netmlbparameters.PropagateFalse)
		_, err = helper.Apiclient.ConfigMaps(netmlbparameters.TestNamespace).Create(
			context.TODO(),
			masterConfigMap,
			metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		frrPod := netmetallbhelper.DefineFrrPodWithTestContainer(masterNode.Name, netmlbparameters.TestNamespace)
		frrPodWithNAD := pod.RedefinePodWithNetwork(frrPod,
			fmt.Sprintf(`[{"name": "%s", "ips": ["%s/%s"]}]`, netmlbparameters.ExternalNADName,
				ipv4metalLBIPList[0], netparameters.IPV4Subnet))
		masterNodeFRRPod := helper.WaitUntilPodCreatedAndRunning(frrPodWithNAD, netmlbparameters.PodWaitingTime)

		By("Checking that BGP sessions are established")
		Eventually(func() bool {
			netmetallbhelper.CheckNeighborsStatus(masterNodeFRRPod, netparameters.IPV4Family,
				workerNodesAdresses)

			return netmetallbhelper.CheckNeighborsStatus(masterNodeFRRPod, netparameters.IPV4Family,
				workerNodesAdresses)
		}, 1*time.Minute, netmlbparameters.Interval).Should(BeTrue())

		By("should validate BGP routes to service")
		routesV4 := []string{netmlbparameters.AddressPoolS1[0], netmlbparameters.AddressPoolS2[0]}
		err = netmetallbhelper.CheckBGPRoutes(
			masterNodeFRRPod,
			workerNodesAdresses,
			routesV4,
			netparameters.IPV4Family,
			netmlbparameters.PrefixLen32)
		Expect(err).ToNot(HaveOccurred())

		By("should validate curl to service 1")
		httpOutput, err := netmetallbhelper.HTTPMlbPod(frrPod, ipv4metalLBIPList[0], netmlbparameters.AddressPoolS1[0],
			netparameters.IPV4Family, netmlbparameters.TestContainerName, netmlbparameters.BGP)
		Expect(err).ToNot(HaveOccurred(), httpOutput)
		Eventually(func() error {
			_, err := netmetallbhelper.HTTPMlbPod(frrPod, ipv4metalLBIPList[0], netmlbparameters.AddressPoolS1[0],
				netparameters.IPV4Family, netmlbparameters.TestContainerName, netmlbparameters.BGP)

			return err
		}, 1*time.Minute, 2*time.Second).ShouldNot(HaveOccurred(), "unable to curl")

		By("should validate curl to service 2")
		Eventually(func() error {
			_, err := netmetallbhelper.HTTPMlbPod(frrPod, ipv4metalLBIPList[0], netmlbparameters.AddressPoolS2[0],
				netparameters.IPV4Family, netmlbparameters.TestContainerName, netmlbparameters.BGP)

			return err
		}, 1*time.Minute, 2*time.Second).ShouldNot(HaveOccurred(), "unable to curl")
	})
})
