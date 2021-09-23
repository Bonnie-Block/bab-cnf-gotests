package tests

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/networkvrfhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
)

var _ = Describe("CNF VRF", func() {

	describe := networkvrfhelper.DescribeParameters

	var (
		nodeListString []string
		vrfBlue        netattdefv1.NetworkAttachmentDefinition
		vrfRed         netattdefv1.NetworkAttachmentDefinition
		testFail       = ""
	)
	config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())

	execute.BeforeAll(func() {
		nodeListString = networkvrfhelper.GetNodeListStringByLabel(strings.Split(config.General.CnfNodeLabel, "/")[1])

		By(fmt.Sprintf("Create %s namespace", parameters.TestNamespace))
		err = namespaces.Create(parameters.TestNamespace, generalHelper.Apiclient)
		if err != nil {
			testFail = fmt.Sprintf("Error to create namespace %s: %s", parameters.TestNamespace, err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
		validMacVlanInterfaces := networkvrfhelper.GetNodeValidMacVlanInterface(nodeListString[0], config, 2)

		By("Adding NADs")
		vrfBlue = networkvrfhelper.AddVRFNad(
			"test-vrf-blue",
			validMacVlanInterfaces[0].Name,
			parameters.VRFBlueName)
		vrfRed = networkvrfhelper.AddVRFNad(
			"test-vrf-red",
			validMacVlanInterfaces[1].Name,
			parameters.VRFRedName)
	})

	BeforeEach(func() {
		if testFail != "" {
			Fail(testFail)
		}
		By("Cleaning up resources before test")
		err := namespaces.CleanPods(parameters.TestNamespace, generalHelper.Apiclient)
		Expect(err).ToNot(HaveOccurred())
	})
	//36306
	DescribeTable("Integration: NAD, IPAM: static, Interfaces: 2, Scheme: 2 Pods 2 VRFs network overlap",
		func(node string, ipStack string) {
			networkvrfhelper.TestVRFScenario(
				node,
				ipStack,
				"overLapToVRF",
				config,
				nodeListString,
				vrfBlue.Name,
				vrfRed.Name)
		},
		Entry(describe, parameters.SameNode, parameters.IPStackIPv4),
		Entry(describe, parameters.DiffNode, parameters.IPStackIPv4),
		Entry(describe, parameters.SameNode, parameters.IPStackIPv6),
		Entry(describe, parameters.DiffNode, parameters.IPStackIPv6),
	)
	//36314
	DescribeTable("Integration: NAD, IPAM: static, Interfaces: 2, Scheme: 2 Pods 2 VRFs Different IP networks",
		func(node string, ipStack string) {
			networkvrfhelper.TestVRFScenario(
				node,
				ipStack,
				"nonOverLap",
				config,
				nodeListString,
				vrfBlue.Name,
				vrfRed.Name)
		},
		Entry(describe, parameters.SameNode, parameters.IPStackIPv4),
		Entry(describe, parameters.DiffNode, parameters.IPStackIPv4),
		Entry(describe, parameters.SameNode, parameters.IPStackIPv6),
		Entry(describe, parameters.DiffNode, parameters.IPStackIPv6),
	)
	//36300
	DescribeTable("Integration: NAD, IPAM: static, Interfaces: 2, Scheme: 2 Pods 2 VRFs OCP Primary network overlap",
		func(node string, ipStack string) {
			networkvrfhelper.TestVRFScenario(
				node,
				ipStack,
				"overLapToSDN",
				config,
				nodeListString,
				vrfBlue.Name,
				vrfRed.Name)
		},
		Entry(describe, parameters.SameNode, parameters.IPStackIPv4),
		Entry(describe, parameters.DiffNode, parameters.IPStackIPv4),
	)
})
