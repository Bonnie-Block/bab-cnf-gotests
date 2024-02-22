package sysctl

import (
	"context"
	"fmt"
	"strings"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcnihelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nad"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
	k8sv1 "k8s.io/api/core/v1"
)

func defineCreatePodWithNetworksAndWaitUntilRunning(podNetworks []multus.NetworkSelectionElement) *k8sv1.Pod {
	return defineCreatePodWithNetworksAndWaitUntilStatus(podNetworks, k8sv1.PodRunning)
}

func createSysctlTuningSriovNetwork(
	sriovInterface *sriovv1.InterfaceExt, sysctlFlags map[string]string, sriovNetworkName string, withIpam bool) {
	sysctlPluginConfig, err := nethelper.MarshalTypeToString(nad.DefineTuningPluginWithSysctl(sysctlFlags))
	Expect(err).ToNot(HaveOccurred(), "error marshal sysctlPlugin")

	ipam := ""

	if withIpam {
		ipam, err = nethelper.MarshalTypeToString(nad.DefineStaticIpam())
		Expect(err).ToNot(HaveOccurred(), "error marshal ipam")
	}

	By("Define and create sr-iov sysctl network")
	nethelper.DefineAndCreateSriovNetwork(
		sriovInterface, sriovNetworkName, netcniparameters.ResourceNameSysctl, ipam, sysctlPluginConfig,
		netcniparameters.TestNamespace)
	By("Wait until NAD is available")
	Eventually(func() error {
		_, err = helper.Apiclient.NetworkAttachmentDefinitions(netcniparameters.TestNamespace).Get(
			context.Background(), sriovNetworkName, v1.GetOptions{})

		return err
	}, 60*time.Second, netcniparameters.RetryInterval).ShouldNot(HaveOccurred())
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

func waitUntilPodInStatus(runningPod *k8sv1.Pod, status k8sv1.PodPhase) *k8sv1.Pod {
	Eventually(func() k8sv1.PodPhase {
		return netcnihelper.GetPodStatus(runningPod)
	}, netcniparameters.PodWaitingTime, netcniparameters.RetryInterval).Should(
		Equal(status), fmt.Sprintf("error, pod is not in %s phase", status))

	if netcnihelper.GetPodStatus(runningPod) != k8sv1.PodRunning {
		return nil
	}

	return runningPod
}

func defineInitContainer(name string, cmd string, securityContext *k8sv1.SecurityContext) *k8sv1.Container {
	return pod.DefineContainer(
		name, []string{cmd}, helper.Config.Network.TestContainerImage, securityContext)
}

func defineAndCreateNadWithPlugins(nadName string, plugins []*nad.Plugin) {
	nadObj, err := nad.NewNadBuilder(nadName, netcniparameters.TestNamespace).WithPlugins(plugins).Build()
	Expect(err).ToNot(HaveOccurred(), "error to build nad config")
	err = nadObj.Create(helper.Apiclient)
	Expect(err).ToNot(HaveOccurred(), "error to define and create Nad")
}

func createSysctlTuningNad(nadName string, sysctlConfig map[string]string, macVlanIf string) {
	nadPlugins := []*nad.Plugin{
		nad.DefineMacVlanPlugin(macVlanIf, nad.DefineStaticIpam()),
		nad.DefineTuningPluginWithSysctl(sysctlConfig),
	}
	defineAndCreateNadWithPlugins(nadName, nadPlugins)
}
