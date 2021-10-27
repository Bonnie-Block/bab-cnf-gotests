package bfd_test

import (
	"log"
	"runtime"
	"testing"

	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/bfd/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	"github.com/onsi/ginkgo/reporters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/bfd/netbfdparameters"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestBfd(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	junitPath := Config.GetReportPath(currentFile)
	dumpFile := Config.GetDumpFailedTestReportLocation(currentFile)

	RegisterFailHandler(Fail)

	reporterList := append([]Reporter{}, reporters.NewJUnitReporter(junitPath))

	if dumpFile != "" {
		reporter, err := testutils.NewReporter(
			dumpFile,
			netbfdparameters.ReporterNamespacesToDump,
			nil)

		if err != nil {
			log.Fatalf("Failed to create log reporter %s", err)
		}

		reporterList = append(reporterList, reporter)
	}

	RunSpecsWithDefaultAndCustomReporters(t, "BFD test", reporterList)
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
