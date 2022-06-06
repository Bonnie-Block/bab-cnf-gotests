package netcnihelper

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	generalParam "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

// DefineSriovNetworkMetaPluginsVRFConfig  plugin for discovering and advertising SR-IOV VFs.
func DefineSriovNetworkMetaPluginsVRFConfig(vrfName string) func(network *sriovv1.SriovNetwork) {
	return func(network *sriovv1.SriovNetwork) {
		network.Spec.MetaPluginsConfig = fmt.Sprintf(`{"type": "vrf", "vrfname": "%s"}`, vrfName)
	}
}

// CleanResources removes all pods from vrf test namespace.
func CleanResources() {
	By("Cleaning up resources before test")

	err := namespaces.CleanPods(netcniparameters.TestNamespace, helper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() bool {
		podsList, err := helper.Apiclient.Pods(netcniparameters.TestNamespace).List(
			context.Background(), metav1.ListOptions{})
		Expect(err).ToNot(HaveOccurred())

		return len(podsList.Items) == 0
	}, 3*time.Minute, 10*time.Second).Should(BeTrue())
}

// SetupSriovBeforeAll prepare env before test.
func SetupSriovBeforeAll(config *config.Config, sriovInfos *cluster.EnabledNodes, ipamType string, dual bool) {
	var (
		resourceNameRange                            = []string{netcniparameters.ResourceNameVRF}
		snoTimeoutMultiplier           time.Duration = 1
		requestedInterface                           = 1
		resourceNameVrfRed                           = netcniparameters.ResourceNameVRF
		resourceNameVrfBlue                          = netcniparameters.ResourceNameVRF
		sriovNetworkInterfaceIndexRed                = 0
		sriovNetworkInterfaceIndexBlue               = 0
	)

	if dual {
		resourceNameRange = []string{netcniparameters.ResourceNameVRFVf1, netcniparameters.ResourceNameVRFVf2}
		requestedInterface = 2
		resourceNameVrfRed = netcniparameters.ResourceNameVRFVf1
		resourceNameVrfBlue = netcniparameters.ResourceNameVRFVf2
		sriovNetworkInterfaceIndexRed = 0
		sriovNetworkInterfaceIndexBlue = 1
	}

	isSingleNode, err := nodes.IsSingleNodeCluster(helper.Apiclient)
	Expect(err).ToNot(HaveOccurred())

	if isSingleNode {
		snoTimeoutMultiplier = 2
		disableDrainState := helper.GetNodeDrainState(generalParam.SriovOperatorNamespace)

		if !disableDrainState {
			helper.SetDisableNodeDrainState(true, generalParam.SriovOperatorNamespace)
			helper.ChangedNodeDrainState = true
		}
	}

	sriovInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
	Expect(err).ToNot(HaveOccurred())
	validSriovInterfaces, err := config.GetSriovInterfaces(sriovInterfaces, requestedInterface)
	Expect(err).ToNot(HaveOccurred())

	By(fmt.Sprintf("Clean test namespace %s", netcniparameters.TestNamespace))
	err = namespaces.Clean(
		generalParam.SriovOperatorNamespace,
		netcniparameters.TestNamespace,
		helper.Apiclient,
		false)
	Expect(err).ToNot(HaveOccurred())

	By("Waiting until SRIOV become stable")
	helper.WaitForSRIOVStable(generalParam.SriovOperatorNamespace, netcniparameters.WaitingTime, snoTimeoutMultiplier)

	By("Verify Node Interface Naming Convention")

	err = nethelper.CompareNodeSriovInterfaces(sriovInfos)
	Expect(err).ToNot(HaveOccurred())

	By("Define SRIOV Policies")

	var sriovPolicyList []*sriovv1.SriovNetworkNodePolicy

	for idx, resourceName := range resourceNameRange {
		sriovPolicyList = append(sriovPolicyList, helper.DefineSriovPolicy(
			fmt.Sprintf("%s%d", netcniparameters.SriovPolicyName, idx),
			generalParam.SriovOperatorNamespace,
			validSriovInterfaces[idx],
			5,
			"#0-4",
			1500,
			resourceName,
			"netdevice"))
	}

	for _, sriovPolicy := range sriovPolicyList {
		err = helper.Apiclient.Create(context.Background(), sriovPolicy)
		Expect(err).ToNot(HaveOccurred())
	}

	By("Define SRIOV Networks")

	ipam := fmt.Sprintf(`{"type": "%s"}`, ipamType)
	err = nethelper.CreateSriovNetwork(
		helper.Apiclient,
		sriovInterfaces[sriovNetworkInterfaceIndexRed],
		netcniparameters.TestSriovNetworkRed,
		netcniparameters.TestNamespace,
		generalParam.SriovOperatorNamespace,
		resourceNameVrfRed,
		ipam,
		DefineSriovNetworkMetaPluginsVRFConfig(netcniparameters.VRFRedName))
	Expect(err).ToNot(HaveOccurred())

	err = nethelper.CreateSriovNetwork(
		helper.Apiclient,
		sriovInterfaces[sriovNetworkInterfaceIndexBlue],
		netcniparameters.TestSriovNetworkBlue,
		netcniparameters.TestNamespace,
		generalParam.SriovOperatorNamespace,
		resourceNameVrfBlue,
		ipam,
		DefineSriovNetworkMetaPluginsVRFConfig(netcniparameters.VRFBlueName))
	Expect(err).ToNot(HaveOccurred())

	By("Waiting until SRIOV become stable")
	helper.WaitForSRIOVStable(generalParam.SriovOperatorNamespace, netcniparameters.WaitingTime, snoTimeoutMultiplier)

	By("Waiting until SRIOV resources become available")
	helper.ValidateSriovVFsAvailableOnNodes(
		sriovInfos.Nodes,
		sriovPolicyList,
		5)

	By("Waiting until SRIOV-NAD become available")

	for _, sriovNetworkName := range []string{
		netcniparameters.TestSriovNetworkRed, netcniparameters.TestSriovNetworkBlue} {
		Eventually(func() error {
			netAttDef := &netattdefv1.NetworkAttachmentDefinition{}

			return helper.Apiclient.Get(
				context.Background(),
				runtimeclient.ObjectKey{
					Name:      sriovNetworkName,
					Namespace: netcniparameters.TestNamespace},
				netAttDef)
		}, 60*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	}
}
