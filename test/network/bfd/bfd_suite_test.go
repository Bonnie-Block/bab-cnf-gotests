package bfd_test

import (
	"runtime"
	"testing"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"

	"github.com/onsi/ginkgo/v2/types"

	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/bfd/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/bfd/netbfdparameters"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestBfd(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = Config.GetReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "BFD test", reporterConfig)
}

var _ = BeforeSuite(func() {
	isSingleNode, err := nodes.IsSingleNodeCluster(Apiclient)
	Expect(err).ToNot(HaveOccurred())
	if isSingleNode {
		Skip("Can't run the BFD test on a Single node cluster")
	}
	err = namespaces.Create(netbfdparameters.TestNamespace, Apiclient)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	err := namespaces.DeleteAndWait(Apiclient, netbfdparameters.TestNamespace, netbfdparameters.DeletionTimeout)
	Expect(err).ToNot(HaveOccurred())
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, netbfdparameters.ReporterNamespacesToDump, nil)
})

var _ = ReportAfterSuite("", func(report Report) {
	polarion.CreateReport(
		report, Config.GetPolarionReportPath(), parameters.PolarionTCPrefix)
})
