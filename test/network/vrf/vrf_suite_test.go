package vrf

import (
	"fmt"
	"log"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"

	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/parameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/tests"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

const (
	waitingTime time.Duration = 20 * time.Minute
	timeout                   = 1800 * time.Second
)

func TestVrf(t *testing.T) {
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
			parameters.ReporterNamespacesToDump,
			parameters.ReporterCrds)
		if err != nil {
			log.Fatalf("Failed to create log reporter %s", err)
		}
		rr = append(rr, reporter)
	}
	RunSpecsWithDefaultAndCustomReporters(t, "VRF tests", rr)
}

var _ = BeforeSuite(func() {
	configuration, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())
	generalHelper.PullTestImage(configuration.General.CnfNodeLabel, configuration.Network.TestContainerImage)
	By(fmt.Sprintf("Create %s namespace", parameters.TestNamespace))
	err = namespaces.Create(parameters.TestNamespace, generalHelper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	By(fmt.Sprintf("Clean test namespace %s", parameters.TestNamespace))
	err := namespaces.Clean(
		generalParameters.SriovOperatorNamespace,
		parameters.TestNamespace,
		generalHelper.Apiclient, false)
	Expect(err).ToNot(HaveOccurred())
	err = namespaces.DeleteAndWait(generalHelper.Apiclient, parameters.TestNamespace, timeout)
	Expect(err).ToNot(HaveOccurred())
	By("Waiting until SRIOV become stable")
	generalHelper.WaitForSRIOVStable(generalParameters.SriovOperatorNamespace, waitingTime)
})
