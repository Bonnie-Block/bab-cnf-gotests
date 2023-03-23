package ptp

import (
	"runtime"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/ptp/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestPtp(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = Config.GetReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "PTP tests", reporterConfig)
}

var _ = BeforeSuite(func() {
	err := namespaces.Create(parameters.PtpTestNamespace, Apiclient)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	err := namespaces.DeleteAndWait(Apiclient, parameters.PtpTestNamespace, 5*time.Minute)
	Expect(err).ToNot(HaveOccurred())
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, parameters.ReporterNamespacesToDump, parameters.ReporterCrds)
})

var _ = ReportAfterSuite("", func(report Report) {
	polarion.CreateReport(
		report, Config.GetPolarionReportPath(), parameters.PolarionTCPrefix)
})
