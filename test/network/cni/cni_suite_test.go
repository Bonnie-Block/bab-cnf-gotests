package cni

import (
	"fmt"
	"runtime"
	"testing"
	"time"

<<<<<<< HEAD
	"github.com/onsi/ginkgo/v2/types"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"

	. "github.com/onsi/ginkgo/v2"
=======
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
>>>>>>> 6db12c57 (Add PTP events for boundary clock)
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/tests/sysctl"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/tests/vrf"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

const (
	waitingTime time.Duration = 20 * time.Minute
	timeout                   = 1800 * time.Second
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestVrf(t *testing.T) {
<<<<<<< HEAD
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)
=======
	_, currentFile, _, _ := runtime.Caller(0)
	junitPath := helper.Config.GetReportPath(currentFile)
	dumpFile := helper.Config.GetDumpFailedTestReportLocation(currentFile)
>>>>>>> 6db12c57 (Add PTP events for boundary clock)

	RegisterFailHandler(Fail)
	RunSpecs(t, "CNI tests", reporterConfig)
}

var _ = BeforeSuite(func() {
	helper.PullTestImage(helper.Config.General.CnfNodeLabel, helper.Config.Network.TestContainerImage)
	By(fmt.Sprintf("Create %s namespace", netcniparameters.TestNamespace))
	err := namespaces.Create(netcniparameters.TestNamespace, helper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	By(fmt.Sprintf("Clean test namespace %s", netcniparameters.TestNamespace))
	var snoTimeoutMultiplier time.Duration = 1
	isSingleNode, err := nodes.IsSingleNodeCluster(helper.Apiclient)
	Expect(err).ToNot(HaveOccurred())
	if isSingleNode {
		snoTimeoutMultiplier = 2
		helper.RestoreNodeDrainState(parameters.SriovOperatorNamespace)
	}
	err = namespaces.Clean(
		parameters.SriovOperatorNamespace,
		netcniparameters.TestNamespace,
		helper.Apiclient, false)
	Expect(err).ToNot(HaveOccurred())
	err = namespaces.DeleteAndWait(helper.Apiclient, netcniparameters.TestNamespace, timeout)
	Expect(err).ToNot(HaveOccurred())
	By("Waiting until SRIOV become stable")
	helper.WaitForSRIOVStable(parameters.SriovOperatorNamespace, waitingTime, snoTimeoutMultiplier)
<<<<<<< HEAD
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, netcniparameters.ReporterNamespacesToDump, netcniparameters.ReporterCrds)
=======
>>>>>>> 6db12c57 (Add PTP events for boundary clock)
})
