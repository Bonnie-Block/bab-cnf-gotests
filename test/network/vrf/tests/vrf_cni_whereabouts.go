package tests

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/netvrfhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/netvrfparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
)

var _ = Describe("CNF VRF", func() {
	describe := netvrfhelper.DescribeParameters

	var (
		nodeListString    []string
		vrfBlueRange1     netattdefv1.NetworkAttachmentDefinition
		vrfRedRange1      netattdefv1.NetworkAttachmentDefinition
		vrfIPv6BlueRange1 netattdefv1.NetworkAttachmentDefinition
		vrfIPv6RedRange1  netattdefv1.NetworkAttachmentDefinition
		testFail          = ""
		vrfBlueIPs        []string
		vrfRedIPs         []string
	)

	execute.BeforeAll(func() {
		nodeListString = generalHelper.GetNodeListStringByLabel(
			strings.Split(generalHelper.Config.General.CnfNodeLabel, "/")[1],
		)
		By(fmt.Sprintf("Create %s namespace", netvrfparameters.TestNamespace))
		err := namespaces.Create(netvrfparameters.TestNamespace, generalHelper.Apiclient)
		if err != nil {
			testFail = fmt.Sprintf("Error to create namespace %s: %s", netvrfparameters.TestNamespace, err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
		validMacVlanInterfaces := netvrfhelper.GetNodeValidMacVlanInterface(nodeListString[0], generalHelper.Config, 1)
		By("Adding NADs")
		vrfBlueRange1 = netvrfhelper.AddVRFNad(
			"test-vrf-blue-1",
			validMacVlanInterfaces[0].Name,
			netvrfparameters.VRFBlueName,
			netvrfparameters.IpamWhereabouts,
			netvrfparameters.WhereaboutsV4Range1)
		vrfRedRange1 = netvrfhelper.AddVRFNad(
			"test-vrf-red-1",
			validMacVlanInterfaces[0].Name,
			netvrfparameters.VRFRedName,
			netvrfparameters.IpamWhereabouts,
			netvrfparameters.WhereaboutsV4Range1)

		vrfIPv6BlueRange1 = netvrfhelper.AddVRFNad(
			"test-vrf-blue-2",
			validMacVlanInterfaces[0].Name,
			netvrfparameters.VRFBlueName,
			netvrfparameters.IpamWhereabouts,
			netvrfparameters.WhereaboutsV6Range1)
		vrfIPv6RedRange1 = netvrfhelper.AddVRFNad(
			"test-vrf-red-2",
			validMacVlanInterfaces[0].Name,
			netvrfparameters.VRFRedName,
			netvrfparameters.IpamWhereabouts,
			netvrfparameters.WhereaboutsV6Range1)

		vrfBlueIPs = append(vrfBlueIPs, vrfBlueRange1.Name, vrfIPv6BlueRange1.Name)
		vrfRedIPs = append(vrfRedIPs, vrfRedRange1.Name, vrfIPv6RedRange1.Name)
	})

	BeforeEach(func() {
		if testFail != "" {
			Fail(testFail)
		}
		By("Cleaning up resources before test")
		err := namespaces.CleanPods(netvrfparameters.TestNamespace, generalHelper.Apiclient)
		Expect(err).ToNot(HaveOccurred())
	})

	// 36325
	DescribeTable("Integration: NAD, IPAM: Whereabouts, Interfaces: 1, Scheme: 2 Pods 2 VRFs ip network overlap",
		func(node string, ipStack string) {
			netvrfhelper.TestVRFWAIPScenario(
				node,
				ipStack,
				generalHelper.Config,
				nodeListString,
				vrfBlueIPs,
				vrfRedIPs)
		},

		Entry(describe, netvrfparameters.SameNode, netvrfparameters.IPStackIPv4),
		Entry(describe, netvrfparameters.DiffNode, netvrfparameters.IPStackIPv4),
		Entry(describe, netvrfparameters.SameNode, netvrfparameters.IPStackIPv6),
		Entry(describe, netvrfparameters.DiffNode, netvrfparameters.IPStackIPv6),
	)
})
