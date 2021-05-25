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
	networkHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/helper"
	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/parameters"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
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
	waitingTime                time.Duration = 35 * time.Minute
	podWaitingTime             time.Duration = 1 * time.Minute
	dualPodWaitingTime         time.Duration = 3 * time.Minute
	podDeafultInterfaceIPStack int
)

var _ = Describe("CNF SRIOV", func() {
	describe := func(mtu int, protocol string, connectivity string) string {
		connectivityParameters, err := parameters.NewConnectivityTestParameters(mtu, connectivity, protocol)
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
		if sriovSmokeTestMode {
			By("Run sriov tests in smoke mode")
		}
		By("Discover POD default interface ip stack versions")
		podDeafultInterfaceIPStack = networkHelper.GetPodIPStacks(
			config,
			generalHelper.Apiclient)
		By("Discover SRIOV interfaces")
		sriovInfos, err = cluster.DiscoverSriov(
			generalHelper.Apiclient,
			generalParameters.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred())
		sriovInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
		Expect(err).ToNot(HaveOccurred())

		validSriovInterfaces, err := config.GetSriovInterfaces(sriovInterfaces, 2)
		Expect(err).ToNot(HaveOccurred())

		By(fmt.Sprintf("Clean test namespace %s", parameters.OperatorTestNamespace))
		namespaces.Clean(
			generalParameters.SriovOperatorNamespace,
			parameters.OperatorTestNamespace,
			generalHelper.Apiclient,
			false)

		By("Waiting until SRIOV become stable")
		generalHelper.WaitForSRIOVStable(generalParameters.SriovOperatorNamespace, waitingTime)
		Expect(err).ToNot(HaveOccurred())

		By("Configuring SriovPolicy resources")
		err = namespaces.Create(parameters.OperatorTestNamespace, generalHelper.Apiclient)
		Expect(err).ToNot(HaveOccurred())
		usualSriovPolicyConfig := generalHelper.DefineSriovPolicy(
			"test-policy-usual",
			generalParameters.SriovOperatorNamespace,
			validSriovInterfaces[0],
			5,
			"#0-1",
			1500,
			"testresourceusual",
			"netdevice")
		customSriovPolicyConfig := generalHelper.DefineSriovPolicy(
			"test-policy-custom",
			generalParameters.SriovOperatorNamespace,
			validSriovInterfaces[0],
			5,
			"#2-3",
			1450,
			"testresourcecustom",
			"netdevice")
		// TODO: Change policy jumboSriovPolicyConfig to
		// jumboSriovPolicyConfig := DefineSriovPolicy(
		//"test-policy-jumbo",
		//validSriovInterfaces[0],
		//5,
		//"#0-1",
		//9000,
		//"testresourcejumbo",
		//"netdevice")
		// when bug https://bugzilla.redhat.com/show_bug.cgi?id=1926279 will be fixed
		jumboSriovPolicyConfig := generalHelper.DefineSriovPolicy(
			"test-policy-jumbo",
			generalParameters.SriovOperatorNamespace,
			validSriovInterfaces[1],
			5,
			"#0-1",
			9000,
			"testresourcejumbo",
			"netdevice")
		usualSriovPolicyConfigDiffPF := generalHelper.DefineSriovPolicy(
			"test-policy-usual-diff",
			generalParameters.SriovOperatorNamespace,
			validSriovInterfaces[1],
			5,
			"#2-2",
			1500,
			"testresourceusualdiff",
			"netdevice")
		customSriovPolicyConfigDiffPF := generalHelper.DefineSriovPolicy(
			"test-policy-custom-diff",
			generalParameters.SriovOperatorNamespace,
			validSriovInterfaces[1],
			5,
			"#3-3",
			1450,
			"testresourcecustomdiff",
			"netdevice")
		jumboSriovPolicyConfigDiffPF := generalHelper.DefineSriovPolicy(
			"test-policy-jumbo-diff",
			generalParameters.SriovOperatorNamespace,
			validSriovInterfaces[1],
			5,
			"#4-4",
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
			Expect(err).ToNot(HaveOccurred())
		}

		By("Configuring SriovNetwork resources")
		usualSriovNetworkConfig := DefineSriovNetwork(
			sriovNetworkUsualMTUName,
			usualSriovPolicyConfig.Spec.ResourceName,
			true)
		customSriovNetworkConfig := DefineSriovNetwork(
			sriovNetworkCustomMTUName,
			customSriovPolicyConfig.Spec.ResourceName,
			true)
		jumboSriovNetworkConfig := DefineSriovNetwork(
			sriovNetworkJumboFrameName,
			jumboSriovPolicyConfig.Spec.ResourceName,
			true)
		usualSriovNetworkConfigDiff := DefineSriovNetwork(
			sriovNetworkUsualMTUNameDiff,
			usualSriovPolicyConfigDiffPF.Spec.ResourceName,
			true)
		customSriovNetworkConfigDiff := DefineSriovNetwork(
			sriovNetworkCustomMTUNameDiff,
			customSriovPolicyConfigDiffPF.Spec.ResourceName,
			true)
		jumboSriovNetworkConfigDiff := DefineSriovNetwork(
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
			Expect(err).ToNot(HaveOccurred())
		}

		By("Waiting until SRIOV become stable")
		generalHelper.WaitForSRIOVStable(generalParameters.SriovOperatorNamespace, waitingTime)

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
		By("Cleaning up resources before test")
		err = namespaces.CleanPods(parameters.OperatorTestNamespace, generalHelper.Apiclient)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() bool {
			podsList, err := generalHelper.Apiclient.Pods(
				parameters.OperatorTestNamespace).List(context.Background(), metav1.ListOptions{})
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
			[]int{parameters.MTUCustom,
				parameters.MTUJumbo,
				parameters.MTUStandart},
			[]string{parameters.ConnectivityDiffNode,
				parameters.ConnectivitySameNodeDiffPF,
				parameters.ConnectivitySameNodeSamePF},
			[]string{
				parameters.CommunicationProtocolUnicastICMP,
				parameters.CommunicationProtocolUnicastTCP,
				parameters.CommunicationProtocolUnicastUDP,
				parameters.CommunicationProtocolMulticastUDP,
				parameters.CommunicationProtocolBroadcastUDP,
				parameters.CommunicationProtocolUnicastSCTP,
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
			[]int{parameters.MTUCustom,
				parameters.MTUJumbo,
				parameters.MTUStandart},
			[]string{parameters.ConnectivityDiffNode,
				parameters.ConnectivitySameNodeDiffPF,
				parameters.ConnectivitySameNodeSamePF},
			[]string{
				parameters.CommunicationProtocolUnicastICMP,
				parameters.CommunicationProtocolUnicastTCP,
				parameters.CommunicationProtocolUnicastUDP,
				parameters.CommunicationProtocolMulticastUDP,
				parameters.CommunicationProtocolBroadcastUDP,
				parameters.CommunicationProtocolUnicastSCTP,
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
			[]int{parameters.MTUCustom,
				parameters.MTUJumbo,
				parameters.MTUStandart},
			[]string{parameters.ConnectivityDiffNode,
				parameters.ConnectivitySameNodeDiffPF,
				parameters.ConnectivitySameNodeSamePF},
			[]string{
				parameters.CommunicationProtocolUnicastICMP,
				parameters.CommunicationProtocolUnicastTCP,
				parameters.CommunicationProtocolUnicastUDP,
				parameters.CommunicationProtocolMulticastUDP,
				parameters.CommunicationProtocolUnicastSCTP,
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
			[]int{parameters.MTUCustom,
				parameters.MTUJumbo,
				parameters.MTUStandart},
			[]string{parameters.ConnectivityDiffNode,
				parameters.ConnectivitySameNodeDiffPF,
				parameters.ConnectivitySameNodeSamePF},
			[]string{
				parameters.CommunicationProtocolUnicastICMP,
				parameters.CommunicationProtocolUnicastTCP,
				parameters.CommunicationProtocolUnicastUDP,
				parameters.CommunicationProtocolMulticastUDP,
				parameters.CommunicationProtocolUnicastSCTP,
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
			[]int{parameters.MTUCustom,
				parameters.MTUJumbo,
				parameters.MTUStandart},
			[]string{parameters.ConnectivityDiffNode,
				parameters.ConnectivitySameNodeDiffPF,
				parameters.ConnectivitySameNodeSamePF},
			[]string{
				parameters.CommunicationProtocolUnicastICMP,
				parameters.CommunicationProtocolUnicastTCP,
				parameters.CommunicationProtocolUnicastUDP,
				parameters.CommunicationProtocolMulticastUDP,
				parameters.CommunicationProtocolBroadcastUDP,
				parameters.CommunicationProtocolUnicastSCTP,
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
			[]int{parameters.MTUCustom,
				parameters.MTUJumbo,
				parameters.MTUStandart,
			},
			[]string{parameters.ConnectivityDiffNode,
				parameters.ConnectivitySameNodeDiffPF,
				parameters.ConnectivitySameNodeSamePF,
			},

			[]string{
				parameters.CommunicationProtocolUnicastICMP,
				parameters.CommunicationProtocolUnicastTCP,
				parameters.CommunicationProtocolUnicastUDP,
				parameters.CommunicationProtocolMulticastUDP,
				parameters.CommunicationProtocolBroadcastUDP,
				parameters.CommunicationProtocolUnicastSCTP,
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

	if (protocol == parameters.CommunicationProtocolMulticastUDP ||
		protocol == parameters.CommunicationProtocolBroadcastUDP ||
		protocol == parameters.CommunicationProtocolUnicastSCTP) && negative == true {
		namespaces.CleanPods(parameters.OperatorTestNamespace, generalHelper.Apiclient)
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
		parameters.OperatorTestNamespace).Create(
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

	if (protocol == parameters.CommunicationProtocolMulticastUDP ||
		protocol == parameters.CommunicationProtocolBroadcastUDP ||
		protocol == parameters.CommunicationProtocolUnicastSCTP) && negative == true {
		namespaces.CleanPods(parameters.OperatorTestNamespace, generalHelper.Apiclient)
	}

	serverIPv4Command, err := serverCommandFor(protocol, mtu, serverIPV4, negative, testPort)
	Expect(err).ToNot(HaveOccurred())

	serverIPv6Command, err := serverCommandFor(protocol, mtu, serverIPV6, negative, testPort+1)
	Expect(err).ToNot(HaveOccurred())

	// IPv6 can't be used for udp-broadcast. tcp-unicast running for ipv4 and ipv6 by default
	if protocol == parameters.CommunicationProtocolBroadcastUDP ||
		protocol == parameters.CommunicationProtocolUnicastTCP {
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

	serverPod, err := generalHelper.Apiclient.Pods(parameters.OperatorTestNamespace).Create(
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

	case parameters.CommunicationProtocolUnicastICMP:
		testCommand = []string{"sleep", "INF"}

	case parameters.CommunicationProtocolUnicastTCP:
		testCommand = []string{"httpd", "-X"}

	case parameters.CommunicationProtocolUnicastSCTP:
		testCommand = []string{
			"testcmd",
			"-protocol=sctp",
			"-listen",
			fmt.Sprintf("-port=%d", testPort),
			fmt.Sprintf("-interface=%s", testInterfaceName),
			fmt.Sprintf("-server=%s", serverIP)}

	case parameters.CommunicationProtocolUnicastUDP:
		testCommand = []string{
			"testcmd",
			"-listen",
			"-protocol=udp",
			fmt.Sprintf("-port=%d", testPort),
			fmt.Sprintf("-mtu=%d", mtu)}

	case parameters.CommunicationProtocolMulticastUDP:
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
			if mtu < parameters.MTUJumbo {
				// TODO: change mtu+100 to mtu+50 once https://bugzilla.redhat.com/show_bug.cgi?id=1926279 will be fixed
				testCommand = append(testCommand, fmt.Sprintf("-mtu=%d", mtu+100))
			} else {
				testCommand = append(testCommand, fmt.Sprintf("-mtu=%d", mtu))
			}
		} else {
			// TODO: change mtu+100 to mtu+50 once https://bugzilla.redhat.com/show_bug.cgi?id=1926279 will be fixed
			testCommand = append(testCommand, fmt.Sprintf("-mtu=%d", mtu-100))
		}
	case parameters.CommunicationProtocolBroadcastUDP:
		if negative {
			if mtu < parameters.MTUJumbo {
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
		return nil, fmt.Errorf(fmt.Sprint(parameters.SriovErrorProtocolMessage, testProtocol))
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
	case parameters.CommunicationProtocolUnicastICMP:
		protocolOption = "icmp"
	case parameters.CommunicationProtocolUnicastTCP:
		protocolOption = "tcp"
		testCommand = append(testCommand, fmt.Sprintf("-port=%d", 80), fmt.Sprintf("-interface=%s", testInterfaceName))
	case parameters.CommunicationProtocolUnicastSCTP:
		protocolOption = "sctp"
		testCommand = append(testCommand, fmt.Sprintf("-server=%s", serverIP),
			fmt.Sprintf("-port=%d", testPort), fmt.Sprintf("-interface=%s", testInterfaceName))
	case parameters.CommunicationProtocolUnicastUDP:
		protocolOption = "udp"
		testCommand = append(testCommand, fmt.Sprintf("-port=%d", testPort))
	case parameters.CommunicationProtocolMulticastUDP:
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
	case parameters.CommunicationProtocolBroadcastUDP:
		protocolOption = "udp"
		serverIP = "255.255.255.255"
		testCommand = append(
			testCommand,
			fmt.Sprintf("-port=%d", testPort),
			"-broadcast",
			fmt.Sprintf("-interface=%s", testInterfaceName))
	default:
		return nil, fmt.Errorf(fmt.Sprint(parameters.SriovErrorProtocolMessage, protocol))
	}
	switch {
	case negative:
		var parameterMtu string
		if mtu < parameters.MTUJumbo {
			// TODO: change mtu+100 to mtu+50 once https://bugzilla.redhat.com/show_bug.cgi?id=1926279 will be fixed
			parameterMtu = fmt.Sprintf("-mtu=%d", mtu+100)
		} else {
			parameterMtu = fmt.Sprintf("-mtu=%d", mtu)
		}
		testCommand = append(testCommand, "-negative", parameterMtu)
	default:
		// TODO: change mtu+100 to mtu+50 once https://bugzilla.redhat.com/show_bug.cgi?id=1926279 will be fixed
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
	} else if connectivity == parameters.ConnectivitySameNodeSamePF ||
		connectivity == parameters.ConnectivitySameNodeDiffPF {
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
			parameters.OperatorTestNamespace, podImage),
		corev1.RestartPolicyNever)

	if serverNeedsPrivilege(protocol) {
		podDefinition = pod.RedefineAsPrivileged(podDefinition)
	}

	podDefinition = DefinePodCommandWithIpamAndMac(podDefinition, networkName, ipaddress, macAddress, podCommand)

	if strings.Contains(ipaddress, ":") {
		podDefinition = redefinePodWithInitCommandPolicy(podDefinition, podImage,
			fmt.Sprintf("for i in {1..10}; do sleep 1; if ping6 -c 3 -w 3 %s ; then ip -6 route add %s/128 dev net1 "+
				"&& ip -6 route add %s/128 dev net1 table local && exit 0; fi; done; exit 1",
				serverPodIpv6, clientPodIPv6, multicastIPv6Address))
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
			parameters.OperatorTestNamespace, podImage),
		corev1.RestartPolicyNever)

	if serverNeedsPrivilege(protocol) {
		podDefinition = pod.RedefineAsPrivileged(podDefinition)
	}

	podDefinition = DefinePodCommandsWithDualIpamAndMac(
		podDefinition,
		networkName,
		ip4address,
		ip6address,
		macAddress,
		podIPv4Command,
		podIPv6Command)

	podDefinition = redefinePodWithInitCommandPolicy(podDefinition, podImage,
		fmt.Sprintf("for i in {1..10}; do sleep 1; if ping6 -c 3 -w 3 %s; then ip -6 route add %s/128 dev net1"+
			" && ip -6 route add %s/128 dev net1 table local && exit 0; fi; done; exit 1",
			serverPodIpv6, clientPodIPv6, multicastIPv6Address))

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
				parameters.OperatorTestNamespace, podImage),
			corev1.RestartPolicyNever)
	} else {
		podDefinition = pod.RedefineWithRestartPolicy(
			pod.DefineWithNodeNetworks(
				nodeSelector[0],
				[]string{networkName},
				parameters.OperatorTestNamespace, podImage),
			corev1.RestartPolicyNever)
	}

	if clientNeedsPrivilege(protocol) {
		podDefinition = pod.RedefineAsPrivileged(podDefinition)
	}

	podDefinition = DefinePodCommandWithIpamAndMac(podDefinition, networkName, ipaddress, macAddress, podCommand)

	if strings.Contains(ipaddress, ":") {
		var validateHttpdCommand string
		if protocol == parameters.CommunicationProtocolUnicastTCP {
			validateHttpdCommand = fmt.Sprintf(
				" && for i in {1..60}; do sleep 1; if curl --max-time 3 -6 -g http://[%s]:80; then exit 0; fi; done; exit 1",
				serverPodIpv6)
		}
		podDefinition = redefinePodWithInitCommandPolicy(podDefinition, podImage,
			fmt.Sprintf("for i in {1..10}; do sleep 2; if ping6 -c 3 -w 3 %s ; then ip -6 route add %s/128 dev net1 "+
				"&& ip -6 route add %s/128 dev net1 table local%s && exit 0; fi; done; exit 1",
				serverPodIpv6, serverPodIpv6, multicastIPv6Address, validateHttpdCommand))
	}

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
				parameters.OperatorTestNamespace, podImage),
			corev1.RestartPolicyNever)
	} else {
		podDefinition = pod.RedefineWithRestartPolicy(
			pod.DefineWithNodeNetworks(nodeSelector[0],
				[]string{networkName},
				parameters.OperatorTestNamespace, podImage),
			corev1.RestartPolicyNever)
	}

	if clientNeedsPrivilege(protocol) {
		podDefinition = pod.RedefineAsPrivileged(podDefinition)
	}

	podDefinition = DefinePodCommandsWithDualIpamAndMac(
		podDefinition,
		networkName,
		ip4address,
		ip6address,
		macAddress,
		podCommand)

	var validateHttpdCommand string
	if protocol == parameters.CommunicationProtocolUnicastTCP {
		validateHttpdCommand = fmt.Sprintf(
			" && for i in {1..60}; do sleep 1; if curl --max-time 3 -6 -g http://[%s]:80; then exit 0; fi; done; exit 1", serverPodIpv6)
	}
	podDefinition = redefinePodWithInitCommandPolicy(
		podDefinition, podImage,
		fmt.Sprintf("for i in {1..10}; do sleep 1; if ping6 -c 3 -w 3 %s ; then ip -6 route add %s/128 dev net1 "+
			"&& ip -6 route add %s/128 dev net1 table local%s && exit 0; fi; done; exit 1",
			serverPodIpv6, serverPodIpv6, multicastIPv6Address, validateHttpdCommand),
	)

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
	return protocol == parameters.CommunicationProtocolUnicastSCTP
}

func clientNeedsPrivilege(protocol string) bool {
	return protocol == parameters.CommunicationProtocolUnicastTCP ||
		protocol == parameters.CommunicationProtocolUnicastSCTP
}

func waitUntilPodInStatus(
	createdPod *corev1.Pod,
	podRole string,
	execCommand []string,
	podStatus corev1.PodPhase,
	waitingTime time.Duration) {

	Eventually(func() corev1.PodPhase {
		createdPod, _ = generalHelper.Apiclient.Pods(parameters.OperatorTestNamespace).Get(
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
	connectivityParameters, err := parameters.NewConnectivityTestParameters(mtu, connectivity, protocol)
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
	clientPod, err := generalHelper.Apiclient.Pods(parameters.OperatorTestNamespace).Create(
		context.Background(),
		clientPodDefinition,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	waitUntilPodInStatus(clientPod, "Client", clientTestCommand, corev1.PodSucceeded, podWaitingTime)

	By("Positive test flow - success. Running negative flow")
	negativeFlag = true
	if protocol == parameters.CommunicationProtocolUnicastSCTP {
		serverNetworkName = defineClientNetworkName(mtu, connectivityParameters.Connectivity)
		clientNetworkName = defineServerNetworkName(mtu)
	}
	if protocol == parameters.CommunicationProtocolMulticastUDP ||
		protocol == parameters.CommunicationProtocolBroadcastUDP ||
		protocol == parameters.CommunicationProtocolUnicastSCTP {
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
	clientPodNegative, err := generalHelper.Apiclient.Pods(parameters.OperatorTestNamespace).Create(
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
	connectivityParameters, err := parameters.NewConnectivityTestParameters(mtu, connectivity, protocol)
	Expect(err).ToNot(HaveOccurred())
	// TODO: Remove this condition once bug https://bugzilla.redhat.com/show_bug.cgi?id=1927750 will be fixed
	if (podDeafultInterfaceIPStack == 6 || podDeafultInterfaceIPStack == 46) &&
		connectivityParameters.MTU == parameters.MTUJumbo {
		Skip("Skip test due to https://bugzilla.redhat.com/show_bug.cgi?id=1927750")
	}
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
	clientPod, err := generalHelper.Apiclient.Pods(parameters.OperatorTestNamespace).Create(
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

	// TODO: Remove this return and add negative support to IPv6 TCP test
	if protocol == parameters.CommunicationProtocolUnicastTCP {
		return
	}

	By("Positive test flow - success. Running negative flow")
	negativeFlag = true
	if protocol == parameters.CommunicationProtocolUnicastSCTP {
		serverNetworkName = defineClientNetworkName(mtu, connectivityParameters.Connectivity)
		clientNetworkName = defineServerNetworkName(mtu)
	}
	if protocol == parameters.CommunicationProtocolMulticastUDP ||
		protocol == parameters.CommunicationProtocolBroadcastUDP ||
		protocol == parameters.CommunicationProtocolUnicastSCTP {
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
	clientPodNegative, err := generalHelper.Apiclient.Pods(parameters.OperatorTestNamespace).Create(
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
	connectivityParameters, err := parameters.NewConnectivityTestParameters(mtu, connectivity, protocol)
	Expect(err).ToNot(HaveOccurred())
	// TODO: Remove this condition once bug https://bugzilla.redhat.com/show_bug.cgi?id=1927750 will be fixed
	if (podDeafultInterfaceIPStack == 6 || podDeafultInterfaceIPStack == 46) &&
		connectivityParameters.MTU == parameters.MTUJumbo {
		Skip("Skip test due to https://bugzilla.redhat.com/show_bug.cgi?id=1927750")
	}
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
	if protocol == parameters.CommunicationProtocolBroadcastUDP {
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
	clientPod, err := generalHelper.Apiclient.Pods(parameters.OperatorTestNamespace).Create(
		context.Background(),
		clientPodDefinition,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	waitUntilPodInStatus(clientPod, "Client", ClientTestCommand, corev1.PodSucceeded, dualPodWaitingTime)

	// TODO: Remove this return and add negative support to IPv6 TCP test
	if protocol == parameters.CommunicationProtocolUnicastTCP {
		return
	}

	By("Positive test flow - success. Running negative flow")
	negativeFlag = true
	if protocol == parameters.CommunicationProtocolUnicastSCTP {
		serverNetworkName = defineClientNetworkName(mtu, connectivityParameters.Connectivity)
		clientNetworkName = defineServerNetworkName(mtu)
	}
	if protocol == parameters.CommunicationProtocolMulticastUDP ||
		protocol == parameters.CommunicationProtocolBroadcastUDP ||
		protocol == parameters.CommunicationProtocolUnicastSCTP {
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

	clientPodNegative, err := generalHelper.Apiclient.Pods(parameters.OperatorTestNamespace).Create(
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
