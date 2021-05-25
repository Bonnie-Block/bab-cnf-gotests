package disovery

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/parameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/tests"
	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
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
	err := helper.CleanAllSriovPolicy()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error removing all sriov policy: %s", err))
	_ = namespaces.DeleteAndWait(Apiclient, parameters.TestNamespace, parameters.NamespaceDeleteTimeout)

	By("Clean All Performance profiles")
	config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error loading config: %s", err))
	err = helper.CleanAllPerformanceProfile(strings.Split(config.General.CnfNodeLabel, "/")[1])
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error removing all Performance profiles: %s", err))

	By("Clean All PTP config")
	CleanAllPtpConfig(
		generalParameters.PtpOperatorNamespace,
		parameters.DiscoveryPtpGrandmasterNodeLabel,
		parameters.DiscoveryPtpSlaveNodeLabel)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to remove ptp configuration: %s", err))
})

var _ = AfterSuite(func() {
	By(fmt.Sprintf("Clean test namespace %s", parameters.TestNamespace))
	_ = namespaces.DeleteAndWait(Apiclient, parameters.TestNamespace, parameters.NamespaceDeleteTimeout)
})
