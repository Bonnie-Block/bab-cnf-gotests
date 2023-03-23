package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"

	nmstatev1Shared "github.com/nmstate/kubernetes-nmstate/api/shared"
	nmstatev1 "github.com/nmstate/kubernetes-nmstate/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nad"
	multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"

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
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
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

		By("Verify that MetalLb deployment is running and metalLb mode is local")
		metalLbIsRunningAndInLocalMode()
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
		_, err = netmetallbhelper.DefineAndCreateLBService(
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
				netmlbparameters.BGPAdvertisementName, netmlbparameters.CommunityNoAdv, "ipv4",
				[]string{netmlbparameters.AddressPoolName}, 32, netmlbparameters.LocalPref100),
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
		tcpDumpContainer := pod.DefineContainer(
			"tcpdump", netmlbparameters.TCPDumpCMD, helper.Config.Network.TestContainerImage,
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

	// 53766
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
			generateConnections(runningClientPod, clientPodIP, netmlbparameters.IPv4AddressesLBList[0])

			By("Check source ip and destination in traffic capture file on client pod")
			err = netmetallbhelper.SrcAndDestIPInHTTPTrafficCapture(
				runningClientPod, clientPodIP, netmlbparameters.IPv4AddressesLBList[0])
			Expect(err).ToNot(HaveOccurred(), "required ips are not detected on client's traffic capture")

			By("Check source ip and destination on traffic capture file on nginx pod")
			err = netmetallbhelper.SrcAndDestIPInHTTPTrafficCapture(webServer, clientPodIP, serverPodIP)
			Expect(err).ToNot(HaveOccurred(), "required ips are not detected on nginx's traffic capture")

		},

		// 53792
		Entry("same node", polarion.ID("53792"), false),
		// 53766
		Entry("different node", polarion.ID("53766"), true),
	)
})

var _ = Describe("system metallb", Ordered, func() {
	var (
		testSetupFail           = true
		workerNodeList          []k8sv1.Node
		primaryClientPod        *k8sv1.Pod
		secondaryClientPod      *k8sv1.Pod
		runningTCPDumpPodOnNode *k8sv1.Pod
		primaryWebServer        *k8sv1.Pod
		secondaryWebServer      *k8sv1.Pod
		primaryService          *k8sv1.Service
		secondaryService        *k8sv1.Service
		vlanIds                 []uint16
		runningFrrPodList       []*k8sv1.Pod
	)

	BeforeAll(func() {

		By("Create privileged namespace")
		err := namespaces.Create(parameters.PrivPodNamespace, helper.Apiclient)
		Expect(err).ToNot(HaveOccurred())

		By("Get vlan ids from environment variable")
		vlanIds, err = helper.Config.GetMetalLbVlanIds()
		if err != nil {
			Skip(fmt.Sprintf("skipping test due to %s. Please check METALLB_VLANS env var", err))
		}

		By(fmt.Sprintf("Should select nodes by role %s ", parameters.RoleWorker))
		workerNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(workerNodeList)).To(BeNumerically(">", 1),
			"test required 2 workers minimum")

		By("Verify that MetalLb deployment is running and metalLb mode is local")
		metalLbIsRunningAndInLocalMode()

		By("Discover secondary interface")
		intFacesFromEnvVar, err := helper.Config.GetCnfInterfaces(1)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error determine secondary interfaces: %s", err))
		validSecondaryInterfaces := getSecondaryInterfaces(workerNodeList[0].Name, intFacesFromEnvVar)
		Expect(len(validSecondaryInterfaces)).To(BeNumerically(">", 1))
		By("Setup MetalLb")
		netmetallbhelper.SetupMetalLB()

		By("Define NMState configuration and wait until it's created on cluster")
		_ = defineAndCreateNMStatePolicy(validSecondaryInterfaces[0], workerNodeList[1].Name, vlanIds)

		By("Create Internal NAD")
		defineAndCreateInternalNad(netmlbparameters.InternalNADName)

		By("Create BFD Profile")
		bfdProfile, err := netmetallbhelper.DefineAndCreateBFDProfile()
		Expect(err).ToNot(HaveOccurred())

		for _, vlanID := range vlanIds {
			By(fmt.Sprintf("Configure setup on vlan %d", vlanID))
			By(fmt.Sprintf("Create External %d NAD", vlanID))
			defineAndCreateExternalNad(vlanID, validSecondaryInterfaces[0])

			// set configuration for different VLANs(==iterations)
			addrPoolLBList := netmlbparameters.IPv4AddressesLBList
			addrPoolName := netmlbparameters.AddressPoolS1Name
			confMapName := netmlbparameters.FrrConfigMapS1Name
			bgpPeerIP := netmlbparameters.NodeIntFacePrimaryIPAddr
			bgpFRRLocalASN := netmlbparameters.EBGPASN
			frrVlanIPAddr := fmt.Sprintf("%s/%s", netmlbparameters.NodeIntFacePrimaryNextHop, netparameters.IPSubnet24)
			speakerNodeIPAddr := netmlbparameters.NodeIntFacePrimaryNextHop
			frrInternalDGAddr := fmt.Sprintf("%s/%s", netmlbparameters.InternalRouter2IPv4, netparameters.IPSubnet24)
			frrInternalDGIP := netmlbparameters.InternalRouter2IPv4
			clientInternalIP := fmt.Sprintf("%s/%s", netmlbparameters.InternalClient1IPv4, netparameters.IPSubnet24)
			vlanNetwork := netmlbparameters.NodeIntFacePrimarySubnet
			serverLabel := netmlbparameters.AppLabel1
			bGPAdvertisementName := netmlbparameters.BGPAdvertisementName

			if vlanID == vlanIds[1] {

				addrPoolLBList = netmlbparameters.IPv4AddressesLB2List
				addrPoolName = netmlbparameters.AddressPoolS2Name
				confMapName = netmlbparameters.FrrConfigMapS2Name
				bgpPeerIP = netmlbparameters.NodeIntFaceSecondaryIPAddr
				bgpFRRLocalASN = netmlbparameters.EBGPASN2
				frrVlanIPAddr = fmt.Sprintf("%s/%s", netmlbparameters.NodeIntFaceSecondaryNextHop, netparameters.IPSubnet24)
				speakerNodeIPAddr = netmlbparameters.NodeIntFaceSecondaryNextHop
				frrInternalDGAddr = fmt.Sprintf("%s/%s", netmlbparameters.InternalRouterSecondNetIPv4, netparameters.IPSubnet24)
				frrInternalDGIP = netmlbparameters.InternalRouterSecondNetIPv4
				clientInternalIP = fmt.Sprintf("%s/%s", netmlbparameters.InternalClient2IPv4, netparameters.IPSubnet24)
				vlanNetwork = netmlbparameters.NodeIntFaceSecondarySubnet
				serverLabel = netmlbparameters.AppLabel2
				bGPAdvertisementName = netmlbparameters.BGPAdvertisement2Name

			}

			By(fmt.Sprintf("Creating an IPAddressPool %s", addrPoolName))
			err = helper.Apiclient.Create(context.Background(),
				netmetallbhelper.DefineMetalLBIPAddressPool(addrPoolLBList, netparameters.IPV4Family, addrPoolName))
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("An unexpected error occurred while"+
				" creating IPAddressPool %s.", addrPoolName))

			By(fmt.Sprintf("Create config map %s", confMapName))
			masterConfigMap := netmetallbhelper.DefineFRRBGPConfigMap(
				[]string{bgpPeerIP}, confMapName, netparameters.IPV4Family, bgpFRRLocalASN, netmlbparameters.IBGPASN)
			_, err = helper.Apiclient.ConfigMaps(netmlbparameters.TestNamespace).Create(
				context.TODO(), masterConfigMap, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			By(fmt.Sprintf("Create worker FRR router on vlan %d", vlanID))
			runningFrrPod := defineAndRunFrrRouter(
				vlanID, frrVlanIPAddr, frrInternalDGAddr, workerNodeList[0].Name, masterConfigMap.Name)

			By(fmt.Sprintf("Verify ip connectivity between FRR router and Node Vlan %d interface", vlanID))
			err = netmetallbhelper.IcmpConnectivityWorks(helper.Apiclient, *runningFrrPod, bgpPeerIP)
			Expect(err).ToNot(HaveOccurred())

			By(fmt.Sprintf("Create client pod on vlan %d", vlanID))
			runningClientPod := defineAndRunClientPodOnVlanNetwork(
				clientInternalIP, frrInternalDGIP, netmlbparameters.InternalNADName, workerNodeList[0].Name)

			By(fmt.Sprintf("Test connectivity between client and frr router via vlan %d", vlanID))
			err = netmetallbhelper.IcmpConnectivityWorks(helper.Apiclient, *runningClientPod, frrInternalDGIP)
			Expect(err).ToNot(HaveOccurred())

			By("Add route toward bgp networks to client pod")
			err = netmetallbhelper.AddRouteToPod(helper.Apiclient, *runningClientPod, addrPoolLBList[1], frrInternalDGIP)
			Expect(err).ToNot(HaveOccurred())
			err = netmetallbhelper.AddRouteToPod(helper.Apiclient, *runningClientPod, vlanNetwork, frrInternalDGIP)
			Expect(err).ToNot(HaveOccurred())

			By(fmt.Sprintf("Create nginx server on vlan %d", vlanID))
			webServer := netmetallbhelper.DefineAndRunNGINXServer(serverLabel, workerNodeList[1].Name)

			By(fmt.Sprintf("Create LB service on vlan %d", vlanID))
			service, err := netmetallbhelper.DefineAndCreateLBService(
				netmlbparameters.TestNamespace, netcniparameters.IPStackIPv4,
				addrPoolName, serverLabel, "TCP", "Cluster")
			Expect(err).ToNot(HaveOccurred())

			By(fmt.Sprintf("Create BGP peer on vlan %d", vlanID))
			bgpPeer, err := netmetallbhelper.DefineAndCreateSpeakerBGPPeer(
				speakerNodeIPAddr, bfdProfile.Name, uint32(bgpFRRLocalASN))
			Expect(err).ToNot(HaveOccurred())

			By("Verify if frr pod has bgp session UP")
			Eventually(func() bool {
				return netmetallbhelper.CheckNeighborStatus(runningFrrPod, bgpPeerIP)
			}, netmlbparameters.Timeout, netmlbparameters.Interval).Should(BeTrue())

			By(fmt.Sprintf("Create BGP advertisement on vlan %d", vlanID))
			err = netmetallbhelper.DefineAndCreateBGPAdvertisement(bGPAdvertisementName, addrPoolName, bgpPeer.Name)
			Expect(err).ToNot(HaveOccurred())

			By(fmt.Sprintf("Validate that BGP routes are present on frr pod on vlan %d", vlanID))
			Eventually(func() error {
				return netmetallbhelper.CheckBGPRoute(runningFrrPod, bgpPeerIP, addrPoolLBList[0], "ipv4", 32)
			}, netmlbparameters.Timeout, netmlbparameters.Interval).ShouldNot(HaveOccurred())

			runningFrrPodList = append(runningFrrPodList, runningFrrPod)
			// set vars for test case
			if vlanID == vlanIds[0] {
				primaryService = service
				primaryWebServer = webServer
				primaryClientPod = runningClientPod
			} else {
				secondaryService = service
				secondaryWebServer = webServer
				secondaryClientPod = runningClientPod
			}
		}

		By("Create TCPDumpContainer on worker node")
		runningTCPDumpPodOnNode = defineAndRunNodeTrafficCapturePod(
			validSecondaryInterfaces[0], primaryService.Spec.ClusterIP, secondaryService.Spec.ClusterIP, workerNodeList[1].Name)
		testSetupFail = false
	})

	BeforeEach(func() {
		if testSetupFail {
			Fail("Test failed due to error in BeforeAll")
		}
	})

	AfterAll(func() {
		// Remove Config
		By("Delete privileged namespace")
		err = namespaces.DeleteAndWait(helper.Apiclient, parameters.PrivPodNamespace,
			netmlbparameters.Timeout)
		Expect(err).ToNot(HaveOccurred())

		nmStateInstalledPolicy := nmstatev1.NodeNetworkConfigurationPolicy{}
		err := helper.Apiclient.Get(
			context.TODO(), goclient.ObjectKey{Name: netmlbparameters.NMStatePolicyName}, &nmStateInstalledPolicy)
		if err == nil {
			By("Remove NMState ip configuration from node")
			updatedNMStatePolicy, err := netmetallbhelper.RemoveNmStateConfig(nmStateInstalledPolicy)
			Expect(err).ToNot(HaveOccurred())
			err = helper.Apiclient.Update(context.TODO(), updatedNMStatePolicy)
			Expect(err).ToNot(HaveOccurred())

			By("Delete NMState policy from node")
			waitUntilNMStatePolicyStable(nmStateInstalledPolicy.Name)
			err = helper.Apiclient.Delete(context.TODO(), updatedNMStatePolicy)
			Expect(err).ToNot(HaveOccurred())
		}

		By("should delete AddressPool, Service, BGP Peers and test Pod after test")
		externalNadList := []string{
			fmt.Sprintf("external-%d", vlanIds[0]),
			fmt.Sprintf("external-%d", vlanIds[1]),
			netmlbparameters.InternalNADName}
		masterConfigMapList := []string{netmlbparameters.FrrConfigMapS1Name, netmlbparameters.FrrConfigMapS2Name}
		netmetallbhelper.RemoveMetallbBGPTestSetup(externalNadList, masterConfigMapList)

		By("Delete all BFD Profiles")
		err = netmetallbhelper.DeleteAllBFDProfiles()
		Expect(err).ToNot(HaveOccurred())
	})

	Context("MetalLB accessing the load balance ip from secondary host interfaces with multiple VLANs", func() {

		// 53894
		It("", polarion.ID("53894"), func() {
			for idx, vlanID := range vlanIds {

				clientIP := netmlbparameters.InternalClient1IPv4
				clientPod := primaryClientPod
				serverPod := primaryWebServer
				serviceIP := primaryService.Spec.ClusterIP
				dstSrvIP := netmlbparameters.IPv4AddressesLBList[0]
				nodeSecondaryIP := netmlbparameters.NodeIntFacePrimaryIPAddr

				if idx == 1 {
					clientIP = netmlbparameters.InternalClient2IPv4
					clientPod = secondaryClientPod
					dstSrvIP = netmlbparameters.IPv4AddressesLB2List[0]
					serviceIP = secondaryService.Spec.ClusterIP
					serverPod = secondaryWebServer
					nodeSecondaryIP = netmlbparameters.NodeIntFaceSecondaryIPAddr
				}

				By("Generate connections")
				generateConnections(clientPod, clientIP, dstSrvIP)

				By("Check source ip and destination in http traffic capture file on client pod")
				err = netmetallbhelper.SrcAndDestIPInHTTPTrafficCapture(clientPod, clientIP, dstSrvIP)
				Expect(err).ToNot(HaveOccurred(), "required ips are not detected on client's traffic capture")

				By("Check source ip and destination in http traffic capture file on node secondary vlan interface")
				err = netmetallbhelper.SrcAndDestIPInVlanHTTPTrafficCapture(runningTCPDumpPodOnNode, clientIP, dstSrvIP, vlanID)
				Expect(err).ToNot(HaveOccurred())

				By("Check source ip and destination in http traffic capture file on node's br-ex interface")
				err = netmetallbhelper.SrcAndDestIPInHTTPTrafficCapture(runningTCPDumpPodOnNode, clientIP, serviceIP, 1)
				Expect(err).ToNot(HaveOccurred())

				nodeRouterIP, err := netmetallbhelper.GetNodeOvnRouterIP(&workerNodeList[1])
				Expect(err).ToNot(HaveOccurred())

				webServerPodIP, err := pod.GetIPFromDefaultNetAnnotation(serverPod)
				Expect(err).ToNot(HaveOccurred())

				By("Check source ip and destination in http traffic capture file on nginx server interface")
				err = netmetallbhelper.SrcAndDestIPInHTTPTrafficCapture(serverPod, nodeRouterIP, webServerPodIP)
				Expect(err).ToNot(HaveOccurred())

				By("Running icmp connectivity test from nginx server to client")
				err = netmetallbhelper.IcmpConnectivityWorks(helper.Apiclient, *serverPod, clientIP)
				Expect(err).ToNot(HaveOccurred())

				By("Check source ip and destination in icmp traffic capture file on nginx server interface")
				err = netmetallbhelper.SrcAndDestIPInVlanICMPTrafficCapture(serverPod, webServerPodIP, clientIP)
				Expect(err).ToNot(HaveOccurred())

				By("Check source ip and destination in icmp traffic capture file on node's vlan interface")
				err = netmetallbhelper.SrcAndDestIPInVlanICMPTrafficCapture(
					runningTCPDumpPodOnNode, nodeSecondaryIP, clientIP, vlanID)
				Expect(err).ToNot(HaveOccurred())

				By("Check source ip and destination in icmp traffic capture file on client pod")
				err = netmetallbhelper.SrcAndDestIPInVlanICMPTrafficCapture(clientPod, nodeSecondaryIP, clientIP)
				Expect(err).ToNot(HaveOccurred())
			}
		})

		// 53947
		It("after node reboot", polarion.ID("53947"), func() {
			By("Reboot worker node")
			helper.CreatePrivilegedPods(helper.Config.Network.TestContainerImage)
			helper.SoftRebootNodeAndWaitForDisconnect(&workerNodeList[1])
			machineConfigPoolName := strings.Split(helper.Config.General.CnfNodeLabel, "/")[1]

			By("Wait for cluster to be stable")
			err := helper.WaitForClusterToBeStable(machineConfigPoolName, 1)
			Expect(err).ToNot(HaveOccurred())

			By("Verify is MetalLb in Running state")
			netmetallbhelper.SetupMetalLB()

			for idx, vlanID := range vlanIds {
				addrPoolLBList := netmlbparameters.IPv4AddressesLBList
				bgpPeerIP := netmlbparameters.NodeIntFacePrimaryIPAddr
				clientIP := netmlbparameters.InternalClient1IPv4
				clientPod := primaryClientPod
				dstSrvIP := netmlbparameters.IPv4AddressesLBList[0]
				serverLabel := netmlbparameters.AppLabel1

				if idx == 1 {
					addrPoolLBList = netmlbparameters.IPv4AddressesLB2List
					bgpPeerIP = netmlbparameters.NodeIntFaceSecondaryIPAddr
					clientIP = netmlbparameters.InternalClient2IPv4
					clientPod = secondaryClientPod
					dstSrvIP = netmlbparameters.IPv4AddressesLB2List[0]
					serverLabel = netmlbparameters.AppLabel2
				}

				By(fmt.Sprintf("Create nginx server on vlan %d", vlanID))
				_ = netmetallbhelper.DefineAndRunNGINXServer(serverLabel, workerNodeList[1].Name)

				By("Verify if frr pod has bgp session UP")
				Eventually(func() bool {
					return netmetallbhelper.CheckNeighborStatus(runningFrrPodList[idx], bgpPeerIP)
				}, 2*time.Minute, netmlbparameters.Interval).Should(BeTrue())

				By(fmt.Sprintf("Validate that BGP routes are present on frr pod on vlan %d", vlanID))
				Eventually(func() error {
					return netmetallbhelper.CheckBGPRoute(runningFrrPodList[idx], bgpPeerIP, addrPoolLBList[0], "ipv4", 32)
				}, netmlbparameters.Timeout, netmlbparameters.Interval).ShouldNot(HaveOccurred())

				By("Generate connections")
				generateConnections(clientPod, clientIP, dstSrvIP)
			}
		})
	})
})

func metalLbIsRunningAndInLocalMode() {
	_, err = helper.Apiclient.Deployments(netmlbparameters.TestNamespace).Get(
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
}

func generateConnections(srcPod *k8sv1.Pod, srcPodIP, dstIP string) {
	for idx := 0; ; idx++ {
		_, err := netmetallbhelper.HTTPMlbPod(
			srcPod, srcPodIP, dstIP,
			"ipv4", srcPod.Spec.Containers[0].Name, "")
		Expect(err).ToNot(HaveOccurred())

		if idx == 3 {
			break
		}
	}
}

func waitUntilNMStatePolicyStable(policyName string) {
	Eventually(func() bool {
		nmstateInstalledPolicy := nmstatev1.NodeNetworkConfigurationPolicy{}
		err = helper.Apiclient.Get(context.TODO(), goclient.ObjectKey{Name: policyName}, &nmstateInstalledPolicy)

		for _, status := range nmstateInstalledPolicy.Status.Conditions {
			if status.Type == nmstatev1Shared.NodeNetworkConfigurationPolicyConditionAvailable {
				return status.Reason == nmstatev1Shared.NodeNetworkConfigurationEnactmentConditionSuccessfullyConfigured &&
					nmstateInstalledPolicy.Status.UnavailableNodeCount == 0
			}
		}

		return false
	}, 2*time.Minute, 2*time.Second).Should(BeTrue())
}

func defineAndRunNodeTrafficCapturePod(extInt, serviceIPPrimary, serviceIPSecondary, workerNodeName string) *k8sv1.Pod {
	tcpdumpHTTPSecondaryInt := []string{fmt.Sprintf("%s -e -i %s > %s", netmlbparameters.TCPDumpCMDNew, extInt,
		netmlbparameters.DumpFileName)}
	tcpdumpHTTPBrEx := []string{fmt.Sprintf("%s and host %s or host %s -i %s > %s",
		netmlbparameters.TCPDumpCMDNew, serviceIPPrimary, serviceIPSecondary, "br-ex", netmlbparameters.DumpFileName)}
	brExContainer := pod.DefineContainer(
		"tcpdump", tcpdumpHTTPBrEx, helper.Config.Network.TestContainerImage, netmlbparameters.NetAdminNetRawSysAdminSC)
	tcpDumpPodOnNode := pod.RedefineAsPrivileged(
		pod.DefinePodOnNode(
			netmlbparameters.TestNamespace, helper.Config.Network.TestContainerImage, workerNodeName,
		),
	)
	tcpDumpPodOnNode = pod.RedefineWithHostNetwork(
		pod.RedefineWithCommand(
			tcpDumpPodOnNode,
			netmlbparameters.BashCMD,
			tcpdumpHTTPSecondaryInt,
		),
	)
	tcpDumpPodOnNode = pod.RedefineWithAdditionalContainer(tcpDumpPodOnNode, *brExContainer)

	return helper.WaitUntilPodCreatedAndRunning(tcpDumpPodOnNode, 2*time.Minute)
}

func defineAndCreateInternalNad(nadName string) {
	nadInternal := nad.NewNadBuilder(nadName, netmlbparameters.TestNamespace)
	masterBridgePluginString, err := json.Marshal(nad.DefineMasterBridgePlugin(nadName, "br0", nad.DefineStaticIpam()))
	Expect(err).ToNot(HaveOccurred())
	nadInternal, err = nadInternal.BuildWithMasterPluginString(string(masterBridgePluginString))
	Expect(err).ToNot(HaveOccurred())
	err = nadInternal.Create(helper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
}

func defineAndCreateExternalNad(vlanID uint16, masterInt string) {
	masterBridgeVlanPlugin := nad.DefineBridgeVlanPlugin(
		fmt.Sprintf("external-%d", vlanID), masterInt, "bridge", vlanID, nad.DefineStaticIpam())
	masterBridgeVlanPluginStr, err := json.Marshal(masterBridgeVlanPlugin)
	Expect(err).ToNot(HaveOccurred())
	nadExternal, err := nad.NewNadBuilder(
		fmt.Sprintf("external-%d", vlanID),
		netmlbparameters.TestNamespace).BuildWithMasterPluginString(string(masterBridgeVlanPluginStr))
	Expect(err).ToNot(HaveOccurred())
	err = nadExternal.Create(helper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
}

func defineAndRunClientPodOnVlanNetwork(clientInternalIP, frrInternalDGIP, nadName, workerName string) *k8sv1.Pod {
	netAnnotationClient := pod.NetworkAnnotation{
		Networks: &[]multus.NetworkSelectionElement{
			*pod.DefinePodNetStaticIP(nadName, clientInternalIP, frrInternalDGIP),
		},
	}
	podClientNetAnnotation, err := netAnnotationClient.ConvertNetworksAnnotationToMap()
	Expect(err).ToNot(HaveOccurred())

	podClient := pod.RedefineWithCommand(
		pod.DefineWithOptions(netmlbparameters.TestNamespace, helper.Config.Network.TestContainerImage,
			netmetallbhelper.RedefineOnNode(workerName),
			nethelper.DefinePodNetworks(podClientNetAnnotation),
			nethelper.DefinePodWIthSecurityContext(netmlbparameters.NetAdminNetRawSysAdminSC)),
		netmlbparameters.BashCMD, netmlbparameters.TCPDumpCMDNet1)

	return helper.WaitUntilPodCreatedAndRunning(podClient, 3*time.Minute)
}

func defineAndCreateNMStatePolicy(
	interfaceName, nodeName string, vlanList []uint16) nmstatev1.NodeNetworkConfigurationPolicy {
	physicalInterface := netmetallbhelper.DefineNMInterface(interfaceName, "ethernet", "up", nil)
	nmStatePrimaryIntConfig := netmetallbhelper.DefineNmStateVlanInterfaceConfig(
		interfaceName, netmlbparameters.NodeIntFacePrimaryIPAddr, 24, vlanList[0])
	nmStatePrimaryIntRoute := netmetallbhelper.DefineNMStateRoute(
		netmlbparameters.BGPDstPrimaryNetwork, netmlbparameters.NodeIntFacePrimaryNextHop, nmStatePrimaryIntConfig.Name)

	nmStateSecondaryIntConfig := netmetallbhelper.DefineNmStateVlanInterfaceConfig(
		interfaceName, netmlbparameters.NodeIntFaceSecondaryIPAddr, 24, vlanList[1])
	nmStateSecondaryIntRoute := netmetallbhelper.DefineNMStateRoute(
		netmlbparameters.BGPDstSecondaryNetwork, netmlbparameters.NodeIntFaceSecondaryNextHop, nmStateSecondaryIntConfig.Name)

	nmStatePolicy, err := netmetallbhelper.DefineNMStatePolicy(
		netmlbparameters.NMStatePolicyName, nodeName,
		[]netmlbparameters.NMStateInterface{*physicalInterface, *nmStatePrimaryIntConfig, *nmStateSecondaryIntConfig},
		[]netmlbparameters.NMStateRoute{*nmStatePrimaryIntRoute, *nmStateSecondaryIntRoute})
	Expect(err).ToNot(HaveOccurred())

	err = helper.Apiclient.Create(context.TODO(), nmStatePolicy)
	Expect(err).ToNot(HaveOccurred())

	waitUntilNMStatePolicyStable(netmlbparameters.NMStatePolicyName)

	var nmStateInstalledPolicy nmstatev1.NodeNetworkConfigurationPolicy
	err = helper.Apiclient.Get(
		context.TODO(), goclient.ObjectKey{Name: netmlbparameters.NMStatePolicyName}, &nmStateInstalledPolicy)
	Expect(err).ToNot(HaveOccurred())

	return nmStateInstalledPolicy
}

func defineAndRunFrrRouter(vlanID uint16, frrVlanIPAddr, frrInternalDGAddr, nodeName, configMapName string) *k8sv1.Pod {
	podNetAnnotation := pod.NetworkAnnotation{
		Networks: &[]multus.NetworkSelectionElement{
			*pod.DefinePodNetStaticIP(fmt.Sprintf("external-%d", vlanID), frrVlanIPAddr),
			*pod.DefinePodNetStaticIP(netmlbparameters.InternalNADName, frrInternalDGAddr),
		},
	}
	podNetConf, err := podNetAnnotation.ConvertNetworksAnnotationToMap()
	Expect(err).ToNot(HaveOccurred(), "error converting pod's network annotation to map")

	frrPod := nethelper.DefineFRRPodWithConfigMap(nodeName, netmlbparameters.TestNamespace, configMapName, false)
	frrPod.Annotations = podNetConf

	return helper.WaitUntilPodCreatedAndRunning(frrPod, 3*time.Minute)
}

// getSecondaryInterfaces returns a filtered list of parameter secondaryInterfaces
// (no bridges, no default routes, physical and link state up).
func getSecondaryInterfaces(nodeName string, secondaryInterfaces []string) []string {
	var requestedSecondaryInterfaces []string

	nodeInterfaceList, err := nodes.GetPhysicalNodeInterfaces(
		helper.Apiclient, nodeName, netmlbparameters.TestNamespace)
	Expect(err).ToNot(HaveOccurred())

	for _, oneInterface := range nodeInterfaceList {
		if !oneInterface.Bridge && !oneInterface.DefRoute && oneInterface.Physical && oneInterface.UP {
			if contains(secondaryInterfaces, oneInterface.Name) {
				requestedSecondaryInterfaces = append(requestedSecondaryInterfaces, oneInterface.Name)
			}
		}
	}

	return requestedSecondaryInterfaces
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}
