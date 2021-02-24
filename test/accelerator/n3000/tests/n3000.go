package tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	fpgav1 "github.com/open-ness/openshift-operator/N3000/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/n3000/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/n3000/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

var _ = Describe("Intel", func() {

	var (
		bitstreamId   string
		deviceId      string
		port          int32 = 80
		n3000NodeList       = &fpgav1.N3000NodeList{}
	)

	execute.BeforeAll(func() {
		var err error
		n3000NodeList, err = helper.GetN3000NodeList(apiclient)
		if err != nil && err.Error() == "no matches for kind \"N3000Node\" in version \"fpga.intel.com/v1\"" {
			Skip("Cluster doesn't have intel fpga pac n3000 card")
		}
		Expect(err).ToNot(HaveOccurred())
		if len(n3000NodeList.Items) < 1 {
			Skip("No n3000 node is detected")
		}
		helper.CreateService(apiclient, parameters.TestNamespace)
		numberReadyN3000Daemonsets, numberDesiredN3000Daemonsets := helper.CountN3000Daemonsets(apiclient, parameters.OperatorNamespace)
		Expect(numberReadyN3000Daemonsets).To(Equal(numberDesiredN3000Daemonsets))
	})
	BeforeEach(func() {
		By("Flushing bitstream")
		fpgaStatus, err := helper.GetN3000FpgaStatus(apiclient)
		Expect(err).NotTo(HaveOccurred())
		bitstreamId = fpgaStatus.BitstreamID
		deviceId = fpgaStatus.DeviceID
		service, err := apiclient.Services(parameters.TestNamespace).List(context.Background(), metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		installNewN3000Image(n3000NodeList, fpgaStatus, parameters.ImageBitstreamFlush, parameters.ChecksumBitstreamImage, port, &service.Items[0])
	})

	AfterEach(func() {
		By("Cleaning up resources after test")
		fpgaStatus, err := helper.GetN3000FpgaStatus(apiclient)
		Expect(err).NotTo(HaveOccurred())
		service, err := apiclient.Services(parameters.TestNamespace).List(context.Background(), metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		installNewN3000Image(n3000NodeList, fpgaStatus, parameters.ImageDefault, parameters.ChecksumDefaultImage, port, &service.Items[0])
		n3000NodeCondition, err := helper.GetN3000NodeCondition(apiclient)
		Expect(err).NotTo(HaveOccurred())
		Expect(n3000NodeCondition.Status).To(Equal(metav1.ConditionStatus("True")), fmt.Sprintf("Default ClusterConfig with image \"%s\" is not applied properly", n3000NodeList.Items[0].Spec.FPGA[0].UserImageURL))
		helper.CleanAllN3000Cluster(apiclient)
	})

	Context("n3000", func() {
		// 39002
		It("Bitstream flushing", func() {
			fpgaStatus, err := helper.GetN3000FpgaStatus(apiclient)
			Expect(err).NotTo(HaveOccurred())
			n3000NodeCondition, err := helper.GetN3000NodeCondition(apiclient)
			Expect(err).NotTo(HaveOccurred())
			Expect(n3000NodeCondition.Message).To(Equal("Flashed successfully"), "Bitstream flushing failed")
			Expect(fpgaStatus.BitstreamID).NotTo(Equal(bitstreamId), "BitstreamId has not been changed after flushing")
			Expect(fpgaStatus.DeviceID).To(Equal(deviceId), "DeviceID has been changed or removed after flushing")
		})
	})

})

func installNewN3000Image(n3000NodeList *fpgav1.N3000NodeList, fpgaStatus *fpgav1.N3000FpgaStatus, image string, checksum string, port int32, service *corev1.Service) {
	helper.CleanAllN3000Cluster(apiclient)
	helper.CreatePodWithPort(apiclient, parameters.TestNamespace, parameters.ImageBitstreamImages, port)
	n3000Node := &n3000NodeList.Items[0]

	err := helper.CreateN3000ClusterConfig(apiclient, n3000Node.Name, image, service.Spec.ClusterIP, checksum, fpgaStatus.PciAddr)
	Expect(err).NotTo(HaveOccurred())
	By("Waiting until cluster become stable")
	err = nodes.WaitForClusterToBeStable(apiclient)
	Expect(err).NotTo(HaveOccurred())
}
