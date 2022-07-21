package ptp

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"

	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"

	"log"
	"runtime"
	"testing"
	"time"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestPTP(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)
	reporterConfig.SlowSpecThreshold = 1500.0

	RegisterFailHandler(Fail)
	// Stop ginkgo complaining about slow tests

	RunSpecs(t, "RAN PTP tests", reporterConfig)

	reporterConfig.SlowSpecThreshold = 5.0
}

var _ = BeforeSuite(func() {
	Expect(helper.Apiclient).NotTo(BeNil())

	// Create privileged pods for ran testing if not already exist, and leave them on system.
	helper.CreatePrivilegedPods("")

	// Check for a PTP namespace is exists
	exists := namespaces.Exists(parameters.PtpOperatorNamespace, helper.Apiclient)
	Expect(exists).To(BeTrue())
})

var _ = AfterSuite(func() {
	if namespaces.Exists(parameters.PrivPodNamespace, helper.Apiclient) {
		log.Println("Deleting test namespace", parameters.PrivPodNamespace)
		err := namespaces.DeleteAndWait(helper.Apiclient, parameters.PrivPodNamespace, 10*time.Minute)
		Expect(err).ToNot(HaveOccurred())
	}
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, ranptpparameters.ReporterNamespacesToDump, ranptpparameters.ReporterCrds)
})
