package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

<<<<<<< HEAD
	fpgav2 "github.com/smart-edge-open/openshift-operator/sriov-fec/api/v2"
	acc100helper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/acc100/helper"
=======
	fpgav1 "github.com/open-ness/openshift-operator/sriov-fec/api/v1"
	acc100helper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/acc100/networkacc100helper"
>>>>>>> b38cd40... Add MetalLB Test 43936
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/acc100/parameters"
	helper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/networkacceleratorhelper"
	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

var _ = Describe("Intel ACC100", func() {

	var (
		sriovFecNodeList     = &fpgav2.SriovFecNodeConfigList{}
		fecConfig            *fpgav2.SriovFecClusterConfig
		testSkip             = ""
		isSingleNode         bool
		snoTimeoutMultiplier time.Duration = 1
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
		if sriovFecNodeList == nil || len(sriovFecNodeList.Items) < 1 {
			testSkip = "No sriov-fec capable node was detected"
			Skip(testSkip)
		}
		Expect(err).ToNot(HaveOccurred())

		_, _, err = acc100helper.GetSriovFecNodeForAcc100(generalHelper.Apiclient)
		if err != nil {
			testSkip = "No acc100 cards on the cluster"
			Skip(testSkip)
		}

		IsSriovFecDeploymentInstalled, _ := generalHelper.IsDeploymentInstalled(generalHelper.Apiclient, parameters.OperatorNamespace, parameters.DeploymentSriovFecName)
		if !IsSriovFecDeploymentInstalled {
			testSkip = "Sriov-fec operator is not installed"
			Skip(testSkip)
		}
		isSriovFecDeploymentReady, err := generalHelper.IsDeploymentReady(generalHelper.Apiclient, parameters.OperatorNamespace, parameters.DeploymentSriovFecName)
		Expect(err).NotTo(HaveOccurred())
		Expect(isSriovFecDeploymentReady).To(Equal(true), "Sriov-fec operator is not ready")

		numberReadySriovFecDaemonsets, numberDesiredSriovFecDaemonsets := helper.CountSriovFecDaemonsets(
			generalHelper.Apiclient, parameters.OperatorNamespace)
		Expect(numberReadySriovFecDaemonsets).To(Equal(numberDesiredSriovFecDaemonsets),
			"Not all sriov-fec daemonsets are ready")

		isSingleNode, err = nodes.IsSingleNodeCluster(generalHelper.Apiclient)
		Expect(err).NotTo(HaveOccurred())
		if isSingleNode {
			snoTimeoutMultiplier = 2
		}

		By("Validating performance profile")
		helper.FindAndValidateOrOverridePerformanceProfile(generalHelper.Apiclient, config.General.CnfNodeLabel, snoTimeoutMultiplier)

		By("Creating SriovFecClusterConfig")
		fecConfig = acc100helper.GetSriovFecAcc100ClusterConfigDefinition(generalHelper.Apiclient, isSingleNode)
		cnfNodelabel := strings.Split(config.General.CnfNodeLabel, "/")[1]
		helper.InstallSriovFecClusterNodeConfig(generalHelper.Apiclient, fecConfig, isSingleNode, cnfNodelabel)
	})

	Context("sriov-fec", func() {

		BeforeEach(func() {
			if testSkip != "" {
				Skip(testSkip)
			}

<<<<<<< HEAD
=======
			By("Creating SriovFecClusterConfig")
			fecConfig = acc100helper.GetSriovFecAcc100ClusterConfigDefinition(generalHelper.Apiclient, false, isSingleNode)
			cnfNodelabel := strings.Split(config.General.CnfNodeLabel, "/")[1]
			helper.InstallSriovFecClusterNodeConfig(generalHelper.Apiclient, fecConfig, isSingleNode, cnfNodelabel)
		})

		AfterEach(func() {
			isSriovFecDeploymentReady, err := generalHelper.IsDeploymentReady(generalHelper.Apiclient, parameters.OperatorNamespace, parameters.DeploymentSriovFecName)
			Expect(err).NotTo(HaveOccurred())
			if isSriovFecDeploymentReady {
				By("Cleaning up resources after sriov-fec tests")
				fecConfig := acc100helper.GetSriovFecAcc100ClusterConfigDefinition(generalHelper.Apiclient, true, isSingleNode)
				cnfNodelabel := strings.Split(config.General.CnfNodeLabel, "/")[1]
				helper.InstallSriovFecClusterNodeConfig(generalHelper.Apiclient, fecConfig, isSingleNode, cnfNodelabel)

				Eventually(func() int64 {
					testedNode, err := generalHelper.Apiclient.CoreV1Interface.Nodes().Get(context.TODO(), fecConfig.Spec.Nodes[0].NodeName, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred())
					resNum, _ := testedNode.Status.Allocatable[corev1.ResourceName(parameters.Acc100ResourceName)]
					allocatable, _ := resNum.AsInt64()
					return allocatable
				}, 10*time.Minute, time.Second).Should(Equal(int64(0)))
			}
>>>>>>> b38cd40... Add MetalLB Test 43936
		})

		It("configuration", func() {
			Eventually(func() int64 {
				testedNode, err := generalHelper.Apiclient.CoreV1Interface.Nodes().Get(context.TODO(), fecConfig.Spec.NodeSelector["kubernetes.io/hostname"], metav1.GetOptions{})
				Expect(err).ToNot(HaveOccurred())
				resNum, _ := testedNode.Status.Allocatable[corev1.ResourceName(parameters.Acc100ResourceName)]
				allocatable, _ := resNum.AsInt64()
				return allocatable
			}, 10*time.Minute, time.Second).Should(Equal(int64(2)))
		})

		It("validation", func() {
			By("Waiting for resource to reported in the node")
			Eventually(func() int64 {
				testedNode, err := generalHelper.Apiclient.CoreV1Interface.Nodes().Get(context.TODO(), fecConfig.Spec.NodeSelector["kubernetes.io/hostname"], metav1.GetOptions{})
				Expect(err).ToNot(HaveOccurred())
				resNum, _ := testedNode.Status.Allocatable[corev1.ResourceName(parameters.Acc100ResourceName)]
				allocatable, _ := resNum.AsInt64()
				return allocatable
			}, 10*time.Minute, time.Second).Should(Equal(int64(2)))

			By("Creating bbdev test pod")
			bbdevPod := helper.CreateBbdevPod(generalHelper.Apiclient, parameters.TestNamespace, parameters.Acc100ResourceName, config)
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
