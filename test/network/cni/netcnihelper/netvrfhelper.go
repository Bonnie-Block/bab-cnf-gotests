package netcnihelper

import (
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
	k8sv1 "k8s.io/api/core/v1"

<<<<<<< HEAD
=======
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
>>>>>>> 6db12c57 (Add PTP events for boundary clock)
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

// AppendToSliceAtIndex appends element in to specific index of the slice.
func AppendToSliceAtIndex(slice []string, element string, index int) []string {
	slice = append(slice[:index+1], slice[index:]...)
	slice[index] = element

	return slice
}

// DefinePodNetAnnotation defines net annotation based on given vrf net config.
func DefinePodNetAnnotation(podNetConfigs []netcniparameters.VrfNetConfig, iPAMType string) (map[string]string, error) {
	podNetAnnotation := pod.NewPodNetBuilder()

	var podNetworks []multus.NetworkSelectionElement

	for _, podNetConfig := range podNetConfigs {
		if iPAMType == netcniparameters.VRFIpamStatic {
			podNetworks = append(
				podNetworks, *pod.DefinePodNetStaticMacIP(podNetConfig.NetName, podNetConfig.Mac,
					fmt.Sprintf("%s/%s", podNetConfig.IPAddr, podNetConfig.NetPrefix)))
		} else {
			podNetworks = append(
				podNetworks, *pod.DefinePodNetStaticMac(podNetConfig.NetName, podNetConfig.Mac))
		}
<<<<<<< HEAD
=======
	case "nonOverLap":
		if ipStack == netcniparameters.IPStackIPv4 {
			By("Setting overlapping non-SDN IP Addresses for VRF Red")

			podClientVRFBlueIPAddress = netcniparameters.VRFClientIPAddress
			podServerVRFBlueIPAddress = netcniparameters.VRFServerIPAddress
			podClientVRFRedIPAddress = "192.168.255.3"
			podServerVRFRedIPAddress = "192.168.255.4"
			blueVRFNetworkPrefix = netparameters.IPV4Subnet
			redVRFNetworkPrefix = netparameters.IPV4Subnet
		} else {
			podClientVRFBlueIPAddress = "2201:100::1"
			podServerVRFBlueIPAddress = "2201:100::2"
			podClientVRFRedIPAddress = "2201:200::3"
			podServerVRFRedIPAddress = "2201:200::4"
			redVRFNetworkPrefix = netparameters.IPV6Subnet
			blueVRFNetworkPrefix = netparameters.IPV6Subnet
		}
	default:
		{
			Fail(fmt.Sprintf("%v scenario doesn't exist", ipOverLap))
		}
	}

	By("Define client/server pods")

	podClientIpamConfig, podServerIpamConfig := defineClientServerIpamConfig(
		ipamType, vrfNetworkBlue, vrfNetworkRed, podClientVRFBlueIPAddress,
		blueVRFNetworkPrefix, podClientVRFRedIPAddress, redVRFNetworkPrefix,
		podServerVRFBlueIPAddress, podServerVRFRedIPAddress)

	if ipamType == netcniparameters.VRFIpamDHCP {
		runDHCPServer(podClientVRFBlueIPAddress, podServerVRFBlueIPAddress,
			podClientVRFRedIPAddress, podServerVRFRedIPAddress, VRFParameters.Node, nodes)
	}

	podClient := pod.RedefineAsNetRaw(
		pod.RedefinePodWithNetwork(
			pod.DefinePodOnNode(netcniparameters.TestNamespace, config.Network.TestContainerImage, podClientNodeLabel),
			podClientIpamConfig,
		),
	)
	podServer := defineServerPodMultiHTTPContainers(config, podServerNodeLabel, podServerIpamConfig)

	By("Running client/server pods")

	runningClientPod := helper.WaitUntilPodCreatedAndRunning(podClient, netcniparameters.PodWaitingTime)
	helper.WaitUntilPodCreatedAndRunning(podServer, netcniparameters.PodWaitingTime)
	By("Validating client/server VRFs configuration")
	podHasCorrectVrfConfig(podClient.Name,
		[]map[string]string{
			{"vrfName": netcniparameters.VRFBlueName, "vrfClientIP": podClientVRFBlueIPAddress, "vrfInterface": "net1"},
			{"vrfName": netcniparameters.VRFRedName, "vrfClientIP": podClientVRFRedIPAddress, "vrfInterface": "net2"}})
	podHasCorrectVrfConfig(podServer.Name,
		[]map[string]string{
			{"vrfName": netcniparameters.VRFBlueName, "vrfClientIP": podServerVRFBlueIPAddress, "vrfInterface": "net1"},
			{"vrfName": netcniparameters.VRFRedName, "vrfClientIP": podServerVRFRedIPAddress, "vrfInterface": "net2"}})

	By("Validating client/server ICMP VRF connectivity")

	err = pingIPViaVRF(*runningClientPod, netcniparameters.VRFRedName, podServerVRFRedIPAddress, false)
	Expect(err).ToNot(HaveOccurred())
	err = pingIPViaVRF(*runningClientPod, netcniparameters.VRFBlueName, podServerVRFBlueIPAddress, false)
	Expect(err).ToNot(HaveOccurred())
	By("Validating client/server TCP VRF connectivity")

	err = httpViaVRF(*runningClientPod, podServerVRFRedIPAddress, netcniparameters.VRFRedName, false)
	Expect(err).ToNot(HaveOccurred())
	err = httpViaVRF(*runningClientPod, podServerVRFBlueIPAddress, netcniparameters.VRFBlueName, false)
	Expect(err).ToNot(HaveOccurred())
	err = pod.DeletePodAndWait(helper.Apiclient, podServer)
	Expect(err).ToNot(HaveOccurred())

	By("Validating client/server ICMP negative test")
	Eventually(func() error {
		_, err := helper.Apiclient.Pods(netcniparameters.TestNamespace).Get(
			context.Background(),
			podServer.Name,
			metav1.GetOptions{})

		return err
	}, netcniparameters.PodWaitingTime, 5*time.Second).Should(HaveOccurred())

	err = pingIPViaVRF(*runningClientPod, netcniparameters.VRFBlueName, podServerVRFBlueIPAddress, true)
	Expect(err).ToNot(HaveOccurred())
	err = pingIPViaVRF(*runningClientPod, netcniparameters.VRFRedName, podServerVRFRedIPAddress, true)
	Expect(err).ToNot(HaveOccurred())
	By("Validating client/server TCP negative test")

	err = httpViaVRF(*runningClientPod, podServerVRFRedIPAddress, netcniparameters.VRFRedName, true)
	Expect(err).ToNot(HaveOccurred())
	err = httpViaVRF(*runningClientPod, podServerVRFBlueIPAddress, netcniparameters.VRFBlueName, true)
	Expect(err).ToNot(HaveOccurred())

	if ipOverLap == "overLapToSDN" {
		err = pingIPViaVRF(*runningClientPod, "eth0", podServerVRFRedIPAddress, false)
		Expect(err).ToNot(HaveOccurred())
		err = httpViaVRF(*runningClientPod, podServerVRFRedIPAddress, "eth0", false)
		Expect(err).ToNot(HaveOccurred())
>>>>>>> 6db12c57 (Add PTP events for boundary clock)
	}

<<<<<<< HEAD
	return podNetAnnotation.WithNetworks(podNetworks).Annotation.ConvertNetworksAnnotationToMap()
=======
func podHasCorrectVrfConfig(podName string, vrfMapsConfig []map[string]string) {
	runningPod, err := helper.Apiclient.Pods(netcniparameters.TestNamespace).Get(
		context.Background(),
		podName,
		metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())

	var ipStack string

	if ipStack == netcniparameters.IPStackIPv4 {
		for _, vrfMapConfig := range vrfMapsConfig {
			validateVrfIPAddrCommand := []string{"ip", "addr", "show", vrfMapConfig["vrfInterface"]}

			Eventually(func() bool {
				vrfIface, _ := pod.ExecCommand(helper.Apiclient, *runningPod, validateVrfIPAddrCommand)

				return strings.Contains(vrfIface.String(), vrfMapConfig["vrfClientIP"])
			}, netcniparameters.PodWaitingTime, 5*time.Second).Should(
				BeTrue(),
				fmt.Errorf("VRF interface is not present"),
			)

			validateVRFRouteTableCommand := []string{"ip", "route", "show", "vrf", vrfMapConfig["vrfName"]}
			Eventually(func() bool {
				vrfRouteTable, _ := pod.ExecCommand(helper.Apiclient, *runningPod, validateVRFRouteTableCommand)

				return strings.Contains(vrfRouteTable.String(), vrfMapConfig["vrfClientIP"])
			}, netcniparameters.PodWaitingTime, 5*time.Second).Should(
				BeTrue(),
				fmt.Errorf(fmt.Sprintf("VRF %s route table is not present", vrfMapConfig["vrfName"])),
			)
		}
	} else if ipStack == netcniparameters.IPStackIPv6 {
		for _, vrfMapConfig := range vrfMapsConfig {
			validateVrfIPAddrCommand := []string{"ip", "-6", "addr", "show", vrfMapConfig["vrfInterface"]}
			Eventually(func() bool {
				vrfIface, _ := pod.ExecCommand(helper.Apiclient, *runningPod, validateVrfIPAddrCommand)

				return strings.Contains(vrfIface.String(), vrfMapConfig["vrfClientIP"])
			}, netcniparameters.PodWaitingTime, 5*time.Second).Should(
				BeTrue(),
				fmt.Errorf("VRF interface is not present"))

			validateVRFRouteTableCommand := []string{"ip", "-6", "route", "show", "vrf", vrfMapConfig["vrfName"]}
			Eventually(func() bool {
				vrfRouteTable, _ := pod.ExecCommand(helper.Apiclient, *runningPod, validateVRFRouteTableCommand)
				_, ipnet, _ := net.ParseCIDR(vrfMapConfig["vrfClientIP"] + "/" + netparameters.IPV6Subnet)

				return strings.Contains(vrfRouteTable.String(), ipnet.String())
			}, netcniparameters.PodWaitingTime, 5*time.Second).Should(
				BeTrue(),
				fmt.Errorf(fmt.Sprintf("VRF %s route table is not present", vrfMapConfig["vrfName"])),
			)
		}
	}
>>>>>>> 6db12c57 (Add PTP events for boundary clock)
}

// DescribeParameters validates given parameters and returns json formatted string.
func DescribeParameters(node string, ipStack string) string {
	VRFParameters, err := netcniparameters.NewVRFTestParameters(node, ipStack)

	if err != nil {
		return fmt.Sprintf("error in parameters: node=%s, ipStack=%s", node, ipStack)
	}

	params, err := json.Marshal(VRFParameters)

	if err != nil {
		return fmt.Sprintf("error in parameters: node=%s, ipStack=%s", node, ipStack)
	}

	return string(params)
}

// GetNodeInterfaces returns list of requested interfaces.
func GetNodeInterfaces(conf *config.Config, nodeInterfaceList []nodes.NodeInterface,
	requestedNumber int) ([]nodes.NodeInterface, error) {
	var validNodeInterfaceList []nodes.NodeInterface

	if conf.Network.SriovInterfaces == "" {
		return nil, fmt.Errorf("environment variable CNF_INTERFACES_LIST is not set")
	}

	requestedNodeInterfaceList := strings.Split(conf.Network.SriovInterfaces, ",")

	if len(requestedNodeInterfaceList) < requestedNumber {
		return nil, fmt.Errorf("CNF_INTERFACES_LIST has less interfaces than requested by test suite")
	}

	for _, availableNodeInterface := range nodeInterfaceList {
		for _, requestedNodeInterface := range requestedNodeInterfaceList {
			if availableNodeInterface.Name == requestedNodeInterface {
				validNodeInterfaceList = append(validNodeInterfaceList, availableNodeInterface)
			}
		}
	}

	if len(validNodeInterfaceList) < requestedNumber {
		return nil, fmt.Errorf(
			"requested interfaces %v are not present on cluster node",
			requestedNodeInterfaceList)
	}

	return validNodeInterfaceList, nil
}

// PingIPViaVRF runs icmp test on pod based on given parameters.
func PingIPViaVRF(client k8sv1.Pod, vrfName, destIPAddr string, negative bool) error {
	command := []string{"testcmd", "-interface", vrfName, "-server", destIPAddr, "-protocol", "icmp", "-mtu", "100"}
	if negative {
		command = append(command, "--negative")
	}

	_, err := pod.ExecCommand(
		helper.Apiclient,
		client,
		command)

	return err
}

// HTTPViaVRF runs http test on pod based on given parameters.
func HTTPViaVRF(client k8sv1.Pod, destIPAddr, interfaceName string, negative bool) error {
	command := []string{
		"testcmd",
		fmt.Sprintf("--interface=%s", interfaceName),
		fmt.Sprintf("--server=%s", destIPAddr),
		"--protocol=tcp",
		"--mtu=100",
		fmt.Sprintf("--port=%d", netcniparameters.TCPPort),
	}
	if negative {
		command = append(command, "--negative")
	}

	_, err := pod.ExecCommand(
		helper.Apiclient,
		client,
		command)

	return err
}

<<<<<<< HEAD
// DefineServerPodMultiHTTPContainersNew defines server pod with multiple http containers.
func DefineServerPodMultiHTTPContainersNew(
	config *config.Config, podServerNodeLabel string, podServerIpamConfig map[string]string) *k8sv1.Pod {
	httpServerCmd := []string{
		"--protocol=tcp", "--listen", "--mtu=100", fmt.Sprintf("--port=%d", netcniparameters.TCPPort),
	}
=======
func getOverlapIP(nodeName string, podImage string) string {
	tempPodDefinition := pod.RedefineWithCommand(
		pod.RedefineAsNetRaw(
			pod.DefinePodOnNode(netcniparameters.TestNamespace, podImage, nodeName)),
		[]string{"testcmd"},
		[]string{
			"--protocol=tcp",
			"--interface=eth0",
			"--listen",
			"--mtu=100",
			fmt.Sprintf("--port=%d", netcniparameters.TCPPort)})
	err := helper.Apiclient.Create(context.Background(), tempPodDefinition)
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() k8sv1.PodPhase {
		tempPod, _ := helper.Apiclient.Pods(netcniparameters.TestNamespace).Get(
			context.Background(),
			tempPodDefinition.Name,
			metav1.GetOptions{})

		return tempPod.Status.Phase
	}, netcniparameters.PodWaitingTime, time.Second).Should(Equal(k8sv1.PodRunning))

	runningPod, err := helper.Apiclient.Pods(netcniparameters.TestNamespace).Get(
		context.Background(),
		tempPodDefinition.Name,
		metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())

	return runningPod.Status.PodIP
}

func defineServerPodMultiHTTPContainers(
	config *config.Config, podServerNodeLabel string, podServerIpamConfig string) *k8sv1.Pod {
>>>>>>> 6db12c57 (Add PTP events for boundary clock)
	podServer := pod.RedefineWithCommand(
		pod.RedefineAsNetRaw(
			pod.RedefinePodWithAnnotation(
				pod.DefinePodOnNode(
					netcniparameters.TestNamespace,
					config.Network.TestContainerImage,
					podServerNodeLabel,
				),
				podServerIpamConfig),
		),
		[]string{"testcmd"},
		append(httpServerCmd, fmt.Sprintf("--interface=%s", "eth0")))

	for idx, multusInterface := range []string{
		netcniparameters.MultusFirstInterfaceName, netcniparameters.MultusSecondInterfaceName} {
		podServer.Spec.Containers = append(podServer.Spec.Containers,
			k8sv1.Container{
				Name:            fmt.Sprintf("%s%d", podServer.Spec.Containers[0].Name, idx),
				Image:           podServer.Spec.Containers[0].Image,
				Command:         podServer.Spec.Containers[0].Command,
				SecurityContext: podServer.Spec.Containers[0].SecurityContext,
				Args:            append(httpServerCmd, fmt.Sprintf("--interface=%s", multusInterface)),
			})
	}

	return podServer
}

// DefineVrfTestParamStaticMac defines ip configuration for client/server red,blue VRFs with static mac.
func DefineVrfTestParamStaticMac(vrfRedName, vrfBlueName string) (
	[]netcniparameters.VrfNetConfig, []netcniparameters.VrfNetConfig) {
	vrfClientNetConfig, vrfServerNetConfig := DefineClientServerVRFsIPConfig(
		vrfRedName, vrfBlueName, "overLapToVRF", netcniparameters.IPStackIPv4)
	vrfClientNetConfig[0].Mac = netcniparameters.VRFClientMacAddressBlue
	vrfClientNetConfig[1].Mac = netcniparameters.VRFClientMacAddressRed
	vrfServerNetConfig[0].Mac = netcniparameters.VRFServerMacAddressBlue
	vrfServerNetConfig[1].Mac = netcniparameters.VRFServerMacAddressRed

	return vrfClientNetConfig, vrfServerNetConfig
}

// DefineClientServerVRFsIPConfig defines ip configuration for client/server red,blue VRFs.
func DefineClientServerVRFsIPConfig(vrfRedName, vrfBlueName, scenario, ipStack string) (
	[]netcniparameters.VrfNetConfig, []netcniparameters.VrfNetConfig) {
	vrfBlueClientIP := netcniparameters.VRFBlueClientIPAddress
	vrfBlueServerIP := netcniparameters.VRFBlueServerIPAddress

	vrfRedServerIP := netcniparameters.VRFRedServerIPAddress
	vrfRedClientIP := netcniparameters.VRFRedClientIPAddress

	if scenario == "nonOverLap" {
		vrfRedClientIP = "192.168.255.3"
		vrfRedServerIP = "192.168.255.4"
	}
<<<<<<< HEAD

	if ipStack == netcniparameters.IPStackIPv6 {
		vrfRedClientIP = netcniparameters.VRFRedClientIPv6Address
		vrfBlueClientIP = netcniparameters.VRFBlueClientIPv6Address
		vrfRedServerIP = netcniparameters.VRFRedServerIPv6Address
		vrfBlueServerIP = netcniparameters.VRFBlueServerIPv6Address

		if scenario == "nonOverLap" {
			vrfRedClientIP = "2001:200::3"
			vrfRedServerIP = "2001:200::4"
		}
=======
	validMacVlanInterfaces := GetNodeValidMacVlanInterface(nodes[0], helper.Config, 1)
	err := nethelper.DefineDhcpServerOnNad(
		netcniparameters.TestNamespace, validMacVlanInterfaces[0].Name, nodes[0], "10.255.255.201",
		addressMap)
	Expect(err).ToNot(HaveOccurred())

	if nodeMode == netcniparameters.DiffNode {
		err = nethelper.DefineDhcpServerOnNad(
			netcniparameters.TestNamespace, validMacVlanInterfaces[0].Name, nodes[1], "10.255.255.202",
			addressMap)
		Expect(err).ToNot(HaveOccurred())
>>>>>>> 6db12c57 (Add PTP events for boundary clock)
	}

	vrfClientNetConfig := []netcniparameters.VrfNetConfig{
		defineVRFIpConfig(vrfBlueName, netcniparameters.VRFBlueName, vrfBlueClientIP),
		defineVRFIpConfig(vrfRedName, netcniparameters.VRFRedName, vrfRedClientIP)}
	vrfServerNetConfig := []netcniparameters.VrfNetConfig{
		defineVRFIpConfig(vrfBlueName, netcniparameters.VRFBlueName, vrfBlueServerIP),
		defineVRFIpConfig(vrfRedName, netcniparameters.VRFRedName, vrfRedServerIP)}

	return vrfClientNetConfig, vrfServerNetConfig
}

func defineVRFIpConfig(nadName, vrfName, ipAddr string) netcniparameters.VrfNetConfig {
	multusIntName := netcniparameters.MultusFirstInterfaceName
	subnet := netparameters.IPSubnet24

	if vrfName == netcniparameters.VRFRedName {
		multusIntName = netcniparameters.MultusSecondInterfaceName
	}

	if strings.Contains(ipAddr, ":") {
		subnet = netparameters.IPSubnet64
	}

	return netcniparameters.VrfNetConfig{
		NetName:      nadName,
		VrfInterface: multusIntName,
		VrfName:      vrfName,
		IPAddr:       ipAddr,
		NetPrefix:    subnet}
}
