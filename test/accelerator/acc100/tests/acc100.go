package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fpgav1 "github.com/open-ness/openshift-operator/sriov-fec/api/v1"
	acc100helper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/acc100/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/acc100/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/helper"
	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
)

var _ = Describe("Intel ACC100", func() {

	var (
		sriovFecNodeList = &fpgav1.SriovFecNodeConfigList{}
		testSkip         = ""
	)
	config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())

	execute.BeforeAll(func() {
		var err error

		sriovFecNodeList, err = helper.GetSriovFecNodeConfigList(generalHelper.Apiclient)
		if err != nil && err.Error() == "no matches for kind \"SriovFecNodeConfig\" in version \"sriovfec.intel.com/v1\"" {
			testSkip = "Cluster doesn't have intel easic acc100 card"
			Skip(testSkip)
		}
		Expect(err).ToNot(HaveOccurred())
		if len(sriovFecNodeList.Items) < 1 {
			testSkip = "No sriov-fec capable node was detected"
			Skip(testSkip)
		}

		_, _, err = acc100helper.GetSriovFecNodeForAcc100(generalHelper.Apiclient)
		if err != nil {
			testSkip = "No acc100 cards on the cluster"
			Skip(testSkip)
		}

		IsSriovFecDeploymentInstalled, _ := helper.IsSriovFecDeploymentInstalled(generalHelper.Apiclient, parameters.OperatorNamespace)
		if !IsSriovFecDeploymentInstalled {
			testSkip = "Sriov-fec operator is not installed"
			Skip(testSkip)
		}
		isSriovFecDeploymentReady, err := helper.IsSriovFecDeploymentReady(generalHelper.Apiclient, parameters.OperatorNamespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(isSriovFecDeploymentReady).To(Equal(true), "Sriov-fec operator is not ready")

		numberReadySriovFecDaemonsets, numberDesiredSriovFecDaemonsets := helper.CountSriovFecDaemonsets(
			generalHelper.Apiclient, parameters.OperatorNamespace)
		Expect(numberReadySriovFecDaemonsets).To(Equal(numberDesiredSriovFecDaemonsets),
			"Not all sriov-fec daemonsets are ready")

	})

	Context("sriov-fec", func() {
		var fecConfig *fpgav1.SriovFecClusterConfig

		BeforeEach(func() {
			if testSkip != "" {
				Skip(testSkip)
			}

			By("Creating SriovFecClusterConfig")
			fecConfig = acc100helper.GetSriovFecAcc100ClusterConfigDefinition(generalHelper.Apiclient, false)
			helper.InstallSriovFecClusterNodeConfig(generalHelper.Apiclient, fecConfig)
		})

		AfterEach(func() {
			isSriovFecDeploymentReady, err := helper.IsSriovFecDeploymentReady(generalHelper.Apiclient, parameters.OperatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			if isSriovFecDeploymentReady {
				By("Cleaning up resources after sriov-fec tests")
				fecConfig := acc100helper.GetSriovFecAcc100ClusterConfigDefinition(generalHelper.Apiclient, true)
				helper.InstallSriovFecClusterNodeConfig(generalHelper.Apiclient, fecConfig)

				Eventually(func() int64 {
					testedNode, err := generalHelper.Apiclient.CoreV1Interface.Nodes().Get(context.TODO(), fecConfig.Spec.Nodes[0].NodeName, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred())
					resNum, _ := testedNode.Status.Allocatable[corev1.ResourceName(parameters.Acc100ResourceName)]
					allocatable, _ := resNum.AsInt64()
					return allocatable
				}, 10*time.Minute, time.Second).Should(Equal(int64(0)))
			}
		})

		It("configuration", func() {
			Eventually(func() int64 {
				testedNode, err := generalHelper.Apiclient.CoreV1Interface.Nodes().Get(context.TODO(), fecConfig.Spec.Nodes[0].NodeName, metav1.GetOptions{})
				Expect(err).ToNot(HaveOccurred())
				resNum, _ := testedNode.Status.Allocatable[corev1.ResourceName(parameters.Acc100ResourceName)]
				allocatable, _ := resNum.AsInt64()
				return allocatable
			}, 10*time.Minute, time.Second).Should(Equal(int64(2)))
		})

		It("validation", func() {
			By("Waiting for resource to reported in the node")
			Eventually(func() int64 {
				testedNode, err := generalHelper.Apiclient.CoreV1Interface.Nodes().Get(context.TODO(), fecConfig.Spec.Nodes[0].NodeName, metav1.GetOptions{})
				Expect(err).ToNot(HaveOccurred())
				resNum, _ := testedNode.Status.Allocatable[corev1.ResourceName(parameters.Acc100ResourceName)]
				allocatable, _ := resNum.AsInt64()
				return allocatable
			}, 10*time.Minute, time.Second).Should(Equal(int64(2)))

			By("Creating bbdev test pod")
			bbdevPod := helper.CreateBbdevPod(generalHelper.Apiclient, helper.TestNamespace, parameters.Acc100ResourceName, config)
			By("Running bbdev tests")
			bbdevTestResults := helper.RunBbdevTests(generalHelper.Apiclient, bbdevPod)
			countOfTests := generalHelper.CountLinesByMatches(bbdevTestResults, "Starting Test Suite :")
			countOfPassed := generalHelper.CountLinesByMatches(bbdevTestResults, "Tests Passed", "1")

			Expect(countOfTests).To(Equal(parameters.TotalNumberBbdevTests), "Not all test have been executed")
			Expect(countOfPassed).To(Equal(parameters.ExpectedNumberBbdevTestsPassed), "Not all expected tests passed")
			Expect(helper.IsBbdevFailedTests(bbdevTestResults)).To(BeFalse(), fmt.Sprintf("There are failed tests.\n %s", bbdevTestResults))
		})
	})

})
