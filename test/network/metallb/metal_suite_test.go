package metallb

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/onsi/ginkgo/v2/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestLB(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "MetalLB tests", reporterConfig)
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
	netmetallbhelper.DeleteAllIPAddressPools()

	err := netmetallbhelper.DeleteAllL2Advertisements()
	Expect(err).ToNot(HaveOccurred())
	err = netmetallbhelper.DeleteAllBGPAdvertisements()
	Expect(err).ToNot(HaveOccurred())

	By(fmt.Sprintf("Clean test namespace %s", netmlbparameters.TestNamespace))
	err = namespaces.DeleteAndWait(helper.Apiclient, netmlbparameters.TestNamespace,
		netmlbparameters.Timeout)
	Expect(err).ToNot(HaveOccurred())

	err = netmetallbhelper.DeleteAllBGPPeers()
	Expect(err).ToNot(HaveOccurred())

	err = netmetallbhelper.DeleteAllBFDProfiles()
	Expect(err).ToNot(HaveOccurred())

	_ = netmetallbhelper.DeleteLabelFromWorkers(netmlbparameters.SpeakerNodeTestLabel)
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, netmlbparameters.ReporterNamespacesToDump, netmlbparameters.ReporterCrds)
})

var _ = ReportAfterSuite("", func(report Report) {
	polarion.CreateReport(
		report, helper.Config.GetPolarionReportPath(), parameters.PolarionTCPrefix)
})
