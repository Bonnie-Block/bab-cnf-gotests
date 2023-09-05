package workloadpartitioning

import (
	"log"
	"runtime"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/workloadpartitioning/ranwpparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/workloadpartitioning/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

const (
	timeout = 10 * time.Minute
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestWorkloadPartitioning(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)
	// Stop ginkgo complaining about slow tests

	RegisterFailHandler(Fail)
	RunSpecs(t, "RAN Workload Partitioning tests", reporterConfig)
}

var _ = BeforeSuite(func() {
	ranhelper.CleanupRanTestResources()
	helper.CreatePrivilegedPods("")
})

var _ = AfterSuite(func() {
	if namespaces.Exists(ran.NamespaceTesting, helper.Apiclient) {
		log.Println("Deleting test namespace", ran.NamespaceTesting)
		err := namespaces.DeleteAndWait(helper.Apiclient, ran.NamespaceTesting, timeout)
		Expect(err).ToNot(HaveOccurred())
	}
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, ranwpparameters.ReporterNamespacesToDump, ranwpparameters.ReporterCrds)
})

var _ = ReportAfterSuite("", func(report Report) {
	polarion.CreateReport(
		report, helper.Config.GetPolarionReportPath(currentFile), parameters.PolarionTCPrefix)
})
