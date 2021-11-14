package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovparameters"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	sriovNetworkUsualMTUName       = "test-sriov-static-usual"
	sriovNetworkCustomMTUName      = "test-sriov-static-custom"
	sriovNetworkJumboFrameName     = "test-sriov-static-jumbo"
	sriovNetworkUsualMTUNameDiff   = "test-sriov-static-usual-diff"
	sriovNetworkCustomMTUNameDiff  = "test-sriov-static-custom-diff"
	sriovNetworkJumboFrameNameDiff = "test-sriov-static-jumbo-diff"
	clientPodIP                    = "192.168.100.1"
	clientPodIPv6                  = "2001:1db8:85a3::1"
	clientMacAddress               = "20:04:0f:f1:88:01"
	serverPodIP                    = "192.168.100.2"
	serverPodIpv6                  = "2001:1db8:85a3::2"
	serverMacAddress               = "20:04:0f:f1:88:03"
	testPort                       = 50000
	testInterfaceName              = "net1"
	multicastIPv6Address           = "FF05:0:0:0:0:0:0:18C"
	multicastIPAddress             = "224.255.0.10"
)

var (
	waitingTime          = 35 * time.Minute
	podWaitingTime       = 1 * time.Minute
	dualPodWaitingTime   = 3 * time.Minute
	isSingleNode         bool
	snoTimeoutMultiplier time.Duration = 1
	testFail                           = ""
)

var _ = Describe("CNF SRIOV", func() {
	describe := func(mtu int, protocol string, connectivity string) string {
		connectivityParameters, err := netsriovparameters.NewConnectivityTestParameters(mtu, connectivity, protocol)
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

	var sriovInfos *cluster.EnabledNodes
	config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())

	execute.BeforeAll(func() {
		isSingleNode, err = nodes.IsSingleNodeCluster(generalHelper.Apiclient)
		if err != nil {
			testFail = fmt.Sprintf("Error to check if the cluster is single node: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
		if isSingleNode {
			snoTimeoutMultiplier = 2
			disableDrainState := generalHelper.GetNodeDrainState(netsriovparameters.OperatorNamespace)
			if !disableDrainState {
				generalHelper.SetDisableNodeDrainState(true, netsriovparameters.OperatorNamespace)
				generalHelper.ChangedNodeDrainState = true
			}
		}

		if sriovSmokeTestMode {
			By("Run sriov tests in smoke mode")
		}
		By("Discover SRIOV interfaces")
		sriovInfos, err = cluster.DiscoverSriov(
			generalHelper.Apiclient,
			generalParameters.SriovOperatorNamespace)
		if err != nil {
			testFail = fmt.Sprintf("Error discover SRIOV node info: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
		sriovInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
		if err != nil {
			testFail = fmt.Sprintf("Error discover SRIOV interfaces: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}

		validSriovInterfaces, err := config.GetSriovInterfaces(sriovInterfaces, 2)
		if err != nil {
			testFail = fmt.Sprintf("Error determine SRIOV interfaces: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}

		By(fmt.Sprintf("Clean test namespace %s", netsriovparameters.OperatorTestNamespace))
		namespaces.Clean(
			generalParameters.SriovOperatorNamespace,
			netsriovparameters.OperatorTestNamespace,
			generalHelper.Apiclient,
			false)

		By("Waiting until SRIOV become stable")
		generalHelper.WaitForSRIOVStable(generalParameters.SriovOperatorNamespace, waitingTime, snoTimeoutMultiplier)

		By("Configuring SriovPolicy resources")
		err = namespaces.Create(netsriovparameters.OperatorTestNamespace, generalHelper.Apiclient)
		if err != nil {
			testFail = fmt.Sprintf("Error to create namespace %s: %s", netsriovparameters.OperatorNamespace, err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
		usualSriovPolicyConfig := generalHelper.DefineSriovPolicy(
			"test-policy-usual",
			generalParameters.SriovOperatorNamespace,
			validSriovInterfaces[0],
			6,
			"#0-1",
			1500,
			"testresourceusual",
			"netdevice")
		customSriovPolicyConfig := generalHelper.DefineSriovPolicy(
			"test-policy-custom",
			generalParameters.SriovOperatorNamespace,
			validSriovInterfaces[0],
			6,
			"#2-3",
			1450,
			"testresourcecustom",
			"netdevice")
		jumboSriovPolicyConfig := generalHelper.DefineSriovPolicy(
			"test-policy-jumbo",
			generalParameters.SriovOperatorNamespace,
			validSriovInterfaces[0],
			6,
			"#4-5",
			9000,
			"testresourcejumbo",
			"netdevice")
		usualSriovPolicyConfigDiffPF := generalHelper.DefineSriovPolicy(
			"test-policy-usual-diff",
			generalParameters.SriovOperatorNamespace,
			validSriovInterfaces[1],
			5,
			"#0-0",
			1500,
			"testresourceusualdiff",
			"netdevice")
		customSriovPolicyConfigDiffPF := generalHelper.DefineSriovPolicy(
			"test-policy-custom-diff",
			generalParameters.SriovOperatorNamespace,
			validSriovInterfaces[1],
			5,
			"#1-1",
			1450,
			"testresourcecustomdiff",
			"netdevice")
		jumboSriovPolicyConfigDiffPF := generalHelper.DefineSriovPolicy(
			"test-policy-jumbo-diff",
			generalParameters.SriovOperatorNamespace,
			validSriovInterfaces[1],
			5,
			"#2-2",
			9000,
			"testresourcejumbodiff",
			"netdevice")
		for _, networkPolicy := range []*sriovv1.SriovNetworkNodePolicy{
			usualSriovPolicyConfig,
			customSriovPolicyConfig,
			jumboSriovPolicyConfig,
			customSriovPolicyConfigDiffPF,
			jumboSriovPolicyConfigDiffPF,
			usualSriovPolicyConfigDiffPF} {
			err = generalHelper.Apiclient.Create(context.Background(), networkPolicy)
			if err != nil {
				testFail = fmt.Sprintf("Error to create SR-IOV networkPolicy %s: %s", networkPolicy, err)
				Expect(err).ToNot(HaveOccurred(), testFail)
			}
		}

		By("Configuring SriovNetwork resources")
		usualSriovNetworkConfig := netsriovhelper.DefineSriovNetwork(
			sriovNetworkUsualMTUName,
			usualSriovPolicyConfig.Spec.ResourceName,
			true)
		customSriovNetworkConfig := netsriovhelper.DefineSriovNetwork(
			sriovNetworkCustomMTUName,
			customSriovPolicyConfig.Spec.ResourceName,
			true)
		jumboSriovNetworkConfig := netsriovhelper.DefineSriovNetwork(
			sriovNetworkJumboFrameName,
			jumboSriovPolicyConfig.Spec.ResourceName,
			true)
		usualSriovNetworkConfigDiff := netsriovhelper.DefineSriovNetwork(
			sriovNetworkUsualMTUNameDiff,
			usualSriovPolicyConfigDiffPF.Spec.ResourceName,
			true)
		customSriovNetworkConfigDiff := netsriovhelper.DefineSriovNetwork(
			sriovNetworkCustomMTUNameDiff,
			customSriovPolicyConfigDiffPF.Spec.ResourceName,
			true)
		jumboSriovNetworkConfigDiff := netsriovhelper.DefineSriovNetwork(
			sriovNetworkJumboFrameNameDiff,
			jumboSriovPolicyConfigDiffPF.Spec.ResourceName,
			true)
		for _, network := range []*sriovv1.SriovNetwork{
			usualSriovNetworkConfig,
			customSriovNetworkConfig,
			jumboSriovNetworkConfig,
			customSriovNetworkConfigDiff,
			jumboSriovNetworkConfigDiff,
			usualSriovNetworkConfigDiff} {
			err = generalHelper.Apiclient.Create(context.Background(), network)
			if err != nil {
				testFail = fmt.Sprintf("Error to create SR-IOV network %s: %s", network, err)
				Expect(err).ToNot(HaveOccurred(), testFail)
			}
		}

		By("Waiting until SRIOV become stable")
		generalHelper.WaitForSRIOVStable(generalParameters.SriovOperatorNamespace, waitingTime, snoTimeoutMultiplier)

		By("Waiting until SRIOV resources become available")
		generalHelper.ValidateSriovVFsAvailableOnNodes(
			sriovInfos.Nodes, []*sriovv1.SriovNetworkNodePolicy{
				usualSriovPolicyConfig,
				customSriovPolicyConfig,
				jumboSriovPolicyConfig},
			2)
		generalHelper.ValidateSriovVFsAvailableOnNodes(
			sriovInfos.Nodes,
			[]*sriovv1.SriovNetworkNodePolicy{
				usualSriovPolicyConfigDiffPF,
				customSriovPolicyConfigDiffPF,
				jumboSriovPolicyConfigDiffPF},
			1)
	})

	BeforeEach(func() {
		if testFail != "" {
			Fail(testFail)
		}
		By("Cleaning up resources before test")
		err = namespaces.CleanPods(netsriovparameters.OperatorTestNamespace, generalHelper.Apiclient)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() bool {
			podsList, err := generalHelper.Apiclient.Pods(
				netsriovparameters.OperatorTestNamespace).List(context.Background(), metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred())
			if len(podsList.Items) > 0 {
				return false
			}
			return true

		}, 3*time.Minute, 10*time.Second).Should(BeTrue())
	})

	DescribeTable(
		"Ipam type: IP Static, Ip Stack: ipv4, Mac address: MAC static",
		func(mtu int, protocol string, connectivity string) {
			buildDescribeTable(mtu, protocol, connectivity, sriovInfos, config, clientMacAddress, serverMacAddress)
		},
		buildTableEntries(
			describe,
			[]int{netsriovparameters.MTUCustom,
				netsriovparameters.MTUJumbo,
				netsriovparameters.MTUStandart},
			[]string{netsriovparameters.ConnectivityDiffNode,
				netsriovparameters.ConnectivitySameNodeDiffPF,
				netsriovparameters.ConnectivitySameNodeSamePF},
			[]string{
				netsriovparameters.CommunicationProtocolUnicastICMP,
				netsriovparameters.CommunicationProtocolUnicastTCP,
				netsriovparameters.CommunicationProtocolUnicastUDP,
				netsriovparameters.CommunicationProtocolMulticastUDP,
				netsriovparameters.CommunicationProtocolBroadcastUDP,
				netsriovparameters.CommunicationProtocolUnicastSCTP,
			},
		)...,
	)

	DescribeTable(
		"Ipam type: IP Static, Ip Stack: ipv4, Mac address: MAC dynamic",
		func(mtu int, protocol string, connectivity string) {
			buildDescribeTable(mtu, protocol, connectivity, sriovInfos, config, "", "")
		},
		buildTableEntries(
			describe,
			[]int{netsriovparameters.MTUCustom,
				netsriovparameters.MTUJumbo,
				netsriovparameters.MTUStandart},
			[]string{netsriovparameters.ConnectivityDiffNode,
				netsriovparameters.ConnectivitySameNodeDiffPF,
				netsriovparameters.ConnectivitySameNodeSamePF},
			[]string{
				netsriovparameters.CommunicationProtocolUnicastICMP,
				netsriovparameters.CommunicationProtocolUnicastTCP,
				netsriovparameters.CommunicationProtocolUnicastUDP,
				netsriovparameters.CommunicationProtocolMulticastUDP,
				netsriovparameters.CommunicationProtocolBroadcastUDP,
				netsriovparameters.CommunicationProtocolUnicastSCTP,
			},
		)...,
	)

	DescribeTable(
		"Ipam type: IP Static, Ip Stack: ipv6, Mac address: MAC static",
		func(mtu int, protocol string, connectivity string) {
			buildDescribeTable6(mtu, protocol, connectivity, sriovInfos, config, clientMacAddress, serverMacAddress)
		},
		buildTableEntries(
			describe,
			[]int{netsriovparameters.MTUCustom,
				netsriovparameters.MTUJumbo,
				netsriovparameters.MTUStandart},
			[]string{netsriovparameters.ConnectivityDiffNode,
				netsriovparameters.ConnectivitySameNodeDiffPF,
				netsriovparameters.ConnectivitySameNodeSamePF},
			[]string{
				netsriovparameters.CommunicationProtocolUnicastICMP,
				netsriovparameters.CommunicationProtocolUnicastTCP,
				netsriovparameters.CommunicationProtocolUnicastUDP,
				netsriovparameters.CommunicationProtocolMulticastUDP,
				netsriovparameters.CommunicationProtocolUnicastSCTP,
			},
		)...,
	)

	DescribeTable(
		"Ipam type: IP Static, Ip Stack: ipv6, Mac address: MAC dynamic",
		func(mtu int, protocol string, connectivity string) {
			buildDescribeTable6(mtu, protocol, connectivity, sriovInfos, config, "", "")
		},
		buildTableEntries(
			describe,
			[]int{netsriovparameters.MTUCustom,
				netsriovparameters.MTUJumbo,
				netsriovparameters.MTUStandart},
			[]string{netsriovparameters.ConnectivityDiffNode,
				netsriovparameters.ConnectivitySameNodeDiffPF,
				netsriovparameters.ConnectivitySameNodeSamePF},
			[]string{
				netsriovparameters.CommunicationProtocolUnicastICMP,
				netsriovparameters.CommunicationProtocolUnicastTCP,
				netsriovparameters.CommunicationProtocolUnicastUDP,
				netsriovparameters.CommunicationProtocolMulticastUDP,
				netsriovparameters.CommunicationProtocolUnicastSCTP,
			},
		)...,
	)

	DescribeTable(
		"Ipam type: IP Static, Ip Stack: dual-stack, Mac address: MAC static",
		func(mtu int, protocol string, connectivity string) {
			buildDescribeTableDual(mtu, protocol, connectivity, sriovInfos, config, clientMacAddress, serverMacAddress)
		},
		buildTableEntries(
			describe,
			[]int{netsriovparameters.MTUCustom,
				netsriovparameters.MTUJumbo,
				netsriovparameters.MTUStandart},
			[]string{netsriovparameters.ConnectivityDiffNode,
				netsriovparameters.ConnectivitySameNodeDiffPF,
				netsriovparameters.ConnectivitySameNodeSamePF},
			[]string{
				netsriovparameters.CommunicationProtocolUnicastICMP,
				netsriovparameters.CommunicationProtocolUnicastTCP,
				netsriovparameters.CommunicationProtocolUnicastUDP,
				netsriovparameters.CommunicationProtocolMulticastUDP,
				netsriovparameters.CommunicationProtocolBroadcastUDP,
				netsriovparameters.CommunicationProtocolUnicastSCTP,
			},
		)...,
	)

	DescribeTable(
		"Ipam type: IP Static, Ip Stack: dual-stack, Mac address: MAC dynamic",
		func(mtu int, protocol string, connectivity string) {
			buildDescribeTableDual(mtu, protocol, connectivity, sriovInfos, config, "", "")
		},
		buildTableEntries(
			describe,
			[]int{netsriovparameters.MTUCustom,
				netsriovparameters.MTUJumbo,
				netsriovparameters.MTUStandart,
			},
			[]string{netsriovparameters.ConnectivityDiffNode,
				netsriovparameters.ConnectivitySameNodeDiffPF,
				netsriovparameters.ConnectivitySameNodeSamePF,
			},
			[]string{
				netsriovparameters.CommunicationProtocolUnicastICMP,
				netsriovparameters.CommunicationProtocolUnicastTCP,
				netsriovparameters.CommunicationProtocolUnicastUDP,
				netsriovparameters.CommunicationProtocolMulticastUDP,
				netsriovparameters.CommunicationProtocolBroadcastUDP,
				netsriovparameters.CommunicationProtocolUnicastSCTP,
			},
		)...,
	)
})

func runServerPod(
	protocol string,
	mtu int,
	connectivity string,
	sriovInfos *cluster.EnabledNodes,
	config *config.Config,
	networkName string,
	serverCommand []string,
	negative bool,
	serverMacAddress string,
	serverIP string) {

	nodeSelector := defineNodeSelector(connectivity, sriovInfos)

	if (protocol == netsriovparameters.CommunicationProtocolMulticastUDP ||
		protocol == netsriovparameters.CommunicationProtocolBroadcastUDP ||
		protocol == netsriovparameters.CommunicationProtocolUnicastSCTP) && negative == true {
		namespaces.CleanPods(netsriovparameters.OperatorTestNamespace, generalHelper.Apiclient)
	}

	serverCommand, err := serverCommandFor(protocol, mtu, serverIP, negative, testPort)
	Expect(err).ToNot(HaveOccurred())
	serverPodDefinition := defineServerPod(
		protocol,
		nodeSelector,
		networkName,
		serverIP,
		serverMacAddress,
		config.Network.TestContainerImage,
		serverCommand)

	serverPod, err := generalHelper.Apiclient.Pods(
		netsriovparameters.OperatorTestNamespace).Create(
		context.Background(),
		serverPodDefinition,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	waitUntilPodInStatus(
		serverPod,
		"Server",
		serverCommand,
		corev1.PodRunning, podWaitingTime)
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
	serverIPV6 string) {

	nodeSelector := defineNodeSelector(connectivity, sriovInfos)

	if (protocol == netsriovparameters.CommunicationProtocolMulticastUDP ||
		protocol == netsriovparameters.CommunicationProtocolBroadcastUDP ||
		protocol == netsriovparameters.CommunicationProtocolUnicastSCTP) && negative == true {
		namespaces.CleanPods(netsriovparameters.OperatorTestNamespace, generalHelper.Apiclient)
	}

	serverIPv4Command, err := serverCommandFor(protocol, mtu, serverIPV4, negative, testPort)
	Expect(err).ToNot(HaveOccurred())

	serverIPv6Command, err := serverCommandFor(protocol, mtu, serverIPV6, negative, testPort+1)
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

	serverPod, err := generalHelper.Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
		context.Background(), serverPodDefinition, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	waitUntilPodInStatus(
		serverPod,
		"Server",
		append(serverIPv4Command, serverIPv6Command...),
		corev1.PodRunning,
		dualPodWaitingTime)
}

func serverCommandFor(
	testProtocol string,
	mtu int,
	serverIP string,
	negative bool,
	testPort int) ([]string, error) {

	testCommand := []string{}
	switch testProtocol {

	case netsriovparameters.CommunicationProtocolUnicastICMP:
		testCommand = []string{"sleep", "INF"}

	case netsriovparameters.CommunicationProtocolUnicastTCP:
		testCommand = []string{
			"testcmd",
			"--listen",
			fmt.Sprintf("-interface=%s", testInterfaceName),
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
			fmt.Sprintf("-interface=%s", testInterfaceName),
			fmt.Sprintf("-server=%s", serverIP)}

	case netsriovparameters.CommunicationProtocolUnicastUDP:
		testCommand = []string{
			"testcmd",
			"-listen",
			"-protocol=udp",
			fmt.Sprintf("-port=%d", testPort),
			fmt.Sprintf("-mtu=%d", mtu)}

	case netsriovparameters.CommunicationProtocolMulticastUDP:
		multicastAddress := multicastIPAddress
		if strings.Contains(serverIP, ":") {
			multicastAddress = multicastIPv6Address
		}
		testCommand = []string{
			"testcmd",
			"-listen",
			"-protocol=udp",
			fmt.Sprintf("-port=%d", testPort),
			"-multicast",
			fmt.Sprintf("-interface=%s", testInterfaceName),
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
					fmt.Sprintf("-interface=%s", testInterfaceName),
					fmt.Sprintf("-mtu=%d", mtu+100)}
			} else {
				testCommand = []string{
					"testcmd",
					"-listen",
					"-protocol=udp",
					"-port=50000",
					"-broadcast",
					fmt.Sprintf("-interface=%s", testInterfaceName),
					fmt.Sprintf("-mtu=%d", mtu)}
			}
		} else {
			testCommand = []string{
				"testcmd",
				"-listen",
				"-protocol=udp",
				"-port=50000",
				"-broadcast",
				fmt.Sprintf("-interface=%s", testInterfaceName),
				fmt.Sprintf("-mtu=%d", mtu-40)}
		}
	default:
		return nil, fmt.Errorf(netsriovparameters.SriovErrorProtocolMessage, testProtocol)
	}
	return testCommand, nil
}

func defineTestCommandParameters(
	negative bool,
	protocol string,
	mtu int,
	connectivity string,
	serverIP string,
	testPort int) ([]string, error) {

	testCommand := []string{"testcmd"}
	var protocolOption string
	protocolVersion := 4
	if strings.Contains(serverIP, ":") {
		protocolVersion = 6
	}
	switch protocol {
	case netsriovparameters.CommunicationProtocolUnicastICMP:
		protocolOption = "icmp"
	case netsriovparameters.CommunicationProtocolUnicastTCP:
		protocolOption = "tcp"
		testCommand = append(testCommand, fmt.Sprintf("-port=%d", testPort), fmt.Sprintf("-interface=%s", testInterfaceName))
	case netsriovparameters.CommunicationProtocolUnicastSCTP:
		protocolOption = "sctp"
		testCommand = append(testCommand, fmt.Sprintf("-server=%s", serverIP),
			fmt.Sprintf("-port=%d", testPort), fmt.Sprintf("-interface=%s", testInterfaceName))
	case netsriovparameters.CommunicationProtocolUnicastUDP:
		protocolOption = "udp"
		testCommand = append(testCommand, fmt.Sprintf("-port=%d", testPort))
	case netsriovparameters.CommunicationProtocolMulticastUDP:
		protocolOption = "udp"
		serverIP = multicastIPAddress
		if protocolVersion == 6 {
			serverIP = multicastIPv6Address
		}
		testCommand = append(
			testCommand,
			fmt.Sprintf("-port=%d", testPort),
			"-multicast",
			fmt.Sprintf("-interface=%s", testInterfaceName))
	case netsriovparameters.CommunicationProtocolBroadcastUDP:
		protocolOption = "udp"
		serverIP = "255.255.255.255"
		testCommand = append(
			testCommand,
			fmt.Sprintf("-port=%d", testPort),
			"-broadcast",
			fmt.Sprintf("-interface=%s", testInterfaceName))
	default:
		return nil, fmt.Errorf(netsriovparameters.SriovErrorProtocolMessage, protocol)
	}
	switch {
	case negative:
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
	switch connectivity {
	case netsriovparameters.ConnectivitySameNodeDiffPF:
		if mtu == netsriovparameters.MTUJumbo {
			networkName = sriovNetworkJumboFrameNameDiff
		} else if mtu == netsriovparameters.MTUStandart {
			networkName = sriovNetworkUsualMTUNameDiff
		} else if mtu == netsriovparameters.MTUCustom {
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
	if mtu == netsriovparameters.MTUJumbo {
		networkName = sriovNetworkJumboFrameName
	} else if mtu == netsriovparameters.MTUStandart {
		networkName = sriovNetworkUsualMTUName
	} else if mtu == netsriovparameters.MTUCustom {
		networkName = sriovNetworkCustomMTUName
	} else {
		Skip(fmt.Sprintf("Unsupported test parameter mtu: %d", mtu))
	}
	return networkName
}

func defineNodeSelector(connectivity string, sriovInfos *cluster.EnabledNodes) []string {
	var nodeSelector []string
	if connectivity == netsriovparameters.ConnectivityDiffNode {
		if len(sriovInfos.Nodes) < 2 {
			Skip("Nodes number less that 2")
		}
		nodeSelector = sriovInfos.Nodes
	} else if connectivity == netsriovparameters.ConnectivitySameNodeSamePF ||
		connectivity == netsriovparameters.ConnectivitySameNodeDiffPF {
		nodeSelector = append(nodeSelector, sriovInfos.Nodes[0])
	} else {
		Skip(fmt.Sprintf("Unsupported test parameter Connectivity: %s", connectivity))
	}
	return nodeSelector
}

func defineServerPod(
	protocol string,
	nodeSelector []string,
	networkName string,
	ipaddress string,
	macAddress string,
	podImage string,
	podCommand []string) *corev1.Pod {

	podDefinition := pod.RedefineWithRestartPolicy(
		pod.DefineWithNodeNetworks(
			nodeSelector[0],
			[]string{networkName},
			netsriovparameters.OperatorTestNamespace, podImage),
		corev1.RestartPolicyNever)

	if serverNeedsPrivilege(protocol) {
		podDefinition = pod.RedefineAsPrivileged(podDefinition)
	}

	podDefinition = netsriovhelper.DefinePodCommandWithIpamAndMac(podDefinition, networkName, ipaddress, macAddress, podCommand)

	if strings.Contains(ipaddress, ":") {
		podDefinition = redefinePodWithInitCommandPolicy(podDefinition, podImage,
			fmt.Sprintf(
				"ping6 -c 3 -w 30 %s && ip -6 route add %s/128 dev net1 table local",
				serverPodIpv6, multicastIPv6Address))
	}

	return redefinePodWithInitDebugCommands(podDefinition, podImage)
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

	podDefinition = netsriovhelper.DefinePodCommandsWithDualIpamAndMac(
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
			serverPodIpv6, multicastIPv6Address))

	return redefinePodWithInitDebugCommands(podDefinition, podImage)
}

func defineClientPod(
	protocol string,
	nodeSelector []string,
	networkName string,
	ipaddress string,
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
			pod.DefineWithNodeNetworks(
				nodeSelector[0],
				[]string{networkName},
				netsriovparameters.OperatorTestNamespace, podImage),
			corev1.RestartPolicyNever)
	}

	if clientNeedsPrivilege(protocol) {
		podDefinition = pod.RedefineAsPrivileged(podDefinition)
	}

	podDefinition = netsriovhelper.DefinePodCommandWithIpamAndMac(podDefinition, networkName, ipaddress, macAddress, podCommand)
	serverIP := serverPodIP
	if strings.Contains(ipaddress, ":") {
		serverIP = serverPodIpv6
	}
	return redefinePodWithInitCommandPolicy(
		podDefinition,
		podImage,
		fmt.Sprintf("ping %s -c 3 -w 30", serverIP))

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

	podDefinition = netsriovhelper.DefinePodCommandsWithDualIpamAndMac(
		podDefinition,
		networkName,
		ip4address,
		ip6address,
		macAddress,
		podCommand)

	return redefinePodWithInitDebugCommands(podDefinition, podImage)
}

func buildTableEntries(
	describe interface{},
	mtuParameters []int,
	connectivityParameters []string,
	protocolParameters []string) []TableEntry {

	var tableEntries []TableEntry
	if sriovSmokeTestMode {
		var protocolIndex int
		var mtuIndex int
		var connectivityIndex int
		lenghtOfParametersArrays := []int{
			len(mtuParameters),
			len(connectivityParameters),
			len(protocolParameters),
		}
		max := lenghtOfParametersArrays[0]
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
						Entry(describe, mtu, protocol, connectivity))
				}
			}
		}
	}
	return tableEntries
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

func waitUntilPodInStatus(
	createdPod *corev1.Pod,
	podRole string,
	execCommand []string,
	podStatus corev1.PodPhase,
	waitingTime time.Duration) {

	Eventually(func() corev1.PodPhase {
		createdPod, _ = generalHelper.Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Get(
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

func buildDescribeTable(
	mtu int,
	protocol string,
	connectivity string,
	sriovInfos *cluster.EnabledNodes,
	config *config.Config,
	clientMacAddress string,
	serverMacAddress string) {

	By("Validating test parameters")
	connectivityParameters, err := netsriovparameters.NewConnectivityTestParameters(mtu, connectivity, protocol)
	Expect(err).ToNot(HaveOccurred())

	By("Defining test resources")
	nodeSelector := defineNodeSelector(connectivity, sriovInfos)
	serverNetworkName := defineServerNetworkName(mtu)
	clientNetworkName := defineClientNetworkName(mtu, connectivityParameters.Connectivity)
	negativeFlag := false
	clientTestCommand, err := defineTestCommandParameters(
		negativeFlag,
		connectivityParameters.Protocol,
		connectivityParameters.MTU,
		connectivityParameters.Connectivity,
		serverPodIP,
		testPort)
	Expect(err).ToNot(HaveOccurred())

	clientPodDefinition := defineClientPod(
		connectivityParameters.Protocol,
		nodeSelector,
		clientNetworkName,
		clientPodIP,
		clientMacAddress,
		config.Network.TestContainerImage,
		clientTestCommand)

	By("Creating Server Pod")
	runServerPod(
		protocol,
		connectivityParameters.MTU,
		connectivityParameters.Connectivity,
		sriovInfos,
		config,
		serverNetworkName,
		nodeSelector,
		negativeFlag,
		serverMacAddress,
		serverPodIP)

	By("Creating Client Pod")
	clientPod, err := generalHelper.Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
		context.Background(),
		clientPodDefinition,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	waitUntilPodInStatus(clientPod, "Client", clientTestCommand, corev1.PodSucceeded, podWaitingTime)

	if protocol == netsriovparameters.CommunicationProtocolUnicastTCP {
		By("Positive test flow - success")
		return
	}
	err = pod.DeletePodAndWait(generalHelper.Apiclient, clientPod)
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
		runServerPod(
			protocol,
			connectivityParameters.MTU,
			connectivityParameters.Connectivity,
			sriovInfos,
			config,
			serverNetworkName,
			nodeSelector,
			negativeFlag,
			serverMacAddress,
			serverPodIP)
	}
	clientTestCommand, err = defineTestCommandParameters(
		negativeFlag,
		connectivityParameters.Protocol,
		connectivityParameters.MTU,
		connectivityParameters.Connectivity,
		serverPodIP,
		testPort)
	Expect(err).ToNot(HaveOccurred())

	By("Creating Client Pod with negative flag")
	clientPodDefinitionNegative := defineClientPod(
		connectivityParameters.Protocol,
		nodeSelector,
		clientNetworkName,
		clientPodIP,
		clientMacAddress,
		config.Network.TestContainerImage,
		clientTestCommand)
	clientPodNegative, err := generalHelper.Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
		context.Background(),
		clientPodDefinitionNegative,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	waitUntilPodInStatus(
		clientPodNegative,
		"Client",
		clientTestCommand,
		corev1.PodSucceeded,
		podWaitingTime)
}

func buildDescribeTable6(
	mtu int,
	protocol string,
	connectivity string,
	sriovInfos *cluster.EnabledNodes,
	config *config.Config,
	clientMacAddress string,
	serverMacAddress string) {

	By("Validating test parameters")
	connectivityParameters, err := netsriovparameters.NewConnectivityTestParameters(mtu, connectivity, protocol)
	Expect(err).ToNot(HaveOccurred())
	By("Defining test resources")
	nodeSelector := defineNodeSelector(connectivity, sriovInfos)
	serverNetworkName := defineServerNetworkName(mtu)
	clientNetworkName := defineClientNetworkName(mtu, connectivityParameters.Connectivity)
	negativeFlag := false
	clientTestCommand, err := defineTestCommandParameters(
		negativeFlag,
		connectivityParameters.Protocol,
		connectivityParameters.MTU,
		connectivityParameters.Connectivity,
		serverPodIpv6,
		testPort)
	Expect(err).ToNot(HaveOccurred())
	clientPodDefinition := defineClientPod(
		connectivityParameters.Protocol,
		nodeSelector,
		clientNetworkName,
		clientPodIPv6,
		clientMacAddress,
		config.Network.TestContainerImage,
		clientTestCommand)

	By("Creating Server Pod")
	runServerPod(
		protocol,
		connectivityParameters.MTU,
		connectivityParameters.Connectivity,
		sriovInfos,
		config,
		serverNetworkName,
		nodeSelector,
		negativeFlag,
		serverMacAddress,
		serverPodIpv6)

	By("Creating Client Pod")
	clientPod, err := generalHelper.Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
		context.Background(),
		clientPodDefinition,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	waitUntilPodInStatus(
		clientPod,
		"Client",
		clientTestCommand,
		corev1.PodSucceeded,
		podWaitingTime)

	if protocol == netsriovparameters.CommunicationProtocolUnicastTCP {
		By("Positive test flow - success")
		return
	}
	err = pod.DeletePodAndWait(generalHelper.Apiclient, clientPod)
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
		runServerPod(
			protocol,
			connectivityParameters.MTU,
			connectivityParameters.Connectivity,
			sriovInfos,
			config,
			serverNetworkName,
			nodeSelector,
			negativeFlag,
			serverMacAddress,
			serverPodIpv6)
	}

	clientTestCommand, err = defineTestCommandParameters(
		negativeFlag,
		connectivityParameters.Protocol,
		connectivityParameters.MTU,
		connectivityParameters.Connectivity,
		serverPodIpv6,
		testPort)
	Expect(err).ToNot(HaveOccurred())

	By("Creating Client Pod with negative flag")
	clientPodDefinitionNegative := defineClientPod(
		connectivityParameters.Protocol,
		nodeSelector,
		clientNetworkName,
		clientPodIPv6,
		clientMacAddress,
		config.Network.TestContainerImage,
		clientTestCommand)
	clientPodNegative, err := generalHelper.Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
		context.Background(),
		clientPodDefinitionNegative,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	waitUntilPodInStatus(
		clientPodNegative,
		"Client",
		clientTestCommand,
		corev1.PodSucceeded,
		podWaitingTime)
}

func buildDescribeTableDual(
	mtu int,
	protocol string,
	connectivity string,
	sriovInfos *cluster.EnabledNodes,
	config *config.Config,
	clientMacAddress string,
	serverMacAddress string) {

	By("Validating test parameters")
	connectivityParameters, err := netsriovparameters.NewConnectivityTestParameters(mtu, connectivity, protocol)
	Expect(err).ToNot(HaveOccurred())
	By("Defining test resources")
	nodeSelector := defineNodeSelector(connectivity, sriovInfos)
	serverNetworkName := defineServerNetworkName(mtu)
	clientNetworkName := defineClientNetworkName(mtu, connectivityParameters.Connectivity)
	negativeFlag := false

	ipv4ClientTestCommand, err := defineTestCommandParameters(
		negativeFlag,
		connectivityParameters.Protocol,
		connectivityParameters.MTU,
		connectivityParameters.Connectivity,
		serverPodIP,
		testPort)
	Expect(err).ToNot(HaveOccurred())

	// IPv6 can't be used for udp-broadcast
	var ipv6ClientTestCommand []string
	if protocol == netsriovparameters.CommunicationProtocolBroadcastUDP {
		ipv6ClientTestCommand = []string{"exit 0"}
	} else {
		ipv6ClientTestCommand, err = defineTestCommandParameters(
			negativeFlag,
			connectivityParameters.Protocol,
			connectivityParameters.MTU,
			connectivityParameters.Connectivity,
			serverPodIpv6,
			testPort+1)
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
		clientPodIP,
		clientPodIPv6,
		clientMacAddress,
		config.Network.TestContainerImage,
		ClientTestCommand,
	)

	By("Creating Server Pod")
	runDualServerPod(
		protocol,
		connectivityParameters.MTU,
		connectivityParameters.Connectivity,
		sriovInfos,
		config,
		serverNetworkName,
		negativeFlag,
		serverMacAddress,
		serverPodIP,
		serverPodIpv6,
	)

	By("Creating Client Pod")
	clientPod, err := generalHelper.Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
		context.Background(),
		clientPodDefinition,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	waitUntilPodInStatus(clientPod, "Client", ClientTestCommand, corev1.PodSucceeded, dualPodWaitingTime)

	if protocol == netsriovparameters.CommunicationProtocolUnicastTCP {
		By("Positive test flow - success")
		return
	}
	err = pod.DeletePodAndWait(generalHelper.Apiclient, clientPod)
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
		runDualServerPod(
			protocol,
			connectivityParameters.MTU,
			connectivityParameters.Connectivity,
			sriovInfos,
			config,
			serverNetworkName,
			negativeFlag,
			serverMacAddress,
			serverPodIP,
			serverPodIpv6,
		)
	}

	By("Creating Client Pod with negative flag")
	ipv4ClientTestCommand, err = defineTestCommandParameters(
		negativeFlag,
		connectivityParameters.Protocol,
		connectivityParameters.MTU,
		connectivityParameters.Connectivity,
		serverPodIP,
		testPort)
	Expect(err).ToNot(HaveOccurred())

	ipv6ClientTestCommand, err = defineTestCommandParameters(
		negativeFlag,
		connectivityParameters.Protocol,
		connectivityParameters.MTU,
		connectivityParameters.Connectivity,
		serverPodIpv6,
		testPort+1)
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
		clientPodIP,
		clientPodIPv6,
		clientMacAddress,
		config.Network.TestContainerImage,
		ClientNegativeTestCommand,
	)

	clientPodNegative, err := generalHelper.Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
		context.Background(),
		clientPodDefinitionNegative,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	waitUntilPodInStatus(
		clientPodNegative,
		"Client",
		ClientTestCommand,
		corev1.PodSucceeded,
		dualPodWaitingTime)
}
