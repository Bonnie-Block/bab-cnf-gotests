package disovery

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/parameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/tests"
	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestDiscovery(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = Config.GetReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "CNF containers discovery mode", reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Clean all Sriov Policy")
	var snoTimeoutMultiplier time.Duration = 1
	isSingleNode, err := nodes.IsSingleNodeCluster(Apiclient)
	Expect(err).ToNot(HaveOccurred())
	if isSingleNode {
		snoTimeoutMultiplier = 2
	}
	err = helper.CleanAllSriovPolicy(snoTimeoutMultiplier)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error removing all sriov policy: %s", err))

	By("Clean All Performance profiles")

	err = CleanAllPerformanceProfile(strings.Split(Config.General.CnfNodeLabel, "/")[1], snoTimeoutMultiplier)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error removing all Performance profiles: %s", err))

	By("Clean All PTP config")
	err = CleanAllPtpConfig(
		generalParameters.PtpOperatorNamespace,
		parameters.DiscoveryPtpGrandmasterNodeLabel,
		parameters.DiscoveryPtpSlaveNodeLabel)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to remove ptp configuration: %s", err))
})

var _ = AfterSuite(func() {
	var snoTimeoutMultiplier time.Duration = 1
	isSingleNode, err := nodes.IsSingleNodeCluster(Apiclient)
	Expect(err).ToNot(HaveOccurred())
	if isSingleNode {
		snoTimeoutMultiplier = 2
		RestoreNodeDrainState(generalParameters.SriovOperatorNamespace)
	}
	By("Clean all Sriov Policy")
	err = helper.CleanAllSriovPolicy(snoTimeoutMultiplier)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error removing all sriov policy: %s", err))
	By("Clean all PtpConfig Policy")
	err = CleanAllPtpConfig(
		generalParameters.PtpOperatorNamespace,
		parameters.DiscoveryPtpGrandmasterNodeLabel,
		parameters.DiscoveryPtpSlaveNodeLabel)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to remove ptp configuration: %s", err))

	By("Clean all PerformanceProfile Policy")
	err = CleanAllPerformanceProfile(strings.Split(Config.General.CnfNodeLabel, "/")[1], snoTimeoutMultiplier)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error removing all Performance profiles: %s", err))
})
