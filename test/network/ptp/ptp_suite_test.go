package ptp

import (
	"log"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/onsi/ginkgo/reporters"
	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/ptp/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

func TestPtp(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	junitPath := Config.GetReportPath(currentFile)
	dumpFile := Config.GetDumpFailedTestReportLocation(currentFile)

	RegisterFailHandler(Fail)
	reporterList := append([]Reporter{}, reporters.NewJUnitReporter(junitPath))

	if dumpFile != "" {
		reporter, err := testutils.NewReporter(
			dumpFile,
			parameters.ReporterNamespacesToDump,
			parameters.ReporterCrds)
		if err != nil {
			log.Fatalf("Failed to create log reporter %s", err)
		}
		reporterList = append(reporterList, reporter)
	}

	RunSpecsWithDefaultAndCustomReporters(t, "PTP tests", reporterList)
}

var _ = BeforeSuite(func() {
	err := namespaces.Create(parameters.PtpTestNamespace, Apiclient)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	err := namespaces.DeleteAndWait(Apiclient, parameters.PtpTestNamespace, 5*time.Minute)
	Expect(err).ToNot(HaveOccurred())
})
