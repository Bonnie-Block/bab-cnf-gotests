package cpu

import (
	"log"
	"runtime"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/cpu/rancpuparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/cpu/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

const (
	timeout = 1800 * time.Second
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestCpu(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)
	reporterConfig.SlowSpecThreshold = 1200.0

	RegisterFailHandler(Fail)
	RunSpecs(t, "RAN CPU management tests", reporterConfig)
	reporterConfig.SlowSpecThreshold = 5.0
}

var _ = BeforeSuite(func() {
	// Create privileged pods for ran testing if not already exist, and leave them on system.
	ranhelper.CleanupRanTestResources()
	helper.CreatePrivilegedPods("")
	// Cleanup and create test namespace
	if namespaces.Exists(ran.NamespaceTesting, helper.Apiclient) {
		log.Println("Deleting test namespace", ran.NamespaceTesting)
		_ = namespaces.DeleteAndWait(helper.Apiclient, ran.NamespaceTesting, 5*time.Minute)
	}
	log.Println("Creating test namespace", ran.NamespaceTesting)
	err := namespaces.Create(ran.NamespaceTesting, helper.Apiclient)
	Expect(err).ToNot(HaveOccurred())

	// Deploy process-exporter pod on each node
	ranhelper.DeployProcessExporter()
})

var _ = AfterSuite(func() {
	log.Println("Deleting test namespace", ran.NamespaceTesting)
	err := namespaces.DeleteAndWait(helper.Apiclient, ran.NamespaceTesting, timeout)
	Expect(err).ToNot(HaveOccurred())
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, rancpuparameters.ReporterNamespacesToDump, rancpuparameters.ReporterCrds)
})
