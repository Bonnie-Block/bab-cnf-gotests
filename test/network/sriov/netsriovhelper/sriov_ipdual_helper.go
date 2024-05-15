package netsriovhelper

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovparameters"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// definePodCommandsWithDualIpamAndMac returns a pod with 2 ip addresses and 2 containers.
func definePodCommandsWithDualIpamAndMac(
	podDefinition *corev1.Pod,
	networkName string,
	ip4address string,
	ip6address string,
	macAddress string,
	podCommands ...[]string) *corev1.Pod {
	if macAddress == "" {
		podDefinition = definePodCommands(
			definePodWithStaticDualIpamAndDynamicMac(podDefinition, networkName, ip4address, ip6address),
			podCommands...)
	} else {
		podDefinition = definePodCommands(
			definePodWithStaticMacAndDualIpam(podDefinition, networkName, ip4address, ip6address, macAddress),
			podCommands...)
	}

	return podDefinition
}

// definePodWithStaticDualIpamAndDynamicMac sets pod network with static dual IPAM config with Dynamic Mac address.
func definePodWithStaticDualIpamAndDynamicMac(
	pod *corev1.Pod, networkName string, ip4address string, ip6address string) *corev1.Pod {
	pod.Annotations = map[string]string{"k8s.v1.cni.cncf.io/networks": fmt.Sprintf(`[
		{
			"name": "%s",
			"ips": ["%s/%s","%s/%s"]
		}
	]`, networkName, ip4address, netparameters.IPSubnet24, ip6address, netparameters.IPSubnet64)}

	return pod
}

// definePodWithStaticMacAndDualIpam sets pod network with static IPAM config with Static Mac address.
func definePodWithStaticMacAndDualIpam(
	pod *corev1.Pod, networkName string, ip4address string, ip6address string, macAddress string) *corev1.Pod {
	pod.Annotations = map[string]string{"k8s.v1.cni.cncf.io/networks": fmt.Sprintf(`[
		{
			"name": "%s", 
			"mac": "%s",
			"ips": ["%s/%s","%s/%s"]
		}
	]`, networkName, macAddress, ip4address, netparameters.IPSubnet24, ip6address, netparameters.IPSubnet64)}

	return pod
}

// definePodCommands sets a container for every command.
func definePodCommands(pod *corev1.Pod, commands ...[]string) *corev1.Pod {
	for index := 0; index < len(commands); index++ {
		if index == len(pod.Spec.Containers) {
			pod.Spec.Containers = append(pod.Spec.Containers, *pod.Spec.Containers[0].DeepCopy())
		}

		pod.Spec.Containers[index].Command = commands[index]
		pod.Spec.Containers[index].Name = fmt.Sprintf("test-%d", index)
	}

	return pod
}

func runDualServerPod(
	protocol string,
	mtu int,
	connectivity string,
	sriovInfos *cluster.EnabledNodes,
	config *config.Config,
	networkName string,
	negative bool,
	serverMacAddress string,
	serverIPV4 string,
	serverIPV6 string) *corev1.Pod {
	nodeSelector := defineNodeSelector(connectivity, sriovInfos)

	serverIPv4Command, err := serverCommandFor(protocol,
		mtu,
		serverIPV4,
		negative,
		netsriovparameters.TestPort,
		netsriovparameters.TestInterfaceName)
	Expect(err).ToNot(HaveOccurred())

	serverIPv6Command, err := serverCommandFor(protocol,
		mtu,
		serverIPV6,
		negative,
		netsriovparameters.TestPort+1,
		netsriovparameters.TestInterfaceName)
	Expect(err).ToNot(HaveOccurred())

	// IPv6 can't be used for udp-broadcast. tcp-unicast running for ipv4 and ipv6 by default
	if protocol == netsriovparameters.CommunicationProtocolBroadcastUDP {
		serverIPv6Command = []string{"sleep", "INF"}
	}

	serverPodDefinition := defineDualServerPod(
		protocol,
		nodeSelector,
		networkName,
		serverIPV4,
		serverIPV6,
		serverMacAddress,
		config.Network.TestContainerImage,
		serverIPv4Command,
		serverIPv6Command)

	serverPod, err := Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
		context.Background(), serverPodDefinition, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	WaitUntilPodInStatus(
		serverPod,
		"Server",
		append(serverIPv4Command, serverIPv6Command...),
		corev1.PodRunning,
		netsriovparameters.DualPodWaitingTime)

	return serverPod
}

func defineDualServerPod(protocol string,
	nodeSelector []string,
	networkName string,
	ip4address string,
	ip6address string,
	macAddress string,
	podImage string,
	podIPv4Command []string,
	podIPv6Command []string) *corev1.Pod {
	podDefinition := pod.RedefineWithRestartPolicy(
		pod.DefineWithNodeNetworks(
			nodeSelector[0],
			[]string{networkName},
			netsriovparameters.OperatorTestNamespace, podImage),
		corev1.RestartPolicyNever)

	if serverNeedsPrivilege(protocol) {
		podDefinition = pod.RedefineAsPrivileged(podDefinition)
	}

	podDefinition = definePodCommandsWithDualIpamAndMac(
		podDefinition,
		networkName,
		ip4address,
		ip6address,
		macAddress,
		podIPv4Command,
		podIPv6Command)

	podDefinition = redefinePodWithInitCommandPolicy(podDefinition, podImage,
		fmt.Sprintf(
			"ping6 -c 3 -w 30 %s && ip -6 route add %s/128 dev net1 table local",
			netsriovparameters.ServerPodIpv6, netsriovparameters.MulticastIPv6Address))

	return redefinePodWithInitDebugCommands(podDefinition, podImage)
}

func defineDualClientPod(
	protocol string,
	nodeSelector []string,
	networkName string,
	ip4address string,
	ip6address string,
	macAddress string,
	podImage string,
	podCommand []string) *corev1.Pod {
	var podDefinition *corev1.Pod

	if len(nodeSelector) > 1 {
		podDefinition = pod.RedefineWithRestartPolicy(
			pod.DefineWithNodeNetworks(
				nodeSelector[1],
				[]string{networkName},
				netsriovparameters.OperatorTestNamespace, podImage),
			corev1.RestartPolicyNever)
	} else {
		podDefinition = pod.RedefineWithRestartPolicy(
			pod.DefineWithNodeNetworks(nodeSelector[0],
				[]string{networkName},
				netsriovparameters.OperatorTestNamespace, podImage),
			corev1.RestartPolicyNever)
	}

	if clientNeedsPrivilege(protocol) {
		podDefinition = pod.RedefineAsPrivileged(podDefinition)
	}

	podDefinition = definePodCommandsWithDualIpamAndMac(
		podDefinition,
		networkName,
		ip4address,
		ip6address,
		macAddress,
		podCommand)

	return redefinePodWithInitDebugCommands(podDefinition, podImage)
}

func TestSriovDualScenario(
	mtu int,
	protocol string,
	connectivity string,
	sriovInfos *cluster.EnabledNodes,
	config *config.Config,
	clientMacAddress string,
	serverMacAddress string) {
	By("Validating test parameters")

	connectivityParameters, err := netsriovparameters.NewConnectivityTestParameters(mtu, connectivity, protocol, false)
	Expect(err).ToNot(HaveOccurred())
	By("Defining test resources")

	nodeSelector := defineNodeSelector(connectivity, sriovInfos)
	serverNetworkName := defineServerNetworkName(mtu, 0, netsriovparameters.IpamStatic, netparameters.IPV4Family)
	clientNetworkName := defineClientNetworkName(mtu, 0, connectivityParameters.Connectivity,
		netsriovparameters.IpamStatic, netparameters.IPV4Family)
	negativeFlag := false

	ipv4ClientTestCommand, err := DefineTestCommandParameters(
		negativeFlag,
		connectivityParameters.Protocol,
		connectivityParameters.MTU,
		netsriovparameters.ServerPodIP,
		netsriovparameters.TestPort,
		netsriovparameters.TestInterfaceName)
	Expect(err).ToNot(HaveOccurred())

	// IPv6 can't be used for udp-broadcast
	var ipv6ClientTestCommand []string
	if protocol == netsriovparameters.CommunicationProtocolBroadcastUDP {
		ipv6ClientTestCommand = []string{"exit 0"}
	} else {
		ipv6ClientTestCommand, err = DefineTestCommandParameters(
			negativeFlag,
			connectivityParameters.Protocol,
			connectivityParameters.MTU,
			netsriovparameters.ServerPodIpv6,
			netsriovparameters.TestPort+1,
			netsriovparameters.TestInterfaceName)
		Expect(err).ToNot(HaveOccurred())
	}

	ipv4StringCommand := strings.Join(ipv4ClientTestCommand, " ")
	ipv6StringCommand := strings.Join(ipv6ClientTestCommand, " ")

	ClientTestCommand := []string{"/bin/bash", "-c"}
	ClientTestCommand = append(ClientTestCommand, ipv4StringCommand+" && "+ipv6StringCommand)

	clientPodDefinition := defineDualClientPod(
		connectivityParameters.Protocol,
		nodeSelector,
		clientNetworkName,
		netsriovparameters.ClientPodIP,
		netsriovparameters.ClientPodIPv6,
		clientMacAddress,
		config.Network.TestContainerImage,
		ClientTestCommand,
	)

	By("Creating Server Pod")

	serverPod := runDualServerPod(
		protocol,
		connectivityParameters.MTU,
		connectivityParameters.Connectivity,
		sriovInfos,
		config,
		serverNetworkName,
		negativeFlag,
		serverMacAddress,
		netsriovparameters.ServerPodIP,
		netsriovparameters.ServerPodIpv6,
	)

	By("Creating Client Pod")

	clientPod, err := Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
		context.Background(),
		clientPodDefinition,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	WaitUntilPodInStatus(
		clientPod, "Client", ClientTestCommand, corev1.PodSucceeded, netsriovparameters.DualPodWaitingTime)

	if protocol == netsriovparameters.CommunicationProtocolUnicastTCP {
		By("Positive test flow - success")

		return
	}

	err = pod.DeletePodAndWait(Apiclient, clientPod)
	Expect(err).ToNot(HaveOccurred())

	By("Positive test flow - success. Running negative flow")

	negativeFlag = true

	if protocol == netsriovparameters.CommunicationProtocolUnicastSCTP {
		serverNetworkName = defineClientNetworkName(mtu, 0, connectivityParameters.Connectivity,
			netsriovparameters.IpamStatic, netparameters.IPV4Family)
		clientNetworkName = defineServerNetworkName(mtu, 0, netsriovparameters.IpamStatic, netparameters.IPV4Family)
	}

	if protocol == netsriovparameters.CommunicationProtocolMulticastUDP ||
		protocol == netsriovparameters.CommunicationProtocolBroadcastUDP ||
		protocol == netsriovparameters.CommunicationProtocolUnicastSCTP {
		err = pod.DeletePodAndWait(Apiclient, serverPod)
		Expect(err).ToNot(HaveOccurred())

		runDualServerPod(
			protocol,
			connectivityParameters.MTU,
			connectivityParameters.Connectivity,
			sriovInfos,
			config,
			serverNetworkName,
			negativeFlag,
			serverMacAddress,
			netsriovparameters.ServerPodIP,
			netsriovparameters.ServerPodIpv6,
		)
	}

	By("Creating Client Pod with negative flag")

	ipv4ClientTestCommand, err = DefineTestCommandParameters(
		negativeFlag,
		connectivityParameters.Protocol,
		connectivityParameters.MTU,
		netsriovparameters.ServerPodIP,
		netsriovparameters.TestPort,
		netsriovparameters.TestInterfaceName)
	Expect(err).ToNot(HaveOccurred())

	ipv6ClientTestCommand, err = DefineTestCommandParameters(
		negativeFlag,
		connectivityParameters.Protocol,
		connectivityParameters.MTU,
		netsriovparameters.ServerPodIpv6,
		netsriovparameters.TestPort+1,
		netsriovparameters.TestInterfaceName)
	Expect(err).ToNot(HaveOccurred())

	ipv4NegativeStringCommand := strings.Join(ipv4ClientTestCommand, " ")
	ipv6NegativeStringCommand := strings.Join(ipv6ClientTestCommand, " ")

	ClientNegativeTestCommand := []string{"/bin/bash", "-c"}
	ClientNegativeTestCommand = append(
		ClientNegativeTestCommand,
		ipv4NegativeStringCommand+" && "+ipv6NegativeStringCommand)

	clientPodDefinitionNegative := defineDualClientPod(
		connectivityParameters.Protocol,
		nodeSelector,
		clientNetworkName,
		netsriovparameters.ClientPodIP,
		netsriovparameters.ClientPodIPv6,
		clientMacAddress,
		config.Network.TestContainerImage,
		ClientNegativeTestCommand,
	)

	clientPodNegative, err := Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
		context.Background(),
		clientPodDefinitionNegative,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	WaitUntilPodInStatus(
		clientPodNegative,
		"Client",
		ClientTestCommand,
		corev1.PodSucceeded,
		netsriovparameters.DualPodWaitingTime)
}
