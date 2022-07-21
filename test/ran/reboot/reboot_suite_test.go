package reboot

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
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/reboot/ranrebootparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/reboot/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

const (
	timeout = 10 * time.Minute
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestReboot(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)

	RegisterFailHandler(Fail)

	// Stop ginkgo complaining about slow tests
	reporterConfig.SlowSpecThreshold = 1500.0
	RunSpecs(t, "RAN reboot tests", reporterConfig)
	reporterConfig.SlowSpecThreshold = 5.0
}

var _ = BeforeSuite(func() {
	// Check nodes status before running reboot test
	nodeErr := nodes.WaitForNodesReady(helper.Apiclient, 1*time.Minute, 3*time.Second)
	Expect(nodeErr).ToNot(HaveOccurred())
	// Create privileged pods for ran testing if not already exist, and leave them on system.
	helper.CreatePrivilegedPods("")
	// Cleanup and create test namespace
	if namespaces.Exists(ran.NamespaceTesting, helper.Apiclient) {
		log.Println("Deleting test namespace", ran.NamespaceTesting)
		err := namespaces.DeleteAndWait(helper.Apiclient, ran.NamespaceTesting, timeout)
		Expect(err).ToNot(HaveOccurred())
	}
	log.Println("Creating test namespace", ran.NamespaceTesting)
	err := namespaces.Create(ran.NamespaceTesting, helper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	if namespaces.Exists(ran.NamespaceTesting, helper.Apiclient) {
		log.Println("Deleting test namespace", ran.NamespaceTesting)
		err := namespaces.DeleteAndWait(helper.Apiclient, ran.NamespaceTesting, timeout)
		Expect(err).ToNot(HaveOccurred())
	}
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, ranrebootparameters.ReporterNamespacesToDump,
		ranrebootparameters.ReporterCrds)
})
