package vrf

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcnihelper"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	k8sv1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testVRFScenario(node, ipStack, iPAMType string, config *config.Config, nodes []string, clientNetConfig,
	serverNetConfig []netcniparameters.VrfNetConfig) {
	By("Validating test parameters")

	VRFParameters, err := netcniparameters.NewVRFTestParameters(node, ipStack)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error to validate vrf test parameters due to %s", err))

	if VRFParameters.Node == netcniparameters.DiffNode && len(nodes) < 2 {
		Skip(fmt.Sprintf("there is not enough nodes to run test with following parameter %s", node))
	}

	nodeLabelForServerPod := nodes[0]

	if VRFParameters.Node == netcniparameters.DiffNode {
		nodeLabelForServerPod = nodes[1]
	}

	By("Define client/server pods")

	podClientNetAnnotation, err := netcnihelper.DefinePodNetAnnotation(clientNetConfig, iPAMType)
	Expect(err).ToNot(HaveOccurred(), "error to define client pod network annotation")

	podServerIpamConfig, err := netcnihelper.DefinePodNetAnnotation(serverNetConfig, iPAMType)
	Expect(err).ToNot(HaveOccurred(), "error to define server pod network annotation")

	if iPAMType == netcniparameters.VRFIpamDHCP {
		runDHCPServer(clientNetConfig[0].IPAddr, serverNetConfig[0].IPAddr,
			clientNetConfig[1].IPAddr, serverNetConfig[1].IPAddr, VRFParameters.Node, nodes)
	}

	podClient := pod.RedefineAsNetRaw(
		pod.RedefinePodWithAnnotation(
			pod.DefinePodOnNode(netcniparameters.TestNamespace, config.Network.TestContainerImage, nodes[0]),
			podClientNetAnnotation,
		),
	)
	podServer := netcnihelper.DefineServerPodMultiHTTPContainersNew(config, nodeLabelForServerPod, podServerIpamConfig)

	By("Running client/server pods")

	runningClientPod := helper.WaitUntilPodCreatedAndRunning(podClient, netcniparameters.PodWaitingTime)
	helper.WaitUntilPodCreatedAndRunning(podServer, netcniparameters.PodWaitingTime)

	By("Validating client/server VRFs configuration")
	podHasCorrectVrfConfig(runningClientPod.Name, clientNetConfig)
	podHasCorrectVrfConfig(podServer.Name, serverNetConfig)

	connectivityViaIcmpAndHTTP(*runningClientPod, serverNetConfig, false)

	err = pod.DeletePodAndWait(helper.Apiclient, podServer)
	Expect(err).ToNot(HaveOccurred(), "failed to remove pod")

	Eventually(func() error {
		_, err := helper.Apiclient.Pods(
			netcniparameters.TestNamespace).Get(context.Background(), podServer.Name, metav1.GetOptions{})

		return err
	}, netcniparameters.PodWaitingTime, 5*time.Second).Should(HaveOccurred())

	By("Validating client/server ICMP and HTTP negative test")
	connectivityViaIcmpAndHTTP(*runningClientPod, serverNetConfig, true)

	if serverNetConfig[0].NetPrefix == "8" {
		err = netcnihelper.PingIPViaVRF(*runningClientPod, "eth0", serverNetConfig[1].IPAddr, false)
		Expect(err).ToNot(HaveOccurred(), "failed occurred during to icmp test via main pod interface")
		err = netcnihelper.HTTPViaVRF(*runningClientPod, serverNetConfig[1].IPAddr, "eth0", false)
		Expect(err).ToNot(HaveOccurred(), "failed occurred during to http test via main pod interface")
	}
}

func podHasCorrectVrfConfig(podName string, vrfNetConfigs []netcniparameters.VrfNetConfig) {
	runningPod, err := helper.Apiclient.Pods(netcniparameters.TestNamespace).Get(
		context.Background(), podName, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error to query pod info for pod %s", podName))

	for _, vrfMapConfig := range vrfNetConfigs {
		validateVrfIPAddrCommand := []string{"ip", "addr", "show", vrfMapConfig.VrfInterface}
		validateVRFRouteTableCommand := []string{"ip", "route", "show", "vrf", vrfMapConfig.VrfName}

		if strings.Contains(vrfMapConfig.IPAddr, ":") {
			validateVrfIPAddrCommand = netcnihelper.AppendToSliceAtIndex(validateVrfIPAddrCommand, "-6", 1)
			validateVRFRouteTableCommand = netcnihelper.AppendToSliceAtIndex(validateVRFRouteTableCommand, "-6", 1)
		}

		Eventually(func() bool {
			vrfIface, _ := pod.ExecCommand(helper.Apiclient, *runningPod, validateVrfIPAddrCommand)

			return strings.Contains(vrfIface.String(), vrfMapConfig.IPAddr)
		}, netcniparameters.PodWaitingTime, 5*time.Second).Should(
			BeTrue(), "VRF interface is not present")

		Eventually(func() bool {
			vrfRouteTable, _ := pod.ExecCommand(helper.Apiclient, *runningPod, validateVRFRouteTableCommand)
			if strings.Contains(vrfMapConfig.IPAddr, ":") {
				_, ipnet, _ := net.ParseCIDR(vrfMapConfig.IPAddr + "/" + netparameters.IPV6Subnet)

				return strings.Contains(vrfRouteTable.String(), ipnet.String())
			}

			return strings.Contains(vrfRouteTable.String(), vrfMapConfig.IPAddr)
		}, netcniparameters.PodWaitingTime, 5*time.Second).Should(
			BeTrue(), fmt.Sprintf("VRF %s route table is not present", vrfMapConfig.VrfName))
	}
}

func runDHCPServer(podClientVRFBlueIPAddress, podServerVRFBlueIPAddress, podClientVRFRedIPAddress,
	podServerVRFRedIPAddress, nodeMode string, nodes []string) {
	By("Run dhcp server pod")

	addressMap := map[string]string{
		netcniparameters.VRFClientMacAddressBlue: podClientVRFBlueIPAddress,
		netcniparameters.VRFServerMacAddressBlue: podServerVRFBlueIPAddress,
		netcniparameters.VRFClientMacAddressRed:  podClientVRFRedIPAddress,
		netcniparameters.VRFServerMacAddressRed:  podServerVRFRedIPAddress,
	}
	validMacVlanInterfaces := getNodeValidMacVlanInterface(nodes[0], helper.Config, 1)
	err := nethelper.DefineDhcpServerOnNad(
		netcniparameters.TestNamespace, validMacVlanInterfaces[0].Name, nodes[0], "10.255.255.201",
		addressMap)
	Expect(err).ToNot(HaveOccurred(), "error to define dhcp pod on nad")

	if nodeMode == netcniparameters.DiffNode {
		err = nethelper.DefineDhcpServerOnNad(
			netcniparameters.TestNamespace, validMacVlanInterfaces[0].Name, nodes[1], "10.255.255.202",
			addressMap)
		Expect(err).ToNot(HaveOccurred(), "error to define dhcp pod on nad")
	}
}

func connectivityViaIcmpAndHTTP(runningClient k8sv1.Pod, srvNetConfig []netcniparameters.VrfNetConfig, negative bool) {
	By("Validating client/server ICMP and TCP VRF connectivity")

	for _, netConfig := range srvNetConfig {
		err := netcnihelper.PingIPViaVRF(runningClient, netConfig.VrfName, netConfig.IPAddr, negative)
		Expect(err).ToNot(HaveOccurred(), "icmp test failed")
		err = netcnihelper.HTTPViaVRF(runningClient, netConfig.IPAddr, netConfig.VrfName, negative)
		Expect(err).ToNot(HaveOccurred(), "http test failed")
	}
}

func getNodeValidMacVlanInterface(nodeName string, config *config.Config, requestNumber int) []nodes.NodeInterface {
	By("Select host interface for mac-vlan")

	var macVlanInterfaces []nodes.NodeInterface

	nodeInterfaceList, err := nodes.GetPhysicalNodeInterfaces(helper.Apiclient, nodeName, netcniparameters.TestNamespace)
	Expect(err).ToNot(HaveOccurred(), "error to collect node physical NICs")

	for _, oneInterface := range nodeInterfaceList {
		if !oneInterface.Bridge && !oneInterface.DefRoute && oneInterface.Physical && oneInterface.UP {
			macVlanInterfaces = append(macVlanInterfaces, oneInterface)
		}
	}

	validMacVlanInterfaces, err := netcnihelper.GetNodeInterfaces(config, macVlanInterfaces, requestNumber)
	Expect(err).ToNot(HaveOccurred(), "error to collect available secondary NICs")

	return validMacVlanInterfaces
}

func getOverlapIP(nodeName string, podImage string) string {
	tempPodDefinition := pod.RedefineWithCommand(
		pod.RedefineAsNetRaw(
			pod.DefinePodOnNode(netcniparameters.TestNamespace, podImage, nodeName)),
		[]string{"testcmd"},
		[]string{"--protocol=tcp", "--interface=eth0", "--listen", "--mtu=100",
			fmt.Sprintf("--port=%d", netcniparameters.TCPPort)})
	runningPod := helper.WaitUntilPodCreatedAndRunning(tempPodDefinition, netcniparameters.PodWaitingTime)

	return runningPod.Status.PodIP
}

func defineClientServerVRFsIPOverlapConfig(vrfRedNadName, vrfBlueNadName, nodeScheme string, nodeList []string) (
	[]netcniparameters.VrfNetConfig, []netcniparameters.VrfNetConfig) {
	clientVRFsNetConfig, serverVRFsNetConfig := netcnihelper.DefineClientServerVRFsIPConfig(
		vrfRedNadName, vrfBlueNadName, "overLapToVRF", netcniparameters.IPStackIPv4)
	podServerNodeLabel := nodeList[0]

	if nodeScheme == netcniparameters.DiffNode {
		podServerNodeLabel = nodeList[1]
		serverVRFsNetConfig[1].NetPrefix = "8"
		clientVRFsNetConfig[1].NetPrefix = "8"
	}

	clientVRFsNetConfig[1].IPAddr = getOverlapIP(nodeList[0], helper.Config.Network.TestContainerImage)
	serverVRFsNetConfig[1].IPAddr = getOverlapIP(podServerNodeLabel, helper.Config.Network.TestContainerImage)

	return clientVRFsNetConfig, serverVRFsNetConfig
}

func removeSRIOVNetworksAndNADsFromNamespace() {
	By("Remove all SriovNetwork")

	err := namespaces.CleanNetworks(parameters.SriovOperatorNamespace, helper.Apiclient)
	Expect(err).ToNot(HaveOccurred(), "error to remove all sriovNetworks")

	By("Remove NADs from namespace")

	err = namespaces.CleanNetworkAttachmentDefinitions(netcniparameters.TestNamespace, helper.Apiclient)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error removing NADs from the namespace due to: %s", err))
}
