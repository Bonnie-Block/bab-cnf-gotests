package vrf

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"

	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcnihelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	generalParam "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
)

var _ = Describe("CNF VRF", func() {

	describe := netcnihelper.DescribeParameters

	var (
		sriovInfos    *cluster.EnabledNodes
		testSetupFail = true
	)

	execute.BeforeAll(func() {
		By("Discover SRIOV Node Interfaces")
		var err error
		sriovInfos, err = cluster.DiscoverSriov(generalHelper.Apiclient, generalParam.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "error to discover SR-IOV interfaces")
		SetupSriovBeforeAll(generalHelper.Config, sriovInfos, netcniparameters.VRFIpamStatic, false)
		testSetupFail = false
	})

	BeforeEach(func() {
		if testSetupFail {
			Fail("Test failed due to error in BeforeAll")
		}

		err := namespaces.CleanPodAndWaitUntilItsEmpty(generalHelper.Apiclient, netcniparameters.TestNamespace)
		Expect(err).ToNot(HaveOccurred(), "failed to remove pods")
	})

	// 36303
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs OCP Primary network overlap",
		polarion.ID("36303"),
		func(node string, ipStack string) {
			vrfClientNetConfig, vrfServerNetConfig := defineClientServerVRFsIPOverlapConfig(
				netcniparameters.TestSriovNetworkRed, netcniparameters.TestSriovNetworkBlue, node, sriovInfos.Nodes)
			if ipStack == netcniparameters.IPStackIPv6 {
				Skip("Skipping SDN IPv6 is not currently tested")
			}
			testVRFScenario(
				node,
				ipStack,
				netcniparameters.VRFIpamStatic,
				generalHelper.Config,
				sriovInfos.Nodes,
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

	// 36311
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs ip network overlap",
		polarion.ID("36311"),
		func(node string, ipStack string) {
			vrfClientNetConfig, vrfServerNetConfig := netcnihelper.DefineClientServerVRFsIPConfig(
				netcniparameters.TestSriovNetworkRed, netcniparameters.TestSriovNetworkBlue, "overLapToVRF", ipStack)
			testVRFScenario(
				node,
				ipStack,
				netcniparameters.VRFIpamStatic,
				generalHelper.Config,
				sriovInfos.Nodes,
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

	// 36319
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs Different IP networks",
		polarion.ID("36319"),
		func(node string, ipStack string) {
			vrfClientNetConfig, vrfServerNetConfig := netcnihelper.DefineClientServerVRFsIPConfig(
				netcniparameters.TestSriovNetworkRed, netcniparameters.TestSriovNetworkBlue, "overLapToVRF", ipStack)
			testVRFScenario(
				node,
				ipStack,
				netcniparameters.VRFIpamStatic,
				generalHelper.Config,
				sriovInfos.Nodes,
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
