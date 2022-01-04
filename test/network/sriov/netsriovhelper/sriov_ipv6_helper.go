package netsriovhelper

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovparameters"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSriovIPv6Scenario(
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
	serverNetworkName := defineServerNetworkName(mtu)
	clientNetworkName := defineClientNetworkName(mtu, connectivityParameters.Connectivity)
	negativeFlag := false
	clientTestCommand, err := DefineTestCommandParameters(
		negativeFlag,
		connectivityParameters.Protocol,
		connectivityParameters.MTU,
		netsriovparameters.ServerPodIpv6,
		netsriovparameters.TestPort,
		netsriovparameters.TestInterfaceName)
	Expect(err).ToNot(HaveOccurred())

	clientPodDefinition := DefineClientPod(
		connectivityParameters.Protocol,
		nodeSelector,
		clientNetworkName,
		[]string{},
		netsriovparameters.ClientPodIPv6,
		clientMacAddress,
		config.Network.TestContainerImage,
		clientTestCommand,
		netsriovparameters.IpamStatic)

	By("Creating Server Pod")
	RunServerPod(
		protocol,
		connectivityParameters.MTU,
		connectivityParameters.Connectivity,
		sriovInfos,
		config,
		serverNetworkName,
		[]string{},
		negativeFlag,
		serverMacAddress,
		netsriovparameters.ServerPodIpv6,
		netsriovparameters.TestInterfaceName,
		netsriovparameters.IpamStatic)

	By("Creating Client Pod")

	clientPod, err := Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
		context.Background(),
		clientPodDefinition,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	WaitUntilPodInStatus(
		clientPod,
		"Client",
		clientTestCommand,
		corev1.PodSucceeded,
		netsriovparameters.PodWaitingTime)

	if protocol == netsriovparameters.CommunicationProtocolUnicastTCP {
		By("Positive test flow - success")

		return
	}

	err = pod.DeletePodAndWait(Apiclient, clientPod)
	Expect(err).ToNot(HaveOccurred())

	By("Positive test flow - success. Running negative flow")

	negativeFlag = true

	if protocol == netsriovparameters.CommunicationProtocolUnicastSCTP {
		serverNetworkName = defineClientNetworkName(mtu, connectivityParameters.Connectivity)
		clientNetworkName = defineServerNetworkName(mtu)
	}

	if protocol == netsriovparameters.CommunicationProtocolMulticastUDP ||
		protocol == netsriovparameters.CommunicationProtocolBroadcastUDP ||
		protocol == netsriovparameters.CommunicationProtocolUnicastSCTP {
		RunServerPod(
			protocol,
			connectivityParameters.MTU,
			connectivityParameters.Connectivity,
			sriovInfos,
			config,
			serverNetworkName,
			[]string{},
			negativeFlag,
			serverMacAddress,
			netsriovparameters.ServerPodIpv6,
			netsriovparameters.TestInterfaceName,
			netsriovparameters.IpamStatic)
	}

	clientTestCommand, err = DefineTestCommandParameters(
		negativeFlag,
		connectivityParameters.Protocol,
		connectivityParameters.MTU,
		netsriovparameters.ServerPodIpv6,
		netsriovparameters.TestPort,
		netsriovparameters.TestInterfaceName)
	Expect(err).ToNot(HaveOccurred())

	By("Creating Client Pod with negative flag")

	clientPodDefinitionNegative := DefineClientPod(
		connectivityParameters.Protocol,
		nodeSelector,
		clientNetworkName,
		[]string{},
		netsriovparameters.ClientPodIPv6,
		clientMacAddress,
		config.Network.TestContainerImage,
		clientTestCommand,
		netsriovparameters.IpamStatic)
	clientPodNegative, err := Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
		context.Background(),
		clientPodDefinitionNegative,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	WaitUntilPodInStatus(
		clientPodNegative,
		"Client",
		clientTestCommand,
		corev1.PodSucceeded,
		netsriovparameters.PodWaitingTime)
}
