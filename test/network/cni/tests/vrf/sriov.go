package vrf

import (
	"fmt"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/tests"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcnihelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	generalParam "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
)

var _ = Describe("CNF VRF", func() {

	describe := netcnihelper.DescribeParameters

	var (
		sriovInfos *cluster.EnabledNodes
		testFail   = ""
	)

	execute.BeforeAll(func() {
		By("Discover SRIOV Node Interfaces")
		var err error
		sriovInfos, err = cluster.DiscoverSriov(generalHelper.Apiclient, generalParam.SriovOperatorNamespace)
		if err != nil {
			testFail = fmt.Sprintf("Error discover SRIOV node info: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
		SetupSriovBeforeAll(generalHelper.Config, sriovInfos, netcniparameters.VRFIpamStatic, false)
	})

	BeforeEach(func() {
		if testFail != "" {
			Fail(testFail)
		}
		tests.CleanPodFromNamespaceAndWaitUntilItsEmpty()
	})

	// 36303
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs OCP Primary network overlap",
		func(node string, ipStack string) {
			netcnihelper.TestVRFScenario(
				node,
				ipStack,
				"overLapToSDN",
				generalHelper.Config,
				sriovInfos.Nodes,
				netcniparameters.TestSriovNetworkBlue,
				netcniparameters.TestSriovNetworkRed,
				netcniparameters.VRFIpamStatic)
		},
		Entry(describe, netcniparameters.SameNode, netcniparameters.IPStackIPv4),
		Entry(describe, netcniparameters.DiffNode, netcniparameters.IPStackIPv4),
	)

	// 36311
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs ip network overlap",
		func(node string, ipStack string) {
			netcnihelper.TestVRFScenario(
				node,
				ipStack,
				"overLapToVRF",
				generalHelper.Config,
				sriovInfos.Nodes,
				netcniparameters.TestSriovNetworkBlue,
				netcniparameters.TestSriovNetworkRed,
				netcniparameters.VRFIpamStatic)
		},
		Entry(describe, netcniparameters.SameNode, netcniparameters.IPStackIPv4),
		Entry(describe, netcniparameters.DiffNode, netcniparameters.IPStackIPv4),
		Entry(describe, netcniparameters.SameNode, netcniparameters.IPStackIPv6),
		Entry(describe, netcniparameters.DiffNode, netcniparameters.IPStackIPv6),
	)

	// 36319
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs Different IP networks",
		func(node string, ipStack string) {
			netcnihelper.TestVRFScenario(
				node,
				ipStack,
				"nonOverLap",
				generalHelper.Config,
				sriovInfos.Nodes,
				netcniparameters.TestSriovNetworkBlue,
				netcniparameters.TestSriovNetworkRed,
				netcniparameters.VRFIpamStatic)
		},
		Entry(describe, netcniparameters.SameNode, netcniparameters.IPStackIPv4),
		Entry(describe, netcniparameters.DiffNode, netcniparameters.IPStackIPv4),
		Entry(describe, netcniparameters.SameNode, netcniparameters.IPStackIPv6),
		Entry(describe, netcniparameters.DiffNode, netcniparameters.IPStackIPv6),
	)
})
