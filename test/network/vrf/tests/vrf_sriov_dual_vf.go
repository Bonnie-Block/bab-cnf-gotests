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

	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	networkHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/parameters"
	generalParam "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
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
	var sriovInfos *cluster.EnabledNodes
	Expect(err).ToNot(HaveOccurred())

	execute.BeforeAll(func() {
		By("Discover SRIOV Node Interfaces")
		sriovInfos, err = cluster.DiscoverSriov(generalHelper.Apiclient, generalParameters.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred())

		numInterfaces := sriovInfos.States
		if len(numInterfaces) < 2 {
			Skip(fmt.Sprintf("There is not enough sriov interfaces to run this test"))
		}

		By(fmt.Sprintf("Clean test namespace %s", parameters.TestNamespace))
		namespaces.Clean(generalParameters.SriovOperatorNamespace, parameters.TestNamespace, generalHelper.Apiclient, false)

		By("Waiting until SRIOV become stable")
		generalHelper.WaitForSRIOVStable(generalParameters.SriovOperatorNamespace, parameters.WaitingTime)

		By("Verify Node Interface Naming Convention")
		err = networkHelper.CompareNodeSriovInterfaces(sriovInfos)
		Expect(err).ToNot(HaveOccurred())

		By("Define SRIOV Policies")
		sriovInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
		Expect(err).ToNot(HaveOccurred())

		usualSriovPolicyConfig1 := generalHelper.DefineSriovPolicy(parameters.SriovPolicyName, generalParam.SriovOperatorNamespace, sriovInterfaces[0],
			5, "#0-4", 1500, parameters.ResourceNameVRFVf1, "netdevice")
		usualSriovPolicyConfig2 := generalHelper.DefineSriovPolicy(parameters.SriovPolicyName, generalParam.SriovOperatorNamespace, sriovInterfaces[1],
			5, "#0-4", 1500, parameters.ResourceNameVRFVf2, "netdevice")

		err = generalHelper.Apiclient.Create(context.Background(), usualSriovPolicyConfig1)
		Expect(err).ToNot(HaveOccurred())
		err = generalHelper.Apiclient.Create(context.Background(), usualSriovPolicyConfig2)
		Expect(err).ToNot(HaveOccurred())

		By("Define SRIOV Networks")
		ipam := `{"type": "static"}`
		err = networkHelper.CreateSriovNetwork(generalHelper.Apiclient, sriovInterfaces[0], parameters.TestSriovNetworkRed, parameters.TestNamespace,
			generalParameters.SriovOperatorNamespace, parameters.ResourceNameVRFVf1, ipam, helper.DefineSriovNetworkMetaPluginsVRFConfig(parameters.VRFRedName))
		Expect(err).ToNot(HaveOccurred())

		err = networkHelper.CreateSriovNetwork(generalHelper.Apiclient, sriovInterfaces[1], parameters.TestSriovNetworkBlue, parameters.TestNamespace,
			generalParameters.SriovOperatorNamespace, parameters.ResourceNameVRFVf2, ipam, helper.DefineSriovNetworkMetaPluginsVRFConfig(parameters.VRFBlueName))
		Expect(err).ToNot(HaveOccurred())

		By("Waiting until SRIOV become stable")
		generalHelper.WaitForSRIOVStable(generalParam.SriovOperatorNamespace, parameters.WaitingTime)

		By("Waiting until SRIOV resources become available")
		generalHelper.ValidateSriovVFsAvailableOnNodes(sriovInfos.Nodes,
			[]*sriovv1.SriovNetworkNodePolicy{usualSriovPolicyConfig1}, 5)
		generalHelper.ValidateSriovVFsAvailableOnNodes(sriovInfos.Nodes,
			[]*sriovv1.SriovNetworkNodePolicy{usualSriovPolicyConfig2}, 5)

		Eventually(func() error {
			netAttDef := &netattdefv1.NetworkAttachmentDefinition{}
			return generalHelper.Apiclient.Get(context.Background(), runtimeclient.ObjectKey{Name: parameters.TestSriovNetworkRed,
				Namespace: parameters.TestNamespace}, netAttDef)
		}, 60*time.Second, 1*time.Second).ShouldNot(HaveOccurred())

		Eventually(func() error {
			netAttDef := &netattdefv1.NetworkAttachmentDefinition{}
			return generalHelper.Apiclient.Get(context.Background(), runtimeclient.ObjectKey{Name: parameters.TestSriovNetworkBlue,
				Namespace: parameters.TestNamespace}, netAttDef)
		}, 60*time.Second, 1*time.Second).ShouldNot(HaveOccurred())

	})

	BeforeEach(func() {
		By("Cleaning up resources before test")
		err := namespaces.CleanPods(parameters.TestNamespace, generalHelper.Apiclient)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() bool {
			podsList, err := generalHelper.Apiclient.Pods(parameters.TestNamespace).List(context.Background(), metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred())
			if len(podsList.Items) > 0 {
				return false
			}
			return true

		}, 3*time.Minute, 10*time.Second).Should(BeTrue())
	})
	//36304
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 2, Scheme: 2 Pods 2 VRFs 2 VFs",
		func(node string, ipStack string) {
			helper.TestVRFScenario(generalHelper.Apiclient, node, ipStack, "overLapToVRF", config, sriovInfos.Nodes,
				parameters.TestSriovNetworkBlue, parameters.TestSriovNetworkRed)
		},
		Entry(describe, parameters.SameNode, parameters.IPStackIPv4),
		Entry(describe, parameters.DiffNode, parameters.IPStackIPv4),
		Entry(describe, parameters.SameNode, parameters.IPStackIPv6),
		Entry(describe, parameters.DiffNode, parameters.IPStackIPv6),
	)
})
