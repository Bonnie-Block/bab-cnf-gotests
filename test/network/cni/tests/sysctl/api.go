package sysctl

import (
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
			[]string{netcniparameters.ResourceNameSysctl}, validSriovInterfaces)

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
			defineAndCreateNadWithTuningAndSysctlPlugins(
				netcniparameters.NetworkConfig.Name, netcniparameters.SingleSysctlFlag, validMacVlanInterfaces[0].Name)

			By("Define and create pod")
			podUnderTest := defineCreatePodWithNetworksAndWaitUntilRunning(
				[]multus.NetworkSelectionElement{netcniparameters.NetworkConfig})

			By("Exec cmd command on running pod")
			verifySysctlKernelParametersConfiguredOnPodInterface(
				podUnderTest, netcniparameters.SingleSysctlFlag, netcniparameters.MultusFirstInterfaceName)
		})

		// 50248
		It("one SriovNetwork, forward all valid interface level flags", func() {
			By("Define and create sr-iov network with all valid sysctl flags")
			defineAndCreateSriovNetworkWithSysctlTuningPlugin(validSriovInterfaces[0],
				netcniparameters.AllFlagsSysctlPluginConfig, netcniparameters.NetworkConfig.Name)

			By("Define and create pod")
			podUnderTest := defineCreatePodWithNetworksAndWaitUntilRunning(
				[]multus.NetworkSelectionElement{netcniparameters.NetworkConfig})

			By("Exec cmd command on running pod")
			verifySysctlKernelParametersConfiguredOnPodInterface(
				podUnderTest, netcniparameters.AllFlagsSysctlPluginConfig, netcniparameters.MultusFirstInterfaceName)
		})
	})
})

func defineAndCreateNadWithTuningAndSysctlPlugins(nadName string, sysctlConfig map[string]string, macVlanIf string) {
	nadPlugins := []*nad.Plugin{
		nad.DefineMacVlanPlugin(macVlanIf, nad.DefineStaticIpam()),
		nad.DefineTuningPluginWithSysctl(sysctlConfig),
	}
	tuningSysctlNad, err := nad.NewNadBuilder(nadName, netcniparameters.TestNamespace).WithPlugins(nadPlugins).Build()
	Expect(err).ToNot(HaveOccurred(), "error to build nad config")
	err = tuningSysctlNad.Create(helper.Apiclient)
	Expect(err).ToNot(HaveOccurred(), "error to define and create Nad")
}

func defineAndCreateSriovNetworkWithSysctlTuningPlugin(
	sriovInterface *sriovv1.InterfaceExt, sysctlFlags map[string]string, sriovNetworkName string) {
	ipam, err := netcnihelper.MarshalTypeToString(nad.DefineStaticIpam())
	Expect(err).ToNot(HaveOccurred(), "error marshal ipam")
	sysctlPluginConfig, err := netcnihelper.MarshalTypeToString(nad.DefineTuningPluginWithSysctl(sysctlFlags))
	Expect(err).ToNot(HaveOccurred(), "error marshal sysctlPlugin")

	By("Define and create sr-iov sysctl network")
	tests.DefineAndCreateSriovNetwork(
		sriovNetworkName, sriovInterface, netcniparameters.ResourceNameSysctl, ipam, sysctlPluginConfig)
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

func defineCreatePodWithNetworksAndWaitUntilRunning(podNetworks []multus.NetworkSelectionElement) *k8sv1.Pod {
	return defineCreatePodWithNetworksAndWaitUntilStatus(podNetworks, k8sv1.PodRunning)
}

func verifySysctlKernelParametersConfiguredOnPodInterface(
	podUnderTest *k8sv1.Pod, sysctlPluginConfig map[string]string, interfaceName string) {
	for key, value := range sysctlPluginConfig {
		sysctlKernelParam := strings.Replace(key, "IFNAME", interfaceName, 1)

		By(fmt.Sprintf("Validate sysctl flag: %s has the right value in pod's interface: %s",
			sysctlKernelParam, interfaceName))

		cmdBuffer, err := pod.ExecCommand(helper.Apiclient, *podUnderTest,
			[]string{"sysctl", "-n", sysctlKernelParam})
		Expect(err).ToNot(HaveOccurred(), "error to execute cmd command on the pod")
		Expect(strings.TrimSpace(cmdBuffer.String())).To(BeIdenticalTo(value),
			"sysctl kernel param is not in expected state")
	}
}
