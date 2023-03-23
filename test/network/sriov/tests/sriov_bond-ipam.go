package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovparameters"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CNF SRIOV: Bond CNI.", func() {
	describe := netsriovhelper.DescribeSRIOVParameters
	var (
		sriovInfos *cluster.EnabledNodes
		err        error
		testFail   string
	)

	execute.BeforeAll(func() {
		By("Discover SRIOV interfaces")
		sriovInfos, err = cluster.DiscoverSriov(
			Apiclient,
			generalParameters.SriovOperatorNamespace)
		if err != nil {
			testFail = fmt.Sprintf("Error discover SRIOV node info: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
	})

	AfterEach(func() {
		By("Cleaning up resources after test")
		err = namespaces.CleanPods(netsriovparameters.OperatorTestNamespace, Apiclient)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() bool {
			podsList, err := Apiclient.Pods(
				netsriovparameters.OperatorTestNamespace).List(context.Background(), metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred())

			return len(podsList.Items) == 0
		}, 3*time.Minute, 10*time.Second).Should(BeTrue())
	})

	Context("ipam-type:", func() {
		BeforeEach(func() {
			if testFail != "" {
				Fail(testFail)
			}
		})
		AfterEach(func() {
			err := nethelper.DeleteNADs(netsriovparameters.OperatorTestNamespace,
				netsriovparameters.BondNadName)
			Expect(err).ToNot(HaveOccurred())
		})

		DescribeTable(
			"whereabouts, Ip Stack: IPv6", polarion.ID("47070"),
			func(mtu int, protocol string, connectivity string, bond bool) {
				netsriovhelper.TestBondScenario(
					mtu,
					sriovInfos,
					protocol,
					connectivity,
					netsriovparameters.BondModeActiveBackup,
					netsriovparameters.ServerPodIpv6,
					netsriovparameters.ClientPodIPv6,
					netsriovparameters.IpamWhereabouts)
			},
			netsriovhelper.BuildTableEntries(
				sriovSmokeTestMode,
				describe,
				true,
				[]int{
					netsriovparameters.MTUCustom,
					netsriovparameters.MTUJumbo,
					netsriovparameters.MTUStandard,
				},
				[]string{
					netsriovparameters.ConnectivityDiffNodeDiffPF,
					netsriovparameters.ConnectivityDiffNodeSamePF,
				},
				[]string{
					netsriovparameters.CommunicationProtocolUnicastICMP,
					netsriovparameters.CommunicationProtocolUnicastTCP,
				},
			),
		)
	})
})
