package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
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
		By("Discover SRIOV interfaces")
		sriovInfos, err = cluster.DiscoverSriov(
			Apiclient,
			generalParameters.SriovOperatorNamespace)
		if err != nil {
			testFail = fmt.Sprintf("Error discover SRIOV node info: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
		err = netsriovhelper.CreateBondNad(netsriovparameters.NADBondName, "active-backup")
		if err != nil {
			testFail = fmt.Sprintf("Failed create Bond NAD: %s", err)
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
		"Bond CNI. Bond-mode: active-backup",
		func(mtu int, protocol string, connectivity string, bond bool) {
			netsriovhelper.TestBondModeScenario(
				mtu,
				protocol,
				connectivity,
				sriovInfos,
				"active-backup")
		},
		netsriovhelper.BuildTableEntries(
			sriovSmokeTestMode,
			describe,
			true,
			[]int{
				// Waiting for a bug fix Bug 2030677
				// netsriovparameters.MTUCustom,
				// netsriovparameters.MTUJumbo,
				netsriovparameters.MTUStandart,
			},
			[]string{
				netsriovparameters.ConnectivityDiffNodeDiffPF,
				netsriovparameters.ConnectivityDiffNodeSamePF,
			},
			[]string{
				netsriovparameters.CommunicationProtocolUnicastICMP,
				netsriovparameters.CommunicationProtocolUnicastTCP,
			},
		)...,
	)
})
