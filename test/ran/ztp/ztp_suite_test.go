package ztp

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/tests"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestZtp(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)
	reporterConfig.SlowSpecThreshold = 1500.0

	RegisterFailHandler(Fail)
	// Stop ginkgo complaining about slow tests

	RunSpecs(t, "RAN ZTP tests", reporterConfig)

	reporterConfig.SlowSpecThreshold = 5.0
}

var _ = BeforeSuite(func() {

})

var _ = AfterSuite(func() {

})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, ranztpparameters.ZtpNamespaces, ranztpparameters.ZtpCrds)
})
