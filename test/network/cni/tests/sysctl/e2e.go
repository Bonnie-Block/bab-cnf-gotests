package sysctl

import (
	"context"
	"fmt"
	"strings"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcnihelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nad"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CNF Sysctl", func() {

	var (
		validMacVlanInterfaces []nodes.NodeInterface
		validSriovInterfaces   []*sriovv1.InterfaceExt
		testFail               = ""
	)

	//  General test scenario:
	//  	The test scenario includes 3 pods: client, server, redirect.
	//  	The client pod will ping the server pod via the redirect pod.
	//  	The redirect pod will send a ICMP redirect back to the client, causing the client to ping the server directly.
	//  	This will cause the client to ping the server directly in a followup ping attempt.
	//  	With the send_redirect=0 sysctl set, the client will reject the ICMP redirect request from the redirect pod,
	// 		causing the ping from client to server to fail.
	execute.BeforeAll(func() {

		nodeLabel := strings.Split(helper.Config.General.CnfNodeLabel, "/")[1]

		By(fmt.Sprintf("Clean test namespace %s", netcniparameters.TestNamespace))
		tests.CleanSriovNetworkSriovPolicyNadsAndPods()

		By("Waiting until SRIOV become stable")
		tests.WaitUntilSriovBecomesStable()

		By(fmt.Sprintf("Collect node list based on label: %s", nodeLabel))
		nodeListString := helper.GetNodeListStringByLabel(nodeLabel)

		By("Collect node valid interface for macvlan configuration")
		validMacVlanInterfaces = netcnihelper.GetNodeValidMacVlanInterface(
			nodeListString[0], helper.Config, 1)

		By("Discover sr-iov Node Interfaces")
		sriovInfos, err := cluster.DiscoverSriov(helper.Apiclient, parameters.SriovOperatorNamespace)
		if err != nil {
			testFail := fmt.Sprintf("Error discover sr-iov node info: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}

		By("Find available sr-iov interfaces")
		validSriovInterfaces = nethelper.GatherSriovInterfaces(sriovInfos, helper.Config, 1)

		By("Define sysctl sr-iov policy")
		nethelper.DefineAndCreateSriovPoliciesListOnSriovInterfaceList(
			[]string{netcniparameters.ResourceNameSysctl}, validSriovInterfaces, 12)

		By("Waiting until SRIOV become stable after sr-iov policy configuration")
		tests.WaitUntilSriovBecomesStable()

	})

	BeforeEach(func() {
		if len(validMacVlanInterfaces) < 1 {
			Skip("cluster doesn't have secondary interfaces available for sysctl test")
		}

		if testFail != "" {
			Fail(testFail)
		}

		By("Clean pods from the namespace")
		err := namespaces.CleanPodAndWaitUntilItsEmpty(helper.Apiclient, netcniparameters.TestNamespace)
		Expect(err).ToNot(HaveOccurred(), "failed to remove pods")

		By("Clean SR-IOV network from the namespace")
		err = namespaces.CleanNetworks(parameters.SriovOperatorNamespace, helper.Apiclient)
		Expect(err).ToNot(HaveOccurred(), "error to clean SR-IOV networks from the namespace")

		By("Clean NADs from the namespace")
		err = namespaces.CleanNetworkAttachmentDefinitions(netcniparameters.TestNamespace, helper.Apiclient)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("error to clean NetworkAttachmentDefinition from the namespace %s",
				netcniparameters.TestNamespace))

		By("Clean event messages from the namespace")
		err = namespaces.CleanEventsInNamespace(netcniparameters.TestNamespace, helper.Apiclient)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("error to clean events from the namespace %s", netcniparameters.TestNamespace))
	})

	Context("pod one secondary interface,", func() {

		// 50437
		It("set accept_redirects=0 on one of two NAD macvlans", func() {
			By("Define and create nad with sysctl mutation flag accept_redirects=0")
			createSysctlTuningNad(netcniparameters.NetworkWithSysctlMutation,
				netcniparameters.SingleSysctlFlag, validMacVlanInterfaces[0].Name)

			By("Define and create nad without sysctl config")
			createStaticIpamNad(
				netcniparameters.NetworkWithoutSysctlMutation, validMacVlanInterfaces[0].Name)

			By("Create server and redirect pod")
			createServerPod(netcnihelper.DefineServerNetCfg(false, false), netcniparameters.SrvInitCMD)
			createRedirectPod(netcnihelper.DefineRedirectNetCfg(false, false), netcniparameters.RdrInitCMD)

			By("Create client pod connected to NAD without sysctl mutation")
			clientNetCfg := netcnihelper.DefineClientNetCfg(false, false)
			runningClientPod := createClientPod(clientNetCfg, netcniparameters.ClientInitCMDs)

			By("Test ping, route and sysctl flag")
			testIcmpRouteSysctlFlag(
				*runningClientPod, netcniparameters.SrvLopIPAddr, netcniparameters.MultusFirstInterfaceName, false)

			By("Recreate client pod connected to nad with sysctl mutation")
			clientNetCfg[0].Name = netcniparameters.NetworkWithSysctlMutation
			runningClientPod = recreateClientPod(runningClientPod, clientNetCfg, netcniparameters.ClientInitCMDs)

			By("Test ping, route and sysctl flag negative")
			testIcmpRouteSysctlFlag(*runningClientPod, netcniparameters.SrvLopIPAddr,
				netcniparameters.MultusFirstInterfaceName, true)
		})

		// 50439
		It("set accept_redirects=0 on one of two SriovNetworks", func() {
			By("Define sr-iov network without sysctl mutation flags")
			createSysctlTuningSriovNetwork(validSriovInterfaces[0],
				nil, netcniparameters.NetworkWithoutSysctlMutation, true)

			By("Default and create sr-iov network with sysctl mutation flag accept_redirects=0")
			createSysctlTuningSriovNetwork(
				validSriovInterfaces[0], netcniparameters.SingleSysctlFlag, netcniparameters.NetworkWithSysctlMutation, true)

			By("Create server and redirect pod")
			createServerPod(netcnihelper.DefineServerNetCfg(false, false), netcniparameters.SrvInitCMD)
			createRedirectPod(netcnihelper.DefineRedirectNetCfg(false, false), netcniparameters.RdrInitCMD)

			By("Create client pod connected to sr-iov network without sysctl mutation")
			clientNetCfg := netcnihelper.DefineClientNetCfg(false, false)
			runningClientPod := createClientPod(clientNetCfg, netcniparameters.ClientInitCMDs)

			By("Test ping, route and sysctl flag")
			testIcmpRouteSysctlFlag(
				*runningClientPod, netcniparameters.SrvLopIPAddr, netcniparameters.MultusFirstInterfaceName, false)

			By("Recreate client pod connected to sr-iov network with sysctl mutation")
			clientNetCfg[0].Name = netcniparameters.NetworkWithSysctlMutation
			runningClientPod = recreateClientPod(runningClientPod, clientNetCfg, netcniparameters.ClientInitCMDs)

			By("Test ping, route and sysctl flag negative")
			testIcmpRouteSysctlFlag(*runningClientPod, netcniparameters.SrvLopIPAddr,
				netcniparameters.MultusFirstInterfaceName, true)
		})

		// 50502
		It("sriov-bond interface. One SriovNetwork 2 sriov interfaces, one bound NAD. "+
			"Set accept_redirects=0 on interface", func() {

			By("Define sr-iov network without sysctl flags")
			createSysctlTuningSriovNetwork(validSriovInterfaces[0],
				nil, parameters.SriovPolicyName, false)

			By("Define bond nad without sysctl plugin")

			bondPlugin := nad.DefineBondPlugin(nad.DefineStaticIpam(), []string{netcniparameters.MultusFirstInterfaceName,
				netcniparameters.MultusSecondInterfaceName}, "active-backup")
			defineAndCreateNadWithPlugins(netcniparameters.NetworkWithoutSysctlMutation, []*nad.Plugin{bondPlugin})

			By("Define bond nad with sysctl mutation flag accept_redirects=0")

			nadPlugins := []*nad.Plugin{bondPlugin, nad.DefineTuningPluginWithSysctl(netcniparameters.SingleSysctlFlag)}
			defineAndCreateNadWithPlugins(netcniparameters.NetworkWithSysctlMutation, nadPlugins)

			By("Create server and redirect pod")
			createServerPod(netcnihelper.DefineServerNetCfg(false, true), netcniparameters.SrvInitCMD)
			createRedirectPod(netcnihelper.DefineRedirectNetCfg(false, true), netcniparameters.RdrInitCMD)

			By("Create client pod connected to bond nad without sysctl mutation")
			clientNetCfg := netcnihelper.DefineClientNetCfg(false, true)
			runningClientPod := createClientPod(clientNetCfg, netcniparameters.ClientInitCMDs)

			By("Test ping, route and sysctl flag")
			testIcmpRouteSysctlFlag(
				*runningClientPod, netcniparameters.SrvLopIPAddr, netcniparameters.BondInterfaceName, false)

			By("Recreate client pod connected to bond nad with sysctl mutation accept_redirects=0")
			clientNetCfg[2].Name = netcniparameters.NetworkWithSysctlMutation
			runningClientPod = recreateClientPod(runningClientPod, clientNetCfg, netcniparameters.ClientInitCMDs)

			By("Test ping, route and sysctl flag negative")
			testIcmpRouteSysctlFlag(*runningClientPod, netcniparameters.SrvLopIPAddr,
				netcniparameters.BondInterfaceName, true)
		})
	})

	Context("pod multiple interfaces,", func() {

		// 50438
		It("set accept_redirects=0 on one first NAD and 1 on the second NAD", func() {
			By("Define and create nad with sysctl mutation plugin")
			createSysctlTuningNad(netcniparameters.NetworkWithSysctlMutation,
				netcniparameters.SingleSysctlFlag, validMacVlanInterfaces[0].Name)

			By("Define and create nad with mac-vlan")
			createStaticIpamNad(
				netcniparameters.NetworkWithoutSysctlMutation, validMacVlanInterfaces[0].Name)

			By("Create server and redirect pod")
			createServerPod(netcnihelper.DefineServerNetCfg(true, false), netcniparameters.SrvDualInitCMD)
			createRedirectPod(netcnihelper.DefineRedirectNetCfg(true, false), netcniparameters.RdrDualInitCMD)

			By("Create client pod connected to both nads")
			clientNetCfg := netcnihelper.DefineClientNetCfg(true, false)
			runningClientPod := createClientPod(clientNetCfg, netcniparameters.ClientDualInitCMD)

			By("Test ping, route and sysctl flag on interface connected to nad without mutation")
			testIcmpRouteSysctlFlag(
				*runningClientPod, netcniparameters.SrvLopIPAddr, netcniparameters.MultusFirstInterfaceName, false)

			By("Test ping, route and sysctl flag on interface connected to nad with mutation, negative")
			testIcmpRouteSysctlFlag(
				*runningClientPod, netcniparameters.SrvLopSecondIPAddr, netcniparameters.MultusSecondInterfaceName, true)
		})

		// 50501
		It("set accept_redirects=0 on first SriovNetwork and 1 on the second SriovNetwork", func() {
			By("Define sr-iov network without sysctl mutation flags")
			createSysctlTuningSriovNetwork(validSriovInterfaces[0],
				nil, netcniparameters.NetworkWithoutSysctlMutation, true)

			By("Default and create sr-iov network with sysctl mutation flag accept_redirects=0")
			createSysctlTuningSriovNetwork(validSriovInterfaces[0], netcniparameters.SingleSysctlFlag,
				netcniparameters.NetworkWithSysctlMutation, true)

			By("Create server and redirect pod")
			createServerPod(netcnihelper.DefineServerNetCfg(true, false), netcniparameters.SrvDualInitCMD)
			createRedirectPod(netcnihelper.DefineRedirectNetCfg(true, false), netcniparameters.RdrDualInitCMD)

			By("Create client pod connected to both sr-iov networks")
			clientNetCfg := netcnihelper.DefineClientNetCfg(true, false)
			runningClientPod := createClientPod(clientNetCfg, netcniparameters.ClientDualInitCMD)

			By("Test ping, route and sysctl flag on interface connected to sr-iov network without mutation")
			testIcmpRouteSysctlFlag(
				*runningClientPod, netcniparameters.SrvLopIPAddr, netcniparameters.MultusFirstInterfaceName, false)

			By("Test ping, route and sysctl flag on interface connected to sr-iov network with mutation, negative")
			testIcmpRouteSysctlFlag(
				*runningClientPod, netcniparameters.SrvLopSecondIPAddr, netcniparameters.MultusSecondInterfaceName, true)
		})

		// 50503
		It("sriov-bond. Set accept_redirects=0 on first bond NAD and 1 on the second bond NAD", func() {
			By("Define sr-iov network without sysctl mutation flags")
			createSysctlTuningSriovNetwork(validSriovInterfaces[0],
				nil, parameters.SriovPolicyName, false)

			By("Define bond nad without sysctl plugin")

			bondPlugin := nad.DefineBondPlugin(nad.DefineStaticIpam(), []string{netcniparameters.MultusFirstInterfaceName,
				netcniparameters.MultusSecondInterfaceName}, "active-backup")
			defineAndCreateNadWithPlugins(netcniparameters.NetworkWithoutSysctlMutation, []*nad.Plugin{bondPlugin})

			By("Define bond nad with sysctl mutation flag accept_redirects=0")
			bondPlugin = nad.DefineBondPlugin(nad.DefineStaticIpam(), []string{"net4", "net5"}, "active-backup")
			nadPlugins := []*nad.Plugin{bondPlugin, nad.DefineTuningPluginWithSysctl(netcniparameters.SingleSysctlFlag)}
			defineAndCreateNadWithPlugins(netcniparameters.NetworkWithSysctlMutation, nadPlugins)

			By("Create server and redirect pod")
			createServerPod(netcnihelper.DefineServerNetCfg(true, true), netcniparameters.SrvDualInitCMD)
			createRedirectPod(netcnihelper.DefineRedirectNetCfg(true, true), netcniparameters.RdrDualInitCMD)

			By("Create client pod connected to both bond nad networks")
			clientNetCfg := netcnihelper.DefineClientNetCfg(true, true)
			runningClientPod := createClientPod(clientNetCfg, netcniparameters.ClientDualInitCMD)

			By("Test ping, route and sysctl flag on interface connected to bond network without mutation")
			testIcmpRouteSysctlFlag(
				*runningClientPod, netcniparameters.SrvLopIPAddr, netcniparameters.BondInterfaceName, false)

			By("Test ping, route and sysctl flag on interface connected to bond network with mutation, negative")
			testIcmpRouteSysctlFlag(
				*runningClientPod, netcniparameters.SrvLopSecondIPAddr, netcniparameters.BondInterfaceNameSecond, true)
		})
	})
})

func createServerPod(netSrvConf []multus.NetworkSelectionElement, serverInitCMD string) {
	By("Create server pod")
	defineCreatePodWithNetworksSecurityContextAndInitContainers(
		netSrvConf,
		&k8sv1.SecurityContext{},
		netcniparameters.NetAdminSC,
		serverInitCMD,
	)
}

func createRedirectPod(netRdrConfig []multus.NetworkSelectionElement, redirectInitCMD string) {
	By("Create redirect pod")
	defineCreatePodWithNetworksSecurityContextAndInitContainers(
		netRdrConfig,
		&k8sv1.SecurityContext{},
		netcniparameters.NetAdminSC,
		redirectInitCMD,
	)
}

func createClientPod(netClientConf []multus.NetworkSelectionElement, clientInitCMD string) *k8sv1.Pod {
	return defineCreatePodWithNetworksSecurityContextAndInitContainers(
		netClientConf,
		netcniparameters.NetRawSC,
		netcniparameters.ClientNetAdmNetRawSysAdmSC,
		clientInitCMD)
}

func createStaticIpamNad(nadName string, macVlanInterfaceName string) {
	macVlanPlugin := nad.DefineMacVlanPlugin(macVlanInterfaceName, nad.DefineStaticIpam())
	defineAndCreateNadWithPlugins(nadName, []*nad.Plugin{macVlanPlugin})
}

func checkRouteToDst(client k8sv1.Pod, destAddress string, negative bool) {
	runningPod, err := helper.Apiclient.Pods(client.ObjectMeta.Namespace).Get(
		context.Background(), client.ObjectMeta.Name, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred(), "error to run pod")

	logs, err := pod.ExecCommand(
		helper.Apiclient,
		*runningPod,
		[]string{"ip", "route", "get", destAddress})
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error to get route to the %s", destAddress))

	if negative {
		Expect(logs.String()).ToNot(ContainSubstring("<redirected>"),
			"pod's route table has redirected route")
	} else {
		Expect(logs.String()).To(ContainSubstring("<redirected>"),
			"pod's route table doesn't have redirected route")
	}
}

func testIcmpRouteSysctlFlag(runningClientPod k8sv1.Pod, dstAddr, intName string, negative bool) {
	sysctlConfig := netcniparameters.SingleAcceptRedirectSysctlFlag

	if negative {
		sysctlConfig = netcniparameters.SingleSysctlFlag
	}

	verifySysctlKernelParametersConfiguredOnPodInterface(&runningClientPod, sysctlConfig, intName)
	retryPing(runningClientPod, dstAddr, intName, 3, negative)
	checkRouteToDst(runningClientPod, dstAddr, negative)
}

func retryPing(runningClientPod k8sv1.Pod, desIP string, intName string, retry int, negative bool) {
	attempt := 0

	Eventually(func() bool {
		err := tests.PingIPViaInterface(runningClientPod, intName, desIP, negative)

		if attempt == retry && err == nil {
			return true
		}
		if attempt > retry {
			Fail("connectivity test failed")
		}
		attempt++

		return false
	}, netcniparameters.PodWaitingTime, netcniparameters.RetryInterval).Should(BeTrue())
}

func defineCreatePodWithNetworksSecurityContextAndInitContainers(
	podNetworks []multus.NetworkSelectionElement,
	securityContext *k8sv1.SecurityContext,
	initContainerSecurityContext *k8sv1.SecurityContext,
	initCmd string) *k8sv1.Pod {
	initContainers := []*k8sv1.Container{defineInitContainer("init1",
		initCmd, initContainerSecurityContext)}

	netAnnotationMultus := pod.NetworkAnnotation{
		Networks: &podNetworks,
	}
	podNetAnnotation, err := netAnnotationMultus.ConvertNetworksAnnotationToMap()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
		"error converting pod's network annotation to map due to %s", err))

	runningPod, err := pod.CreateWithOptions(
		helper.Apiclient,
		netcniparameters.TestNamespace,
		helper.Config.Network.TestContainerImage,
		netcnihelper.DefinePodNetworks(podNetAnnotation),
		netcnihelper.DefinePodWithInitContainers(initContainers),
		netcnihelper.DefinePodWIthSecurityContext(securityContext),
	)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error creating pod with options %s", err))

	return waitUntilPodInStatus(runningPod, k8sv1.PodRunning)
}

func recreateClientPod(
	runningClientPod *k8sv1.Pod, clientNetCfg []multus.NetworkSelectionElement, clientInitCMD string) *k8sv1.Pod {
	By("Remove pod")

	err := pod.DeletePodAndWait(helper.Apiclient, runningClientPod)
	Expect(err).ToNot(HaveOccurred(), "error to remove pod")

	return createClientPod(clientNetCfg, clientInitCMD)
}
