package networkvrfhelper

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
	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	networkHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/parameters"
	generalParam "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

// DefineSriovNetworkMetaPluginsVRFConfig  plugin for discovering and advertising SR-IOV VFs
func DefineSriovNetworkMetaPluginsVRFConfig(VRFName string) func(network *sriovv1.SriovNetwork) {
	return func(network *sriovv1.SriovNetwork) {
		network.Spec.MetaPluginsConfig = fmt.Sprintf(`{"type": "vrf", "vrfname": "%s"}`, VRFName)
	}
}

// CleanResources removes all pods from vrf test namespace
func CleanResources() {
	By("Cleaning up resources before test")
	err := namespaces.CleanPods(parameters.TestNamespace, generalHelper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() bool {
		podsList, err := generalHelper.Apiclient.Pods(parameters.TestNamespace).List(
			context.Background(), metav1.ListOptions{})
		Expect(err).ToNot(HaveOccurred())
		if len(podsList.Items) > 0 {
			return false
		}
		return true

	}, 3*time.Minute, 10*time.Second).Should(BeTrue())
}

// SetupSriovBeforeAll prepare env before test
func SetupSriovBeforeAll(config *config.Config, sriovInfos *cluster.EnabledNodes, dual bool) {
	var (
		resourceNameRange    []string
		snoTimeoutMultiplier time.Duration = 1
	)
	requestedIntefcace := 1
	resourceNameRange = []string{parameters.ResourceNameVRF}
	resourceNameVrfRed := parameters.ResourceNameVRF
	resourceNameVrfBlue := parameters.ResourceNameVRF
	sriovNetworkInterfaceIndexRed := 0
	sriovNetworkInterfaceIndexBlue := 0

	if dual {
		resourceNameRange = []string{parameters.ResourceNameVRFVf1, parameters.ResourceNameVRFVf2}
		requestedIntefcace = 2
		resourceNameVrfRed = parameters.ResourceNameVRFVf1
		resourceNameVrfBlue = parameters.ResourceNameVRFVf2
		sriovNetworkInterfaceIndexRed = 0
		sriovNetworkInterfaceIndexBlue = 1
	}
	isSingleNode, err := nodes.IsSingleNodeCluster(generalHelper.Apiclient)
	Expect(err).ToNot(HaveOccurred())

	if isSingleNode {
		snoTimeoutMultiplier = 2
		disableDrainState := generalHelper.GetNodeDrainState(generalParam.SriovOperatorNamespace)
		if !disableDrainState {
			generalHelper.SetDisableNodeDrainState(true, generalParam.SriovOperatorNamespace)
			generalHelper.ChangedNodeDrainState = true
		}
	}

	sriovInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
	Expect(err).ToNot(HaveOccurred())
	validSriovInterfaces, err := config.GetSriovInterfaces(sriovInterfaces, requestedIntefcace)
	Expect(err).ToNot(HaveOccurred())

	By(fmt.Sprintf("Clean test namespace %s", parameters.TestNamespace))
	namespaces.Clean(
		generalParam.SriovOperatorNamespace,
		parameters.TestNamespace,
		generalHelper.Apiclient,
		false)

	By("Waiting until SRIOV become stable")
	generalHelper.WaitForSRIOVStable(generalParam.SriovOperatorNamespace, parameters.WaitingTime, snoTimeoutMultiplier)

	By("Verify Node Interface Naming Convention")
	err = networkHelper.CompareNodeSriovInterfaces(sriovInfos)
	Expect(err).ToNot(HaveOccurred())

	By("Define SRIOV Policies")
	var sriovPolicyList []*sriovv1.SriovNetworkNodePolicy
	for idx, resourceName := range resourceNameRange {
		sriovPolicyList = append(sriovPolicyList, generalHelper.DefineSriovPolicy(
			fmt.Sprintf("%s%d", parameters.SriovPolicyName, idx),
			generalParam.SriovOperatorNamespace,
			validSriovInterfaces[idx],
			5,
			"#0-4",
			1500,
			resourceName,
			"netdevice"))
	}
	for _, sriovPolicy := range sriovPolicyList {
		err = generalHelper.Apiclient.Create(context.Background(), sriovPolicy)
		Expect(err).ToNot(HaveOccurred())
	}

	By("Define SRIOV Networks")
	ipam := `{"type": "static"}`
	err = networkHelper.CreateSriovNetwork(
		generalHelper.Apiclient,
		sriovInterfaces[sriovNetworkInterfaceIndexRed],
		parameters.TestSriovNetworkRed,
		parameters.TestNamespace,
		generalParam.SriovOperatorNamespace,
		resourceNameVrfRed,
		ipam,
		DefineSriovNetworkMetaPluginsVRFConfig(parameters.VRFRedName))
	Expect(err).ToNot(HaveOccurred())

	err = networkHelper.CreateSriovNetwork(
		generalHelper.Apiclient,
		sriovInterfaces[sriovNetworkInterfaceIndexBlue],
		parameters.TestSriovNetworkBlue,
		parameters.TestNamespace,
		generalParam.SriovOperatorNamespace,
		resourceNameVrfBlue,
		ipam,
		DefineSriovNetworkMetaPluginsVRFConfig(parameters.VRFBlueName))
	Expect(err).ToNot(HaveOccurred())

	By("Waiting until SRIOV become stable")
	generalHelper.WaitForSRIOVStable(generalParam.SriovOperatorNamespace, parameters.WaitingTime, snoTimeoutMultiplier)

	By("Waiting until SRIOV resources become available")
	generalHelper.ValidateSriovVFsAvailableOnNodes(
		sriovInfos.Nodes,
		sriovPolicyList,
		5)

	By("Waiting until SRIOV-NAD become available")
	for _, sriovNetworkName := range []string{parameters.TestSriovNetworkRed, parameters.TestSriovNetworkBlue} {
		Eventually(func() error {
			netAttDef := &netattdefv1.NetworkAttachmentDefinition{}
			return generalHelper.Apiclient.Get(
				context.Background(),
				runtimeclient.ObjectKey{
					Name:      sriovNetworkName,
					Namespace: parameters.TestNamespace},
				netAttDef)
		}, 60*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	}
}
