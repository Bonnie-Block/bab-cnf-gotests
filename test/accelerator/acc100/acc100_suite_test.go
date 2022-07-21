package acc100

import (
	"runtime"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/acc100/netacc100parameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/acc100/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/netacceleratorhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestACC100(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "ACC100 tests", reporterConfig)
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

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, netacc100parameters.ReporterNamespacesToDump,
		netacc100parameters.ReporterCrds)
})
