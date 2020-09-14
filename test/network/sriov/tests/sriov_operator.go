package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	sriovv1 "github.com/openshift/sriov-network-operator/pkg/apis/sriovnetwork/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
)

const (
	sriovNetworkUsualMTUName       = "test-sriov-static-usual"
	sriovNetworkCustomMTUName      = "test-sriov-static-custom"
	sriovNetworkJumboFrameName     = "test-sriov-static-jumbo"
	sriovNetworkUsualMTUNameDiff   = "test-sriov-static-usual-diff"
	sriovNetworkCustomMTUNameDiff  = "test-sriov-static-custom-diff"
	sriovNetworkJumboFrameNameDiff = "test-sriov-static-jumbo-diff"
	clientPodIP                    = "192.168.100.1"
	clientMacAddress               = "20:04:0f:f1:88:01"
	serverPodIP                    = "192.168.100.2"
	serverMacAddress               = "20:04:0f:f1:88:03"
	testPort                       = 50000
	testInterfaceName              = "net1"
)

var (
	waitingTime    time.Duration = 20 * time.Minute
	podWaitingTime time.Duration = 2 * time.Minute
)

var _ = Describe("CNF SRIOV", func() {
	describe := func(desc string) func(mtu int, protocol string, connectivity string) string {

		return func(mtu int, protocol string, connectivity string) string {
			connectivityParameters, err := parameters.NewConnectivityTestParameters(mtu, connectivity, protocol)
			if err != nil {
				log.Print(err)
				return fmt.Sprintf("error in parameters: %s MTU=%d, Connectivity=%s, Protocol=%s", desc, mtu, connectivity, protocol)
			}
			myPrams, err := json.Marshal(connectivityParameters)
			if err != nil {
				log.Print(err)
				return fmt.Sprintf("error in parameters: %s MTU=%d, Connectivity=%s, Protocol=%s", desc, mtu, connectivity, protocol)
			}

			return fmt.Sprintf("%s %s", desc, string(myPrams))
		}
	}
	var sriovInfos *cluster.EnabledNodes
	config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())

	execute.BeforeAll(func() {
		By("Discover SRIOV interfaces")
		sriovInfos, err = cluster.DiscoverSriov(clients, operatorNamespace)
		Expect(err).ToNot(HaveOccurred())
		sriovInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
		Expect(err).ToNot(HaveOccurred())

		By(fmt.Sprintf("Clean test namespace %s", parameters.OperatorTestNamespace))
		namespaces.Clean(operatorNamespace, parameters.OperatorTestNamespace, clients, false)

		By("Waiting until SRIOV become stable")
		WaitForSRIOVStable(clients, operatorNamespace, waitingTime)
		Expect(err).ToNot(HaveOccurred())

		By("Configuring SriovPolicy resources")
		err = namespaces.Create(parameters.OperatorTestNamespace, clients)
		Expect(err).ToNot(HaveOccurred())
		usualSriovPolicyConfig := DefineSriovPolicy("test-policy-usual", sriovInterfaces[0], "#0-1", 1500, "testresourceusual", "netdevice")
		customSriovPolicyConfig := DefineSriovPolicy("test-policy-custom", sriovInterfaces[0], "#2-3", 1450, "testresourcecustom", "netdevice")
		jumboSriovPolicyConfig := DefineSriovPolicy("test-policy-jumbo", sriovInterfaces[1], "#0-1", 9000, "testresourcejumbo", "netdevice")
		usualSriovPolicyConfigDiffPF := DefineSriovPolicy("test-policy-usual-diff", sriovInterfaces[1], "#2-2", 1500, "testresourceusualdiff", "netdevice")
		customSriovPolicyConfigDiffPF := DefineSriovPolicy("test-policy-custom-diff", sriovInterfaces[1], "#3-3", 1450, "testresourcecustomdiff", "netdevice")
		jumboSriovPolicyConfigDiffPF := DefineSriovPolicy("test-policy-jumbo-diff", sriovInterfaces[1], "#4-4", 9000, "testresourcejumbodiff", "netdevice")
		for _, networkPolicy := range []*sriovv1.SriovNetworkNodePolicy{usualSriovPolicyConfig, customSriovPolicyConfig,
			jumboSriovPolicyConfig, customSriovPolicyConfigDiffPF, jumboSriovPolicyConfigDiffPF, usualSriovPolicyConfigDiffPF} {
			err = clients.Create(context.Background(), networkPolicy)
			Expect(err).ToNot(HaveOccurred())
		}

		By("Configuring SriovNetwork resources")
		usualSriovNetworkConfig := DefineSriovNetwork(sriovNetworkUsualMTUName, usualSriovPolicyConfig.Spec.ResourceName, true)
		customSriovNetworkConfig := DefineSriovNetwork(sriovNetworkCustomMTUName, customSriovPolicyConfig.Spec.ResourceName, true)
		jumboSriovNetworkConfig := DefineSriovNetwork(sriovNetworkJumboFrameName, jumboSriovPolicyConfig.Spec.ResourceName, true)
		usualSriovNetworkConfigDiff := DefineSriovNetwork(sriovNetworkUsualMTUNameDiff, usualSriovPolicyConfigDiffPF.Spec.ResourceName, true)
		customSriovNetworkConfigDiff := DefineSriovNetwork(sriovNetworkCustomMTUNameDiff, customSriovPolicyConfigDiffPF.Spec.ResourceName, true)
		jumboSriovNetworkConfigDiff := DefineSriovNetwork(sriovNetworkJumboFrameNameDiff, jumboSriovPolicyConfigDiffPF.Spec.ResourceName, true)
		for _, network := range []*sriovv1.SriovNetwork{usualSriovNetworkConfig, customSriovNetworkConfig, jumboSriovNetworkConfig,
			customSriovNetworkConfigDiff, jumboSriovNetworkConfigDiff, usualSriovNetworkConfigDiff} {
			err = clients.Create(context.Background(), network)
			Expect(err).ToNot(HaveOccurred())
		}

		By("Waiting until SRIOV become stable")
		WaitForSRIOVStable(clients, operatorNamespace, waitingTime)

		By("Waiting until SRIOV resources become avaliable")
		ValidateSriovVFsAvailableOnNodes(clients, sriovInfos.Nodes, []*sriovv1.SriovNetworkNodePolicy{usualSriovPolicyConfig, customSriovPolicyConfig,
			jumboSriovPolicyConfig}, 2)
		ValidateSriovVFsAvailableOnNodes(clients, sriovInfos.Nodes, []*sriovv1.SriovNetworkNodePolicy{usualSriovPolicyConfigDiffPF,
			customSriovPolicyConfigDiffPF, jumboSriovPolicyConfigDiffPF}, 1)
	})

	AfterEach(func() {
		By("Cleaning up resources after test")
		err = namespaces.CleanPods(parameters.OperatorTestNamespace, clients)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() bool {
			podsList, err := clients.Pods(parameters.OperatorTestNamespace).List(context.Background(), metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred())
			if len(podsList.Items) > 0 {
				return false
			}
			return true

		}, 3*time.Minute, 10*time.Second).Should(BeTrue())
	})

	DescribeTable("IP Static, Ip Stack: ipv4, Mac address: MAC static", func(mtu int, protocol string, connectivity string) {
		By("Validating test paremetes")
		connectivityParameters, err := parameters.NewConnectivityTestParameters(mtu, connectivity, protocol)
		Expect(err).ToNot(HaveOccurred())

		By("Defining test resources")
		nodeSelector := defineNodeSelector(connectivity, sriovInfos)
		serverNetworkName := defineServerNetworkName(mtu)
		clientNetworkName := defineClientNetworkName(mtu, connectivityParameters.Connectivity)
		negativeFlag := false
		clientTestCommand, err := defineTestCommandParameters(negativeFlag, connectivityParameters.Protocol, connectivityParameters.MTU,
			connectivityParameters.Connectivity, serverPodIP)
		Expect(err).ToNot(HaveOccurred())
		clientPodDefinition := defineClientPod(connectivityParameters.Protocol, nodeSelector, clientNetworkName, clientPodIP,
			clientMacAddress, config.Network.TestContainerImage, clientTestCommand)

		By("Creating Server Pod")
		runServerPod(protocol, connectivityParameters.MTU, connectivityParameters.Connectivity, sriovInfos, config, serverNetworkName,
			nodeSelector, negativeFlag)

		By("Creating Client Pod")
		clientPod, err := clients.Pods(parameters.OperatorTestNamespace).Create(context.Background(), clientPodDefinition, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() corev1.PodPhase {
			clientPod, _ = clients.Pods(parameters.OperatorTestNamespace).Get(context.Background(), clientPod.Name, metav1.GetOptions{})
			return clientPod.Status.Phase
		}, podWaitingTime, time.Second).Should(Equal(corev1.PodSucceeded), fmt.Sprintf("Invalid return code. Command: %s", fmt.Sprint(clientTestCommand)))

		By("Positive test flow - success. Running negative flow")
		negativeFlag = true
		if protocol == parameters.CommunicationProtocolMulticastUDP || protocol == parameters.CommunicationProtocolBroadcastUDP {
			runServerPod(protocol, connectivityParameters.MTU, connectivityParameters.Connectivity, sriovInfos, config, serverNetworkName, nodeSelector, negativeFlag)
		}
		clientTestCommand, err = defineTestCommandParameters(
			negativeFlag, connectivityParameters.Protocol, connectivityParameters.MTU, connectivityParameters.Connectivity, serverPodIP)
		Expect(err).ToNot(HaveOccurred())

		By("Creating Client Pod with negative flag")
		clientPodDefinitionNegative := defineClientPod(connectivityParameters.Protocol, nodeSelector, clientNetworkName,
			clientPodIP, clientMacAddress, config.Network.TestContainerImage, clientTestCommand)
		clientPodNegative, err := clients.Pods(parameters.OperatorTestNamespace).Create(context.Background(), clientPodDefinitionNegative, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() corev1.PodPhase {
			clientPodNegative, _ = clients.Pods(parameters.OperatorTestNamespace).Get(context.Background(), clientPodNegative.Name, metav1.GetOptions{})
			return clientPodNegative.Status.Phase
		}, podWaitingTime, time.Second).Should(Equal(corev1.PodSucceeded), fmt.Sprintf("Invalid return code. Command: %s", fmt.Sprint(clientTestCommand)))
	},
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolUnicastSCTP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolUnicastSCTP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolUnicastSCTP, parameters.ConnectivitySameNodeDiffPF),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolUnicastSCTP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolUnicastSCTP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolUnicastSCTP, parameters.ConnectivitySameNodeDiffPF),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolUnicastSCTP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolUnicastSCTP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolUnicastSCTP, parameters.ConnectivitySameNodeDiffPF),
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolUnicastICMP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolUnicastICMP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolUnicastICMP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolUnicastICMP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolUnicastICMP, parameters.ConnectivitySameNodeDiffPF),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolUnicastICMP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolUnicastICMP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolUnicastICMP, parameters.ConnectivitySameNodeDiffPF),
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolUnicastTCP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolUnicastTCP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolUnicastTCP, parameters.ConnectivitySameNodeDiffPF),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolUnicastTCP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolUnicastTCP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolUnicastTCP, parameters.ConnectivitySameNodeDiffPF),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolUnicastTCP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolUnicastTCP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolUnicastTCP, parameters.ConnectivitySameNodeDiffPF),
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolUnicastUDP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolUnicastUDP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolUnicastUDP, parameters.ConnectivitySameNodeDiffPF),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolUnicastUDP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolUnicastUDP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolUnicastUDP, parameters.ConnectivitySameNodeDiffPF),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolUnicastUDP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolUnicastUDP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolUnicastUDP, parameters.ConnectivitySameNodeDiffPF),
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolMulticastUDP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolMulticastUDP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolMulticastUDP, parameters.ConnectivitySameNodeDiffPF),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolMulticastUDP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolMulticastUDP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolMulticastUDP, parameters.ConnectivitySameNodeDiffPF),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolMulticastUDP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolMulticastUDP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolMulticastUDP, parameters.ConnectivitySameNodeDiffPF),
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolBroadcastUDP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolBroadcastUDP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUStandart, parameters.CommunicationProtocolBroadcastUDP, parameters.ConnectivitySameNodeDiffPF),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolBroadcastUDP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolBroadcastUDP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUCustom, parameters.CommunicationProtocolBroadcastUDP, parameters.ConnectivitySameNodeDiffPF),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolBroadcastUDP, parameters.ConnectivityDiffNode),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolBroadcastUDP, parameters.ConnectivitySameNodeSamePF),
		Entry(describe(""), parameters.MTUJumbo, parameters.CommunicationProtocolBroadcastUDP, parameters.ConnectivitySameNodeDiffPF),
	)
})

func runServerPod(protocol string, mtu int, connectivity string, sriovInfos *cluster.EnabledNodes,
	config *config.Config, networkName string, serverCommand []string, negative bool) {
	nodeSelector := defineNodeSelector(connectivity, sriovInfos)
	if (protocol == parameters.CommunicationProtocolMulticastUDP || protocol == parameters.CommunicationProtocolBroadcastUDP) && negative == true {
		namespaces.CleanPods(parameters.OperatorTestNamespace, clients)
	}
	serverCommand, err := serverCommandFor(protocol, mtu, negative)
	Expect(err).ToNot(HaveOccurred())
	serverPodDefinition := defineServerPod(protocol, nodeSelector, networkName, serverPodIP, serverMacAddress,
		config.Network.TestContainerImage, serverCommand)
	serverPod, err := clients.Pods(parameters.OperatorTestNamespace).Create(context.Background(), serverPodDefinition, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() corev1.PodPhase {
		serverPod, _ = clients.Pods(parameters.OperatorTestNamespace).Get(context.Background(), serverPod.Name, metav1.GetOptions{})
		return serverPod.Status.Phase
	}, podWaitingTime, time.Second).Should(Equal(corev1.PodRunning))
}

func serverCommandFor(testProtocol string, mtu int, negative bool) ([]string, error) {
	testCommand := []string{}
	switch testProtocol {
	case parameters.CommunicationProtocolUnicastICMP:
		testCommand = []string{"sleep", "INF"}
	case parameters.CommunicationProtocolUnicastTCP:
		testCommand = []string{"httpd", "-X"}
	case parameters.CommunicationProtocolUnicastSCTP:
		testCommand = []string{"testcmd", "-protocol=sctp", "-listen", fmt.Sprintf("-port=%d", testPort),
			fmt.Sprintf("-interface=%s", testInterfaceName), fmt.Sprintf("-server=%s", serverPodIP)}
	case parameters.CommunicationProtocolUnicastUDP:
		testCommand = []string{"testcmd", "-listen", "-protocol=udp", fmt.Sprintf("-port=%d", testPort),
			fmt.Sprintf("-mtu=%d", mtu)}
	case parameters.CommunicationProtocolMulticastUDP:
		testCommand = []string{"testcmd", "-listen", "-protocol=udp", fmt.Sprintf("-port=%d", testPort),
			"-multicast", "-server=224.255.0.10", fmt.Sprintf("-interface=%s", testInterfaceName)}
		if negative {
			if mtu < parameters.MTUJumbo {
				testCommand = append(testCommand, fmt.Sprintf("-mtu=%d", mtu+40))
			} else {
				testCommand = append(testCommand, fmt.Sprintf("-mtu=%d", mtu))
			}
		} else {
			testCommand = append(testCommand, fmt.Sprintf("-mtu=%d", mtu-50))
		}
	case parameters.CommunicationProtocolBroadcastUDP:
		if negative {
			testCommand = []string{"testcmd", "-listen", "-protocol=udp", "-port=50000", "-broadcast",
				fmt.Sprintf("-interface=%s", testInterfaceName), fmt.Sprintf("-mtu=%d", mtu+40)}
		} else {
			testCommand = []string{"testcmd", "-listen", "-protocol=udp", "-port=50000", "-broadcast",
				fmt.Sprintf("-interface=%s", testInterfaceName), fmt.Sprintf("-mtu=%d", mtu-40)}
		}
	default:
		return nil, fmt.Errorf(fmt.Sprint(parameters.SriovErrorProtocolMessage, testProtocol))
	}
	return testCommand, nil
}

func defineTestCommandParameters(negative bool, protocol string, mtu int, connectivity string, serverIP string) ([]string, error) {
	testCommand := []string{"testcmd"}
	var protocolOption string
	switch protocol {
	case parameters.CommunicationProtocolUnicastICMP:
		protocolOption = "icmp"
	case parameters.CommunicationProtocolUnicastTCP:
		protocolOption = "tcp"
		testCommand = append(testCommand, fmt.Sprintf("-port=%d", 80))
	case parameters.CommunicationProtocolUnicastSCTP:
		protocolOption = "sctp"
		testCommand = append(testCommand, fmt.Sprintf("-port=%d", testPort), fmt.Sprintf("-interface=%s", testInterfaceName))
	case parameters.CommunicationProtocolUnicastUDP:
		protocolOption = "udp"
		testCommand = append(testCommand, fmt.Sprintf("-port=%d", testPort))
	case parameters.CommunicationProtocolMulticastUDP:
		protocolOption = "udp"
		serverIP = "224.255.0.10"
		testCommand = append(testCommand, fmt.Sprintf("-port=%d", testPort), "-multicast", fmt.Sprintf("-interface=%s", testInterfaceName))
	case parameters.CommunicationProtocolBroadcastUDP:
		protocolOption = "udp"
		serverIP = "255.255.255.255"
		testCommand = append(testCommand, fmt.Sprintf("-port=%d", testPort), "-broadcast", fmt.Sprintf("-interface=%s", testInterfaceName))
	default:
		return nil, fmt.Errorf(fmt.Sprint(parameters.SriovErrorProtocolMessage, protocol))
	}
	switch {
	case negative:
		var parameterMtu string
		if mtu < parameters.MTUJumbo {
			parameterMtu = fmt.Sprintf("-mtu=%d", mtu+40)
		} else {
			parameterMtu = fmt.Sprintf("-mtu=%d", mtu)
		}
		testCommand = append(testCommand, "-negative", parameterMtu)
	default:
		testCommand = append(testCommand, fmt.Sprintf("-mtu=%d", mtu-50))
	}
	testCommand = append(testCommand, fmt.Sprintf("-server=%s", serverIP), fmt.Sprintf("-protocol=%s", protocolOption))
	return testCommand, nil
}

func defineServerNetworkName(mtu int) string {
	return defineNetworkName(mtu)
}

func defineClientNetworkName(mtu int, connectivity string) string {
	var networkName string
	switch connectivity {
	case parameters.ConnectivitySameNodeDiffPF:
		if mtu == parameters.MTUJumbo {
			networkName = sriovNetworkJumboFrameNameDiff
		} else if mtu == parameters.MTUStandart {
			networkName = sriovNetworkUsualMTUNameDiff
		} else if mtu == parameters.MTUCustom {
			networkName = sriovNetworkCustomMTUNameDiff
		} else {
			Skip(fmt.Sprintf("Unsupported test parameter mtu: %d", mtu))
		}
	default:
		networkName = defineNetworkName(mtu)
	}
	return networkName
}

func defineNetworkName(mtu int) string {
	var networkName string
	if mtu == parameters.MTUJumbo {
		networkName = sriovNetworkJumboFrameName
	} else if mtu == parameters.MTUStandart {
		networkName = sriovNetworkUsualMTUName
	} else if mtu == parameters.MTUCustom {
		networkName = sriovNetworkCustomMTUName
	} else {
		Skip(fmt.Sprintf("Unsupported test parameter mtu: %d", mtu))
	}
	return networkName
}

func defineNodeSelector(connectivity string, sriovInfos *cluster.EnabledNodes) []string {
	var nodeSelector []string
	if connectivity == parameters.ConnectivityDiffNode {
		if len(sriovInfos.Nodes) < 2 {
			Skip("Nodes number less that 2")
		}
		nodeSelector = sriovInfos.Nodes
	} else if connectivity == parameters.ConnectivitySameNodeSamePF || connectivity == parameters.ConnectivitySameNodeDiffPF {
		nodeSelector = append(nodeSelector, sriovInfos.Nodes[0])
	} else {
		Skip(fmt.Sprintf("Unsupported test parameter Connectivity: %s", connectivity))
	}
	return nodeSelector
}

func defineServerPod(protocol string, nodeSelector []string, networkName string, ipaddress string, macAddress string, podImage string, podCommand []string) *corev1.Pod {
	podDefinition := pod.DefineWithNodeNetworks(nodeSelector[0], []string{networkName},
		parameters.OperatorTestNamespace, podImage)
	if protocol == parameters.CommunicationProtocolUnicastSCTP {
		podDefinition = pod.RedefineAsPrivileged(podDefinition)
	}
	podDefinition = DefinePodCommand(
		DefinePodWithStaticIpam(podDefinition, networkName, ipaddress, macAddress),
		podCommand)
	return podDefinition
}

func defineClientPod(protocol string, nodeSelector []string, networkName string, ipaddress string, macAddress string, podImage string, podCommand []string) *corev1.Pod {
	var podDefinition *corev1.Pod
	if len(nodeSelector) > 1 {
		podDefinition = pod.RedefineWithRestartPolicy(
			pod.DefineWithNodeNetworks(nodeSelector[1], []string{networkName}, parameters.OperatorTestNamespace, podImage),
			corev1.RestartPolicyNever)
	} else {
		podDefinition = pod.RedefineWithRestartPolicy(
			pod.DefineWithNodeNetworks(nodeSelector[0], []string{networkName}, parameters.OperatorTestNamespace, podImage),
			corev1.RestartPolicyNever)
	}
	if protocol == parameters.CommunicationProtocolUnicastTCP || protocol == parameters.CommunicationProtocolUnicastSCTP {
		podDefinition = pod.RedefineAsPrivileged(podDefinition)
	}
	podDefinition = DefinePodCommand(
		DefinePodWithStaticIpam(podDefinition, networkName, ipaddress, macAddress),
		podCommand)
	return podDefinition
}
