package netsriovhelper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	v1 "github.com/operator-framework/api/pkg/operators/v1"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovparameters"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	protocolUPD = "udp"
)

// defineSriovNetworkStaticIPAM builds SriovNetwork resource.
func defineSriovNetworkWithStaticIPAM(name string, resourceName string) *sriovv1.SriovNetwork {
	sriovNetwork := DefineSriovNetwork(name, resourceName)
	sriovNetwork.Spec.IPAM = `{ "type": "static" }`
	sriovNetwork.Spec.Capabilities = `{ "mac": true, "ips": true }`

	return sriovNetwork
}

// DefineSriovNetwork builds SriovNetwork resource.
func DefineSriovNetwork(name string, resourceName string) *sriovv1.SriovNetwork {
	return &sriovv1.SriovNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: netsriovparameters.OperatorNamespace,
		},
		Spec: sriovv1.SriovNetworkSpec{
			ResourceName:     resourceName,
			NetworkNamespace: netsriovparameters.OperatorTestNamespace,
		}}
}

// definePodWithIpam sets pod network with IPAM config.
func definePodWithIpam(pod *corev1.Pod, mainNetwork string,
	slaveNetworkNames []string,
	ipAddress string,
	macAddress string,
	ipam string) *corev1.Pod {
	annotation := ""
	subnet := netparameters.IPV4Subnet

	if strings.Contains(ipAddress, ":") {
		subnet = netparameters.IPV6Subnet
	}

	if len(slaveNetworkNames) > 0 {
		for _, slave := range slaveNetworkNames {
			annotation += fmt.Sprintf(`{"name": "%s"},`, slave)
		}
	}

	switch ipam {
	case netsriovparameters.IpamStatic:
		annotation += fmt.Sprintf(`{"name": "%s","ips": ["%s/%s"],"mac": "%s" }`,
			mainNetwork, ipAddress, subnet, macAddress)
	case netsriovparameters.IpamWhereabouts:
		annotation += fmt.Sprintf(`{"name": "%s"}`, mainNetwork)
	}

	pod.Annotations = map[string]string{"k8s.v1.cni.cncf.io/networks": fmt.Sprintf(`[%s]`, annotation)}

	return pod
}

// definePodCommand sets pod command.
func definePodCommand(pod *corev1.Pod, command []string) *corev1.Pod {
	pod.Spec.Containers[0].Command = command

	return pod
}

// definePodCommandWithIpamAndMac checks this  static Mac or dynamic Mac.
func definePodCommandWithIpamAndMac(
	podDefinition *corev1.Pod,
	mainNetwork string,
	slaveNetworkNames []string,
	ipaddress string,
	macAddress string,
	podCommand []string,
	ipam string) *corev1.Pod {
	podDefinition = definePodWithIpam(podDefinition, mainNetwork, slaveNetworkNames, ipaddress, macAddress, ipam)

	return definePodCommand(podDefinition, podCommand)
}

// DescribeSRIOVParameters validates given parameters and returns json formatted string.
func DescribeSRIOVParameters(mtu int, protocol string, connectivity string, bond bool) string {
	connectivityParameters, err := netsriovparameters.NewConnectivityTestParameters(mtu, connectivity, protocol, bond)
	if err != nil {
		log.Print(err)

		return fmt.Sprintf("error in parameters: MTU=%d, Connectivity=%s, Protocol=%s", mtu, connectivity, protocol)
	}

	myPrams, err := json.Marshal(connectivityParameters)

	if err != nil {
		log.Print(err)

		return fmt.Sprintf("error in parameters: MTU=%d, Connectivity=%s, Protocol=%s", mtu, connectivity, protocol)
	}

	return string(myPrams)
}

func BuildTableEntries(
	sriovSmokeTestMode bool,
	describe interface{},
	bond bool,
	mtuParameters []int,
	connectivityParameters []string,
	protocolParameters []string) []TableEntry {
	var tableEntries []TableEntry

	if sriovSmokeTestMode {
		var (
			protocolIndex            int
			mtuIndex                 int
			connectivityIndex        int
			lenghtOfParametersArrays = []int{
				len(mtuParameters),
				len(connectivityParameters),
				len(protocolParameters),
			}
			max = lenghtOfParametersArrays[0]
		)

		for _, listLeght := range lenghtOfParametersArrays {
			if listLeght > max {
				max = listLeght
			}
		}

		for i := 0; i < max; i++ {
			if protocolIndex >= len(protocolParameters) {
				protocolIndex = 0
			}

			if mtuIndex >= len(mtuParameters) {
				mtuIndex = 0
			}

			if connectivityIndex >= len(connectivityParameters) {
				connectivityIndex = 0
			}
			tableEntries = append(
				tableEntries,
				Entry(
					describe,
					mtuParameters[mtuIndex],
					protocolParameters[protocolIndex],
					connectivityParameters[connectivityIndex],
					bond,
				),
			)
			mtuIndex++
			connectivityIndex++
			protocolIndex++
		}
	} else {
		for _, protocol := range protocolParameters {
			for _, mtu := range mtuParameters {
				for _, connectivity := range connectivityParameters {
					tableEntries = append(
						tableEntries,
						Entry(describe, mtu, protocol, connectivity, bond))
				}
			}
		}
	}

	return tableEntries
}

func redefinePodWithInitDebugCommands(podObject *corev1.Pod, initImage string) *corev1.Pod {
	podObject.Spec.InitContainers = append(podObject.Spec.InitContainers, corev1.Container{Name: "initlogs",
		Image: initImage,
		Command: []string{
			"/bin/bash", "-c", "echo $(date) DEBUG && hostname && ip addr show && ip route && ip -6 route"}})

	return podObject
}

func serverNeedsPrivilege(protocol string) bool {
	return protocol == netsriovparameters.CommunicationProtocolUnicastSCTP ||
		protocol == netsriovparameters.CommunicationProtocolUnicastTCP
}

func clientNeedsPrivilege(protocol string) bool {
	return protocol == netsriovparameters.CommunicationProtocolUnicastTCP ||
		protocol == netsriovparameters.CommunicationProtocolUnicastSCTP
}

func serverCommandFor(
	testProtocol string,
	mtu int,
	serverIP string,
	negative bool,
	testPort int,
	testInterface string) ([]string, error) {
	var testCommand []string

	switch testProtocol {
	case netsriovparameters.CommunicationProtocolUnicastICMP:
		testCommand = []string{"sleep", "INF"}

	case netsriovparameters.CommunicationProtocolUnicastTCP:
		testCommand = []string{
			"testcmd",
			"--listen",
			fmt.Sprintf("-interface=%s", testInterface),
			"--protocol=tcp",
			fmt.Sprintf("--mtu=%d", mtu),
			fmt.Sprintf("-port=%d", testPort),
		}

	case netsriovparameters.CommunicationProtocolUnicastSCTP:
		testCommand = []string{
			"testcmd",
			"-protocol=sctp",
			"-listen",
			fmt.Sprintf("-port=%d", testPort),
			fmt.Sprintf("-interface=%s", testInterface),
			fmt.Sprintf("-server=%s", serverIP)}

	case netsriovparameters.CommunicationProtocolUnicastUDP:
		testCommand = []string{
			"testcmd",
			"-listen",
			"-protocol=udp",
			fmt.Sprintf("-port=%d", testPort),
			fmt.Sprintf("-mtu=%d", mtu)}

	case netsriovparameters.CommunicationProtocolMulticastUDP:
		multicastAddress := netsriovparameters.MulticastIPAddress
		if strings.Contains(serverIP, ":") {
			multicastAddress = netsriovparameters.MulticastIPv6Address
		}

		testCommand = []string{
			"testcmd",
			"-listen",
			"-protocol=udp",
			fmt.Sprintf("-port=%d", testPort),
			"-multicast",
			fmt.Sprintf("-interface=%s", testInterface),
			fmt.Sprintf("-server=%s", multicastAddress)}

		if negative {
			if mtu < netsriovparameters.MTUJumbo {
				testCommand = append(testCommand, fmt.Sprintf("-mtu=%d", mtu+50))
			} else {
				testCommand = append(testCommand, fmt.Sprintf("-mtu=%d", mtu))
			}
		} else {
			testCommand = append(testCommand, fmt.Sprintf("-mtu=%d", mtu-100))
		}
	case netsriovparameters.CommunicationProtocolBroadcastUDP:
		if negative {
			if mtu < netsriovparameters.MTUJumbo {
				testCommand = []string{
					"testcmd",
					"-listen",
					"-protocol=udp",
					"-port=50000",
					"-broadcast",
					fmt.Sprintf("-interface=%s", testInterface),
					fmt.Sprintf("-mtu=%d", mtu+100)}
			} else {
				testCommand = []string{
					"testcmd",
					"-listen",
					"-protocol=udp",
					"-port=50000",
					"-broadcast",
					fmt.Sprintf("-interface=%s", testInterface),
					fmt.Sprintf("-mtu=%d", mtu)}
			}
		} else {
			testCommand = []string{
				"testcmd",
				"-listen",
				"-protocol=udp",
				"-port=50000",
				"-broadcast",
				fmt.Sprintf("-interface=%s", testInterface),
				fmt.Sprintf("-mtu=%d", mtu-40)}
		}
	default:
		return nil, fmt.Errorf("%s %s", netsriovparameters.SriovErrorProtocolMessage, testProtocol)
	}

	return testCommand, nil
}

func RunServerPod(
	protocol string,
	mtu int,
	connectivity string,
	sriovInfos *cluster.EnabledNodes,
	config *config.Config,
	mainNetwork string,
	slaveNetworkNames []string,
	negative bool,
	serverMacAddress string,
	serverIP string,
	interfaceName string,
	ipam string) {
	nodeSelector := defineNodeSelector(connectivity, sriovInfos)

	if (protocol == netsriovparameters.CommunicationProtocolMulticastUDP ||
		protocol == netsriovparameters.CommunicationProtocolBroadcastUDP ||
		protocol == netsriovparameters.CommunicationProtocolUnicastSCTP) && negative {
		err := namespaces.CleanPods(netsriovparameters.OperatorTestNamespace, Apiclient)
		Expect(err).ToNot(HaveOccurred())
	}

	serverCommand, err := serverCommandFor(protocol, mtu, serverIP, negative, netsriovparameters.TestPort,
		interfaceName)
	Expect(err).ToNot(HaveOccurred())

	serverPodDefinition := defineServerPod(
		protocol,
		nodeSelector,
		mainNetwork,
		slaveNetworkNames,
		serverIP,
		serverMacAddress,
		config.Network.TestContainerImage,
		serverCommand,
		ipam)

	serverPod, err := Apiclient.Pods(
		netsriovparameters.OperatorTestNamespace).Create(
		context.Background(),
		serverPodDefinition,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	WaitUntilPodInStatus(
		serverPod,
		"Server",
		serverCommand,
		corev1.PodRunning, netsriovparameters.PodWaitingTime)
}

func DefineTestCommandParameters(
	negative bool,
	protocol string,
	mtu int,
	serverIP string,
	testPort int,
	testInterface string) ([]string, error) {
	var (
		testCommand     = []string{"testcmd"}
		protocolOption  string
		protocolVersion = 4
	)

	if strings.Contains(serverIP, ":") {
		protocolVersion = 6
	}

	switch protocol {
	case netsriovparameters.CommunicationProtocolUnicastICMP:
		protocolOption = "icmp"
	case netsriovparameters.CommunicationProtocolUnicastTCP:
		protocolOption = "tcp"
		testCommand = append(testCommand,
			fmt.Sprintf("-port=%d", testPort),
			fmt.Sprintf("-interface=%s", testInterface))
	case netsriovparameters.CommunicationProtocolUnicastSCTP:
		protocolOption = "sctp"
		testCommand = append(testCommand, fmt.Sprintf("-server=%s", serverIP),
			fmt.Sprintf("-port=%d", testPort), fmt.Sprintf("-interface=%s", testInterface))
	case netsriovparameters.CommunicationProtocolUnicastUDP:
		protocolOption = protocolUPD
		testCommand = append(testCommand, fmt.Sprintf("-port=%d", testPort))
	case netsriovparameters.CommunicationProtocolMulticastUDP:
		protocolOption = protocolUPD
		serverIP = netsriovparameters.MulticastIPAddress

		if protocolVersion == 6 {
			serverIP = netsriovparameters.MulticastIPv6Address
		}
		testCommand = append(
			testCommand,
			fmt.Sprintf("-port=%d", testPort),
			"-multicast",
			fmt.Sprintf("-interface=%s", testInterface))
	case netsriovparameters.CommunicationProtocolBroadcastUDP:
		protocolOption = protocolUPD
		serverIP = "255.255.255.255"
		testCommand = append(
			testCommand,
			fmt.Sprintf("-port=%d", testPort),
			"-broadcast",
			fmt.Sprintf("-interface=%s", testInterface))
	default:
		return nil, fmt.Errorf("%s %s", netsriovparameters.SriovErrorProtocolMessage, protocol)
	}

	switch negative {
	case true:
		var parameterMtu string
		if mtu < netsriovparameters.MTUJumbo {
			parameterMtu = fmt.Sprintf("-mtu=%d", mtu+50)
		} else {
			parameterMtu = fmt.Sprintf("-mtu=%d", mtu)
		}
		testCommand = append(testCommand, "-negative", parameterMtu)
	default:
		testCommand = append(testCommand, fmt.Sprintf("-mtu=%d", mtu-100))
	}
	testCommand = append(testCommand, fmt.Sprintf("-server=%s", serverIP), fmt.Sprintf("-protocol=%s", protocolOption))

	return testCommand, nil
}

func defineServerNetworkName(mtu int) string {
	return defineNetworkName(mtu)
}

func defineClientNetworkName(mtu int, connectivity string) string {
	var networkName string

	if connectivity == netsriovparameters.ConnectivitySameNodeDiffPF ||
		connectivity == netsriovparameters.ConnectivityDiffNodeDiffPF {
		switch mtu {
		case netsriovparameters.MTUJumbo:
			networkName = netsriovparameters.SriovNetworkJumboFrameNameDiff
		case netsriovparameters.MTUStandart:
			networkName = netsriovparameters.SriovNetworkUsualMTUNameDiff
		case netsriovparameters.MTUCustom:
			networkName = netsriovparameters.SriovNetworkCustomMTUNameDiff
		default:
			Skip(fmt.Sprintf("Unsupported test parameter mtu: %d", mtu))
		}
	} else {
		networkName = defineNetworkName(mtu)
	}

	return networkName
}

func defineNetworkName(mtu int) string {
	var networkName string

	switch mtu {
	case netsriovparameters.MTUJumbo:
		networkName = netsriovparameters.SriovNetworkJumboFrameName
	case netsriovparameters.MTUStandart:
		networkName = netsriovparameters.SriovNetworkUsualMTUName
	case netsriovparameters.MTUCustom:
		networkName = netsriovparameters.SriovNetworkCustomMTUName
	default:
		Skip(fmt.Sprintf("Unsupported test parameter mtu: %d", mtu))
	}

	return networkName
}

func defineNodeSelector(connectivity string, sriovInfos *cluster.EnabledNodes) []string {
	var nodeSelector []string

	switch connectivity {
	case netsriovparameters.ConnectivityDiffNode,
		netsriovparameters.ConnectivityDiffNodeDiffPF,
		netsriovparameters.ConnectivityDiffNodeSamePF:
		if len(sriovInfos.Nodes) < 2 {
			Skip("Nodes number less that 2")
		}

		nodeSelector = sriovInfos.Nodes
	case netsriovparameters.ConnectivitySameNodeSamePF:
		nodeSelector = append(nodeSelector, sriovInfos.Nodes[0])
	case netsriovparameters.ConnectivitySameNodeDiffPF:
		nodeSelector = append(nodeSelector, sriovInfos.Nodes[0])
	default:
		Skip(fmt.Sprintf("Unsupported test parameter Connectivity: %s", connectivity))
	}

	return nodeSelector
}

func defineServerPod(
	protocol string,
	nodeSelector []string,
	mainNetwork string,
	slaveNetworkNames []string,
	ipaddress string,
	macAddress string,
	podImage string,
	podCommand []string,
	ipam string) *corev1.Pod {
	podDefinition := pod.RedefineWithRestartPolicy(
		pod.DefineWithNodeNetworks(
			nodeSelector[0],
			[]string{mainNetwork},
			netsriovparameters.OperatorTestNamespace, podImage),
		corev1.RestartPolicyNever)

	if serverNeedsPrivilege(protocol) {
		podDefinition = pod.RedefineAsPrivileged(podDefinition)
	}

	podDefinition = definePodCommandWithIpamAndMac(
		podDefinition,
		mainNetwork,
		slaveNetworkNames,
		ipaddress,
		macAddress,
		podCommand, ipam)

	if strings.Contains(ipaddress, ":") {
		podDefinition = redefinePodWithInitCommandPolicy(podDefinition, podImage,
			fmt.Sprintf(
				"ping6 -c 3 -w 30 %s && ip -6 route add %s/128 dev net1 table local",
				netsriovparameters.ServerPodIpv6, netsriovparameters.MulticastIPv6Address))
	}

	return redefinePodWithInitDebugCommands(podDefinition, podImage)
}

func DefineClientPod(
	protocol string,
	nodeSelector []string,
	mainNetwork string,
	slaveNetworkNames []string,
	ipaddress string,
	macAddress string,
	podImage string,
	podCommand []string,
	ipam string) *corev1.Pod {
	var podDefinition *corev1.Pod

	if len(nodeSelector) > 1 {
		podDefinition = pod.RedefineWithRestartPolicy(
			pod.DefineWithNodeNetworks(
				nodeSelector[1],
				[]string{mainNetwork},
				netsriovparameters.OperatorTestNamespace, podImage),
			corev1.RestartPolicyNever)
	} else {
		podDefinition = pod.RedefineWithRestartPolicy(
			pod.DefineWithNodeNetworks(
				nodeSelector[0],
				[]string{mainNetwork},
				netsriovparameters.OperatorTestNamespace, podImage),
			corev1.RestartPolicyNever)
	}

	if clientNeedsPrivilege(protocol) {
		podDefinition = pod.RedefineAsPrivileged(podDefinition)
	}

	if len(slaveNetworkNames) > 0 {
		podDefinition = pod.RedefineAsPrivileged(podDefinition)
	}

	podDefinition = definePodCommandWithIpamAndMac(
		podDefinition,
		mainNetwork,
		slaveNetworkNames,
		ipaddress,
		macAddress,
		podCommand, ipam)
	serverIP := netsriovparameters.ServerPodIP

	if strings.Contains(ipaddress, ":") {
		serverIP = netsriovparameters.ServerPodIpv6
	}

	return redefinePodWithInitCommandPolicy(
		podDefinition,
		podImage,
		fmt.Sprintf("ping %s -c 3 -w 90", serverIP))
}

func redefinePodWithInitCommandPolicy(podObject *corev1.Pod, initImage string, command string) *corev1.Pod {
	b := true
	podObject.Spec.InitContainers = []corev1.Container{{Name: "inittest",
		Image: initImage,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &b,
		},
		Command: []string{"/bin/bash", "-c", command}}}

	return podObject
}

func WaitUntilPodInStatus(
	createdPod *corev1.Pod,
	podRole string,
	execCommand []string,
	podStatus corev1.PodPhase,
	waitingTime time.Duration) {
	Eventually(func() corev1.PodPhase {
		createdPod, _ = Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Get(
			context.Background(),
			createdPod.Name,
			metav1.GetOptions{})
		if createdPod.Status.Phase == corev1.PodFailed {
			Fail(fmt.Sprintf("Pod role %s.Invalid return code. Command: %s", podRole, execCommand))
		}

		return createdPod.Status.Phase
	}, waitingTime, time.Second).Should(Equal(podStatus),
		fmt.Sprintf("Pod role %s. Invalid return code. Command: %s", podRole, execCommand))
}

func doesSriovInerfaceSupportVFsNumber(totalVFs int, sriovInterface *sriovv1.InterfaceExt) bool {
	var isSriovNodesSupportVFsNumber bool

	nodeStates, err := Apiclient.SriovNetworkNodeStates(netsriovparameters.OperatorNamespace).
		List(context.Background(), metav1.ListOptions{})
	Expect(err).ToNot(HaveOccurred())
	Expect(len(nodeStates.Items)).ToNot(BeNumerically("==", 0))

	for _, nodeState := range nodeStates.Items {
		for _, nodeInterface := range nodeState.Status.Interfaces {
			if nodeInterface.Name == sriovInterface.Name {
				// Mellanox has known behavior that totalVfs == numVFs BZ 1855139
				if strings.EqualFold(sriovInterface.Driver, "mlx5_core") {
					// VF maximum number is 127 according to the Mellanox docs
					// https://docs.nvidia.com/networking/pages/viewpage.action?pageId=39264752
					if totalVFs > 127 {
						return false
					}
				} else {
					if nodeInterface.TotalVfs < totalVFs {
						return false
					}
				}

				isSriovNodesSupportVFsNumber = true
			}
		}
	}

	return isSriovNodesSupportVFsNumber
}

func IsScaleSupported(totalVFs int, sriovInterfaces []*sriovv1.InterfaceExt) bool {
	Expect(len(sriovInterfaces)).ToNot(BeNumerically("==", 0))

	for _, sriovInterface := range sriovInterfaces {
		if !doesSriovInerfaceSupportVFsNumber(totalVFs, sriovInterface) {
			return false
		}
	}

	return true
}

func SetupSriovConfig(sriovInfos *cluster.EnabledNodes, snoTimeoutMultiplier time.Duration) {
	var (
		isScaleSupported    bool
		vfsNumberDiffConfig = 5
		vfsNumberSameConfig = 6
	)

	if snoTimeoutMultiplier == 2 {
		disableDrainState()
	}

	validSriovInterfaces := validateSriovInterfaces(sriovInfos)
	isScaleSupported = IsScaleSupported(netsriovparameters.ScaleVFsNumber, validSriovInterfaces)

	if isScaleSupported {
		vfsNumberDiffConfig = netsriovparameters.ScaleVFsNumber
		vfsNumberSameConfig = netsriovparameters.ScaleVFsNumber
	}

	By(fmt.Sprintf("Clean test namespace %s", netsriovparameters.OperatorTestNamespace))
	err := namespaces.Clean(generalParameters.SriovOperatorNamespace, netsriovparameters.OperatorTestNamespace,
		Apiclient, false)
	Expect(err).ToNot(HaveOccurred())

	By("Waiting until SRIOV become stable")
	WaitForSRIOVStable(generalParameters.SriovOperatorNamespace, netsriovparameters.WaitingTime, snoTimeoutMultiplier)

	By("Configuring SriovPolicy resources")

	err = namespaces.Create(netsriovparameters.OperatorTestNamespace, Apiclient)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Error to create namespace %s: %s", netsriovparameters.OperatorNamespace, err))

	usualSriovPolicyConfig := DefineSriovPolicy(
		"test-policy-usual", generalParameters.SriovOperatorNamespace, validSriovInterfaces[0],
		vfsNumberSameConfig, "#0-1", 1500, "testresourceusual", "netdevice")
	customSriovPolicyConfig := DefineSriovPolicy(
		"test-policy-custom", generalParameters.SriovOperatorNamespace, validSriovInterfaces[0],
		vfsNumberSameConfig, "#2-3", 1450, "testresourcecustom", "netdevice")
	jumboSriovPolicyConfig := DefineSriovPolicy(
		"test-policy-jumbo", generalParameters.SriovOperatorNamespace, validSriovInterfaces[0],
		vfsNumberSameConfig, "#4-5", 9000, "testresourcejumbo", "netdevice")
	usualSriovPolicyConfigDiffPF := DefineSriovPolicy(
		"test-policy-usual-diff", generalParameters.SriovOperatorNamespace, validSriovInterfaces[1],
		vfsNumberDiffConfig, "#0-0", 1500, "testresourceusualdiff", "netdevice")
	customSriovPolicyConfigDiffPF := DefineSriovPolicy(
		"test-policy-custom-diff", generalParameters.SriovOperatorNamespace, validSriovInterfaces[1],
		vfsNumberDiffConfig, "#1-1", 1450, "testresourcecustomdiff", "netdevice")
	jumboSriovPolicyConfigDiffPF := DefineSriovPolicy(
		"test-policy-jumbo-diff", generalParameters.SriovOperatorNamespace, validSriovInterfaces[1],
		vfsNumberDiffConfig, "#2-2", 9000, "testresourcejumbodiff", "netdevice")
	scaleSriovPolicyConfigDiffPF := DefineSriovPolicy(
		"test-policy-scale-diff", generalParameters.SriovOperatorNamespace, validSriovInterfaces[1],
		netsriovparameters.ScaleVFsNumber, "#3-34", 1500, "testresourcescalediff", "netdevice")
	scaleSriovPolicyConfig := DefineSriovPolicy(
		"test-policy-scale", generalParameters.SriovOperatorNamespace, validSriovInterfaces[0],
		netsriovparameters.ScaleVFsNumber, "#6-37", 1500, "testresourcescaled", "netdevice")

	networkPolicies := []*sriovv1.SriovNetworkNodePolicy{usualSriovPolicyConfig, customSriovPolicyConfig,
		jumboSriovPolicyConfig, customSriovPolicyConfigDiffPF, jumboSriovPolicyConfigDiffPF,
		usualSriovPolicyConfigDiffPF}
	if isScaleSupported {
		networkPolicies = append(networkPolicies, scaleSriovPolicyConfigDiffPF, scaleSriovPolicyConfig)
	}

	for _, networkPolicy := range networkPolicies {
		err = Apiclient.Create(context.Background(), networkPolicy)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to create SR-IOV networkPolicy %v: %s", networkPolicy, err))
	}

	By("Configuring SriovNetwork resources")

	usualSriovNetworkConfig := defineSriovNetworkWithStaticIPAM(
		netsriovparameters.SriovNetworkUsualMTUName,
		usualSriovPolicyConfig.Spec.ResourceName)
	customSriovNetworkConfig := defineSriovNetworkWithStaticIPAM(
		netsriovparameters.SriovNetworkCustomMTUName,
		customSriovPolicyConfig.Spec.ResourceName)
	jumboSriovNetworkConfig := defineSriovNetworkWithStaticIPAM(
		netsriovparameters.SriovNetworkJumboFrameName,
		jumboSriovPolicyConfig.Spec.ResourceName)
	usualSriovNetworkConfigDiff := defineSriovNetworkWithStaticIPAM(
		netsriovparameters.SriovNetworkUsualMTUNameDiff,
		usualSriovPolicyConfigDiffPF.Spec.ResourceName)
	customSriovNetworkConfigDiff := defineSriovNetworkWithStaticIPAM(
		netsriovparameters.SriovNetworkCustomMTUNameDiff,
		customSriovPolicyConfigDiffPF.Spec.ResourceName)
	jumboSriovNetworkConfigDiff := defineSriovNetworkWithStaticIPAM(
		netsriovparameters.SriovNetworkJumboFrameNameDiff,
		jumboSriovPolicyConfigDiffPF.Spec.ResourceName)
	sriovBondNetworkConfig := defineSriovBondNetwork(
		netsriovparameters.SriovNetworkBondName,
		jumboSriovPolicyConfig.Spec.ResourceName)
	sriovBondConfigDiff := defineSriovBondNetwork(
		netsriovparameters.SriovNetworkBondNameDiff,
		jumboSriovPolicyConfigDiffPF.Spec.ResourceName)
	sriovScaleBondConfig := defineSriovBondNetwork(
		netsriovparameters.SriovScaleBondName,
		scaleSriovPolicyConfig.Spec.ResourceName)
	sriovScaleBondConfigDiff := defineSriovBondNetwork(
		netsriovparameters.SriovScaleBondNameDiff,
		scaleSriovPolicyConfigDiffPF.Spec.ResourceName)

	sriovNetworks := []*sriovv1.SriovNetwork{usualSriovNetworkConfig, customSriovNetworkConfig,
		jumboSriovNetworkConfig, sriovBondNetworkConfig, customSriovNetworkConfigDiff, jumboSriovNetworkConfigDiff,
		usualSriovNetworkConfigDiff, sriovBondConfigDiff}
	if isScaleSupported {
		sriovNetworks = append(sriovNetworks, sriovScaleBondConfig, sriovScaleBondConfigDiff)
	}

	for _, network := range sriovNetworks {
		err = Apiclient.Create(context.Background(), network)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to create SR-IOV network %v: %s", network, err))
	}

	By("Waiting until SRIOV become stable")
	WaitForSRIOVStable(generalParameters.SriovOperatorNamespace, netsriovparameters.WaitingTime, snoTimeoutMultiplier)

	By("Waiting until SRIOV resources become available")
	ValidateSriovVFsAvailableOnNodes(
		sriovInfos.Nodes, []*sriovv1.SriovNetworkNodePolicy{usualSriovPolicyConfig, customSriovPolicyConfig,
			jumboSriovPolicyConfig},
		2)
	ValidateSriovVFsAvailableOnNodes(
		sriovInfos.Nodes,
		[]*sriovv1.SriovNetworkNodePolicy{usualSriovPolicyConfigDiffPF, customSriovPolicyConfigDiffPF,
			jumboSriovPolicyConfigDiffPF},
		1)

	if isScaleSupported {
		ValidateSriovVFsAvailableOnNodes(
			sriovInfos.Nodes,
			[]*sriovv1.SriovNetworkNodePolicy{
				scaleSriovPolicyConfigDiffPF,
				scaleSriovPolicyConfig},
			32)
	}
}

func validateSriovInterfaces(sriovInfos *cluster.EnabledNodes) []*sriovv1.InterfaceExt {
	sriovInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
	Expect(err).ToNot(HaveOccurred())

	validSriovInterfaces, err := Config.GetSriovInterfaces(sriovInterfaces, 2)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error determine SRIOV interfaces: %s", err))

	return validSriovInterfaces
}

func disableDrainState() {
	disableDrainState := GetNodeDrainState(netsriovparameters.OperatorNamespace)
	if !disableDrainState {
		SetDisableNodeDrainState(true, netsriovparameters.OperatorNamespace)
		ChangedNodeDrainState = true
	}
}

func IsSriovPreConfigured() bool {
	sriovPolicies, err := Apiclient.SriovNetworkNodePolicies(generalParameters.SriovOperatorNamespace).List(
		context.TODO(), metav1.ListOptions{})
	Expect(err).ToNot(HaveOccurred())

	if len(sriovPolicies.Items) < 2 {
		return false
	}

	sriovNetworks, err := Apiclient.SriovNetworks(
		generalParameters.SriovOperatorNamespace).List(context.TODO(), metav1.ListOptions{})
	Expect(err).ToNot(HaveOccurred())

	return len(sriovNetworks.Items) >= 2
}

func SriovPreConfiguration() {
	var snoTimeoutMultiplier time.Duration = 1

	isSingleNode, err := nodes.IsSingleNodeCluster(Apiclient)
	Expect(err).ToNot(HaveOccurred())

	if isSingleNode {
		snoTimeoutMultiplier = 2
	}

	sriovInfos, err := cluster.DiscoverSriov(Apiclient, netsriovparameters.OperatorNamespace)
	Expect(err).ToNot(HaveOccurred())
	SetupSriovConfig(sriovInfos, snoTimeoutMultiplier)
}

func VerifySriovOperatorInstalledAndPreconfigured(operatorGroup v1.OperatorGroup,
	sriovSubscription *v1alpha1.Subscription) {
	By("Checking if SR-IOV operator installed")

	if IsSriovOperatorInstalled() != nil {
		By("SR-IOV Operator is not installed, start deployment")

		err := DeploySriovOperator(&operatorGroup, sriovSubscription)
		Expect(err).ToNot(HaveOccurred())

		Eventually(
			IsSriovOperatorInstalled,
			netsriovparameters.SriovOperatorDeploymentTime,
			netsriovparameters.SriovOperatorDeploymentRetry).ShouldNot(HaveOccurred())

		Eventually(func() error {
			_, err = cluster.DiscoverSriov(Apiclient, netsriovparameters.OperatorNamespace)

			return err
		}, netsriovparameters.SriovOperatorDeploymentTime,
			netsriovparameters.SriovOperatorDeploymentRetry).ShouldNot(HaveOccurred())
	}

	By("Check if SR-IOV operator preconfigured")

	if !IsSriovPreConfigured() {
		By("Configure SR-IOV operator")
		SriovPreConfiguration()
	}
}
