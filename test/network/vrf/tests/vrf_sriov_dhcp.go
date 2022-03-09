package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/netvrfhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/netvrfparameters"
	generalParam "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
)

var _ = Describe("CNF VRF", func() {

	describe := netvrfhelper.DescribeParameters

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
		netvrfhelper.SetupSriovBeforeAll(generalHelper.Config, sriovInfos, netvrfparameters.VRFIpamDHCP, false)
	})

	BeforeEach(func() {
		if testFail != "" {
			Fail(testFail)
		}
		netvrfhelper.CleanResources()
	})

	// 36323
	DescribeTable("Integration: SRIOV, IPAM: dynamic, Interfaces: 1, Scheme: 2 Pods 2 VRFs ip network overlap",
		func(node string, ipStack string) {
			netvrfhelper.TestVRFScenario(
				node,
				ipStack,
				"overLapToVRF",
				generalHelper.Config,
				sriovInfos.Nodes,
				netvrfparameters.TestSriovNetworkBlue,
				netvrfparameters.TestSriovNetworkRed,
				netvrfparameters.VRFIpamDHCP)
		},
		Entry(describe, netvrfparameters.SameNode, netvrfparameters.IPStackIPv4),
		Entry(describe, netvrfparameters.DiffNode, netvrfparameters.IPStackIPv4),
	)
})
