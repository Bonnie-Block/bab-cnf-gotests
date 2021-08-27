package reboot

import (
	"fmt"
	"log"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	cfg "github.com/onsi/ginkgo/config"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/reboot/ranrebootparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/reboot/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

const (
	timeout = 10 * time.Minute
)

func TestReboot(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	configSuite, err := config.NewConfig()
	if err != nil {
		fmt.Print(err)
		return
	}
	junitPath := configSuite.GetReportPath(currentFile)
	dumpFile := configSuite.GetDumpFailedTestReportLocation(currentFile)
	RegisterFailHandler(Fail)
	rr := append([]Reporter{}, reporters.NewJUnitReporter(junitPath))
	if dumpFile != "" {
		reporter, err := testutils.NewReporter(
			dumpFile,
			ranrebootparameters.ReporterNamespacesToDump,
			ranrebootparameters.ReporterCrds)
		if err != nil {
			log.Fatalf("Failed to create log reporter %s", err)
		}
		rr = append(rr, reporter)
	}
	// Stop ginkgo complaining about slow tests
	cfg.DefaultReporterConfig.SlowSpecThreshold = 1200.0
	RunSpecsWithDefaultAndCustomReporters(t, "RAN reboot tests", rr)
	cfg.DefaultReporterConfig.SlowSpecThreshold = 5.0
}

var _ = BeforeSuite(func() {
	// Check nodes status before running reboot test
	nodeErr := nodes.WaitForNodesReady(helper.Apiclient, 1 * time.Minute, 3 * time.Second)
	Expect(nodeErr).ToNot(HaveOccurred())
	// Create privileged pods for ran testing if not already exist, and leave them on system.
	ranhelper.CreatePrivilegedPods("")
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
