package tests

import (
	"context"
	"fmt"
	"time"

	k8sv1 "k8s.io/api/core/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// PingIPViaInterface runs ping on the given pod and returns exit code.
func PingIPViaInterface(clientPod k8sv1.Pod, vrfName string, destIPAddr string, negative bool) error {
	command := []string{"testcmd", "-interface", vrfName, "-server", destIPAddr, "-protocol", "icmp", "-mtu", "100"}
	if negative {
		command = append(command, "--negative")
	}

	_, err := pod.ExecCommand(
		helper.Apiclient,
		clientPod,
		command)

	return err
}

// DefineAndCreateSriovNetwork defines and creates sr-iov network.
func DefineAndCreateSriovNetwork(
	name string, sriovInterface *sriovv1.InterfaceExt, resourceName string, ipam string, metaPluginConfig string) {
	err := nethelper.CreateSriovNetwork(
		helper.Apiclient,
		sriovInterface,
		name,
		netcniparameters.TestNamespace,
		parameters.SriovOperatorNamespace,
		resourceName,
		ipam,
		defineSriovNetworkMetaPlugins(metaPluginConfig))
	Expect(err).ToNot(HaveOccurred(), "error creating sr-iov network")
}

// WaitUntilSriovBecomesStable waits until sr-iov operator become stable.
func WaitUntilSriovBecomesStable() {
	var snoTimeoutMultiplier time.Duration = 1

	isSingleNode, err := nodes.IsSingleNodeCluster(helper.Apiclient)
	Expect(err).ToNot(HaveOccurred(), "error to check if cluster if SNO")

	if isSingleNode {
		snoTimeoutMultiplier = 2
		disableDrainState := helper.GetNodeDrainState(parameters.SriovOperatorNamespace)

		if !disableDrainState {
			helper.SetDisableNodeDrainState(true, parameters.SriovOperatorNamespace)
			helper.ChangedNodeDrainState = true
		}
	}

	helper.WaitForSRIOVStable(parameters.SriovOperatorNamespace, netcniparameters.WaitingTime, snoTimeoutMultiplier)
}

// DefineAndCreateSriovPoliciesListOnSriovInterfaceList defines sr-iov policy on interface list.
func DefineAndCreateSriovPoliciesListOnSriovInterfaceList(
	resourceNamesList []string, validSriovInterfaces []*sriovv1.InterfaceExt, vfNumber int) {
	Expect(len(resourceNamesList)).To(BeNumerically("<=", len(validSriovInterfaces)),
		"not enough sr-iov interfaces available to create requested policies")

	var sriovPolicyList []*sriovv1.SriovNetworkNodePolicy

	for idx, resourceName := range resourceNamesList {
		sriovPolicyList = append(sriovPolicyList, helper.DefineSriovPolicy(
			fmt.Sprintf("%s%d", netcniparameters.SriovPolicyName, idx),
			parameters.SriovOperatorNamespace,
			validSriovInterfaces[idx],
			vfNumber,
			fmt.Sprintf("#0-%d", vfNumber-1),
			1500,
			resourceName,
			"netdevice"))
	}

	for _, sriovPolicy := range sriovPolicyList {
		err := helper.Apiclient.Create(context.Background(), sriovPolicy)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error to create sr-iov policy %v", sriovPolicy))
	}
}

// WaitUntilSriovNadListBecomeAvailable waits until sr-iov configures nads in the needed namespace.
func WaitUntilSriovNadListBecomeAvailable(nadListNames []string) {
	for _, sriovNetworkName := range nadListNames {
		Eventually(func() error {
			netAttDef := &netattdefv1.NetworkAttachmentDefinition{}

			return helper.Apiclient.Get(
				context.Background(),
				runtimeclient.ObjectKey{
					Name:      sriovNetworkName,
					Namespace: netcniparameters.TestNamespace},
				netAttDef)
		}, 60*time.Second, 1*time.Second).ShouldNot(HaveOccurred(),
			"error occurred while waiting for NetworkAttachmentDefinition to be up ")
	}
}

// GatherSriovInterfaces returns the request number of sr-iov interfaces that are present on cluster.
func GatherSriovInterfaces(
	sriovInfos *cluster.EnabledNodes, config *config.Config, requestedInterface int) []*sriovv1.InterfaceExt {
	sriovInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
	Expect(err).ToNot(HaveOccurred(), "error to find sr-iov interfaces")
	validSriovInterfaces, err := config.GetSriovInterfaces(sriovInterfaces, requestedInterface)
	Expect(err).ToNot(HaveOccurred(),
		"error to find valid sr-iov interfaces from env var CNF_INTERFACES_LIST")

	return validSriovInterfaces
}

// CleanSriovNetworkSriovPolicyNadsAndPods removes all sr-iov network, policy, nads and pods.
func CleanSriovNetworkSriovPolicyNadsAndPods() {
	err := namespaces.Clean(
		parameters.SriovOperatorNamespace,
		netcniparameters.TestNamespace,
		helper.Apiclient,
		false)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error to clean namespace %s", netcniparameters.TestNamespace))
}

// CleanPodFromNamespaceAndWaitUntilItsEmpty removes pod from namespace and wait until it's empty.
func CleanPodFromNamespaceAndWaitUntilItsEmpty() {
	err := namespaces.CleanPods(netcniparameters.TestNamespace, helper.Apiclient)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error to remove list of pods from the namepsace %s",
		netcniparameters.TestNamespace))
	By("Wait until all pods are removed for the namespace")
	Eventually(func() bool {
		podsList, err := helper.Apiclient.Pods(netcniparameters.TestNamespace).List(
			context.Background(), metav1.ListOptions{})
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("error to list pods in the given namespace %s", netcniparameters.TestNamespace))

		return len(podsList.Items) == 0
	}, netcniparameters.PodWaitingTime, netcniparameters.RetryInterval).Should(
		BeTrue(),
		fmt.Sprintf("error to remove all pods in the given namespace %s", netcniparameters.TestNamespace))
}

func defineSriovNetworkMetaPlugins(pluginConfig string) func(network *sriovv1.SriovNetwork) {
	return func(network *sriovv1.SriovNetwork) {
		network.Spec.MetaPluginsConfig = pluginConfig
	}
}
