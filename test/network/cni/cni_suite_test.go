package cni

import (
	"fmt"
	"log"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/tests/vrf"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

const (
	waitingTime time.Duration = 20 * time.Minute
	timeout                   = 1800 * time.Second
)

func TestVrf(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	junitPath := generalHelper.Config.GetReportPath(currentFile)
	dumpFile := generalHelper.Config.GetDumpFailedTestReportLocation(currentFile)

	RegisterFailHandler(Fail)
	reporterList := append([]Reporter{}, reporters.NewJUnitReporter(junitPath))

	if dumpFile != "" {
		reporter, err := testutils.NewReporter(
			dumpFile,
			netcniparameters.ReporterNamespacesToDump,
			netcniparameters.ReporterCrds)
		if err != nil {
			log.Fatalf("Failed to create log reporter %s", err)
		}
		reporterList = append(reporterList, reporter)
	}

	RunSpecsWithDefaultAndCustomReporters(t, "CNI tests", reporterList)
}

var _ = BeforeSuite(func() {
	generalHelper.PullTestImage(generalHelper.Config.General.CnfNodeLabel, generalHelper.Config.Network.TestContainerImage)
	By(fmt.Sprintf("Create %s namespace", netcniparameters.TestNamespace))
	err := namespaces.Create(netcniparameters.TestNamespace, generalHelper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	By(fmt.Sprintf("Clean test namespace %s", netcniparameters.TestNamespace))
	var snoTimeoutMultiplier time.Duration = 1
	isSingleNode, err := nodes.IsSingleNodeCluster(generalHelper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
	if isSingleNode {
		snoTimeoutMultiplier = 2
		generalHelper.RestoreNodeDrainState(generalParameters.SriovOperatorNamespace)
	}
	err = namespaces.Clean(
		generalParameters.SriovOperatorNamespace,
		netcniparameters.TestNamespace,
		generalHelper.Apiclient, false)
	Expect(err).ToNot(HaveOccurred())
	err = namespaces.DeleteAndWait(generalHelper.Apiclient, netcniparameters.TestNamespace, timeout)
	Expect(err).ToNot(HaveOccurred())
	By("Waiting until SRIOV become stable")
	generalHelper.WaitForSRIOVStable(generalParameters.SriovOperatorNamespace, waitingTime, snoTimeoutMultiplier)
})
