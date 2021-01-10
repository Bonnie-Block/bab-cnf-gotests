package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"

	networkHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/helper"
	sriovHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
)

var _ = Describe("CNF VRF", func() {
	describe := func(node string, ipStack string) string {
		VRFParameters, err := parameters.NewVRFTestParameters(node, ipStack)
		if err != nil {
			return fmt.Sprintf("error in parameters: node=%s, ipStack=%s", node, ipStack)
		}
		params, err := json.Marshal(VRFParameters)
		if err != nil {
			return fmt.Sprintf("error in parameters: node=%s, ipStack=%s", node, ipStack)
		}
		return fmt.Sprintf("%s", string(params))
	}
	config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())

	By("Discover SRIOV Nodes")
	sriovInfos, err := cluster.DiscoverSriov(apiclient, networkHelper.SriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred())

	execute.BeforeAll(func() {
		By(fmt.Sprintf("Clean test namespace %s", parameters.TestNamespace))
		namespaces.Clean(networkHelper.SriovOperatorNamespace, parameters.TestNamespace, apiclient, false)

		By("Waiting until SRIOV become stable")
		sriovHelper.WaitForSRIOVStable(apiclient, networkHelper.SriovOperatorNamespace, parameters.WaitingTime)

		sriovInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
		Expect(err).ToNot(HaveOccurred())

		By("Define SRIOV Policy")
		usualSriovPolicyConfig := sriovHelper.DefineSriovPolicy(parameters.SriovPolicyName, sriovInterfaces[0],
			5, "#0-4", 1500, parameters.ResourceNameVRF, "netdevice")

		err = apiclient.Create(context.Background(), usualSriovPolicyConfig)
		Expect(err).ToNot(HaveOccurred())

		By("Define SRIOV Networks")
		ipam := `{"type": "static"}`
		err = networkHelper.CreateSriovNetwork(apiclient, sriovInterfaces[0], parameters.TestSriovNetworkRed, parameters.TestNamespace,
			networkHelper.SriovOperatorNamespace, parameters.ResourceNameVRF, ipam, defineSriovNetworkMetaPluginsVRFConfig(parameters.VRFRedName))
		Expect(err).ToNot(HaveOccurred())

		err = networkHelper.CreateSriovNetwork(apiclient, sriovInterfaces[0], parameters.TestSriovNetworkBlue, parameters.TestNamespace,
			networkHelper.SriovOperatorNamespace, parameters.ResourceNameVRF, ipam, defineSriovNetworkMetaPluginsVRFConfig(parameters.VRFBlueName))
		Expect(err).ToNot(HaveOccurred())

		By("Waiting until SRIOV become stable")
		sriovHelper.WaitForSRIOVStable(apiclient, networkHelper.SriovOperatorNamespace, parameters.WaitingTime)

		By("Waiting until SRIOV resources become available")
		sriovHelper.ValidateSriovVFsAvailableOnNodes(apiclient, sriovInfos.Nodes,
			[]*sriovv1.SriovNetworkNodePolicy{usualSriovPolicyConfig}, 5)

		Eventually(func() error {
			netAttDef := &netattdefv1.NetworkAttachmentDefinition{}
			return apiclient.Get(context.Background(), runtimeclient.ObjectKey{Name: parameters.TestSriovNetworkRed,
				Namespace: parameters.TestNamespace}, netAttDef)
		}, 60*time.Second, 1*time.Second).ShouldNot(HaveOccurred())

		Eventually(func() error {
			netAttDef := &netattdefv1.NetworkAttachmentDefinition{}
			return apiclient.Get(context.Background(), runtimeclient.ObjectKey{Name: parameters.TestSriovNetworkBlue,
				Namespace: parameters.TestNamespace}, netAttDef)
		}, 60*time.Second, 1*time.Second).ShouldNot(HaveOccurred())

	})

	AfterEach(func() {
		By("Cleaning up resources after test")
		err := namespaces.CleanPods(parameters.TestNamespace, apiclient)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() bool {
			podsList, err := apiclient.Pods(parameters.TestNamespace).List(context.Background(), metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred())
			if len(podsList.Items) > 0 {
				return false
			}
			return true

		}, 3*time.Minute, 10*time.Second).Should(BeTrue())
	})

	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs OCP Primary network overlap",
		func(node string, ipStack string) {
			helper.TestVRFScenario(apiclient, node, ipStack, config, sriovInfos.Nodes,
				parameters.TestSriovNetworkBlue, parameters.TestSriovNetworkRed)
		},
		Entry(describe, parameters.SameNode, parameters.IPStackIPv4),
		Entry(describe, parameters.DiffNode, parameters.IPStackIPv4))
})

func defineSriovNetworkMetaPluginsVRFConfig(VRFName string) func(network *sriovv1.SriovNetwork) {
	return func(network *sriovv1.SriovNetwork) {
		network.Spec.MetaPluginsConfig = fmt.Sprintf(`{"type": "vrf", "vrfname": "%s"}`, VRFName)
	}
}
