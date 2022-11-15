package ptp

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"context"
	"fmt"
	"log"
	"runtime"
	"testing"
	"time"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestPTP(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)
	reporterConfig.SlowSpecThreshold = 1500.0

	RegisterFailHandler(Fail)
	// Stop ginkgo complaining about slow tests

	RunSpecs(t, "RAN PTP tests", reporterConfig)

	reporterConfig.SlowSpecThreshold = 5.0
}

var _ = BeforeSuite(func() {
	Expect(helper.Apiclient).NotTo(BeNil())

	// Create privileged pods for ran testing if not already exist, and leave them on system.
	helper.CreatePrivilegedPods("")

	// Check for a PTP namespace is exists
	ptpNamespaceExists := namespaces.Exists(parameters.PtpOperatorNamespace, helper.Apiclient)
	Expect(ptpNamespaceExists).To(BeTrue())

	// Check for an aqm-router namespace is exists
	aqmNamespaceExists := namespaces.Exists(parameters.AmqNamespace, helper.Apiclient)
	Expect(aqmNamespaceExists).To(BeTrue())

	ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{
			LabelSelector: parameters.PtpDaemonsetLabelSelector})
	Expect(err).NotTo(HaveOccurred())

	for _, ptpDaemonPod := range ptpDaemonPods.Items {
		err = helper.IsPodHealthy(&ptpDaemonPod)
		Expect(err).NotTo(HaveOccurred())

		// Make suse event-proxy-container exists
		if !ranhelper.IsContainerExistInPod(ptpDaemonPod, ranptpparameters.ContainerName) {
			Skip(fmt.Sprintf("cannot run test if %s is not exists in the pod", ranptpparameters.ContainerName))
		}
	}
})

var _ = AfterSuite(func() {
	if namespaces.Exists(parameters.PrivPodNamespace, helper.Apiclient) {
		log.Println("Deleting test namespace", parameters.PrivPodNamespace)
		err := namespaces.DeleteAndWait(helper.Apiclient, parameters.PrivPodNamespace, 10*time.Minute)
		Expect(err).ToNot(HaveOccurred())
	}
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, ranptpparameters.ReporterNamespacesToDump, ranptpparameters.ReporterCrds)
})
