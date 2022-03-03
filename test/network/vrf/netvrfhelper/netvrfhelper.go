package netvrfhelper

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	globalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/netvrfparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestVRFScenario verifies that VRF feature works as expected.
func TestVRFScenario(node string, ipStack string, ipOverLap string, config *config.Config, nodes []string,
	vrfNetworkBlue string, vrfNetworkRed string, ipamType string) {
	var (
		podClientNodeLabel        string
		podServerNodeLabel        string
		redVRFNetworkPrefix       string
		blueVRFNetworkPrefix      string
		podClientVRFRedIPAddress  string
		podServerVRFRedIPAddress  string
		podClientVRFBlueIPAddress string
		podServerVRFBlueIPAddress string
	)

	VRFParameters, err := netvrfparameters.NewVRFTestParameters(node, ipStack)
	Expect(err).ToNot(HaveOccurred())

	if VRFParameters.Node == netvrfparameters.DiffNode && len(nodes) < 2 {
		Skip(fmt.Sprintf("There is not enough nodes to run test with following parameter %s", node))
	}

	By("Validating test parameters")

	if VRFParameters.Node == netvrfparameters.SameNode {
		podClientNodeLabel = nodes[0]
		podServerNodeLabel = nodes[0]
	} else if VRFParameters.Node == netvrfparameters.DiffNode {
		podClientNodeLabel = nodes[0]
		podServerNodeLabel = nodes[1]
	}

	switch ipOverLap {
	case "overLapToSDN":
		if ipStack == netvrfparameters.IPStackIPv6 {
			Skip("Skipping SDN IPv6 is not currently tested")
		}

		if VRFParameters.Node == netvrfparameters.SameNode {
			redVRFNetworkPrefix = "24"
		} else if VRFParameters.Node == netvrfparameters.DiffNode {
			redVRFNetworkPrefix = "8"
		}

		By("Getting overlapping SDN IP Addresses for VRF Red")

		podClientVRFRedIPAddress = getOverlapIP(podClientNodeLabel, config.Network.TestContainerImage)
		podServerVRFRedIPAddress = getOverlapIP(podServerNodeLabel, config.Network.TestContainerImage)

		if ipStack == netvrfparameters.IPStackIPv4 && net.ParseIP(podClientVRFRedIPAddress).To4() == nil {
			Skip("Skipping IPv4 test. Cluster only supports IPv6 protocol")
		}

		By("Setting overlapping IP Address for VRF Blue")

		podClientVRFBlueIPAddress = netvrfparameters.VRFClientIPAddress
		podServerVRFBlueIPAddress = netvrfparameters.VRFServerIPAddress
		blueVRFNetworkPrefix = "24"
	case "overLapToVRF":
		if ipStack == netvrfparameters.IPStackIPv4 {
			By("Setting overlapping non-SDN IP Addresses for VRF Red")

			podClientVRFBlueIPAddress = netvrfparameters.VRFClientIPAddress
			podServerVRFBlueIPAddress = netvrfparameters.VRFServerIPAddress
			podClientVRFRedIPAddress = "10.255.255.3"
			podServerVRFRedIPAddress = "10.255.255.4"
			blueVRFNetworkPrefix = "24"
			redVRFNetworkPrefix = "24"
		} else {
			podClientVRFBlueIPAddress = "2001:100::1"
			podServerVRFBlueIPAddress = "2001:100::2"
			podClientVRFRedIPAddress = "2001:100::3"
			podServerVRFRedIPAddress = "2001:100::4"
			redVRFNetworkPrefix = "64"
			blueVRFNetworkPrefix = "64"
		}
	case "nonOverLap":
		if ipStack == netvrfparameters.IPStackIPv4 {
			By("Setting overlapping non-SDN IP Addresses for VRF Red")

			podClientVRFBlueIPAddress = netvrfparameters.VRFClientIPAddress
			podServerVRFBlueIPAddress = netvrfparameters.VRFServerIPAddress
			podClientVRFRedIPAddress = "192.168.255.3"
			podServerVRFRedIPAddress = "192.168.255.4"
			blueVRFNetworkPrefix = "24"
			redVRFNetworkPrefix = "24"
		} else {
			podClientVRFBlueIPAddress = "2201:100::1"
			podServerVRFBlueIPAddress = "2201:100::2"
			podClientVRFRedIPAddress = "2201:200::3"
			podServerVRFRedIPAddress = "2201:200::4"
			redVRFNetworkPrefix = "64"
			blueVRFNetworkPrefix = "64"
		}
	default:
		{
			Fail(fmt.Sprintf("%v scenario doesn't exsit", ipOverLap))
		}
	}

	By("Define client/server pods")

	podClientIpamConfig, podServerIpamConfig := defineClientServerIpamConfig(
		ipamType, vrfNetworkBlue, vrfNetworkRed, podClientVRFBlueIPAddress,
		blueVRFNetworkPrefix, podClientVRFRedIPAddress, redVRFNetworkPrefix,
		podServerVRFBlueIPAddress, podServerVRFRedIPAddress)

	if ipamType == netvrfparameters.VRFIpamDHCP {
		runDHCPServer(podClientVRFBlueIPAddress, podServerVRFBlueIPAddress,
			podClientVRFRedIPAddress, podServerVRFRedIPAddress, nodes[0])
	}

	podClient := pod.RedefineAsNetRaw(
		pod.RedefinePodWithNetwork(
			pod.DefinePodOnNode(netvrfparameters.TestNamespace, config.Network.TestContainerImage, podClientNodeLabel),
			podClientIpamConfig,
		),
	)
	podServer := defineServerPodMultiHTTPContainers(config, podServerNodeLabel, podServerIpamConfig)

	By("Running client/server pods")

	runningClientPod := globalHelper.WaitUntilPodCreatedAndRunning(podClient, netvrfparameters.PodWaitingTime)
	globalHelper.WaitUntilPodCreatedAndRunning(podServer, netvrfparameters.PodWaitingTime)
	By("Validating client/server VRFs configuration")
	podHasCorrectVrfConfig(podClient.Name,
		[]map[string]string{
			{"vrfName": netvrfparameters.VRFBlueName, "vrfClientIP": podClientVRFBlueIPAddress, "vrfInterface": "net1"},
			{"vrfName": netvrfparameters.VRFRedName, "vrfClientIP": podClientVRFRedIPAddress, "vrfInterface": "net2"}})
	podHasCorrectVrfConfig(podServer.Name,
		[]map[string]string{
			{"vrfName": netvrfparameters.VRFBlueName, "vrfClientIP": podServerVRFBlueIPAddress, "vrfInterface": "net1"},
			{"vrfName": netvrfparameters.VRFRedName, "vrfClientIP": podServerVRFRedIPAddress, "vrfInterface": "net2"}})

	By("Validating client/server ICMP VRF connectivity")

	err = pingIPViaVRF(*runningClientPod, netvrfparameters.VRFRedName, podServerVRFRedIPAddress, false)
	Expect(err).ToNot(HaveOccurred())
	err = pingIPViaVRF(*runningClientPod, netvrfparameters.VRFBlueName, podServerVRFBlueIPAddress, false)
	Expect(err).ToNot(HaveOccurred())
	By("Validating client/server TCP VRF connectivity")

	err = httpViaVRF(*runningClientPod, podServerVRFRedIPAddress, netvrfparameters.VRFRedName, false)
	Expect(err).ToNot(HaveOccurred())
	err = httpViaVRF(*runningClientPod, podServerVRFBlueIPAddress, netvrfparameters.VRFBlueName, false)
	Expect(err).ToNot(HaveOccurred())
	err = pod.DeletePodAndWait(globalHelper.Apiclient, podServer)
	Expect(err).ToNot(HaveOccurred())

	By("Validating client/server ICMP negative test")
	Eventually(func() error {
		_, err := globalHelper.Apiclient.Pods(netvrfparameters.TestNamespace).Get(
			context.Background(),
			podServer.Name,
			metav1.GetOptions{})

		return err
	}, netvrfparameters.PodWaitingTime, 5*time.Second).Should(HaveOccurred())

	err = pingIPViaVRF(*runningClientPod, netvrfparameters.VRFBlueName, podServerVRFBlueIPAddress, true)
	Expect(err).ToNot(HaveOccurred())
	err = pingIPViaVRF(*runningClientPod, netvrfparameters.VRFRedName, podServerVRFRedIPAddress, true)
	Expect(err).ToNot(HaveOccurred())
	By("Validating client/server TCP negative test")

	err = httpViaVRF(*runningClientPod, podServerVRFRedIPAddress, netvrfparameters.VRFRedName, true)
	Expect(err).ToNot(HaveOccurred())
	err = httpViaVRF(*runningClientPod, podServerVRFBlueIPAddress, netvrfparameters.VRFBlueName, true)
	Expect(err).ToNot(HaveOccurred())

	if ipOverLap == "overLapToSDN" {
		err = pingIPViaVRF(*runningClientPod, "eth0", podServerVRFRedIPAddress, false)
		Expect(err).ToNot(HaveOccurred())
		err = httpViaVRF(*runningClientPod, podServerVRFRedIPAddress, "eth0", false)
		Expect(err).ToNot(HaveOccurred())
	}
}

func podHasCorrectVrfConfig(podName string, vrfMapsConfig []map[string]string) {
	runningPod, err := globalHelper.Apiclient.Pods(netvrfparameters.TestNamespace).Get(
		context.Background(),
		podName,
		metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())

	var ipStack string

	if ipStack == netvrfparameters.IPStackIPv4 {
		for _, vrfMapConfig := range vrfMapsConfig {
			validateVrfIPAddrCommand := []string{"ip", "addr", "show", vrfMapConfig["vrfInterface"]}

			Eventually(func() bool {
				vrfIface, _ := pod.ExecCommand(globalHelper.Apiclient, *runningPod, validateVrfIPAddrCommand)

				return strings.Contains(vrfIface.String(), vrfMapConfig["vrfClientIP"])
			}, netvrfparameters.PodWaitingTime, 5*time.Second).Should(
				BeTrue(),
				fmt.Errorf("VRF interface is not present"),
			)

			validateVRFRouteTableCommand := []string{"ip", "route", "show", "vrf", vrfMapConfig["vrfName"]}
			Eventually(func() bool {
				vrfRouteTable, _ := pod.ExecCommand(globalHelper.Apiclient, *runningPod, validateVRFRouteTableCommand)

				return strings.Contains(vrfRouteTable.String(), vrfMapConfig["vrfClientIP"])
			}, netvrfparameters.PodWaitingTime, 5*time.Second).Should(
				BeTrue(),
				fmt.Errorf(fmt.Sprintf("VRF %s route table is not present", vrfMapConfig["vrfName"])),
			)
		}
	} else if ipStack == netvrfparameters.IPStackIPv6 {
		for _, vrfMapConfig := range vrfMapsConfig {
			validateVrfIPAddrCommand := []string{"ip", "-6", "addr", "show", vrfMapConfig["vrfInterface"]}
			Eventually(func() bool {
				vrfIface, _ := pod.ExecCommand(globalHelper.Apiclient, *runningPod, validateVrfIPAddrCommand)

				return strings.Contains(vrfIface.String(), vrfMapConfig["vrfClientIP"])
			}, netvrfparameters.PodWaitingTime, 5*time.Second).Should(
				BeTrue(),
				fmt.Errorf("VRF interface is not present"))

			validateVRFRouteTableCommand := []string{"ip", "-6", "route", "show", "vrf", vrfMapConfig["vrfName"]}
			Eventually(func() bool {
				vrfRouteTable, _ := pod.ExecCommand(globalHelper.Apiclient, *runningPod, validateVRFRouteTableCommand)
				_, ipnet, _ := net.ParseCIDR(vrfMapConfig["vrfClientIP"] + "/64")

				return strings.Contains(vrfRouteTable.String(), ipnet.String())
			}, netvrfparameters.PodWaitingTime, 5*time.Second).Should(
				BeTrue(),
				fmt.Errorf(fmt.Sprintf("VRF %s route table is not present", vrfMapConfig["vrfName"])),
			)
		}
	}
}

// DescribeParameters validates given parameters and returns json formatted string.
func DescribeParameters(node string, ipStack string) string {
	VRFParameters, err := netvrfparameters.NewVRFTestParameters(node, ipStack)

	if err != nil {
		return fmt.Sprintf("error in parameters: node=%s, ipStack=%s", node, ipStack)
	}

	params, err := json.Marshal(VRFParameters)

	if err != nil {
		return fmt.Sprintf("error in parameters: node=%s, ipStack=%s", node, ipStack)
	}

	return string(params)
}

func pingIPViaVRF(client k8sv1.Pod, vrfName string, destIPAddr string, negative bool) error {
	command := []string{"testcmd", "-interface", vrfName, "-server", destIPAddr, "-protocol", "icmp", "-mtu", "100"}
	if negative {
		command = append(command, "--negative")
	}

	_, err := pod.ExecCommand(
		globalHelper.Apiclient,
		client,
		command)

	return err
}

func httpViaVRF(client k8sv1.Pod, destIPAddr string, interfaceName string, negative bool) error {
	command := []string{
		"testcmd",
		fmt.Sprintf("--interface=%s", interfaceName),
		fmt.Sprintf("--server=%s", destIPAddr),
		"--protocol=tcp",
		"--mtu=100",
		fmt.Sprintf("--port=%d", netvrfparameters.TCPPort),
	}
	if negative {
		command = append(command, "--negative")
	}

	_, err := pod.ExecCommand(
		globalHelper.Apiclient,
		client,
		command)

	return err
}

func getOverlapIP(nodeName string, podImage string) string {
	tempPodDefinition := pod.RedefineWithCommand(
		pod.RedefineAsNetRaw(
			pod.DefinePodOnNode(netvrfparameters.TestNamespace, podImage, nodeName)),
		[]string{"testcmd"},
		[]string{
			"--protocol=tcp",
			"--interface=eth0",
			"--listen",
			"--mtu=100",
			fmt.Sprintf("--port=%d", netvrfparameters.TCPPort)})
	err := globalHelper.Apiclient.Create(context.Background(), tempPodDefinition)
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() k8sv1.PodPhase {
		tempPod, _ := globalHelper.Apiclient.Pods(netvrfparameters.TestNamespace).Get(
			context.Background(),
			tempPodDefinition.Name,
			metav1.GetOptions{})

		return tempPod.Status.Phase
	}, netvrfparameters.PodWaitingTime, time.Second).Should(Equal(k8sv1.PodRunning))

	runningPod, err := globalHelper.Apiclient.Pods(netvrfparameters.TestNamespace).Get(
		context.Background(),
		tempPodDefinition.Name,
		metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())

	return runningPod.Status.PodIP
}

func defineServerPodMultiHTTPContainers(
	config *config.Config, podServerNodeLabel string, podServerIpamConfig string) *k8sv1.Pod {
	podServer := pod.RedefineWithCommand(
		pod.RedefineAsNetRaw(
			pod.RedefinePodWithNetwork(
				pod.DefinePodOnNode(
					netvrfparameters.TestNamespace,
					config.Network.TestContainerImage,
					podServerNodeLabel,
				),
				podServerIpamConfig),
		),
		[]string{"testcmd"},
		[]string{
			"--protocol=tcp",
			"--interface=eth0",
			"--listen",
			"--mtu=100",
			fmt.Sprintf("--port=%d", netvrfparameters.TCPPort)})
	podServer.Spec.Containers = append(podServer.Spec.Containers, k8sv1.Container{
		Name:    fmt.Sprintf("%s%d", podServer.Spec.Containers[0].Name, 1),
		Image:   podServer.Spec.Containers[0].Image,
		Command: []string{"testcmd"},
		SecurityContext: &k8sv1.SecurityContext{
			Capabilities: &k8sv1.Capabilities{
				Add: []k8sv1.Capability{"NET_RAW"},
			},
		},
		Args: []string{
			"--protocol=tcp",
			"--interface=net1",
			"--listen",
			"--mtu=100",
			fmt.Sprintf("--port=%d", netvrfparameters.TCPPort)},
	}, k8sv1.Container{
		Name:    fmt.Sprintf("%s%d", podServer.Spec.Containers[0].Name, 2),
		Image:   podServer.Spec.Containers[0].Image,
		Command: []string{"testcmd"},
		SecurityContext: &k8sv1.SecurityContext{
			Capabilities: &k8sv1.Capabilities{
				Add: []k8sv1.Capability{"NET_RAW"},
			},
		},
		Args: []string{
			"--protocol=tcp",
			"--interface=net2",
			"--listen",
			"--mtu=100",
			fmt.Sprintf("--port=%d", netvrfparameters.TCPPort)},
	})

	return podServer
}

func runDHCPServer(podClientVRFBlueIPAddress string, podServerVRFBlueIPAddress string,
	podClientVRFRedIPAddress string, podServerVRFRedIPAddress, nodeName string) {
	By("Run dhcp server pod")

	addressMap := map[string]string{
		netvrfparameters.VRFClientMacAddressBlue: podClientVRFBlueIPAddress,
		netvrfparameters.VRFServerMacAddressBlue: podServerVRFBlueIPAddress,
		netvrfparameters.VRFClientMacAddressRed:  podClientVRFRedIPAddress,
		netvrfparameters.VRFServerMacAddressRed:  podServerVRFRedIPAddress,
	}
	validMacVlanInterfaces := GetNodeValidMacVlanInterface(nodeName, globalHelper.Config, 1)
	err := nethelper.DefineDhcpServerOnNad(
		netvrfparameters.TestNamespace, validMacVlanInterfaces[0].Name, nodeName, "10.255.255.201",
		addressMap)
	Expect(err).ToNot(HaveOccurred())
}

func defineClientServerIpamConfig(
	ipamType string, vrfNetworkBlue string, vrfNetworkRed string, podClientVRFBlueIPAddress string,
	blueVRFNetworkPrefix string, podClientVRFRedIPAddress string, redVRFNetworkPrefix string,
	podServerVRFBlueIPAddress string, podServerVRFRedIPAddress string) (string, string) {
	podClientIpamConfig := fmt.Sprintf(
		`[{"name": "%s", "mac": "%s", "ips": ["%s/%s"]}, {"name": "%s", "mac": "%s", "ips": ["%s/%s"]}]`,
		vrfNetworkBlue, netvrfparameters.VRFClientMacAddressBlue, podClientVRFBlueIPAddress, blueVRFNetworkPrefix,
		vrfNetworkRed, netvrfparameters.VRFClientMacAddressRed, podClientVRFRedIPAddress, redVRFNetworkPrefix)
	podServerIpamConfig := fmt.Sprintf(
		`[{"name": "%s", "mac": "%s", "ips": ["%s/%s"]}, {"name": "%s", "mac": "%s", "ips": ["%s/%s"]}]`,
		vrfNetworkBlue, netvrfparameters.VRFServerMacAddressBlue, podServerVRFBlueIPAddress, blueVRFNetworkPrefix,
		vrfNetworkRed, netvrfparameters.VRFServerMacAddressRed, podServerVRFRedIPAddress, redVRFNetworkPrefix)

	if ipamType == netvrfparameters.VRFIpamDHCP {
		podClientIpamConfig = fmt.Sprintf(`[{"name": "%s", "mac": "%s"}, {"name": "%s", "mac": "%s"}]`,
			vrfNetworkBlue, netvrfparameters.VRFClientMacAddressBlue,
			vrfNetworkRed, netvrfparameters.VRFClientMacAddressRed)
		podServerIpamConfig = fmt.Sprintf(`[{"name": "%s", "mac": "%s"}, {"name": "%s", "mac": "%s"}]`,
			vrfNetworkBlue, netvrfparameters.VRFServerMacAddressBlue,
			vrfNetworkRed, netvrfparameters.VRFServerMacAddressRed)
	}

	return podClientIpamConfig, podServerIpamConfig
}
