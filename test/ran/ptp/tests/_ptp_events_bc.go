package tests

import (
	"context"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("PTP Events with boundary clock", func() {
	var (
		containersName = []string{"kube-rbac-proxy", "linuxptp-daemon-container", "cloud-event-proxy"}
		pod            *corev1.Pod
		node           *corev1.Node
	)
	execute.BeforeAll(func() {
		Expect(helper.Apiclient).NotTo(BeNil())

		// Validate that PTP event container is running
		pods, err := helper.Apiclient.Pods(ranptpparameters.PtpNameSpace).List(context.Background(), metav1.ListOptions{
			LabelSelector: ranptpparameters.LabelSelector})
		Expect(err).NotTo(HaveOccurred())
		pod = &pods.Items[0]
		err = ranptphelper.CheckContainersInPodByNames(pod, containersName)
		Expect(err).NotTo(HaveOccurred())
		nodesList, err := nodes.GetByRole(helper.Apiclient, "worker")
		node = &nodesList[0]
		Expect(err).NotTo(HaveOccurred())

	})

	Context("PTP event config", func() {

		//
		It("should behave correctly for different thresholds on ordinaryClock", func() {
			By("verify event LOCKED")
			lastEvent, err := ranptphelper.GetLastEventValue()
			Expect(err).NotTo(HaveOccurred())

			Expect(lastEvent).Should(Equal(ranptpparameters.LOCKED))

			// kill ptp pod
			By("verify event LOCKED after killing the publisher pod")
			err = helper.Apiclient.Pods(ranptpparameters.PtpNameSpace).Delete(context.Background(), pod.Name,
				metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			err = ranptphelper.WaitForClusterRecover(node)
			Expect(err).NotTo(HaveOccurred())

			lastEvent, err = ranptphelper.GetLastEventValue()
			Expect(err).NotTo(HaveOccurred())

			Expect(lastEvent).Should(Equal(ranptpparameters.LOCKED))

			// SNO reboot
			By("verify event LOCKED after SNO port went down")

			helper.SoftRebootNodeAndWaitForDisconnect(node)

			err = ranptphelper.WaitForClusterRecover(node)
			Expect(err).NotTo(HaveOccurred())

			lastEvent, err = ranptphelper.GetLastEventValue()
			Expect(err).NotTo(HaveOccurred())

			Expect(lastEvent).Should(Equal(ranptpparameters.LOCKED))
		})

	})

})
