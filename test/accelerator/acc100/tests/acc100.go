package tests

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fpgav2 "github.com/smart-edge-open/sriov-fec-operator/sriov-fec/api/v2"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/acc100/netacc100helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/acc100/netacc100parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/netacceleratorhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
)

var _ = Describe("Intel ACC100", func() {

	var (
		sriovFecNodeList     = &fpgav2.SriovFecNodeConfigList{}
		fecConfig            *fpgav2.SriovFecClusterConfig
		testSkip             = ""
		isSingleNode         bool
		snoTimeoutMultiplier time.Duration = 1
		isSecureBootEnabled  bool
	)

	execute.BeforeAll(func() {
		var err error

		sriovFecNodeList, err = netacceleratorhelper.GetSriovFecNodeConfigList(helper.Apiclient)
		if err != nil && err.Error() == "no matches for kind \"SriovFecNodeConfig\" in version \"sriovfec.intel.com/v1\"" {
			testSkip = "Cluster doesn't have intel easic acc100 card"
			Skip(testSkip)
		}
		if sriovFecNodeList == nil || len(sriovFecNodeList.Items) < 1 {
			testSkip = "No sriov-fec capable node was detected"
			Skip(testSkip)
		}
		Expect(err).ToNot(HaveOccurred())

		_, _, err = netacc100helper.GetSriovFecNodeForAcc100(helper.Apiclient)
		if err != nil {
			testSkip = "No acc100 cards on the cluster"
			Skip(testSkip)
		}

		IsSriovFecDeploymentInstalled, _ := helper.IsDeploymentInstalled(
			helper.Apiclient, netacc100parameters.OperatorNamespace, netacc100parameters.DeploymentSriovFecName)
		if !IsSriovFecDeploymentInstalled {
			testSkip = "Sriov-fec operator is not installed"
			Skip(testSkip)
		}
		isSriovFecDeploymentReady, err := helper.IsDeploymentReady(helper.Apiclient,
			netacc100parameters.OperatorNamespace, netacc100parameters.DeploymentSriovFecName)
		Expect(err).NotTo(HaveOccurred())
		Expect(isSriovFecDeploymentReady).To(Equal(true), "Sriov-fec operator is not ready")

		numberReadySriovFecDaemonsets, numberDesiredSriovFecDaemonsets := netacceleratorhelper.CountSriovFecDaemonsets(
			helper.Apiclient, netacc100parameters.OperatorNamespace)
		Expect(numberReadySriovFecDaemonsets).To(Equal(numberDesiredSriovFecDaemonsets),
			"Not all sriov-fec daemonsets are ready")

		isSingleNode, err = nodes.IsSingleNodeCluster(helper.Apiclient)
		Expect(err).NotTo(HaveOccurred())
		if isSingleNode {
			snoTimeoutMultiplier = 2
		}

		By("Verify Secure Boot status on worker nodes")
		workerNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(workerNodeList)).To(BeNumerically(">", 1))

		workerNodeNames := []string{workerNodeList[0].Name, workerNodeList[1].Name}
		isSecureBootEnabled, err = netacceleratorhelper.GetNodeSecureBootState(workerNodeNames,
			netacc100parameters.TestNamespace)

		Expect(err).NotTo(HaveOccurred(), "Unable to determine if Secure Boot is enabled")

		if isSecureBootEnabled {
			log.Println("Secure Boot is enabled on worker nodes")

		} else {
			log.Println("Secure Boot is not enabled on worker nodes")
		}

		By("Validating performance profile")
		netacceleratorhelper.FindAndValidateOrOverridePerformanceProfile(
			helper.Apiclient, helper.Config.General.CnfNodeLabel, snoTimeoutMultiplier)

		By("Creating SriovFecClusterConfig")
		fecConfig = netacc100helper.GetSriovFecAcc100ClusterConfigDefinition(
			helper.Apiclient, isSingleNode, isSecureBootEnabled)
		cnfNodelabel := strings.Split(helper.Config.General.CnfNodeLabel, "/")[1]
		netacceleratorhelper.InstallSriovFecClusterNodeConfig(
			helper.Apiclient, fecConfig, isSingleNode, cnfNodelabel)
	})

	Context("sriov-fec", func() {

		BeforeEach(func() {
			if testSkip != "" {
				Skip(testSkip)
			}

		})

		It("configuration", polarion.ID("41073"), func() {
			Eventually(func() int64 {
				testedNode, err := helper.Apiclient.CoreV1Interface.Nodes().Get(
					context.TODO(),
					fecConfig.Spec.NodeSelector["kubernetes.io/hostname"],
					metav1.GetOptions{},
				)
				Expect(err).ToNot(HaveOccurred())
				resNum := testedNode.Status.Allocatable[corev1.ResourceName(netacc100parameters.Acc100ResourceName)]
				allocatable, _ := resNum.AsInt64()

				return allocatable
			}, 10*time.Minute, time.Second).Should(Equal(int64(2)))
		})

		It("validation", polarion.ID("41216"), func() {
			By("Waiting for resource to reported in the node")
			Eventually(func() int64 {
				testedNode, err := helper.Apiclient.CoreV1Interface.Nodes().Get(
					context.TODO(),
					fecConfig.Spec.NodeSelector["kubernetes.io/hostname"],
					metav1.GetOptions{},
				)
				Expect(err).ToNot(HaveOccurred())
				resNum := testedNode.Status.Allocatable[corev1.ResourceName(netacc100parameters.Acc100ResourceName)]
				allocatable, _ := resNum.AsInt64()

				return allocatable
			}, 10*time.Minute, time.Second).Should(Equal(int64(2)))

			By("Creating bbdev test pod")
			bbdevPod := netacceleratorhelper.CreateBbdevPod(
				netacc100parameters.TestNamespace, netacc100parameters.Acc100ResourceName, helper.Config)

			By("Running bbdev tests")
			bbdevTestResults := netacceleratorhelper.RunBbdevTests(helper.Apiclient, bbdevPod, isSecureBootEnabled)
			countOfTests := helper.CountLinesByMatches(bbdevTestResults, "Starting Test Suite :")
			countOfPassed := helper.CountLinesByMatches(bbdevTestResults, "Tests Passed", "1")

			Expect(countOfTests).To(Equal(netacc100parameters.TotalNumberBbdevTests), "Not all test have been executed")
			Expect(countOfPassed).To(Equal(netacc100parameters.ExpectedNumberBbdevTestsPassed), "Not all expected tests passed")
			Expect(netacceleratorhelper.IsBbdevFailedTests(bbdevTestResults)).To(BeFalse(),
				fmt.Sprintf("There are failed tests.\n %s", bbdevTestResults))
		})
	})
})
