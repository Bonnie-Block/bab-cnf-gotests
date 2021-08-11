package tests

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	generalParam "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"

	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/networkvrfhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
)

var _ = Describe("CNF VRF", func() {

	describe := networkvrfhelper.DescribeParameters

	var sriovInfos *cluster.EnabledNodes
	config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())

	execute.BeforeAll(func() {
		By("Discover SRIOV Node Interfaces")
		sriovInfos, err = cluster.DiscoverSriov(generalHelper.Apiclient, generalParam.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred())
		networkvrfhelper.SetupSriovBeforeAll(config, sriovInfos, true)
	})

	BeforeEach(func() {
		networkvrfhelper.CleanResources()
	})

	//36299
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 2, Scheme: 2 Pods 2 VRFs OCP Primary network overlap",
		func(node string, ipStack string) {
			networkvrfhelper.TestVRFScenario(
				node,
				ipStack,
				"overLapToSDN",
				config,
				sriovInfos.Nodes,
				parameters.TestSriovNetworkBlue,
				parameters.TestSriovNetworkRed)
		},
		Entry(describe, parameters.SameNode, parameters.IPStackIPv4),
		Entry(describe, parameters.DiffNode, parameters.IPStackIPv4),
	)

	//36308
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 2, Scheme: 2 Pods 2 VRFs ip network overlap",
		func(node string, ipStack string) {
			networkvrfhelper.TestVRFScenario(
				node,
				ipStack,
				"overLapToVRF",
				config,
				sriovInfos.Nodes,
				parameters.TestSriovNetworkBlue,
				parameters.TestSriovNetworkRed)
		},
		Entry(describe, parameters.SameNode, parameters.IPStackIPv4),
		Entry(describe, parameters.DiffNode, parameters.IPStackIPv4),
		Entry(describe, parameters.SameNode, parameters.IPStackIPv6),
		Entry(describe, parameters.DiffNode, parameters.IPStackIPv6),
	)
	//36312
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 2, Scheme: 2 Pods 2 VRFs Different IP networks",
		func(node string, ipStack string) {
			networkvrfhelper.TestVRFScenario(
				node,
				ipStack,
				"nonOverLap",
				config,
				sriovInfos.Nodes,
				parameters.TestSriovNetworkBlue,
				parameters.TestSriovNetworkRed)
		},
		Entry(describe, parameters.SameNode, parameters.IPStackIPv4),
		Entry(describe, parameters.DiffNode, parameters.IPStackIPv4),
		Entry(describe, parameters.SameNode, parameters.IPStackIPv6),
		Entry(describe, parameters.DiffNode, parameters.IPStackIPv6),
	)
})
