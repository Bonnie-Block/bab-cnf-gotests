package vrf

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcnihelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nad"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
)

var _ = Describe("CNF VRF", func() {
	describe := netcnihelper.DescribeParameters

	var (
		nodeListString    []string
		vrfBlueRange1     netattdefv1.NetworkAttachmentDefinition
		vrfRedRange1      netattdefv1.NetworkAttachmentDefinition
		vrfIPv6BlueRange1 netattdefv1.NetworkAttachmentDefinition
		vrfIPv6RedRange1  netattdefv1.NetworkAttachmentDefinition
		testSetupFail     = true
		vrfBlueIPs        []string
		vrfRedIPs         []string
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
		vrfBlueRange1 = netcnihelper.AddVRFNad(
			"test-vrf-blue-1",
			validMacVlanInterfaces[0].Name,
			netcniparameters.VRFBlueName,
			nad.DefineIpamWhereabouts(
				fmt.Sprintf("%s-%s/24", netcniparameters.VRFBlueClientIPAddress, netcniparameters.VRFBlueServerIPAddress)))
		vrfRedRange1 = netcnihelper.AddVRFNad(
			"test-vrf-red-1",
			validMacVlanInterfaces[0].Name,
			netcniparameters.VRFRedName,
			nad.DefineIpamWhereabouts(
				fmt.Sprintf("%s-%s/24", netcniparameters.VRFRedClientIPAddress, netcniparameters.VRFRedServerIPAddress)))
		vrfIPv6BlueRange1 = netcnihelper.AddVRFNad(
			"test-vrf-blue-2",
			validMacVlanInterfaces[0].Name,
			netcniparameters.VRFBlueName,
			nad.DefineIpamWhereabouts(fmt.Sprintf("%s-%s/64",
				netcniparameters.VRFBlueClientIPv6Address, netcniparameters.VRFBlueServerIPv6Address)))

		vrfIPv6RedRange1 = netcnihelper.AddVRFNad(
			"test-vrf-red-2",
			validMacVlanInterfaces[0].Name,
			netcniparameters.VRFRedName,
			nad.DefineIpamWhereabouts(fmt.Sprintf("%s-%s/64",
				netcniparameters.VRFRedClientIPv6Address, netcniparameters.VRFRedServerIPv6Address)))

		vrfBlueIPs = append(vrfBlueIPs, vrfBlueRange1.Name, vrfIPv6BlueRange1.Name)
		vrfRedIPs = append(vrfRedIPs, vrfRedRange1.Name, vrfIPv6RedRange1.Name)
		testSetupFail = false
	})

	BeforeEach(func() {
		if testSetupFail {
			Fail("Test failed due to error in BeforeAll")
		}
		By("Cleaning up resources before test")
		err := namespaces.CleanPods(netcniparameters.TestNamespace, generalHelper.Apiclient)
		Expect(err).ToNot(HaveOccurred())
		nethelper.WaitUntilIPPoolIsEmpty(fmt.Sprintf("%s-%s", "10.255.255.0", netparameters.IPSubnet24))
		nethelper.WaitUntilIPPoolIsEmpty(fmt.Sprintf("2001-100---%s", netparameters.IPSubnet64))
	})

	// 49731
	DescribeTable("Integration: NAD, IPAM: Whereabouts, Interfaces: 1, Scheme: 2 Pods 2 VRFs ip network overlap",
		polarion.ID("49731"),
		func(node string, ipStack string) {
			vrfRedRangeName := vrfRedRange1.Name
			vrfBlueRangeName := vrfBlueRange1.Name
			if ipStack == netcniparameters.IPStackIPv6 {
				vrfRedRangeName = vrfIPv6RedRange1.Name
				vrfBlueRangeName = vrfIPv6BlueRange1.Name
			}
			vrfClientNetConfig, vrfServerNetConfig := netcnihelper.DefineClientServerVRFsIPConfig(
				vrfRedRangeName, vrfBlueRangeName, "overLapToVRF", ipStack)
			testVRFScenario(
				node,
				ipStack,
				netcniparameters.IpamWhereabouts,
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
			polarion.SetProperty("Node", netcniparameters.DiffNode),
			polarion.SetProperty("IPStack", netcniparameters.IPStackIPv6)),
		Entry(describe, netcniparameters.DiffNode, netcniparameters.IPStackIPv6,
			polarion.SetProperty("Node", netcniparameters.SameNode),
			polarion.SetProperty("IPStack", netcniparameters.IPStackIPv6)),
	)
})
