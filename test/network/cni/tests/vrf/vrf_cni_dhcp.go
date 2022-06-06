package vrf

import (
	"fmt"
	"strings"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcnihelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
)

var _ = Describe("CNF VRF", func() {

	describe := netcnihelper.DescribeParameters

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
		By(fmt.Sprintf("Create %s namespace", netcniparameters.TestNamespace))
		err := namespaces.Create(netcniparameters.TestNamespace, generalHelper.Apiclient)
		if err != nil {
			testFail = fmt.Sprintf("Error to create namespace %s: %s", netcniparameters.TestNamespace, err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
		validMacVlanInterfaces := netcnihelper.GetNodeValidMacVlanInterface(nodeListString[0], generalHelper.Config, 1)

		By("Adding NADs")
		vrfBlue = netcnihelper.AddVRFNad(
			"test-vrf-blue",
			validMacVlanInterfaces[0].Name,
			netcniparameters.VRFBlueName,
			netcniparameters.VRFIpamDHCP,
			"")
		vrfRed = netcnihelper.AddVRFNad(
			"test-vrf-red",
			validMacVlanInterfaces[0].Name,
			netcniparameters.VRFRedName,
			netcniparameters.VRFIpamDHCP,
			"")
	})

	BeforeEach(func() {
		if testFail != "" {
			Fail(testFail)
		}
		By("Cleaning up resources before test")
		err := namespaces.CleanPods(netcniparameters.TestNamespace, generalHelper.Apiclient)
		Expect(err).ToNot(HaveOccurred())
	})

	// 36325
	DescribeTable("Integration: NAD, IPAM: dynamic, Interfaces: 1, Scheme: 2 Pods 2 VRFs ip network overlap",
		func(node string, ipStack string) {
			netcnihelper.TestVRFScenario(
				node,
				ipStack,
				"overLapToVRF",
				generalHelper.Config,
				nodeListString,
				vrfBlue.Name,
				vrfRed.Name,
				netcniparameters.VRFIpamDHCP)
		},
		Entry(describe, netcniparameters.SameNode, netcniparameters.IPStackIPv4),
		Entry(describe, netcniparameters.DiffNode, netcniparameters.IPStackIPv4),
	)
})
