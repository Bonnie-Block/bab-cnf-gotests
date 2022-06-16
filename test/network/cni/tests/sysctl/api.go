package sysctl

import (
	"encoding/json"
	"fmt"
	"strings"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcnihelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nad"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
	k8sv1 "k8s.io/api/core/v1"
)

var _ = Describe("CNF Sysctl", func() {

	var (
		validMacVlanInterfaces []nodes.NodeInterface
		validSriovInterfaces   []*sriovv1.InterfaceExt
		testFail               = ""
	)

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
		validSriovInterfaces = tests.GatherSriovInterfaces(sriovInfos, helper.Config, 1)

		By("Define sysctl sr-iov policy")
		tests.DefineAndCreateSriovPoliciesListOnSriovInterfaceList(
			[]string{netcniparameters.ResourceNameSysctl}, validSriovInterfaces, 5)

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
		tests.CleanPodFromNamespaceAndWaitUntilItsEmpty()

		By("Clean SR-IOV network from the namespace")
		err := namespaces.CleanNetworks(parameters.SriovOperatorNamespace, helper.Apiclient)
		Expect(err).ToNot(HaveOccurred(), "error to clean SR-IOV networks from the namespace")

		By("Clean NADs from the namespace")
		err = namespaces.CleanNetworkAttachmentDefinitionInNamespace(netcniparameters.TestNamespace, helper.Apiclient)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("error to clean NetworkAttachmentDefinition from the namespace %s",
				netcniparameters.TestNamespace))

		By("Clean event messages from the namespace")
		err = namespaces.CleanEventsInNamespace(netcniparameters.TestNamespace, helper.Apiclient)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("error to clean events from the namespace %s", netcniparameters.TestNamespace))
	})

	Context("pod one secondary interface,", func() {

		// 50247
		It("one NAD, forward one valid interface level flag", func() {
			By("Define and create NAD with valid single sysctl flag")
			createSysctlTuningNad(
				netcniparameters.FirstNetworkConfig.Name, netcniparameters.SingleSysctlFlag, validMacVlanInterfaces[0].Name)

			By("Define and create pod")
			podUnderTest := defineCreatePodWithNetworksAndWaitUntilRunning(
				[]multus.NetworkSelectionElement{netcniparameters.FirstNetworkConfig})

			By("Exec cmd command on running pod")
			verifySysctlKernelParametersConfiguredOnPodInterface(
				podUnderTest, netcniparameters.SingleSysctlFlag, netcniparameters.MultusFirstInterfaceName)
		})

		// 50248
		It("one SriovNetwork, forward all valid interface level flags", func() {
			By("Define and create sr-iov network with all valid sysctl flags")
			createSysctlTuningSriovNetwork(validSriovInterfaces[0],
				netcniparameters.AllFlagsSysctlPluginConfig, netcniparameters.FirstNetworkConfig.Name, true)

			By("Define and create pod")
			podUnderTest := defineCreatePodWithNetworksAndWaitUntilRunning(
				[]multus.NetworkSelectionElement{netcniparameters.FirstNetworkConfig})

			By("Exec cmd command on running pod")
			verifySysctlKernelParametersConfiguredOnPodInterface(
				podUnderTest, netcniparameters.AllFlagsSysctlPluginConfig, netcniparameters.MultusFirstInterfaceName)
		})

		// 50340
		It("one SR-IOV network, forward one invalid flag", func() {
			By("Define and create sr-iov network with single invalid sysctl flags")
			createSysctlTuningSriovNetwork(
				validSriovInterfaces[0],
				netcniparameters.SingleInvalidFlag,
				netcniparameters.FirstNetworkConfig.Name, true,
			)

			By("Define and create pod")
			defineCreatePodWithNetworksAndWaitUntilPending([]multus.NetworkSelectionElement{netcniparameters.FirstNetworkConfig})

			By("Wait until sysctl failed event")
			waitUntilEventListContainsSysctlFailedCreatePodSandBoxMessage(netcniparameters.InvalidSysctlKey)
		})

		// 50342
		It("one NAD, forward all valid interface level flags one global kernel flag", func() {

			By("Define and create NAD with invalid sysctl flag")
			nadWithInvalidSysctlFlag := netcnihelper.CopyMap(netcniparameters.AllFlagsSysctlPluginConfig)
			nadWithInvalidSysctlFlag[netcniparameters.GlobalSysctlFlag] = "1"

			createSysctlTuningNad(
				netcniparameters.FirstNetworkConfig.Name, nadWithInvalidSysctlFlag, validMacVlanInterfaces[0].Name)

			By("Define and create pod")
			defineCreatePodWithNetworksAndWaitUntilPending([]multus.NetworkSelectionElement{netcniparameters.FirstNetworkConfig})

			By("Wait until sysctl failed event")
			waitUntilEventListContainsSysctlFailedCreatePodSandBoxMessage(netcniparameters.GlobalSysctlFlag)
		})

		// 50343
		It("one SR-IOV, forward all valid flags one static kernel interface flag", func() {

			By("Define and create sr-iov network with invalid interface level flag")
			nadWithInvalidSysctlFlag := netcnihelper.CopyMap(netcniparameters.AllFlagsSysctlPluginConfig)
			interfaceLevelSysctlFlag := fmt.Sprintf("net.ipv4.conf.%s.disable_policy", netcniparameters.MultusFirstInterfaceName)
			nadWithInvalidSysctlFlag[interfaceLevelSysctlFlag] = "1"
			createSysctlTuningSriovNetwork(validSriovInterfaces[0], nadWithInvalidSysctlFlag,
				netcniparameters.FirstNetworkConfig.Name, true)

			By("Define and create pod")
			defineCreatePodWithNetworksAndWaitUntilPending([]multus.NetworkSelectionElement{netcniparameters.FirstNetworkConfig})

			By("Wait until sysctl failed event")
			waitUntilEventListContainsSysctlFailedCreatePodSandBoxMessage(interfaceLevelSysctlFlag)
		})

		// 50346
		It("one NAD, forward interface level duplicated flags", func() {
			Skip("TC skipped due to BZ:2077683")
			By("Define and create nad network with duplicated sysctl flag")
			duplicatedFlag := defineTuningSysctlNadWithDuplicatedKernelArg(
				netcniparameters.FirstNetworkConfig.Name, validMacVlanInterfaces[0].Name,
				netcniparameters.AllFlagsSysctlPluginConfig)

			By("Define and create pod")
			defineCreatePodWithNetworksAndWaitUntilPending([]multus.NetworkSelectionElement{netcniparameters.FirstNetworkConfig})

			By("Wait until sysctl failed event")
			waitUntilEventListContainsSysctlFailedCreatePodSandBoxMessage(duplicatedFlag)
		})

		// 50436
		It("one NAD, set kernel flag manually using sysctl cmd", func() {

			By("Define and create NAD without single sysctl flag")
			createSysctlTuningNad(netcniparameters.FirstNetworkConfig.Name,
				netcniparameters.SingleSysctlFlag, validMacVlanInterfaces[0].Name)

			By("Define and create pod")
			podUnderTest := defineCreatePodWithNetworksAndWaitUntilRunning(
				[]multus.NetworkSelectionElement{netcniparameters.FirstNetworkConfig})
			verifySysctlKernelParametersConfiguredOnPodInterface(
				podUnderTest, netcniparameters.SingleSysctlFlag, netcniparameters.MultusFirstInterfaceName)

			By("Try to set interface sysctl key manually using sysctl command")
			staticInterfaceSysctlKey := fmt.Sprintf(
				"net.ipv4.conf.%s.accept_redirects", netcniparameters.MultusFirstInterfaceName)
			cmd := []string{"sysctl", "-w", fmt.Sprintf("%s=1", staticInterfaceSysctlKey)}

			By("Exec cmd command on running pod")
			buffer, err := pod.ExecCommand(
				helper.Apiclient, *podUnderTest, cmd, podUnderTest.Spec.Containers[0].Name)
			Expect(err).To(HaveOccurred(),
				"getting nil instead expected nil. sysctl set should not be allowed in container")
			Expect(buffer).ToNot(BeIdenticalTo(
				fmt.Sprintf("sysctl: setting key \"%s\": Read-only file system", staticInterfaceSysctlKey)),
				"error output from sysctl is not as expected")
		})
	})

	Context("pod multiple secondary interfaces,", func() {

		// 50249
		It("one NAD, Forward all valid interface level flags", func() {

			By("Define and create NAD with all valid interface level flags")
			createSysctlTuningNad(netcniparameters.FirstNetworkConfig.Name,
				netcniparameters.AllFlagsSysctlPluginConfig, validMacVlanInterfaces[0].Name)

			By("Define and create pod")
			podNetConfig := []multus.NetworkSelectionElement{
				netcniparameters.FirstNetworkConfig, netcniparameters.SecondNetworkConfig}
			podNetConfig[1].Name = netcniparameters.FirstNetworkConfig.Name
			podUnderTest := defineCreatePodWithNetworksAndWaitUntilRunning(podNetConfig)

			By("Exec cmd command on running pod")
			for _, interfaceName := range []string{
				netcniparameters.MultusFirstInterfaceName, netcniparameters.MultusSecondInterfaceName} {
				verifySysctlKernelParametersConfiguredOnPodInterface(
					podUnderTest, netcniparameters.AllFlagsSysctlPluginConfig, interfaceName)
			}
		})

		// 50250
		It("two SR-IOV, Forward all valid interface level flags to one interface and single interface level flag "+
			"to the second interface", func() {

			By("Define and create NAD with all valid interface level flags")
			createSysctlTuningSriovNetwork(validSriovInterfaces[0],
				netcniparameters.AllFlagsSysctlPluginConfig, netcniparameters.FirstNetworkConfig.Name, true)

			By("Define and create NAD with single valid interface level flag")
			createSysctlTuningSriovNetwork(validSriovInterfaces[0],
				netcniparameters.SingleSysctlFlag, netcniparameters.SecondNetworkConfig.Name, true)

			By("Define and create pod")
			podUnderTest := defineCreatePodWithNetworksAndWaitUntilRunning(
				[]multus.NetworkSelectionElement{netcniparameters.FirstNetworkConfig, netcniparameters.SecondNetworkConfig})

			By("Exec cmd command on running pod")
			verifySysctlKernelParametersConfiguredOnPodInterface(podUnderTest, netcniparameters.AllFlagsSysctlPluginConfig,
				netcniparameters.MultusFirstInterfaceName)
			verifySysctlKernelParametersConfiguredOnPodInterface(podUnderTest, netcniparameters.SingleSysctlFlag,
				netcniparameters.MultusSecondInterfaceName)
		})

		// 50432
		It("two NADs, Forward all valid interface level flags to one interface and multiple flags to the second "+
			"interface with one general network kernel flag", func() {

			By("Define and create NAD with all valid interface level flags")
			createSysctlTuningNad(netcniparameters.FirstNetworkConfig.Name,
				netcniparameters.AllFlagsSysctlPluginConfig, validMacVlanInterfaces[0].Name)

			By("Define and create NAD with multiple valid interface level flags plus single network kernel flag")
			sysctlGlobalFlagKey := "net.ipv4.tcp_fastopen"
			MultipleFlagsSysctlOneGlobal := netcnihelper.CopyMap(netcniparameters.MultipleFlagsSyscl)
			MultipleFlagsSysctlOneGlobal[sysctlGlobalFlagKey] = "0"
			createSysctlTuningNad(netcniparameters.SecondNetworkConfig.Name,
				MultipleFlagsSysctlOneGlobal, validMacVlanInterfaces[0].Name)

			By("Define and create pod")
			defineCreatePodWithNetworksAndWaitUntilPending([]multus.NetworkSelectionElement{
				netcniparameters.FirstNetworkConfig,
				netcniparameters.SecondNetworkConfig})

			By("Wait until event failed message")
			waitUntilEventListContainsSysctlFailedCreatePodSandBoxMessage(sysctlGlobalFlagKey)
		})

		// 50433
		It("two SR-IOV, Forward all valid interface level flags to one interface and one general kernel flag to "+
			"the second interface", func() {

			By("Define sr-iov network with all sysctl flags")
			createSysctlTuningSriovNetwork(validSriovInterfaces[0],
				netcniparameters.AllFlagsSysctlPluginConfig, netcniparameters.FirstNetworkConfig.Name, true)

			By("Define sr-iov network with global kernel flag")
			oneGlobalSysctlFlag := map[string]string{netcniparameters.GlobalSysctlFlag: "1"}
			createSysctlTuningSriovNetwork(validSriovInterfaces[0], oneGlobalSysctlFlag,
				netcniparameters.SecondNetworkConfig.Name, true)

			By("Define and create pod")
			defineCreatePodWithNetworksAndWaitUntilPending([]multus.NetworkSelectionElement{netcniparameters.FirstNetworkConfig,
				netcniparameters.SecondNetworkConfig})

			By("Wait until event failed message")
			waitUntilEventListContainsSysctlFailedCreatePodSandBoxMessage(netcniparameters.GlobalSysctlFlag)
		})

		// 50434
		It("two NADs, try to inject static interface level flag for net1 interface using net2 NAD", func() {

			By("Define and create NAD")
			createSysctlTuningNad(netcniparameters.FirstNetworkConfig.Name,
				netcniparameters.SingleSysctlFlag, validMacVlanInterfaces[0].Name)

			By("Define and create NAD with static sysctl interface flag")
			staticSysctlInterfaceKernelKey := fmt.Sprintf(
				"net.ipv4.conf.%s.accept_redirects", netcniparameters.MultusFirstInterfaceName)
			oneStaticInterfaceSysctlFlag := map[string]string{staticSysctlInterfaceKernelKey: "0"}
			createSysctlTuningNad(netcniparameters.SecondNetworkConfig.Name,
				oneStaticInterfaceSysctlFlag, validMacVlanInterfaces[0].Name)

			By("Define and create pod")
			defineCreatePodWithNetworksAndWaitUntilPending([]multus.NetworkSelectionElement{netcniparameters.FirstNetworkConfig,
				netcniparameters.SecondNetworkConfig})

			By("Wait until event failed message")
			waitUntilEventListContainsSysctlFailedCreatePodSandBoxMessage(staticSysctlInterfaceKernelKey)
		})

		// 50435
		It("two NADs, forward all valid flags to both interfaces and the second "+
			"interface has static interface level duplicated flag", func() {

			By("Define and create NAD with all sysctl flags")
			defineTuningSysctlNadWithDuplicatedKernelArg(netcniparameters.FirstNetworkConfig.Name,
				validMacVlanInterfaces[0].Name,
				netcniparameters.AllFlagsSysctlPluginConfig)

			By("Define and create NAD with static interface duplicated sysctl flags")
			staticSysctlInterfaceKernelKey := fmt.Sprintf(
				"net.ipv4.conf.%s.accept_redirects", netcniparameters.MultusFirstInterfaceName)
			allFlagsWithOneStaticInterfaceSysctlFlag := netcnihelper.CopyMap(netcniparameters.AllFlagsSysctlPluginConfig)
			allFlagsWithOneStaticInterfaceSysctlFlag[staticSysctlInterfaceKernelKey] = "0"
			defineTuningSysctlNadWithDuplicatedKernelArg(netcniparameters.SecondNetworkConfig.Name,
				validMacVlanInterfaces[0].Name,
				allFlagsWithOneStaticInterfaceSysctlFlag)

			By("Define and create pod")
			defineCreatePodWithNetworksAndWaitUntilPending([]multus.NetworkSelectionElement{netcniparameters.FirstNetworkConfig,
				netcniparameters.SecondNetworkConfig})

			By("Wait until event failed message")
			waitUntilEventListContainsSysctlFailedCreatePodSandBoxMessage(staticSysctlInterfaceKernelKey)
		})
	})
})

func waitUntilEventListContainsSysctlFailedCreatePodSandBoxMessage(sysctlFlag string) {
	expectedSysctlFailedMessage := fmt.Sprintf("Sysctl %s is not allowed. Only the following sysctls are allowed",
		sysctlFlag)

	Eventually(func() bool {
		status, err := netcnihelper.IsNamespacedEventListContainsMessage(
			netcniparameters.TestNamespace, expectedSysctlFailedMessage)
		Expect(err).ToNot(HaveOccurred(), "error to collect events")

		return status
	}, netcniparameters.PodWaitingTime, netcniparameters.RetryInterval).Should(BeTrue(),
		"error to detect require event")
}

func defineTuningSysctlNadWithDuplicatedKernelArg(
	nadName string, macVlanIf string, sysctlConfig map[string]string) string {
	nadPlugins := []nad.Plugin{
		*nad.DefineMacVlanPlugin(macVlanIf, nad.DefineStaticIpam()),
		*nad.DefineTuningPluginWithSysctl(sysctlConfig),
	}
	masterPlugin := nad.DefineMasterPlugin(nadName, nadPlugins)

	cniSting, err := json.Marshal(masterPlugin)
	Expect(err).ToNot(HaveOccurred(), "error to marshal master cni plugin")

	duplicatedFlag, stringToReplace := prepDuplicatedSysctlConfig(sysctlConfig)

	dupCniString := strings.Replace(
		string(cniSting), stringToReplace, fmt.Sprintf("%s,%s", stringToReplace, duplicatedFlag), 1)

	nadBuilder, err := nad.NewNadBuilder(nadName, netcniparameters.TestNamespace).BuildWithMasterPluginString(dupCniString)
	Expect(err).ToNot(HaveOccurred(), "error to build nad config with duplicated flag")

	err = nadBuilder.Create(helper.Apiclient)
	Expect(err).ToNot(HaveOccurred(), "error nad with duplicated flag was created")

	return duplicatedFlag
}

func defineCreatePodWithNetworks(podNetworks []multus.NetworkSelectionElement) *k8sv1.Pod {
	netAnnotationMultus := pod.NetworkAnnotation{
		Networks: &podNetworks,
	}
	podNetAnnotation, err := netAnnotationMultus.ConvertNetworksAnnotationToMap()
	Expect(err).ToNot(HaveOccurred(), "error converting pod's network annotation to map")

	runningPod, err := pod.CreateWithOptions(
		helper.Apiclient,
		netcniparameters.TestNamespace,
		helper.Config.Network.TestContainerImage,
		netcnihelper.DefinePodNetworks(podNetAnnotation))
	Expect(err).ToNot(HaveOccurred(), "error creating pod with options")

	return runningPod
}

func defineCreatePodWithNetworksAndWaitUntilStatus(
	podNetworks []multus.NetworkSelectionElement, status k8sv1.PodPhase) *k8sv1.Pod {
	runningPod := defineCreatePodWithNetworks(podNetworks)

	By(fmt.Sprintf("Wait until pod is %s", status))
	Eventually(func() k8sv1.PodPhase {
		return netcnihelper.GetPodStatus(runningPod)
	}, netcniparameters.PodWaitingTime, netcniparameters.RetryInterval).Should(
		Equal(status), fmt.Sprintf("error, pod is not in %s phase", status))

	if netcnihelper.GetPodStatus(runningPod) != k8sv1.PodRunning {
		return nil
	}

	return runningPod
}

func defineCreatePodWithNetworksAndWaitUntilPending(podNetworks []multus.NetworkSelectionElement) {
	defineCreatePodWithNetworksAndWaitUntilStatus(podNetworks, k8sv1.PodPending)
}

func prepDuplicatedSysctlConfig(sysctlConfig map[string]string) (string, string) {
	var (
		duplicatedFlag  string
		stringToReplace string
	)

	cnt := 0

	for key, value := range sysctlConfig {
		stringToReplace = fmt.Sprintf("\"%s\":\"%s\"", key, value)

		if cnt == 0 {
			duplicatedFlag = fmt.Sprintf("\"%s\":\"%s\"", key, value)
		}
		cnt++
	}

	return duplicatedFlag, stringToReplace
}
