package ptp

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	cfg "github.com/onsi/ginkgo/config"

	"github.com/onsi/ginkgo/reporters"
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

func TestPTP(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	junitPath := helper.Config.GetReportPath(currentFile)
	dumpFile := helper.Config.GetDumpFailedTestReportLocation(currentFile)

	RegisterFailHandler(Fail)
	reporterList := append([]Reporter{}, reporters.NewJUnitReporter(junitPath))

	if dumpFile != "" {
		reporter, err := testutils.NewReporter(
			dumpFile,
			ranptpparameters.ReporterNamespacesToDump,
			ranptpparameters.ReporterCrds)
		if err != nil {
			log.Fatalf("Failed to create log reporter %s", err)
		}
		reporterList = append(reporterList, reporter)
	}
	// Stop ginkgo complaining about slow tests
	cfg.DefaultReporterConfig.SlowSpecThreshold = 1500.0

	RunSpecsWithDefaultAndCustomReporters(t, "RAN PTP tests", reporterList)

	cfg.DefaultReporterConfig.SlowSpecThreshold = 5.0
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
