package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

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
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("system metallb", func() {
	var (
		testSetupFail  = true
		workerNodeList []k8sv1.Node
		masterNodeList []k8sv1.Node
		webServer      *k8sv1.Pod
		serverPodIP    string
	)

	execute.BeforeAll(func() {

		ipv4metalLBIPList, _, err = netmetallbhelper.GetMetalLBIPByFamily()
		Expect(err).ToNot(HaveOccurred(), "An unexpected error occurred while determining the "+
			"IP addresses from the METALLB_ADDR_LIST environment variable")

		By(fmt.Sprintf("should select nodes by role %s ", parameters.RoleWorker))
		workerNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
		Expect(len(workerNodeList)).To(BeNumerically(">", 0))
		Expect(err).ToNot(HaveOccurred())

		By(fmt.Sprintf("should select nodes by role %s ", parameters.RoleMaster))
		masterNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
		Expect(len(masterNodeList)).To(BeNumerically(">", 0))
		Expect(err).ToNot(HaveOccurred())

		By("Verify that MetalLb deployment is running")
		_, err := helper.Apiclient.Deployments(netmlbparameters.TestNamespace).Get(
			context.TODO(), netmlbparameters.MetalLBOperatorDeploymentName, metav1.GetOptions{})
		Expect(err).To(HaveOccurred(), "metallb operator deployment is not installed")

		localGWMode := netmetallbhelper.GetGWMode()
		// if false - share GW, if true - local GW
		if !localGWMode {
			By("Configuring Local GW mode")
			netmetallbhelper.SetLocalGWMode(true)
			netmetallbhelper.WaitNetworkOperator()
			netmetallbhelper.ChangedGWMode = true
		}
		Expect(netmetallbhelper.GetGWMode()).To(BeTrue())

		testSetupFail = false

	})

	BeforeEach(func() {
		if testSetupFail {
			Fail("Test failed due to error in BeforeAll")
		}

		By("Setup Metallb")
		netmetallbhelper.SetupMetalLB()

		By("Creating an IPAddressPool")
		err = helper.Apiclient.Create(context.Background(),
			netmetallbhelper.DefineMetalLBIPAddressPool(netmlbparameters.IPv4AddressesLBList, netparameters.IPV4Family,
				netmlbparameters.AddressPoolName))
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("An unexpected error occurred while"+
			" creating IPAddressPool %s.", netmlbparameters.AddressPoolName))

		By("Create service")
		err = netmetallbhelper.DefineAndCreateLBService(
			netmlbparameters.TestNamespace,
			"ipv4",
			netmlbparameters.AddressPoolName,
			netmlbparameters.AppLabel1,
			netmlbparameters.ProtocolTCP,
			netmlbparameters.ExtTrafPolCluster)
		Expect(err).ToNot(HaveOccurred())

		By("Create BGPAdvertisement")
		err = helper.Apiclient.Create(
			context.Background(),
			netmetallbhelper.DefineBGPAdvertisement(
				netmlbparameters.BGPAdvertisementName,
				[]string{netmlbparameters.AddressPoolName},
				"ipv4", 32, netmlbparameters.LocalPref100),
		)

		Expect(err).ToNot(HaveOccurred())

		By("Collect Nodes IP addresses")
		workerNodesAdresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)

		By("should create external FRR container")
		masterNodeFRRPod := netmetallbhelper.CreateFRRContainerOnMaster(
			workerNodeList,
			masterNodeList[0],
			ipv4metalLBIPList[0],
			"",
			netparameters.IPV4Family,
			netmlbparameters.IBGPASN,
			netmlbparameters.ExternalNADName,
			netparameters.MasterConfigMapName,
			netmlbparameters.PropagateFalse,
		)

		err = netmetallbhelper.CreateSpeakerBGPPeerIPStack(netparameters.IPV4Family,
			ipv4metalLBIPList[0], "", netmlbparameters.IBGPASN, netmlbparameters.BGPPeerName1v4)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() bool {
			return netmetallbhelper.CheckNeighborsStatus(masterNodeFRRPod, netparameters.IPV4Family,
				workerNodesAdresses)
		}, 1*time.Minute, netmlbparameters.Interval).Should(BeTrue())

		By("Create backend web server")
		privilegedTrue := true
		tcpDumpCMD := append(netmlbparameters.BashCMD, netmlbparameters.TCPDumpCMD...)
		tcpDumpContainer := pod.DefineContainer("tcpdump", tcpDumpCMD, helper.Config.Network.TestContainerImage,
			&k8sv1.SecurityContext{Privileged: &privilegedTrue})
		webServer = netmetallbhelper.DefineAndRunMlbServerPodWithSecondContainer(workerNodeList[0].Name,
			helper.Config.Network.TestContainerImage,
			netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandNGINX}, tcpDumpContainer)

		serverPodIP, err = pod.GetIPFromDefaultNetAnnotation(webServer)
		Expect(err).ToNot(HaveOccurred(), "error to collect main server pod ip")
	})

	AfterEach(func() {
		By("should delete AddressPool, Service, BGP Peers and test Pod after test")
		externalNadList := []string{netmlbparameters.ExternalNADName}
		masterConfigMapList := []string{netparameters.MasterConfigMapName}
		netmetallbhelper.RemoveMetallbBGPTestSetup(externalNadList, masterConfigMapList)
	})

	DescribeTable("MetalLB Load balance external IP accessible to internal cluster IPs",
		func(diffNode bool) {
			workerNodeName := workerNodeList[0].Name

			if diffNode {

				if len(workerNodeList) < 2 {
					Skip("cluster doesn't have enough worker nodes to run the test case")
				}

				workerNodeName = workerNodeList[1].Name
			}

			clientPod := pod.RedefineAsPrivileged(pod.RedefineWithCommand(
				pod.DefinePodOnNode(
					netmlbparameters.TestNamespace, helper.Config.Network.TestContainerImage, workerNodeName,
				), netmlbparameters.BashCMD, netmlbparameters.TCPDumpCMD))

			runningClientPod := helper.WaitUntilPodCreatedAndRunning(clientPod, netmlbparameters.PodWaitingTime)
			clientPodIP, err := pod.GetIPFromDefaultNetAnnotation(runningClientPod)
			Expect(err).ToNot(HaveOccurred(), "error to collect main pod ip")

			By("Generate connections")
			generateConnections(runningClientPod, clientPodIP)

			By("Check source ip and destination in traffic capture file on client pod")
			err = srcAndDstIPInTrafficCapture(runningClientPod, clientPodIP, netmlbparameters.IPv4AddressesLBList[0])
			Expect(err).ToNot(HaveOccurred(), "required ips are not detected on client's traffic capture")

			By("Check source ip and destination on traffic capture file on nginx1 pod")
			err = srcAndDstIPInTrafficCapture(webServer, clientPodIP, serverPodIP)
			Expect(err).ToNot(HaveOccurred(), "required ips are not detected on nginx's traffic capture")

		},
		Entry("same node", false),
		Entry("different node", true),
	)
})

func srcAndDstIPInTrafficCapture(runningPod *k8sv1.Pod, srcIP, dstIP string) error {
	trafficCapture, err := pod.ExecCommand(
		helper.Apiclient, *runningPod,
		[]string{"cat", netmlbparameters.DumpFileName}, runningPod.Spec.Containers[0].Name)
	Expect(err).ToNot(HaveOccurred())

	for _, line := range strings.Split(trafficCapture.String(), "\n") {
		if strings.Contains(line, srcIP) && strings.Contains(line, fmt.Sprintf("%s.80", dstIP)) {
			return nil
		}
	}

	return fmt.Errorf(
		"failed to detect reguired src: %s dst: %s ip address in traffic caputure", srcIP, dstIP)
}

func generateConnections(srcPod *k8sv1.Pod, srcPodIP string) {
	for idx := 0; ; idx++ {
		_, err := netmetallbhelper.HTTPMlbPod(
			srcPod, srcPodIP, netmlbparameters.IPv4AddressesLBList[0],
			"ipv4", srcPod.Spec.Containers[0].Name, "")
		Expect(err).ToNot(HaveOccurred())

		if idx == 3 {
			break
		}
	}
}
