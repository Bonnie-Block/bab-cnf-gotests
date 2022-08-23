package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovparameters"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		err = namespaces.CleanPods(netsriovparameters.OperatorTestNamespace, Apiclient)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() bool {
			podsList, err := Apiclient.Pods(
				netsriovparameters.OperatorTestNamespace).List(context.Background(), metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred())

			return len(podsList.Items) == 0
		}, 3*time.Minute, 10*time.Second).Should(BeTrue())
	})

	DescribeTable(
		"Ipam type: IP Static, Ip Stack: dual-stack, Mac address: MAC static",
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
		"Ipam type: IP Static, Ip Stack: dual-stack, Mac address: MAC dynamic",
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
