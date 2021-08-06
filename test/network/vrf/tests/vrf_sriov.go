package tests

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	generalParam "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"

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
		networkvrfhelper.SetupSriovBeforeAll(config, sriovInfos, false)
	})

	BeforeEach(func() {
		networkvrfhelper.CleanResources()
	})

	//36303
	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs OCP Primary network overlap",
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

	DescribeTable("Integration: SRIOV, IPAM: static, Interfaces: 1, Scheme: 2 Pods 2 VRFs ip network overlap",
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
})
