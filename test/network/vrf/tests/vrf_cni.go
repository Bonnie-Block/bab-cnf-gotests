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
		nodeListString []string
		vrfBlue        netattdefv1.NetworkAttachmentDefinition
		vrfRed         netattdefv1.NetworkAttachmentDefinition
		testFail       = ""
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
		vrfBlue = netvrfhelper.AddVRFNad(
			"test-vrf-blue",
			validMacVlanInterfaces[0].Name,
			netvrfparameters.VRFBlueName)
		vrfRed = netvrfhelper.AddVRFNad(
			"test-vrf-red",
			validMacVlanInterfaces[0].Name,
			netvrfparameters.VRFRedName)
	})

	BeforeEach(func() {
		if testFail != "" {
			Fail(testFail)
		}
		By("Cleaning up resources before test")
		err := namespaces.CleanPods(netvrfparameters.TestNamespace, generalHelper.Apiclient)
		Expect(err).ToNot(HaveOccurred())
	})

	// 36305
	DescribeTable("Integration: NAD, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs OCP Primary network overlap",
		func(node string, ipStack string) {
			netvrfhelper.TestVRFScenario(
				node,
				ipStack,
				"overLapToSDN",
				generalHelper.Config,
				nodeListString,
				vrfBlue.Name,
				vrfRed.Name)
		},
		Entry(describe, netvrfparameters.SameNode, netvrfparameters.IPStackIPv4),
		Entry(describe, netvrfparameters.DiffNode, netvrfparameters.IPStackIPv4),
	)

	// 36313
	DescribeTable("Integration: NAD, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs ip network overlap",
		func(node string, ipStack string) {
			netvrfhelper.TestVRFScenario(
				node,
				ipStack,
				"overLapToVRF",
				generalHelper.Config,
				nodeListString,
				vrfBlue.Name,
				vrfRed.Name)
		},
		Entry(describe, netvrfparameters.SameNode, netvrfparameters.IPStackIPv4),
		Entry(describe, netvrfparameters.DiffNode, netvrfparameters.IPStackIPv4),
		Entry(describe, netvrfparameters.SameNode, netvrfparameters.IPStackIPv6),
		Entry(describe, netvrfparameters.DiffNode, netvrfparameters.IPStackIPv6),
	)

	// 36320
	DescribeTable("Integration: NAD, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs Different IP networks",
		func(node string, ipStack string) {
			netvrfhelper.TestVRFScenario(
				node,
				ipStack,
				"nonOverLap",
				generalHelper.Config,
				nodeListString,
				vrfBlue.Name,
				vrfRed.Name)
		},
		Entry(describe, netvrfparameters.SameNode, netvrfparameters.IPStackIPv4),
		Entry(describe, netvrfparameters.DiffNode, netvrfparameters.IPStackIPv4),
		Entry(describe, netvrfparameters.SameNode, netvrfparameters.IPStackIPv6),
		Entry(describe, netvrfparameters.DiffNode, netvrfparameters.IPStackIPv6),
	)
})
