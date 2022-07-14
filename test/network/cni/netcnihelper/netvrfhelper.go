package netcnihelper

import (
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
	k8sv1 "k8s.io/api/core/v1"

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
	}

	return podNetAnnotation.WithNetworks(podNetworks).Annotation.ConvertNetworksAnnotationToMap()
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

// DefineServerPodMultiHTTPContainersNew defines server pod with multiple http containers.
func DefineServerPodMultiHTTPContainersNew(
	config *config.Config, podServerNodeLabel string, podServerIpamConfig map[string]string) *k8sv1.Pod {
	httpServerCmd := []string{
		"--protocol=tcp", "--listen", "--mtu=100", fmt.Sprintf("--port=%d", netcniparameters.TCPPort),
	}
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

	if ipStack == netcniparameters.IPStackIPv6 {
		vrfRedClientIP = netcniparameters.VRFRedClientIPv6Address
		vrfBlueClientIP = netcniparameters.VRFBlueClientIPv6Address
		vrfRedServerIP = netcniparameters.VRFRedServerIPv6Address
		vrfBlueServerIP = netcniparameters.VRFBlueServerIPv6Address

		if scenario == "nonOverLap" {
			vrfRedClientIP = "2001:200::3"
			vrfRedServerIP = "2001:200::4"
		}
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
