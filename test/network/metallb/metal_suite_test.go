package metallb

import (
	"fmt"
	"log"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

func TestLB(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	junitPath := generalHelper.Config.GetReportPath(currentFile)
	dumpFile := generalHelper.Config.GetDumpFailedTestReportLocation(currentFile)

	RegisterFailHandler(Fail)
	reporterList := append([]Reporter{}, reporters.NewJUnitReporter(junitPath))

	if dumpFile != "" {
		reporter, err := testutils.NewReporter(
			dumpFile,
			netmlbparameters.ReporterNamespacesToDump,
			netmlbparameters.ReporterCrds)
		if err != nil {
			log.Fatalf("Failed to create log reporter %s", err)
		}
		reporterList = append(reporterList, reporter)
	}

	RunSpecsWithDefaultAndCustomReporters(t, "MetalLB tests", reporterList)
}

var _ = BeforeSuite(func() {
	generalHelper.PullTestImage(generalHelper.Config.General.CnfNodeLabel, generalHelper.Config.Network.TestContainerImage)
	By(fmt.Sprintf("Create %s namespace", netmlbparameters.TestNamespace))
	err := namespaces.Create(netmlbparameters.TestNamespace, generalHelper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	By(fmt.Sprintf("Clean test namespace %s", netmlbparameters.TestNamespace))
	err := namespaces.DeleteAndWait(generalHelper.Apiclient, netmlbparameters.TestNamespace, netmlbparameters.Timeout)
	Expect(err).ToNot(HaveOccurred())
})
