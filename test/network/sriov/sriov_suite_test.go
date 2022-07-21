package sriov

import (
	"runtime"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2/types"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovhelper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

const (
	timeout = 1800 * time.Second
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestSriov(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = Config.GetReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "SRIOV Operator conformance tests", reporterConfig)
}

var _ = BeforeSuite(func() {
	var snoTimeoutMultiplier time.Duration = 1
	isSingleNode, err := nodes.IsSingleNodeCluster(Apiclient)
	Expect(err).ToNot(HaveOccurred())
	if isSingleNode {
		snoTimeoutMultiplier = 2
	}
	PullTestImage(Config.General.CnfNodeLabel, Config.Network.TestContainerImage)
	sriovInfos, err := cluster.DiscoverSriov(Apiclient, netsriovparameters.OperatorNamespace)
	Expect(err).ToNot(HaveOccurred())
	err = nethelper.CompareNodeSriovInterfaces(sriovInfos)
	Expect(err).ToNot(HaveOccurred())
	err = namespaces.Clean(
		netsriovparameters.OperatorNamespace,
		netsriovparameters.OperatorTestNamespace,
		Apiclient,
		false)
	Expect(err).ToNot(HaveOccurred())
	WaitForSRIOVStable(netsriovparameters.OperatorNamespace, timeout, snoTimeoutMultiplier)
	netsriovhelper.SetupSriovConfig(sriovInfos, snoTimeoutMultiplier)
})

var _ = AfterSuite(func() {
	var snoTimeoutMultiplier time.Duration = 1
	isSingleNode, err := nodes.IsSingleNodeCluster(Apiclient)
	Expect(err).ToNot(HaveOccurred())
	if isSingleNode {
		snoTimeoutMultiplier = 2
		RestoreNodeDrainState(netsriovparameters.OperatorNamespace)
	}
	err = namespaces.Clean(
		netsriovparameters.OperatorNamespace,
		netsriovparameters.OperatorTestNamespace,
		Apiclient,
		false)
	Expect(err).ToNot(HaveOccurred())
	err = namespaces.DeleteAndWait(Apiclient, netsriovparameters.OperatorTestNamespace, timeout)
	Expect(err).ToNot(HaveOccurred())
	WaitForSRIOVStable(netsriovparameters.OperatorNamespace, timeout, snoTimeoutMultiplier)
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, netsriovparameters.ReporterNamespacesToDump,
		netsriovparameters.ReporterCrds)
})
