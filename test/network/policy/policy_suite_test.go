package policy

import (
	"fmt"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"

	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/policy/tests"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/policy/netpolicyparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestPolicy(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "NetworkPolicy tests", reporterConfig)
}

var _ = BeforeSuite(func() {
	helper.PullTestImage(helper.Config.General.CnfNodeLabel, helper.Config.Network.TestContainerImage)
	By(fmt.Sprintf("Create %s namespace", netpolicyparameters.TestNamespace))
	err := namespaces.Create(netpolicyparameters.TestNamespace, helper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	By(fmt.Sprintf("Clean test namespace %s", netpolicyparameters.TestNamespace))
	err := namespaces.Clean(
		parameters.SriovOperatorNamespace,
		netpolicyparameters.TestNamespace,
		helper.Apiclient, false)
	Expect(err).ToNot(HaveOccurred())
	err = namespaces.DeleteAndWait(helper.Apiclient, netpolicyparameters.TestNamespace, netpolicyparameters.WaitingTime)
	Expect(err).ToNot(HaveOccurred())
	By("Waiting until SRIOV become stable")
	helper.WaitForSRIOVStable(parameters.SriovOperatorNamespace, netpolicyparameters.WaitingTime, 1)
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, netpolicyparameters.ReporterNamespacesToDump,
		netpolicyparameters.ReporterCrds)
})
