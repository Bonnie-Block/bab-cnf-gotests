package netsriovhelper

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/switchcmd"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBondScenario(
	mtu int,
	sriovInfos *cluster.EnabledNodes,
	protocol, connectivity, bondType, ipAddrServer, ipAddrClient, ipam string) {
	var (
		secondNetwork string
		slaveNetworks []string
	)

	By("Validating test parameters")

	connectivityParameters, err := netsriovparameters.NewConnectivityTestParameters(mtu, connectivity, protocol, true)
	Expect(err).ToNot(HaveOccurred())

	By(fmt.Sprintf("Creating Bond interface - %s", bondType))
	nadBond, err := DefineBondNad(netsriovparameters.BondNadName, bondType, mtu, 2, ipam)
	Expect(err).ToNot(HaveOccurred())

	err = Apiclient.Create(context.Background(), nadBond)
	Expect(err).ToNot(HaveOccurred())

	By("Defining test resources")

	if connectivityParameters.Connectivity == netsriovparameters.ConnectivityDiffNodeDiffPF {
		secondNetwork = netsriovparameters.SriovNetworkBondNameDiff
	} else {
		secondNetwork = netsriovparameters.SriovNetworkBondName
	}

	slaveNetworks = append(slaveNetworks, netsriovparameters.SriovNetworkBondName,
		secondNetwork)

	clientTestCommand, err := DefineTestCommandParameters(
		false,
		connectivityParameters.Protocol,
		connectivityParameters.MTU,
		ipAddrServer,
		netsriovparameters.TestPort,
		netsriovparameters.TestBondInterfaceName)
	Expect(err).ToNot(HaveOccurred())

	clientPod := createTestPods(sriovInfos, slaveNetworks, connectivityParameters.MTU,
		ipam, connectivity, protocol, ipAddrServer, ipAddrClient)

	By("Checking that Bond interface is configured")
	isBondInterfaceConfigured(clientPod, bondType)

	// Uncomment once BZ 2082360 is fixed
	// err = verifyPodAnnotation(clientPod, ipAddrClient, "")
	// Expect(err).ToNot(HaveOccurred())
	By(fmt.Sprintf("Checking traffic - %s", protocol))

	_, err = pod.ExecCommand(Apiclient, *clientPod, clientTestCommand)
	Expect(err).ToNot(HaveOccurred(), "Traffic failed")

	activeVF, err := findBondActiveInterface(clientPod)
	Expect(err).ToNot(HaveOccurred())

	By(fmt.Sprintf("Disabling active interface %s and check the traffic again", activeVF))
	err = setInterfaceStatus(clientPod, activeVF, "down")
	Expect(err).ToNot(HaveOccurred())

	secondaryVF, err := findBondActiveInterface(clientPod)
	Expect(err).ToNot(HaveOccurred())
	Expect(secondaryVF).ToNot(Equal(activeVF), "Active Bond interface  not changed after failover")

	_, err = pod.ExecCommand(Apiclient, *clientPod, clientTestCommand)
	Expect(err).ToNot(HaveOccurred(), "Traffic failed")

	By(fmt.Sprintf("Disabling secondary interface %s, bring active interface %s back"+
		" and check the traffic again", secondaryVF, activeVF))

	err = setInterfaceStatus(clientPod, activeVF, "up")
	Expect(err).ToNot(HaveOccurred())
	err = setInterfaceStatus(clientPod, secondaryVF, "down")
	Expect(err).ToNot(HaveOccurred())
	Expect(activeVF).ToNot(Equal(secondaryVF), "Active Bond interface  not changed after failover")

	_, err = pod.ExecCommand(Apiclient, *clientPod, clientTestCommand)
	Expect(err).ToNot(HaveOccurred(), "Traffic failed")
}

func TestActiveActiveBondScenario(
	mtu int,
	sriovInfos *cluster.EnabledNodes,
	protocol, bondMode, ipAddrServer, ipAddrClient string) {
	By("Validating test parameters")

	switchCredentials, err := nethelper.NewSwitchCredentials()
	if err != nil {
		Skip(fmt.Sprintf("Failed to get switch credentials: %s", err))
	}

	switchInterfaces, err := Config.GetSwitchInterfaces()
	if err != nil {
		Skip(fmt.Sprintf("Failed to get switch interfaces: %s", err))
	}

	switchLagNames, err := Config.GetSwitchLagNames()
	if err != nil {
		Skip(fmt.Sprintf("Failed to get switch LAG names: %s", err))
	}

	bondActiveActiveParameters, err := netsriovparameters.NewBondActiveActive(mtu,
		bondMode, protocol)
	Expect(err).ToNot(HaveOccurred())

	if len(switchInterfaces) != 4 {
		Skip(fmt.Sprintf("Wrong number of switch interfaces %v, should be 4", switchInterfaces))
	}

	By(fmt.Sprintf("Remove all configuration from the switch interfaces %v", switchInterfaces))
	err = dumpInterfaceConfigs(switchCredentials, switchInterfaces)
	Expect(err).ToNot(HaveOccurred())
	err = RemoveAllConfigurationFromInterfaces(switchCredentials, switchInterfaces)
	Expect(err).ToNot(HaveOccurred())

	By("Configure LAGs on a switch")
	configureLAGsOnSwitch(switchCredentials, switchInterfaces, switchLagNames)

	By(fmt.Sprintf("Creating Bond interface - %s", bondMode))
	nadBond, err := DefineBondNad(netsriovparameters.BondNadName, bondMode, mtu, 2, netsriovparameters.IpamStatic)
	Expect(err).ToNot(HaveOccurred())

	err = Apiclient.Create(context.Background(), nadBond)
	Expect(err).ToNot(HaveOccurred())

	By("Defining test resources")

	clientTestCommand, err := DefineTestCommandParameters(
		false,
		bondActiveActiveParameters.Protocol,
		bondActiveActiveParameters.MTU,
		ipAddrServer,
		netsriovparameters.TestPort,
		netsriovparameters.TestBondInterfaceName)
	Expect(err).ToNot(HaveOccurred())

	slaveNetworks := []string{netsriovparameters.SriovNetworkBondName,
		netsriovparameters.SriovNetworkBondNameDiff}

	clientPod := createTestPods(sriovInfos, slaveNetworks, bondActiveActiveParameters.MTU,
		netsriovparameters.IpamStatic, netsriovparameters.ConnectivityDiffNode, protocol, ipAddrServer, ipAddrClient)

	By("Checking that Bond interface is configured")
	isBondInterfaceConfigured(clientPod, bondMode)

	By(fmt.Sprintf("Checking traffic - %s", protocol))

	_, err = pod.ExecCommand(Apiclient, *clientPod, clientTestCommand)
	Expect(err).ToNot(HaveOccurred(), "Traffic failed")

	By("Disabling interface on the switch and check the traffic again")

	err = setSwitchInterfaceStatus(switchCredentials, switchInterfaces[0], switchcmd.SetAction)
	Expect(err).ToNot(HaveOccurred())

	_, err = pod.ExecCommand(Apiclient, *clientPod, clientTestCommand)
	Expect(err).ToNot(HaveOccurred(), "Traffic failed")

	By(fmt.Sprintf("Disabling secondary LAG slave interface %s, bring first LAG slave interface %s back"+
		" and check the traffic again", switchInterfaces[1], switchInterfaces[0]))

	err = setSwitchInterfaceStatus(switchCredentials, switchInterfaces[0], switchcmd.DeleteAction)
	Expect(err).ToNot(HaveOccurred())

	err = setSwitchInterfaceStatus(switchCredentials, switchInterfaces[1], switchcmd.SetAction)
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() bool {
		isBondInterfaceUp, err := isSwitchInterfaceUp(switchCredentials, switchLagNames[0])
		Expect(err).ToNot(HaveOccurred())

		return isBondInterfaceUp
	}, 1*time.Minute, 5*time.Second).Should(BeTrue(), "Bond interface is not Up on the switch")

	_, err = pod.ExecCommand(Apiclient, *clientPod, clientTestCommand)
	Expect(err).ToNot(HaveOccurred(), "Traffic failed")
}

// setInterfaceStatus enable or disable an interface on a pod using the IP command.
func setInterfaceStatus(clientPod *corev1.Pod, nic string, status string) error {
	_, err := pod.ExecCommand(Apiclient, *clientPod, []string{"ip", "link", "set", "dev", nic, status})

	return err
}

// findBondActiveInterface returns active interface in a bond.
func findBondActiveInterface(clientPod *corev1.Pod) (string, error) {
	activeInterface, err := pod.ExecCommand(Apiclient, *clientPod, []string{"cat",
		fmt.Sprintf("/sys/class/net/%s/bonding/active_slave", netsriovparameters.TestBondInterfaceName)})

	return strings.TrimSpace(activeInterface.String()), err
}

// DefineBondNad returns network attachment definition for a Bond interface.
func DefineBondNad(nadName string,
	bondType string,
	mtu int,
	numberSlaveInterfaces int, ipam string) (*netattdefv1.NetworkAttachmentDefinition, error) {
	slaveInterfaces := bondNADSlaveInterfaces(numberSlaveInterfaces)
	bondNad := &netattdefv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nadName,
			Namespace: netsriovparameters.OperatorTestNamespace,
		},
		Spec: netattdefv1.NetworkAttachmentDefinitionSpec{
			Config: fmt.Sprintf(
				`{"type": "bond", "cniVersion": "0.3.1", "name": "%s",
"mode": "%s", "failOverMac": 1, "linksInContainer": true, "miimon": "100", "mtu": %d,
"links": [%s], "capabilities": {"ips": true}, `,
				nadName, bondType, mtu, slaveInterfaces),
		}}

	switch ipam {
	case netsriovparameters.IpamStatic:
		bondNad.Spec.Config += fmt.Sprintf(`"ipam": {"type": "%s"}}`, ipam)
	case netsriovparameters.IpamWhereabouts:
		bondNad.Spec.Config += fmt.Sprintf(`"ipam": {"type": "%s", "range": "%s"}}`,
			ipam, netsriovparameters.WhereaboutsRangeIPv6)
	default:
		return nil, fmt.Errorf("wrong ipam type %s", ipam)
	}

	return bondNad, nil
}

// bondNADSlaveInterfaces returns string with slave interfaces for Bond interface Network Attachment Definition.
func bondNADSlaveInterfaces(numberInterfaces int) string {
	slaveInterfaces := `{"name": "net1"}`

	for i := 2; i <= numberInterfaces; i++ {
		slaveInterfaces += fmt.Sprintf(`,{"name": "net%d"}`, i)
	}

	return slaveInterfaces
}

// BondInterfaceIsUp verifies that the bond is Up.
func BondInterfaceIsUp(clientPod *corev1.Pod, bondInterfaceName string) (bool, error) {
	status, err := pod.ExecCommand(Apiclient, *clientPod, []string{"cat",
		fmt.Sprintf("/sys/class/net/%s/bonding/mii_status", bondInterfaceName)})
	if err != nil {
		return false, err
	}

	return strings.Contains(status.String(), "up"), nil
}

// BondInterfaceHasMode verifies that the bond has expected bond type.
func BondInterfaceHasMode(clientPod *corev1.Pod, bondInterfaceName string, bondMode string) (bool, error) {
	bondModeOut, err := pod.ExecCommand(Apiclient, *clientPod, []string{"cat",
		fmt.Sprintf("/sys/class/net/%s/bonding/mode", bondInterfaceName)})
	if err != nil {
		return false, err
	}

	return strings.Contains(bondModeOut.String(), bondMode), nil
}

// BondInterfaceHasSlaves verifies that the bond has expected number of slaves.
func BondInterfaceHasSlaves(clientPod *corev1.Pod, bondInterfaceName string, numSlaves string) (bool, error) {
	command := fmt.Sprintf("wc -w /sys/class/net/%s/bonding/slaves | awk '{print $1;}'", bondInterfaceName)
	numSlavesOut, err := pod.ExecCommand(Apiclient, *clientPod, []string{"bash", "-c", command})

	if err != nil {
		return false, err
	}

	return strings.Contains(numSlavesOut.String(), numSlaves), nil
}

// defineSriovBondNetwork builds SriovNetwork resource for Bond Interface.
func defineSriovBondNetwork(name string, resourceName string) *sriovv1.SriovNetwork {
	sriovNetwork := DefineSriovNetwork(name, resourceName)
	sriovNetwork.Spec.Trust = "on"
	sriovNetwork.Spec.SpoofChk = "off"
	sriovNetwork.Spec.LinkState = "auto"

	return sriovNetwork
}

func isBondInterfaceConfigured(testPod *corev1.Pod, bondMode string) {
	isBondInterfaceConfigured, err := BondInterfaceIsUp(testPod, netsriovparameters.TestBondInterfaceName)
	Expect(err).ToNot(HaveOccurred())
	Expect(isBondInterfaceConfigured).To(BeTrue(), "Bond interface is not Up")

	isBondInterfaceConfigured, err = BondInterfaceHasMode(testPod, netsriovparameters.TestBondInterfaceName, bondMode)
	Expect(err).ToNot(HaveOccurred())
	Expect(isBondInterfaceConfigured).To(BeTrue(), "Bond interface has incorrect bond type")

	isBondInterfaceConfigured, err = BondInterfaceHasSlaves(testPod, netsriovparameters.TestBondInterfaceName, "2")
	Expect(err).ToNot(HaveOccurred())
	Expect(isBondInterfaceConfigured).To(BeTrue(), "Bond interface has wrong number of slaves")
}

func createTestPods(sriovInfos *cluster.EnabledNodes, slaveNetworks []string, mtu int,
	ipam, connectivity, protocol, ipAddrServer, ipAddrClient string) *corev1.Pod {
	nodeSelector := defineNodeSelector(connectivity, sriovInfos)

	clientPodDefinition := DefineClientPod(
		protocol,
		nodeSelector,
		netsriovparameters.BondNadName,
		slaveNetworks,
		ipAddrClient,
		"",
		Config.Network.TestContainerImage,
		parameters.SleepCommand,
		ipam,
		netsriovparameters.TestBondInterfaceName)

	By("Creating Server Pod")
	RunServerPod(
		protocol,
		mtu,
		connectivity,
		sriovInfos,
		Config,
		netsriovparameters.BondNadName,
		slaveNetworks,
		false,
		"",
		ipAddrServer,
		netsriovparameters.TestBondInterfaceName,
		ipam)

	By("Creating Client Pod")

	clientPod, err := Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
		context.Background(),
		clientPodDefinition,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())

	WaitUntilPodInStatus(
		clientPod,
		"Client",
		parameters.SleepCommand,
		corev1.PodRunning,
		netsriovparameters.PodWaitingTime)

	return clientPod
}

func configureLAGsOnSwitch(switchCredentials *nethelper.SwitchCredentials, switchInterfaces []string,
	lagInterfaceNames []string) {
	err := setNonLACPLAGOnJunos(switchCredentials, []string{switchInterfaces[0], switchInterfaces[1]},
		lagInterfaceNames[0])
	Expect(err).ToNot(HaveOccurred())

	err = setNonLACPLAGOnJunos(switchCredentials, []string{switchInterfaces[2], switchInterfaces[3]},
		lagInterfaceNames[1])
	Expect(err).ToNot(HaveOccurred())

	err = configureMTUOnSwitchInterfaces(switchCredentials, []string{lagInterfaceNames[0], lagInterfaceNames[1]}, "9216")
	Expect(err).ToNot(HaveOccurred())
}

// Uncomment once BZ 2082360 is fixed
// // verifyPodAnnotation verifies that the testPod annotation includes the given IP and MAC.
// func verifyPodAnnotation(testPod *corev1.Pod, ipAddress string, mac string) error {
//	var err error
//
//	testPod, err = Apiclient.Pods(testPod.GetNamespace()).
//		Get(context.Background(), testPod.GetName(), metav1.GetOptions{})
//	if err != nil {
//		return err
//	}
//
//	networkStatus := testPod.Annotations["k8s.v1.cni.cncf.io/networks-status"]
//
//	if !strings.Contains(networkStatus, ipAddress) {
//		return fmt.Errorf("IP %s is not in pod annotation", ipAddress)
//	}
//
//	if mac != "" && !strings.Contains(networkStatus, mac) {
//		return fmt.Errorf("MAC %s is not in pod annotation", mac)
//	}
//
//	return nil
// }
