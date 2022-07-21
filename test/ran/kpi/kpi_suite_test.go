package kpi

import (
	"runtime"
	"testing"

	"github.com/onsi/ginkgo/v2/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/kpi/tests"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestKpi(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "RAN KPI tests", reporterConfig)
}

var _ = BeforeSuite(func() {
})

var _ = AfterSuite(func() {
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	utils.ReportIfFailed(report, currentFile, parameters.ReporterNamespacesToDump, parameters.ReporterCrds)
})
