package tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fpgav1 "github.com/open-ness/openshift-operator/N3000/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/helper"
	n3000helper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/n3000/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/n3000/parameters"
	networkHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
)

var _ = Describe("Intel N3000", func() {

	var (
		initialBitstreamId string
		initialDeviceId    string
		n3000NodeList      = &fpgav1.N3000NodeList{}
		fpgaStatus         = &fpgav1.N3000FpgaStatus{}
	)

	execute.BeforeAll(func() {
		var err error
		n3000NodeList, err = n3000helper.GetN3000NodeList(apiclient)
		if err != nil && err.Error() == "no matches for kind \"N3000Node\" in version \"fpga.intel.com/v1\"" {
			Skip("Cluster doesn't have intel fpga pac n3000 card")
		}
		Expect(err).ToNot(HaveOccurred())
		if len(n3000NodeList.Items) < 1 {
			Skip("No n3000 node is detected")
		}
		n3000helper.CreateService(apiclient, parameters.TestNamespace)
		numberReadyN3000Daemonsets, numberDesiredN3000Daemonsets := n3000helper.CountN3000Daemonsets(apiclient, parameters.OperatorNamespace)
		Expect(numberReadyN3000Daemonsets).To(Equal(numberDesiredN3000Daemonsets), "Not all n3000 daemonsets are ready")
		n3000Node, err := n3000helper.GetN3000Node(apiclient)
		Expect(err).NotTo(HaveOccurred())
		fpgaStatus, err = n3000helper.GetN3000FpgaStatus(n3000Node)
		Expect(err).NotTo(HaveOccurred())
		initialBitstreamId = fpgaStatus.BitstreamID
		initialDeviceId = fpgaStatus.DeviceID
		service, err := apiclient.Services(parameters.TestNamespace).List(context.Background(), metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		n3000helper.InstallNewN3000Image(apiclient, n3000Node.Name, fpgaStatus, parameters.ImageBitstreamFlash, parameters.ChecksumBitstreamImage, parameters.Port, &service.Items[0])
	})

	Context("opae", func() {
		//39002
		It("Bitstream flashing", func() {
			n3000Node, err := n3000helper.GetN3000Node(apiclient)
			Expect(err).NotTo(HaveOccurred())
			fpgaStatus, err = n3000helper.GetN3000FpgaStatus(n3000Node)
			Expect(err).NotTo(HaveOccurred())
			Expect(fpgaStatus.BitstreamID).NotTo(Equal(initialBitstreamId), "BitstreamId has not been changed after flashing")
			Expect(fpgaStatus.DeviceID).To(Equal(initialDeviceId), "DeviceID has been changed or removed after flashing")
		})
	})

	Context("sriov-fec", func() {
		BeforeEach(func() {
			IsSriovFecDeploymentInstalled, _ := helper.IsSriovFecDeploymentInstalled(apiclient, parameters.OperatorNamespace)
			if !IsSriovFecDeploymentInstalled {
				Skip("Sriov-fec operator is not installed")
			}
			isSriovFecDeploymentReady, err := helper.IsSriovFecDeploymentReady(apiclient, parameters.OperatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(isSriovFecDeploymentReady).To(Equal(true), "Sriov-fec operator is not ready")
			By("Creating SriovFecClusterConfig")

			// TODO: this is a workaround to allow the sriovfecnode to update by force
			helper.DeleteSriovFecPods(apiclient, parameters.OperatorNamespace)

			fecConfig := n3000helper.GetSriovFecN30005GClusterConfigDefinition(apiclient, false)
			helper.InstallSriovFecClusterNodeConfig(apiclient, fecConfig)
		})

		AfterEach(func() {
			isSriovFecDeploymentReady, err := helper.IsSriovFecDeploymentReady(apiclient, parameters.OperatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			if isSriovFecDeploymentReady {
				By("Cleaning up resources after sriov-fec tests")
				fecConfig := n3000helper.GetSriovFecN30005GClusterConfigDefinition(apiclient, true)
				helper.InstallSriovFecClusterNodeConfig(apiclient, fecConfig)

				// TODO: remove this after the sriov-fec operator clean spec automatically
				helper.CleanSriovFecNodeSpec(apiclient, fecConfig.Spec.Nodes[0].NodeName, parameters.OperatorNamespace)
			}
		})

		// 39008
		It("configuration", func() {
			By("Creating bbdev test pod")
			bbdevPod := helper.CreateBbdevPod(apiclient, parameters.TestNamespace, parameters.N3000resource5G)

			By("Running bbdev tests")
			bbdevTestResults := helper.RunBbdevTests(apiclient, bbdevPod)
			countOfTests := networkHelper.CountStringsByGreps(bbdevTestResults, "Starting Test Suite :")
			countOfPassed := networkHelper.CountStringsByGreps(bbdevTestResults, "Tests Passed", "1")

			Expect(countOfTests).To(Equal(parameters.TotalNumberBbdevTests), "Not all test have been executed")
			Expect(countOfPassed).To(Equal(parameters.ExpectedNumberBbdevTestsPassed), "Not all expected tests passed")
			Expect(helper.IsBbdevFailedTests(bbdevTestResults)).To(BeFalse(), fmt.Sprintf("There are failed tests.\n %s", bbdevTestResults))
		})
	})

})
