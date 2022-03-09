package metallb

import (
	"fmt"

	"log"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/onsi/ginkgo/reporters"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

func TestLB(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)

	junitPath := helper.Config.GetReportPath(currentFile)
	dumpFile := helper.Config.GetDumpFailedTestReportLocation(currentFile)

	RegisterFailHandler(Fail)
	reporterList := append([]Reporter{}, reporters.NewJUnitReporter(junitPath))

	if dumpFile != "" {
		reporter, err := testutils.NewReporter(
			dumpFile,
			netmlbparameters.ReporterNamespacesToDump,
			netmlbparameters.ReporterCrds)
		if err != nil {
			log.Fatalf("Failed to create log reporter %s", err)
		}
		reporterList = append(reporterList, reporter)
	}

	RunSpecsWithDefaultAndCustomReporters(t, "MetalLB tests", reporterList)
}

var _ = BeforeSuite(func() {
	isSingleNode, err := nodes.IsSingleNodeCluster(helper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
	if isSingleNode {
		Skip("Can't run the Metallb tests on a Single node cluster")
	}
	helper.PullTestImage(helper.Config.General.CnfNodeLabel, helper.Config.Network.TestContainerImage)
	By(fmt.Sprintf("Create %s namespace", netmlbparameters.TestNamespace))
	err = namespaces.Create(netmlbparameters.TestNamespace, helper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("Cleaning after suite")
	netmetallbhelper.DeleteAllAddressPools()

	By(fmt.Sprintf("Clean test namespace %s", netmlbparameters.TestNamespace))
	err := namespaces.DeleteAndWait(helper.Apiclient, netmlbparameters.TestNamespace,
		netmlbparameters.Timeout)
	Expect(err).ToNot(HaveOccurred())

	err = netmetallbhelper.DeleteAllBFDProfiles()
	Expect(err).ToNot(HaveOccurred())

	err = netmetallbhelper.DeleteAllBGPPeers()
	Expect(err).ToNot(HaveOccurred())

	_ = netmetallbhelper.DeleteLabelFromWorkers(netmlbparameters.SpeakerNodeTestLabel)

	netmetallbhelper.RestoreNodeGWMode()
})
