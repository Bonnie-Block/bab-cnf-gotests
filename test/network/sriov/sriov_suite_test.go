package sriov

import (
	"fmt"
	"log"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"

	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

const (
	timeout = 1800 * time.Second
)

func TestSriov(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	configSuite, err := config.NewConfig()
	if err != nil {
		fmt.Print(err)
		return
	}
	junitPath := configSuite.GetReportPath(currentFile)
	dumpFile := configSuite.GetDumpFailedTestReportLocation(currentFile)
	RegisterFailHandler(Fail)
	rr := append([]Reporter{}, reporters.NewJUnitReporter(junitPath))
	if dumpFile != "" {
		reporter, err := testutils.NewReporter(
			dumpFile,
			netsriovparameters.ReporterNamespacesToDump,
			netsriovparameters.ReporterCrds)
		if err != nil {
			log.Fatalf("Failed to create log reporter %s", err)
		}
		rr = append(rr, reporter)
	}
	RunSpecsWithDefaultAndCustomReporters(t, "SRIOV Operator conformance tests", rr)
}

var _ = BeforeSuite(func() {
	var snoTimeoutMultiplier time.Duration = 1
	isSingleNode, err := nodes.IsSingleNodeCluster(Apiclient)
	Expect(err).ToNot(HaveOccurred())
	if isSingleNode {
		snoTimeoutMultiplier = 2
	}
	configuration, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())
	PullTestImage(configuration.General.CnfNodeLabel, configuration.Network.TestContainerImage)
	sriovInfos, err := cluster.DiscoverSriov(Apiclient, netsriovparameters.OperatorNamespace)
	Expect(err).ToNot(HaveOccurred())
	err = nethelper.CompareNodeSriovInterfaces(sriovInfos)
	Expect(err).ToNot(HaveOccurred())
	namespaces.Clean(netsriovparameters.OperatorNamespace, netsriovparameters.OperatorTestNamespace, Apiclient, false)
	WaitForSRIOVStable(netsriovparameters.OperatorNamespace, timeout, snoTimeoutMultiplier)
})

var _ = AfterSuite(func() {
	var snoTimeoutMultiplier time.Duration = 1
	isSingleNode, err := nodes.IsSingleNodeCluster(Apiclient)
	Expect(err).ToNot(HaveOccurred())
	if isSingleNode {
		snoTimeoutMultiplier = 2
		RestoreNodeDrainState(netsriovparameters.OperatorNamespace)
	}
	namespaces.Clean(netsriovparameters.OperatorNamespace, netsriovparameters.OperatorTestNamespace, Apiclient, false)
	err = namespaces.DeleteAndWait(Apiclient, netsriovparameters.OperatorTestNamespace, timeout)
	Expect(err).ToNot(HaveOccurred())
	WaitForSRIOVStable(netsriovparameters.OperatorNamespace, timeout, snoTimeoutMultiplier)
})
