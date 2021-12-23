package netsriovhelper

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovparameters"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBondModeScenario(
	mtu int,
	protocol string,
	connectivity string,
	sriovInfos *cluster.EnabledNodes,
	bondType string) {
	var (
		secondNetwork string
		slaveNetworks []string
	)

	By("Validating test parameters")

	connectivityParameters, err := netsriovparameters.NewConnectivityTestParameters(mtu, connectivity, protocol, true)
	Expect(err).ToNot(HaveOccurred())

	By("Defining test resources")

	nodeSelector := defineNodeSelector(connectivity, sriovInfos)

	if connectivityParameters.Connectivity == netsriovparameters.ConnectivityDiffNodeDiffPF {
		secondNetwork = netsriovparameters.SriovNetworkBondNameDiff
	} else {
		secondNetwork = netsriovparameters.SriovNetworkBondName
	}

	slaveNetworks = append(slaveNetworks, netsriovparameters.SriovNetworkBondName,
		secondNetwork)

	clientTestCommand, err := defineTestCommandParameters(
		false,
		connectivityParameters.Protocol,
		connectivityParameters.MTU,
		netsriovparameters.ServerPodIP,
		netsriovparameters.TestPort,
		netsriovparameters.TestBondInterfaceName)
	Expect(err).ToNot(HaveOccurred())

	clientPodDefinition := defineClientPod(
		protocol,
		nodeSelector,
		netsriovparameters.NADBondName,
		slaveNetworks,
		netsriovparameters.ClientPodIP,
		"",
		Config.Network.TestContainerImage,
		[]string{"/bin/bash", "-c", "sleep INF"})

	By("Creating Server Pod")

	runServerPod(
		protocol,
		connectivityParameters.MTU,
		connectivityParameters.Connectivity,
		sriovInfos,
		Config,
		netsriovparameters.NADBondName,
		slaveNetworks,
		false,
		"",
		netsriovparameters.ServerPodIP,
		netsriovparameters.TestBondInterfaceName)

	By("Creating Client Pod")

	clientPod, err := Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
		context.Background(),
		clientPodDefinition,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	waitUntilPodInStatus(
		clientPod,
		"Client",
		[]string{"/bin/bash", "-c", "sleep INF"},
		corev1.PodRunning,
		netsriovparameters.PodWaitingTime)

	By("Checking that Bond interface is configured")

	isBondInterfaceConfigured, err := bondInterfaceIsUp(clientPod, netsriovparameters.TestBondInterfaceName)
	Expect(err).ToNot(HaveOccurred())
	Expect(isBondInterfaceConfigured).To(BeTrue(), "Bond interface is not Up")

	isBondInterfaceConfigured, err = bondInterfaceHasMode(clientPod, netsriovparameters.TestBondInterfaceName, bondType)
	Expect(err).ToNot(HaveOccurred())
	Expect(isBondInterfaceConfigured).To(BeTrue(), "Bond interface has incorrect bond type")

	isBondInterfaceConfigured, err = bondInterfaceHasSlaves(clientPod, netsriovparameters.TestBondInterfaceName, "2")
	Expect(err).ToNot(HaveOccurred())
	Expect(isBondInterfaceConfigured).To(BeTrue(), "Bond interface has wrong number of slaves")

	By(fmt.Sprintf("Checking traffic - %s", protocol))

	_, err = pod.ExecCommand(Apiclient, *clientPod, clientTestCommand)
	Expect(err).ToNot(HaveOccurred(), "Traffic failed")

	activeVF, err := findBondActiveInterface(clientPod, netsriovparameters.TestBondInterfaceName)
	Expect(err).ToNot(HaveOccurred())

	By(fmt.Sprintf("Disabling  active interface %s and check the traffic again", activeVF))

	err = setInterfaceStatus(clientPod, activeVF, "down")
	Expect(err).ToNot(HaveOccurred())

	secondaryVF, err := findBondActiveInterface(clientPod, netsriovparameters.TestBondInterfaceName)
	Expect(err).ToNot(HaveOccurred())
	Expect(secondaryVF).ToNot(Equal(activeVF), "Active Bond interface  not changed after failover")

	_, err = pod.ExecCommand(Apiclient, *clientPod, clientTestCommand)
	Expect(err).ToNot(HaveOccurred(), "Traffic failed")

	By(fmt.Sprintf("Disabling secondary interface %s and bring active interface %s back"+
		" and check the traffic again", secondaryVF, activeVF))

	err = setInterfaceStatus(clientPod, activeVF, "up")
	Expect(err).ToNot(HaveOccurred())
	err = setInterfaceStatus(clientPod, secondaryVF, "down")
	Expect(err).ToNot(HaveOccurred())
	Expect(activeVF).ToNot(Equal(secondaryVF), "Active Bond interface  not changed after failover")

	_, err = pod.ExecCommand(Apiclient, *clientPod, clientTestCommand)
	Expect(err).ToNot(HaveOccurred(), "Traffic failed")
}

// setInterfaceStatus enable or disable an interface on a pod using the IP command.
func setInterfaceStatus(clientPod *corev1.Pod, nic string, status string) error {
	_, err := pod.ExecCommand(Apiclient, *clientPod, []string{"ip", "link", "set", "dev", nic, status})

	return err
}

// findBondActiveInterface returns active interface in a bond.
func findBondActiveInterface(clientPod *corev1.Pod, bondInterfaceName string) (string, error) {
	activeInterface, err := pod.ExecCommand(Apiclient, *clientPod, []string{"cat",
		fmt.Sprintf("/sys/class/net/%s/bonding/active_slave", bondInterfaceName)})

	return strings.TrimSpace(activeInterface.String()), err
}

// CreateBondNad creates a Bond Network Attachment Definition.
// Need to add MTU support.
func CreateBondNad(nadName string, bondType string) error {
	bondDefinition := netattdefv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nadName,
			Namespace: netsriovparameters.OperatorTestNamespace,
		},
		Spec: netattdefv1.NetworkAttachmentDefinitionSpec{
			Config: fmt.Sprintf(
				`{"type": "bond", "cniVersion": "0.3.1", "name": "%s", "ifname": "%s",
"mode": "%s", "failOverMac": 1, "linksInContainer": true, "miimon": "100", "links": [{"name": "net1"},{"name": "net2"}],
"capabilities": {"mac": true, "ips": true}, "ipam": {"type": "static"}}`,
				netsriovparameters.NADBondName, netsriovparameters.TestBondInterfaceName, bondType),
		},
	}

	return Apiclient.Create(context.Background(), &bondDefinition)
}

// bondInterfaceIsUp verifies that the bond is Up.
func bondInterfaceIsUp(clientPod *corev1.Pod, bondInterfaceName string) (bool, error) {
	status, err := pod.ExecCommand(Apiclient, *clientPod, []string{"cat",
		fmt.Sprintf("/sys/class/net/%s/bonding/mii_status", bondInterfaceName)})
	if err != nil {
		return false, err
	}

	return strings.Contains(status.String(), "up"), nil
}

// bondInterfaceHasMode verifies that the bond has expected bond type.
func bondInterfaceHasMode(clientPod *corev1.Pod, bondInterfaceName string, bondMode string) (bool, error) {
	bondModeOut, err := pod.ExecCommand(Apiclient, *clientPod, []string{"cat",
		fmt.Sprintf("/sys/class/net/%s/bonding/mode", bondInterfaceName)})
	if err != nil {
		return false, err
	}

	return strings.Contains(bondModeOut.String(), bondMode), nil
}

// bondInterfaceHasSlaves verifies that the bond has expected number of slaves.
func bondInterfaceHasSlaves(clientPod *corev1.Pod, bondInterfaceName string, numSlaves string) (bool, error) {
	command := fmt.Sprintf("wc -w /sys/class/net/%s/bonding/slaves | awk '{print $1;}'", bondInterfaceName)
	numSlavesOut, err := pod.ExecCommand(Apiclient, *clientPod, []string{"bash", "-c", command})

	if err != nil {
		return false, err
	}

	return strings.Contains(numSlavesOut.String(), numSlaves), nil
}

// defineSriovBondNetwork builds SriovNetwork resource for Bond Interface.
func defineSriovBondNetwork(name string, resourceName string) *sriovv1.SriovNetwork {
	sriovNetwork := defineSriovNetwork(name, resourceName)
	sriovNetwork.Spec.Trust = "on"
	sriovNetwork.Spec.SpoofChk = "off"

	return sriovNetwork
}
