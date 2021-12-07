package kpi

import (
	"log"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/kpi/tests"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

func TestKpi(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	junitPath := helper.Config.GetReportPath(currentFile)
	dumpFile := helper.Config.GetDumpFailedTestReportLocation(currentFile)

	RegisterFailHandler(Fail)
	reporterList := append([]Reporter{}, reporters.NewJUnitReporter(junitPath))

	if dumpFile != "" {
		reporter, err := utils.NewReporter(
			dumpFile,
			parameters.ReporterNamespacesToDump,
			parameters.ReporterCrds,
		)
		if err != nil {
			log.Fatalf("Failed to create log reporter %s", err)
		}
		reporterList = append(reporterList, reporter)
	}

	RunSpecsWithDefaultAndCustomReporters(t, "RAN KPI tests", reporterList)
}

var _ = BeforeSuite(func() {
})

var _ = AfterSuite(func() {
})
