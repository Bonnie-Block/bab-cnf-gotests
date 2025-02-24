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
		"Ipam type: IP Static, Ip Stack: dual-stack, Mac address: MAC static", polarion.ID("31789"),
		func(mtu int, protocol string, connectivity string, bond bool) {
			netsriovhelper.TestSriovDualScenario(
				mtu,
				protocol,
				connectivity,
				sriovInfos,
				Config,
				netsriovparameters.ClientMacAddress,
				netsriovparameters.ServerMacAddress)
		},
		netsriovhelper.BuildTableEntries(
			sriovSmokeTestMode,
			describe,
			false,
			[]int{netsriovparameters.MTUCustom,
				netsriovparameters.MTUJumbo,
				netsriovparameters.MTUStandard},
			[]string{netsriovparameters.ConnectivityDiffNode,
				netsriovparameters.ConnectivitySameNodeDiffPF,
				netsriovparameters.ConnectivitySameNodeSamePF},
			[]string{
				netsriovparameters.CommunicationProtocolUnicastICMP,
				netsriovparameters.CommunicationProtocolUnicastTCP,
				netsriovparameters.CommunicationProtocolUnicastUDP,
				netsriovparameters.CommunicationProtocolMulticastUDP,
				netsriovparameters.CommunicationProtocolBroadcastUDP,
				netsriovparameters.CommunicationProtocolUnicastSCTP,
			},
		),
	)

	DescribeTable(
		"Ipam type: IP Static, Ip Stack: dual-stack, Mac address: MAC dynamic", polarion.ID("31795"),
		func(mtu int, protocol string, connectivity string, bond bool) {
			netsriovhelper.TestSriovDualScenario(mtu, protocol, connectivity, sriovInfos, Config, "", "")
		},
		netsriovhelper.BuildTableEntries(
			sriovSmokeTestMode,
			describe,
			false,
			[]int{netsriovparameters.MTUCustom,
				netsriovparameters.MTUJumbo,
				netsriovparameters.MTUStandard,
			},
			[]string{netsriovparameters.ConnectivityDiffNode,
				netsriovparameters.ConnectivitySameNodeDiffPF,
				netsriovparameters.ConnectivitySameNodeSamePF,
			},
			[]string{
				netsriovparameters.CommunicationProtocolUnicastICMP,
				netsriovparameters.CommunicationProtocolUnicastTCP,
				netsriovparameters.CommunicationProtocolUnicastUDP,
				netsriovparameters.CommunicationProtocolMulticastUDP,
				netsriovparameters.CommunicationProtocolBroadcastUDP,
				netsriovparameters.CommunicationProtocolUnicastSCTP,
			},
		),
	)
})
