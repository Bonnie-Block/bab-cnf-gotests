package metallb

import (
	"fmt"
	"log"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/networkmlbparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

func TestLB(t *testing.T) {
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
			networkmlbparameters.ReporterNamespacesToDump,
			networkmlbparameters.ReporterCrds)
		if err != nil {
			log.Fatalf("Failed to create log reporter %s", err)
		}
		rr = append(rr, reporter)
	}
	RunSpecsWithDefaultAndCustomReporters(t, "MetalLB tests", rr)
}

var _ = BeforeSuite(func() {
	configuration, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())
	generalHelper.PullTestImage(configuration.General.CnfNodeLabel, configuration.Network.TestContainerImage)
	By(fmt.Sprintf("Create %s namespace", networkmlbparameters.TestNamespace))
	err = namespaces.Create(networkmlbparameters.TestNamespace, generalHelper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	By(fmt.Sprintf("Clean test namespace %s", networkmlbparameters.TestNamespace))
	err := namespaces.DeleteAndWait(generalHelper.Apiclient, networkmlbparameters.TestNamespace, networkmlbparameters.Timeout)
	Expect(err).ToNot(HaveOccurred())
})
