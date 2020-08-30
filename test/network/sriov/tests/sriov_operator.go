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
	sriovNetworkUsualMTUName   = "test-sriov-static-usual"
	sriovNetworkCustomMTUName  = "test-sriov-static-custom"
	sriovNetworkJumboFrameName = "test-sriov-static-jumbo"
	clientPodIP                = "192.168.100.1"
	clientMacAddress           = "20:04:0f:f1:88:01"
	serverPodIP                = "192.168.100.2"
	serverMacAddress           = "20:04:0f:f1:88:03"
)

var (
	waitingTime    time.Duration = 20 * time.Minute
	podWaitingTime time.Duration = 3 * time.Minute
)

var _ = Describe("CNF SRIOV", func() {
	describe := func(desc string) func(mtu int, connectivity string, protocol string) string {

		return func(mtu int, connectivity string, protocol string) string {
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
	var err error
	config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())

	execute.BeforeAll(func() {
		sriovInfos, err = cluster.DiscoverSriov(clients, operatorNamespace)
		Expect(err).ToNot(HaveOccurred())
		sriovInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
		Expect(err).ToNot(HaveOccurred())
		namespaces.Clean(operatorNamespace, parameters.OperatorTestNamespace, clients, false)
		WaitForSRIOVStable(clients, operatorNamespace, waitingTime)
		Expect(err).ToNot(HaveOccurred())
		err = namespaces.Create(parameters.OperatorTestNamespace, clients)
		Expect(err).ToNot(HaveOccurred())
		usualSriovPolicyConfig := DefineSriovPolicy("test-policy-usual", sriovInterfaces[0], "#0-1", 1500, "testresourceusual", "netdevice")
		customSriovPolicyConfig := DefineSriovPolicy("test-policy-custom", sriovInterfaces[0], "#2-3", 1450, "testresourcecustom", "netdevice")
		jumboSriovPolicyConfig := DefineSriovPolicy("test-policy-jumbo", sriovInterfaces[1], "#0-1", 9000, "testresourcejumbo", "netdevice")
		for _, networkPolicy := range []*sriovv1.SriovNetworkNodePolicy{usualSriovPolicyConfig, customSriovPolicyConfig, jumboSriovPolicyConfig} {
			err = clients.Create(context.Background(), networkPolicy)
			Expect(err).ToNot(HaveOccurred())
		}
		usualSriovNetworkConfig := DefineSriovNetwork(sriovNetworkUsualMTUName, usualSriovPolicyConfig.Spec.ResourceName, true)
		customSriovNetworkConfig := DefineSriovNetwork(sriovNetworkCustomMTUName, customSriovPolicyConfig.Spec.ResourceName, true)
		jumboSriovNetworkConfig := DefineSriovNetwork(sriovNetworkJumboFrameName, jumboSriovPolicyConfig.Spec.ResourceName, true)
		for _, network := range []*sriovv1.SriovNetwork{usualSriovNetworkConfig, customSriovNetworkConfig, jumboSriovNetworkConfig} {
			err = clients.Create(context.Background(), network)
			Expect(err).ToNot(HaveOccurred())
		}
		WaitForSRIOVStable(clients, operatorNamespace, waitingTime)
	})

	AfterEach(func() {
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

	DescribeTable("IP Static, Ip Stack: Dual-stack, Mac address: MAC static", func(mtu int, connectivity string, protocol string) {
		connectivityParameters, err := parameters.NewConnectivityTestParameters(mtu, connectivity, protocol)
		Expect(err).ToNot(HaveOccurred())
		testCommand := defineTestCommandParameters(
			false, connectivityParameters.Protocol, connectivityParameters.MTU, connectivityParameters.Connectivity, serverPodIP)
		networkName := defineNetworkName(connectivityParameters.MTU)
		nodeSelector := defineNodeSelector(connectivityParameters.Connectivity, sriovInfos)
		serverPodDefinition := defineTestPod(
			"server", nodeSelector, networkName, serverPodIP, serverMacAddress, "centos", []string{"sleep", "INF"})
		clientPodDefinition := defineTestPod(
			"client", nodeSelector, networkName, clientPodIP, clientMacAddress, config.Network.ClientContainerImage, testCommand)
		serverPod, err := clients.Pods(parameters.OperatorTestNamespace).Create(context.Background(), serverPodDefinition, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() corev1.PodPhase {
			serverPod, _ = clients.Pods(parameters.OperatorTestNamespace).Get(context.Background(), serverPod.Name, metav1.GetOptions{})
			return serverPod.Status.Phase
		}, 3*time.Minute, time.Second).Should(Equal(corev1.PodRunning))
		clientPod, err := clients.Pods(parameters.OperatorTestNamespace).Create(context.Background(), clientPodDefinition, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() corev1.PodPhase {
			clientPod, _ = clients.Pods(parameters.OperatorTestNamespace).Get(context.Background(), clientPod.Name, metav1.GetOptions{})
			return clientPod.Status.Phase
		}, 3*time.Minute, time.Second).Should(Equal(corev1.PodSucceeded))
		testCommand = defineTestCommandParameters(
			true, connectivityParameters.Protocol, connectivityParameters.MTU, connectivityParameters.Connectivity, serverPodIP)
		clientPodDefinitionNegative := defineTestPod(
			"client", nodeSelector, networkName, clientPodIP, clientMacAddress, config.Network.ClientContainerImage, testCommand)
		clientPodNegative, err := clients.Pods(parameters.OperatorTestNamespace).Create(context.Background(), clientPodDefinitionNegative, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() corev1.PodPhase {
			clientPodNegative, _ = clients.Pods(parameters.OperatorTestNamespace).Get(context.Background(), clientPodNegative.Name, metav1.GetOptions{})
			return clientPodNegative.Status.Phase
		}, 3*time.Minute, time.Second).Should(Equal(corev1.PodSucceeded))
	},

		Entry(describe(""), parameters.MTUCustom, parameters.ConnectivityDiffNode, parameters.CommunicationProtocolUnicastICMP),
		Entry(describe(""), parameters.MTUCustom, parameters.ConnectivitySameNodeSamePF, parameters.CommunicationProtocolUnicastICMP),
		Entry(describe(""), parameters.MTUStandart, parameters.ConnectivityDiffNode, parameters.CommunicationProtocolUnicastICMP),
		Entry(describe(""), parameters.MTUStandart, parameters.ConnectivitySameNodeSamePF, parameters.CommunicationProtocolUnicastICMP),
		Entry(describe(""), parameters.MTUJumbo, parameters.ConnectivityDiffNode, parameters.CommunicationProtocolUnicastICMP),
		Entry(describe(""), parameters.MTUJumbo, parameters.ConnectivitySameNodeSamePF, parameters.CommunicationProtocolUnicastICMP),
	)
})

func defineTestCommandParameters(negative bool, protocol string, mtu int, connectivity string, serverIP string) []string {
	testCommand := []string{"testcmd"}
	var protocolOption string
	if protocol == parameters.CommunicationProtocolUnicastICMP {
		protocolOption = "icmp"
	} else {
		Skip(fmt.Sprintf("Unsupported test parameter %s", protocol))
	}
	if negative {
		var parameterMtu string
		if mtu < parameters.MTUJumbo {
			parameterMtu = fmt.Sprintf("-mtu=%d", mtu+40)
		} else {
			parameterMtu = fmt.Sprintf("-mtu=%d", mtu)
		}
		testCommand = append(testCommand, "-negative", parameterMtu)
	} else {
		testCommand = append(testCommand, fmt.Sprintf("-mtu=%d", mtu-40))
	}
	testCommand = append(testCommand, fmt.Sprintf("-server=%s", serverIP), fmt.Sprintf("-protocol=%s", protocolOption))
	return testCommand
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
	} else if connectivity == parameters.ConnectivitySameNodeSamePF {
		nodeSelector = append(nodeSelector, sriovInfos.Nodes[0])
	} else {
		Skip(fmt.Sprintf("Unsupported test parameter Connectivity: %s", connectivity))
	}
	return nodeSelector
}

func defineTestPod(podType string, nodeSelector []string, networkName string, ipaddress string, macAddress string, podImage string, podCommand []string) *corev1.Pod {
	var podDefinition *corev1.Pod
	if podType == "server" {
		podDefinition = pod.DefineWithNodeNetworks(nodeSelector[0], []string{networkName}, parameters.OperatorTestNamespace, podImage)
	} else {
		if len(nodeSelector) > 1 {
			podDefinition = pod.RedefineWithRestartPolicy(
				pod.DefineWithNodeNetworks(nodeSelector[1], []string{networkName}, parameters.OperatorTestNamespace, podImage),
				corev1.RestartPolicyNever)
		} else {
			podDefinition = pod.RedefineWithRestartPolicy(
				pod.DefineWithNodeNetworks(nodeSelector[0], []string{networkName}, parameters.OperatorTestNamespace, podImage),
				corev1.RestartPolicyNever)
		}
	}
	podDefinition = DefinePodCommand(
		DefinePodWithStaticIpam(podDefinition, networkName, ipaddress, macAddress),
		podCommand)
	return podDefinition
}
