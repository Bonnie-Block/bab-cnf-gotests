package disovery

import (
	"context"
	"fmt"
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/onsi/ginkgo/reporters"
	performance "github.com/openshift-kni/performance-addon-operators/api/v2"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/parameters"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
)

const (
	waitingTime time.Duration = 20 * time.Minute
	timeout                   = 1800 * time.Second
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
	sriovNodePolicyList := &sriovv1.SriovNetworkNodePolicyList{}
	err := Apiclient.Client.List(context.TODO(), sriovNodePolicyList)
	Expect(err).ToNot(HaveOccurred())
	if len(sriovNodePolicyList.Items) > 1 {
		for _, sriovNodePolicy := range sriovNodePolicyList.Items {
			if sriovNodePolicy.Name != "default" {
				err := Apiclient.Client.Delete(
					context.TODO(),
					&sriovNodePolicy)
				Expect(err).ToNot(HaveOccurred())
			}
		}
		WaitForSRIOVStable(Apiclient, generalParameters.SriovOperatorNamespace, parameters.SriovWaitingTime)
	}
	_ = namespaces.DeleteAndWait(Apiclient, parameters.TestNamespace, timeout)

	By("Clean All Performance profiles")
	config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())
	performanceProfileList := &performance.PerformanceProfileList{}
	err = Apiclient.Client.List(context.TODO(), performanceProfileList)
	Expect(err).ToNot(HaveOccurred())
	if len(performanceProfileList.Items) > 0 {
		for _, performanceProfile := range performanceProfileList.Items {
			err := Apiclient.Client.Delete(
				context.TODO(),
				&performanceProfile)
			Expect(err).ToNot(HaveOccurred())
		}
		err = helper.WaitForClusterToBeStable(Apiclient, strings.Split(config.General.CnfNodeLabel,"/")[1])
	}
	Expect(err).ToNot(HaveOccurred())

	By("Clean All PTP config")
	CleanAllPtpConfig(
		Apiclient,
		generalParameters.PtpOperatorNamespace,
		parameters.DiscoveryPtpGrandmasterNodeLabel,
		parameters.DiscoveryPtpSlaveNodeLabel)
	Expect(err).ToNot(HaveOccurred())

})

var _ = AfterSuite(func() {
	By(fmt.Sprintf("Clean test namespace %s", parameters.TestNamespace))
	_ = namespaces.DeleteAndWait(Apiclient, parameters.TestNamespace, timeout)
})
