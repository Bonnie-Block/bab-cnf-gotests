package ptp

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"context"
	"fmt"
	"log"
	"runtime"
	"testing"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestPTP(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)

	RegisterFailHandler(Fail)
	// Stop ginkgo complaining about slow tests

	RunSpecs(t, "RAN PTP tests", reporterConfig)
}

var _ = BeforeSuite(func() {
	Expect(helper.Apiclient).NotTo(BeNil())

	if !namespaces.Exists(parameters.PtpOperatorNamespace, helper.Apiclient) {
		Skip(parameters.PtpOperatorNamespace + " namespace does not exist")
	}

	ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{
			LabelSelector: parameters.PtpDaemonsetLabelSelector})
	Expect(err).NotTo(HaveOccurred())

	if len(ptpDaemonPods.Items) == 0 {
		Skip("PTP linux Daemon pod does not exist")
	}

	for _, ptpDaemonPod := range ptpDaemonPods.Items {
		err = helper.IsPodHealthy(&ptpDaemonPod)
		Expect(err).NotTo(HaveOccurred())

		// Make suse event-proxy-container exists
		if !ranhelper.IsContainerExistInPod(ptpDaemonPod, ranptpparameters.CloudEventContainer) {
			Skip(fmt.Sprintf("cannot run test if %s is not exists in the pod", ranptpparameters.CloudEventContainer))
		}
	}

	_, err = ranptphelper.GetOcpInterface(ptpDaemonPods.Items[0], parameters.PtpContainerName)
	Expect(err).ToNot(HaveOccurred())

	// Get ptp version
	ranptpparameters.PtpVersion, err = ranhelper.GetOperatorVersionFromCSV(
		helper.Apiclient,
		ranptpparameters.PtpOperatorName,
		parameters.PtpOperatorNamespace,
	)
	Expect(err).NotTo(HaveOccurred())
	log.Println("PTP Operator version:", ranptpparameters.PtpVersion)

	ranparameters.TransportType, err = ranptphelper.GetPtpTransport()
	Expect(err).NotTo(HaveOccurred())
	log.Println("PTP event transport:", ranparameters.TransportType)

	By("Deploy PTP consumer")
	consumersList, err := ranptphelper.DeployPtpConsumer()
	Expect(err).NotTo(HaveOccurred())
	log.Println("Deployed consumer:", consumersList.Items[0].Name)

	// Create privileged pods for ran testing if not already exist, and leave them on system.
	helper.CreatePrivilegedPods("")
})

var _ = AfterSuite(func() {
	var teardownErrors []error
	By("Remove consumer pods")
	destroyErrors := ranhelper.DestroyConsumers(parameters.CloudEventNamespace)
	teardownErrors = append(teardownErrors, destroyErrors...)

	By("Check errors in tear-down")
	for _, err := range teardownErrors {
		Expect(err).ShouldNot(HaveOccurred())
	}
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, ranptpparameters.ReporterNamespacesToDump,
		ranptpparameters.ReporterCrds)
})

var _ = ReportAfterSuite("", func(report Report) {
	polarion.CreateReport(
		report, helper.Config.GetPolarionReportPath(currentFile), parameters.PolarionTCPrefix)
})
