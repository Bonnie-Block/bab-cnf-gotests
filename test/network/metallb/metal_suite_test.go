package metallb

import (
	"fmt"
	"log"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/metallb/metallb-operator/api/v1beta1"
	metallbutils "github.com/metallb/metallb-operator/test/e2e/metallb"
	"github.com/onsi/ginkgo/reporters"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
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

var metallb *v1beta1.MetalLB

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
	metallb = netmetallbhelper.CreateMetallb()
})

var _ = AfterSuite(func() {
	By("Cleaning after suite")
	err := netmetallbhelper.DeleteAllBFDProfiles()
	Expect(err).ToNot(HaveOccurred())

	// Failed due to BZ 2050824. The BFD configuration check should be removed after the BZ fix.
	isBFDConfigured := netmetallbhelper.IsProtocolConfigured(netmlbparameters.BFDConfigPrefix)
	if isBFDConfigured {
		log.Println("Error: BFD config is not removed due to BZ 2050824")
	}
	err = netmetallbhelper.DeleteAllBGPPeers()
	Expect(err).ToNot(HaveOccurred())
	err = netmetallbhelper.DeleteLabelFromWorkers(netmlbparameters.SpeakerNodeTestLabel)
	Expect(err).ToNot(HaveOccurred())

	metallbutils.Delete(metallb)

	By(fmt.Sprintf("Clean test namespace %s", netmlbparameters.TestNamespace))
	err = namespaces.DeleteAndWait(helper.Apiclient, netmlbparameters.TestNamespace,
		netmlbparameters.Timeout)
	Expect(err).ToNot(HaveOccurred())

	By(fmt.Sprintf("Clean privileged namespace %s", parameters.PrivPodNamespace))
	if namespaces.Exists(parameters.PrivPodNamespace, helper.Apiclient) {
		err := namespaces.DeleteAndWait(helper.Apiclient, parameters.PrivPodNamespace, netmlbparameters.Timeout)
		Expect(err).ToNot(HaveOccurred())
	}
})
