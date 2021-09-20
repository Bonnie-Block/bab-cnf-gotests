package disovery

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/parameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/tests"
	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

func TestDiscovery(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	configSuite, err := config.NewConfig()
	if err != nil {
		fmt.Print(err)
		return
	}
	junitPath := configSuite.GetReportPath(currentFile)
	RegisterFailHandler(Fail)
	rr := append([]Reporter{}, reporters.NewJUnitReporter(junitPath))
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "CNF containers discovery mode", rr)
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
	config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error loading config: %s", err))

	err = CleanAllPerformanceProfile(strings.Split(config.General.CnfNodeLabel, "/")[1], snoTimeoutMultiplier)
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
	config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())
	By("Clean all PerformanceProfile Policy")
	err = CleanAllPerformanceProfile(strings.Split(config.General.CnfNodeLabel, "/")[1], snoTimeoutMultiplier)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error removing all Performance profiles: %s", err))
})
