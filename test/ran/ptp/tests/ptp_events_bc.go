package tests

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("PTP Events", func() {
	var (
		workerNodesList []corev1.Node
	)

	execute.BeforeAll(func() {
		var err error

		// Get all worker nodes
		workerNodesList, err = nodes.GetByRole(helper.Apiclient, "worker")
		Expect(err).NotTo(HaveOccurred())

		ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
			metav1.ListOptions{
				LabelSelector: parameters.PtpDaemonsetLabelSelector})
		Expect(err).NotTo(HaveOccurred())

		// Get the metrics details before changes
		ptpMetricsBuff, err := ranptphelper.GetPTPMetrics(ptpDaemonPods.Items[0])
		Expect(err).NotTo(HaveOccurred())
		err = ranptphelper.MetricParser(ptpMetricsBuff)
		Expect(err).NotTo(HaveOccurred())
	})

	BeforeEach(func() {

		// Validate that PTP event container is running
		ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
			metav1.ListOptions{
				LabelSelector: parameters.PtpDaemonsetLabelSelector})
		Expect(err).NotTo(HaveOccurred())

		for _, ptpDaemonPod := range ptpDaemonPods.Items {
			err = helper.IsPodHealthy(&ptpDaemonPod)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("PTP event config", func() {
		// 47047
		It("should behave correctly for different thresholds on ordinaryClock", func() {
			// Get ptp daemon pods
			ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
				metav1.ListOptions{
					LabelSelector: parameters.PtpDaemonsetLabelSelector})
			Expect(err).NotTo(HaveOccurred())

			nodeToPtpDaemonPod := ranptphelper.NodesToPtpDaemonPods(workerNodesList, ptpDaemonPods)

			for workerNode, ptpDaemonPod := range nodeToPtpDaemonPod {
				By("verify event LOCKED")
				lastEvent, err := ranptphelper.GetLastEventValue(ptpDaemonPod)
				Expect(err).NotTo(HaveOccurred())
				Expect(lastEvent).Should(Equal(ranptpparameters.Locked))

				// kill ptp pod.
				By("verify event LOCKED after killing the publisher pod")
				err = helper.Apiclient.Pods(parameters.PtpOperatorNamespace).Delete(context.Background(),
					ptpDaemonPod.Name,
					metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				err = ranptphelper.WaitForClusterRecover(workerNode)
				Expect(err).NotTo(HaveOccurred())

				newPtpDaemonPod, err := ranptphelper.GetPtpDaemonPodFromNode(workerNode)
				Expect(err).NotTo(HaveOccurred())

				// Update the map with the new pod
				nodeToPtpDaemonPod[workerNode] = newPtpDaemonPod

				lastEvent, err = ranptphelper.GetLastEventValue(newPtpDaemonPod)
				Expect(err).NotTo(HaveOccurred())
				Expect(lastEvent).Should(Equal(ranptpparameters.Locked))
			}

			// Node reboot for only one of the nodes
			workerNode := workerNodesList[0]
			By("verify event LOCKED after node port went down")
			helper.SoftRebootNodeAndWaitForDisconnect(&workerNode)

			err = ranptphelper.WaitForClusterRecover(&workerNode)
			Expect(err).NotTo(HaveOccurred())

			ptpDaemonPod, err := ranptphelper.GetPtpDaemonPodFromNode(&workerNode)
			Expect(err).NotTo(HaveOccurred())

			lastEvent, err := ranptphelper.GetLastEventValue(ptpDaemonPod)
			Expect(err).NotTo(HaveOccurred())

			Expect(lastEvent).Should(Equal(ranptpparameters.Locked))
		})

		Context("PTP events metrics", func() {
			It("should have 'LOCKED' clock state", func() {
				for _, clockValueState := range ranptpparameters.MetricMap["openshift_ptp_clock_state"] {
					if clockValueState.Interface != "master" {
						Expect(clockValueState.ClockStateValue).Should(Equal(ranptpparameters.LockedState))
					}
				}
			})

			It("should have the 'phc2sys' and  'ptp4l' process in 'UP' state", func() {
				for _, processState := range ranptpparameters.MetricMap["openshift_ptp_process_status"] {
					if ranptpparameters.PTP4L == processState.Process {
						Expect(processState.ProcessStatusValue).Should(Equal(ranptpparameters.Up))
					}
					if ranptpparameters.PHC2SYS == processState.Process {
						Expect(processState.ProcessStatusValue).Should(Equal(ranptpparameters.Up))
					}
				}
			})
		})
	})
})
