package acc100

import (
	"log"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/acc100/netacc100parameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/acc100/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/netacceleratorhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

func TestACC100(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	junitPath := helper.Config.GetReportPath(currentFile)
	dumpFile := helper.Config.GetDumpFailedTestReportLocation(currentFile)
	RegisterFailHandler(Fail)
	rr := append([]Reporter{}, reporters.NewJUnitReporter(junitPath))
	if dumpFile != "" {
		reporter, err := testutils.NewReporter(
			dumpFile,
			netacc100parameters.ReporterNamespacesToDump,
			netacc100parameters.ReporterCrds)
		if err != nil {
			log.Fatalf("Failed to create log reporter %s", err)
		}
		rr = append(rr, reporter)
	}
	RunSpecsWithDefaultAndCustomReporters(t, "ACC100 tests", rr)
}

var _ = BeforeSuite(func() {
	helper.PullTestImage(helper.Config.General.CnfNodeLabel, helper.Config.Network.TestContainerImage)
	err := namespaces.Create(netacc100parameters.TestNamespace, helper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	sriovFecNodeList, err := netacceleratorhelper.GetSriovFecNodeConfigList(helper.Apiclient)
	if err == nil && len(sriovFecNodeList.Items) > 0 {
		netacceleratorhelper.CleanAllSriovFecClusterConfig(helper.Apiclient)
	}
	err = namespaces.DeleteAndWait(helper.Apiclient, netacc100parameters.TestNamespace, 5*time.Minute)
	Expect(err).ToNot(HaveOccurred())
})
