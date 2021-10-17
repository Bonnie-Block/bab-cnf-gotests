package acc100

import (
	"fmt"
	"log"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/acc100/parameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/acc100/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/networkacceleratorhelper"
	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

func TestACC100(t *testing.T) {
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
	RunSpecsWithDefaultAndCustomReporters(t, "ACC100 tests", rr)
}

var _ = BeforeSuite(func() {
	configuration, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())
	generalHelper.PullTestImage(configuration.General.CnfNodeLabel, configuration.Network.TestContainerImage)
	err = namespaces.Create(parameters.TestNamespace, generalHelper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	sriovFecNodeList, err := networkacceleratorhelper.GetSriovFecNodeConfigList(generalHelper.Apiclient)
	if err == nil && len(sriovFecNodeList.Items) > 0 {
		networkacceleratorhelper.CleanAllSriovFecClusterConfig(generalHelper.Apiclient)
	}
	err = namespaces.DeleteAndWait(generalHelper.Apiclient, parameters.TestNamespace, 5*time.Minute)
	Expect(err).ToNot(HaveOccurred())
})
