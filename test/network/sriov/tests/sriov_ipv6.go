package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovparameters"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
)

var _ = Describe("CNF SRIOV", func() {
	describe := netsriovhelper.DescribeSRIOVParameters

	var (
		sriovInfos *cluster.EnabledNodes
		err        error
		testFail   string
	)

	execute.BeforeAll(func() {
		netsriovhelper.VerifySriovOperatorInstalledAndPreconfigured(namespace, operatorGroup, sriovSubscription)
		By("Discover SRIOV interfaces")
		sriovInfos, err = cluster.DiscoverSriov(
			Apiclient,
			generalParameters.SriovOperatorNamespace)
		if err != nil {
			testFail = fmt.Sprintf("Error discover SRIOV node info: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
	})

	BeforeEach(func() {
		if testFail != "" {
			Fail(testFail)
		}

		By("Cleaning up resources before test")
		err = namespaces.CleanPodAndWaitUntilItsEmpty(Apiclient, netsriovparameters.OperatorTestNamespace)
		Expect(err).ToNot(HaveOccurred())
	})

	DescribeTable(
		"Ipam type: IP Static, Ip Stack: ipv6, Mac address: MAC static", polarion.ID("31794"),
		func(mtu int, protocol string, connectivity string, bond bool) {
			netsriovhelper.TestSriovIPv6Scenario(
				mtu,
				sriovInfos,
				Config,
				protocol,
				connectivity,
				netsriovparameters.ClientMacAddress,
				netsriovparameters.ServerMacAddress,
				netsriovparameters.IpamStatic)
		},
		netsriovhelper.BuildTableEntries(
			sriovSmokeTestMode,
			describe,
			false,
			[]int{
				netsriovparameters.MTUCustom,
				netsriovparameters.MTUJumbo,
				netsriovparameters.MTUStandard,
			},
			[]string{
				netsriovparameters.ConnectivityDiffNode,
				netsriovparameters.ConnectivitySameNodeDiffPF,
				netsriovparameters.ConnectivitySameNodeSamePF,
			},
			[]string{
				netsriovparameters.CommunicationProtocolUnicastICMP,
				netsriovparameters.CommunicationProtocolUnicastTCP,
				netsriovparameters.CommunicationProtocolUnicastUDP,
				netsriovparameters.CommunicationProtocolMulticastUDP,
				netsriovparameters.CommunicationProtocolUnicastSCTP,
			},
		),
	)

	DescribeTable(
		"Ipam type: IP Static, Ip Stack: ipv6, Mac address: MAC dynamic", polarion.ID("31804"),
		func(mtu int, protocol string, connectivity string, bond bool) {
			netsriovhelper.TestSriovIPv6Scenario(mtu, sriovInfos, Config, protocol, connectivity, "", "",
				netsriovparameters.IpamStatic)
		},
		netsriovhelper.BuildTableEntries(
			sriovSmokeTestMode,
			describe,
			false,
			[]int{
				netsriovparameters.MTUCustom,
				netsriovparameters.MTUJumbo,
				netsriovparameters.MTUStandard,
			},
			[]string{
				netsriovparameters.ConnectivityDiffNode,
				netsriovparameters.ConnectivitySameNodeDiffPF,
				netsriovparameters.ConnectivitySameNodeSamePF,
			},
			[]string{
				netsriovparameters.CommunicationProtocolUnicastICMP,
				netsriovparameters.CommunicationProtocolUnicastTCP,
				netsriovparameters.CommunicationProtocolUnicastUDP,
				netsriovparameters.CommunicationProtocolMulticastUDP,
				netsriovparameters.CommunicationProtocolUnicastSCTP,
			},
		),
	)

	// 31807
	DescribeTable(
		"Ipam type: IP whereabouts, Ip Stack: ipv6, Mac address: Dynamic", polarion.ID("61251"),
		func(mtu int, protocol, connectivity string, bond bool) {
			netsriovhelper.TestSriovIPv6Scenario(mtu, sriovInfos, Config, protocol, connectivity, "", "",
				netsriovparameters.IpamWhereabouts)
		},
		netsriovhelper.BuildTableEntries(
			sriovSmokeTestMode,
			describe,
			false,
			[]int{
				netsriovparameters.MTUCustom,
				netsriovparameters.MTUJumbo,
				netsriovparameters.MTUStandard,
			},
			[]string{
				netsriovparameters.ConnectivityDiffNode,
				netsriovparameters.ConnectivitySameNodeDiffPF,
				netsriovparameters.ConnectivitySameNodeSamePF,
			},
			[]string{
				netsriovparameters.CommunicationProtocolUnicastICMP,
				netsriovparameters.CommunicationProtocolUnicastTCP,
				netsriovparameters.CommunicationProtocolUnicastUDP,
				netsriovparameters.CommunicationProtocolMulticastUDP,
				netsriovparameters.CommunicationProtocolUnicastSCTP,
			},
		),
	)

})
