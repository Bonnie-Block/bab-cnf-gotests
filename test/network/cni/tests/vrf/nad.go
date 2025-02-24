package vrf

import (
	"fmt"
	"strings"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nad"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcnihelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
)

var _ = Describe("CNF VRF", func() {

	describe := netcnihelper.DescribeParameters

	var (
		nodeListString []string
		vrfBlue        netattdefv1.NetworkAttachmentDefinition
		vrfRed         netattdefv1.NetworkAttachmentDefinition
		testSetupFail  = true
	)

	execute.BeforeAll(func() {
		nodeListString = generalHelper.GetNodeListStringByLabel(
			strings.Split(generalHelper.Config.General.CnfNodeLabel, "/")[1],
		)
		By(fmt.Sprintf("Create %s namespace", netcniparameters.TestNamespace))
		err := namespaces.Create(netcniparameters.TestNamespace, generalHelper.Apiclient)
		Expect(err).ToNot(HaveOccurred(), "error creating namespace")
		validMacVlanInterfaces := netcnihelper.GetNodeValidMacVlanInterface(nodeListString[0], generalHelper.Config, 1)
		removeSRIOVNetworksAndNADsFromNamespace()

		By("Adding NADs")
		vrfBlue = netcnihelper.AddVRFNad(
			"test-vrf-blue",
			validMacVlanInterfaces[0].Name,
			netcniparameters.VRFBlueName,
			nad.DefineIpam("static"))
		vrfRed = netcnihelper.AddVRFNad(
			"test-vrf-red",
			validMacVlanInterfaces[0].Name,
			netcniparameters.VRFRedName,
			nad.DefineIpam("static"))
		testSetupFail = false
	})

	BeforeEach(func() {
		if testSetupFail {
			Fail("Test failed due to error in BeforeAll")
		}

		By("Cleaning up resources before test")
		err := namespaces.CleanPodAndWaitUntilItsEmpty(generalHelper.Apiclient, netcniparameters.TestNamespace)
		Expect(err).ToNot(HaveOccurred())
	})

	// 36305
	DescribeTable("Integration: NAD, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs OCP Primary network overlap",
		polarion.ID("36305"),
		func(node string, ipStack string) {
			vrfClientNetConfig, vrfServerNetConfig := defineClientServerVRFsIPOverlapConfig(
				vrfRed.Name, vrfBlue.Name, node, nodeListString)

			if ipStack == netcniparameters.IPStackIPv6 {
				Skip("Skipping SDN IPv6 is not currently tested")
			}

			testVRFScenario(
				node,
				ipStack,
				netcniparameters.VRFIpamStatic,
				generalHelper.Config,
				nodeListString,
				vrfClientNetConfig,
				vrfServerNetConfig)
		},
		Entry(describe, netcniparameters.SameNode, netcniparameters.IPStackIPv4,
			polarion.SetProperty("Node", netcniparameters.SameNode),
			polarion.SetProperty("IPStack", netcniparameters.IPStackIPv4)),
		Entry(describe, netcniparameters.DiffNode, netcniparameters.IPStackIPv4,
			polarion.SetProperty("Node", netcniparameters.DiffNode),
			polarion.SetProperty("IPStack", netcniparameters.IPStackIPv4)),
	)

	// 36313
	DescribeTable("Integration: NAD, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs ip network overlap",
		polarion.ID("36313"),
		func(node string, ipStack string) {
			vrfClientNetConfig, vrfServerNetConfig := netcnihelper.DefineClientServerVRFsIPConfig(
				vrfRed.Name, vrfBlue.Name, "overLapToVRF", ipStack)
			testVRFScenario(
				node,
				ipStack,
				netcniparameters.VRFIpamStatic,
				generalHelper.Config,
				nodeListString,
				vrfClientNetConfig,
				vrfServerNetConfig)
		},
		Entry(describe, netcniparameters.SameNode, netcniparameters.IPStackIPv4,
			polarion.SetProperty("Node", netcniparameters.SameNode),
			polarion.SetProperty("IPStack", netcniparameters.IPStackIPv4)),
		Entry(describe, netcniparameters.DiffNode, netcniparameters.IPStackIPv4,
			polarion.SetProperty("Node", netcniparameters.DiffNode),
			polarion.SetProperty("IPStack", netcniparameters.IPStackIPv4)),
		Entry(describe, netcniparameters.SameNode, netcniparameters.IPStackIPv6,
			polarion.SetProperty("Node", netcniparameters.SameNode),
			polarion.SetProperty("IPStack", netcniparameters.IPStackIPv6)),
		Entry(describe, netcniparameters.DiffNode, netcniparameters.IPStackIPv6,
			polarion.SetProperty("Node", netcniparameters.DiffNode),
			polarion.SetProperty("IPStack", netcniparameters.IPStackIPv6)),
	)

	// 36320
	DescribeTable("Integration: NAD, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs Different IP networks",
		polarion.ID("36320"),
		func(node string, ipStack string) {
			vrfClientNetConfig, vrfServerNetConfig := netcnihelper.DefineClientServerVRFsIPConfig(
				vrfRed.Name, vrfBlue.Name, "nonOverLap", ipStack)
			testVRFScenario(
				node,
				ipStack,
				netcniparameters.VRFIpamStatic,
				generalHelper.Config,
				nodeListString,
				vrfClientNetConfig,
				vrfServerNetConfig)
		},
		Entry(describe, netcniparameters.SameNode, netcniparameters.IPStackIPv4,
			polarion.SetProperty("Node", netcniparameters.SameNode),
			polarion.SetProperty("IPStack", netcniparameters.IPStackIPv4)),
		Entry(describe, netcniparameters.DiffNode, netcniparameters.IPStackIPv4,
			polarion.SetProperty("Node", netcniparameters.DiffNode),
			polarion.SetProperty("IPStack", netcniparameters.IPStackIPv4)),
		Entry(describe, netcniparameters.SameNode, netcniparameters.IPStackIPv6,
			polarion.SetProperty("Node", netcniparameters.SameNode),
			polarion.SetProperty("IPStack", netcniparameters.IPStackIPv6)),
		Entry(describe, netcniparameters.DiffNode, netcniparameters.IPStackIPv6,
			polarion.SetProperty("Node", netcniparameters.DiffNode),
			polarion.SetProperty("IPStack", netcniparameters.IPStackIPv6)),
	)
})
