package tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ptpv1 "github.com/openshift/ptp-operator/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"context"
	"fmt"
	"log"
	"time"
)

var _ = Describe("PTP Events", func() {
	var (
		workerNodesList []corev1.Node
		err             error
		ptpDaemonPods   *corev1.PodList
	)
	execute.BeforeAll(func() {
		// Get all worker nodes
		workerNodesList, err = nodes.GetByRole(helper.Apiclient, "worker")
		Expect(err).NotTo(HaveOccurred())

		ptpDaemonPods, err = helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
			metav1.ListOptions{
				LabelSelector: parameters.PtpDaemonsetLabelSelector})
		Expect(err).NotTo(HaveOccurred())

		// Get the metrics details before changes_ptp_events_bc
		err := ranptphelper.GetPTPMetrics(ptpDaemonPods.Items[0])
		Expect(err).NotTo(HaveOccurred())
	})

	BeforeEach(func() {
		// Validate that PTP event container is running1
		ptpDaemonPods, err = helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
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
		It("should return to same stable status after delete daemon pod and node reboot ", func() {
			// Get ptp daemon pods
			ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
				metav1.ListOptions{
					LabelSelector: parameters.PtpDaemonsetLabelSelector})
			Expect(err).NotTo(HaveOccurred())

			nodeToPtpDaemonPod := ranptphelper.NodesToPtpDaemonPods(workerNodesList, ptpDaemonPods)

			for workerNode, ptpDaemonPod := range nodeToPtpDaemonPod {
				By("verify event [LOCKED]")
				lastEvent, err := ranptphelper.GetLastEventValue(ptpDaemonPod)
				Expect(err).NotTo(HaveOccurred())
				Expect(lastEvent).Should(Equal(ranptpparameters.Locked))

				// kill ptp pod.
				By("verify event [LOCKED] after killing the publisher pod")
				err = helper.Apiclient.Pods(parameters.PtpOperatorNamespace).Delete(context.Background(),
					ptpDaemonPod.Name,
					metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				err = ranhelper.WaitForClusterRecover(workerNode, []string{parameters.PtpOperatorNamespace})
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
			By(fmt.Sprintf("verify event [LOCKED] after node %s port went down", workerNode.Name))
			helper.SoftRebootNodeAndWaitForDisconnect(&workerNode)

			err = ranhelper.WaitForClusterRecover(&workerNode, []string{parameters.PtpOperatorNamespace})
			Expect(err).NotTo(HaveOccurred())

			ptpDaemonPod, err := ranptphelper.GetPtpDaemonPodFromNode(&workerNode)
			Expect(err).NotTo(HaveOccurred())

			lastEvent, err := ranptphelper.GetLastEventValue(ptpDaemonPod)
			Expect(err).NotTo(HaveOccurred())

			Expect(lastEvent).Should(Equal(ranptpparameters.Locked))
		})
	})
	Context("PTP events metrics", func() {
		It("should have [LOCKED] clock state", func() {
			for _, clockValueState := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpClockState] {
				if clockValueState.Interface != ranptpparameters.Master {
					Expect(clockValueState.ClockStateValue).Should(Equal(ranptpparameters.LockedState))
				}
			}
		})

		It("should have the 'phc2sys' and  'ptp4l' process in 'UP' state", func() {
			for _, processState := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpProcessStatus] {
				if ranptpparameters.PTP4L == processState.Process {
					Expect(processState.ProcessStatusValue).Should(Equal(ranptpparameters.Up))
				}
				if ranptpparameters.PHC2SYS == processState.Process {
					Expect(processState.ProcessStatusValue).Should(Equal(ranptpparameters.Up))
				}
			}
		})
	})

	Context("reset Interfaces", func() {
		It("should generate events when slave interface goes down and up", func() {
			nodeToPtpDaemonPod := ranptphelper.NodesToPtpDaemonPods(workerNodesList, ptpDaemonPods)
			ifaces, err := ranptphelper.GetInterfaces(ptpv1.Slave)
			Expect(err).NotTo(HaveOccurred())
			for workerNode, ptpDaemonPod := range nodeToPtpDaemonPod {
				for _, iface := range ifaces {
					By(fmt.Sprintf("verify phc2sys is on [LOCKED] state "+
						"before slave NIC interface %s goes down on node %s", iface, workerNode.Name))
					// metrics check
					log.Printf("Metrics check")
					err := ranptphelper.GetPTPMetrics(*ptpDaemonPod)
					Expect(err).NotTo(HaveOccurred())
					for _, processState := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpClockState] {
						if iface == processState.Interface {
							Expect(processState.ClockStateValue).Should(Equal(ranptpparameters.LockedState))
						}
					}

					By(fmt.Sprintf("verify event entering to"+
						" [HOLDOVER] state after salve interface %s goes down", iface))
					err = ranptphelper.SetInterfaceStatus(ptpDaemonPod,
						parameters.PtpContainerName,
						iface,
						ranptpparameters.Off)
					Expect(err).NotTo(HaveOccurred())
					// events check
					log.Printf("Events check")
					eventSlaveDown, err := ranptphelper.GetEventValueFromEndOfEventByKeyValue(ptpDaemonPod,
						map[string]string{"type": "event.sync.ptp-status.ptp-state-change"}, 0)
					Expect(err).NotTo(HaveOccurred())
					Expect(eventSlaveDown).Should(Equal(ranptpparameters.HoldOver))
					// metrics check
					log.Printf("Metrics check")
					err = ranptphelper.GetPTPMetrics(*ptpDaemonPod)
					Expect(err).NotTo(HaveOccurred())

					timeout, err := ranptphelper.GetTimeoutVal()
					Expect(err).NotTo(HaveOccurred())
					timeout += 10 * time.Second
					log.Printf("wait for thershold holdover + 10s timeout: %s\n", timeout)
					time.Sleep(timeout)

					By(fmt.Sprintf("verify event [FREERUN] after salve interface "+
						"%s goes down on node %s", iface, workerNode.Name))
					// events check
					log.Printf("Events check")
					eventHoldoverTimeout, err := ranptphelper.GetEventValueFromEndOfEventByKeyValue(ptpDaemonPod,
						map[string]string{"type": "event.sync.ptp-status.ptp-state-change"}, 0)
					Expect(err).NotTo(HaveOccurred())
					Expect(eventHoldoverTimeout).Should(Equal(ranptpparameters.FreeRun))
					// metrics check
					log.Printf("Metrics check")
					err = ranptphelper.GetPTPMetrics(*ptpDaemonPod)
					Expect(err).NotTo(HaveOccurred())
					for _, processState := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpClockState] {
						if iface == processState.Interface {
							Expect(processState.ClockStateValue).Should(Equal(ranptpparameters.FreeRunState))
						}
					}

					By(fmt.Sprintf("verify event entering to [LOCKED] state after salve interface"+
						" %s goes up on node %s", iface, workerNode.Name))
					err = ranptphelper.SetInterfaceStatus(ptpDaemonPod,
						parameters.PtpContainerName,
						iface,
						ranptpparameters.On)
					Expect(err).NotTo(HaveOccurred())

					log.Println("10 seconds timeout after interface went up")
					time.Sleep(10 * time.Second)
					// events check
					log.Printf("Events check")
					eventSlaveUp, err := ranptphelper.GetEventValueFromEndOfEventByKeyValue(ptpDaemonPod,
						map[string]string{"type": "event.sync.ptp-status.ptp-state-change"}, 0)
					Expect(err).NotTo(HaveOccurred())
					Expect(eventSlaveUp).Should(Equal(ranptpparameters.Locked))
					// metrics check
					log.Printf("Metrics check")
					err = ranptphelper.GetPTPMetrics(*ptpDaemonPod)
					Expect(err).NotTo(HaveOccurred())
					for _, processState := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpClockState] {
						if iface == processState.Interface {
							Expect(processState.ClockStateValue).Should(Equal(ranptpparameters.LockedState))
						}
					}
				}
			}
		})

		It("should have no effect when master interface goes down and up", func() {
			nodeToPtpDaemonPod := ranptphelper.NodesToPtpDaemonPods(workerNodesList, ptpDaemonPods)
			ifaces, err := ranptphelper.GetInterfaces(ptpv1.Master)
			Expect(err).NotTo(HaveOccurred())
			for workerNode, ptpDaemonPod := range nodeToPtpDaemonPod {
				for _, iface := range ifaces {
					By(fmt.Sprintf(
						"verify event is still on [LOCKED] state after master interface"+
							" %s goes down on node %s", iface, workerNode.Name))
					err = ranptphelper.SetInterfaceStatus(ptpDaemonPod,
						parameters.PtpContainerName,
						iface,
						ranptpparameters.Off)
					Expect(err).NotTo(HaveOccurred())

					lastEvent, err := ranptphelper.WaitForLastEvent(ptpDaemonPod)
					Expect(err).NotTo(HaveOccurred())
					Expect(lastEvent).Should(Equal(ranptpparameters.Locked))

					By(fmt.Sprintf("verify event is still on [LOCKED] state after master interface"+
						"%s goes up on node %s",
						iface, workerNode.Name))
					err = ranptphelper.SetInterfaceStatus(ptpDaemonPod,
						parameters.PtpContainerName,
						iface,
						ranptpparameters.On)
					Expect(err).NotTo(HaveOccurred())
					lastEvent, err = ranptphelper.WaitForLastEvent(ptpDaemonPod)
					Expect(err).NotTo(HaveOccurred())
					Expect(lastEvent).Should(Equal(ranptpparameters.Locked))
				}
			}
		})
	})

	Context("rests process", func() {
		It("should recover the phc2sys process after killing it", func() {
			nodeToPtpDaemonPod := ranptphelper.NodesToPtpDaemonPods(workerNodesList, ptpDaemonPods)
			for workerNode, ptpDaemonPod := range nodeToPtpDaemonPod {
				// get the phc2sys pid before killing it
				oldPID, err := ranptphelper.GetProcessPID(ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())
				err = ranptphelper.KillPtpProcess(ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())

				eventAfterKill, err := ranptphelper.GetEventValueFromEndOfEventByKeyValue(ptpDaemonPod,
					map[string]string{"type": "event.sync.sync-status.os-clock-sync-state-change"}, 0)
				Expect(err).NotTo(HaveOccurred())
				newPID, err := ranptphelper.GetProcessPID(ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())

				eventAfterRecovery, err := ranptphelper.GetEventValueFromEndOfEventByKeyValue(ptpDaemonPod,
					map[string]string{"type": "event.sync.sync-status.os-clock-sync-state-change"}, 0)
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("a new new phc2sys process is running after kill phc2sys process"+
					" on node %s", workerNode.Name))
				Expect(newPID).ShouldNot(Equal(oldPID))

				By("event status changed to [FREERUN] after phc2sys process was killed")
				Expect(eventAfterKill).Should(Equal(ranptpparameters.FreeRun))

				By("event status changed to [LOCKED] after phc2sys process reset")
				Expect(eventAfterRecovery).Should(Equal(ranptpparameters.Locked))
			}
		})

		It("should create a new ptp4l process after killing a ptp4l process that is not "+
			"related to the phc2sy process", func() {
			nodeToPtpDaemonPod := ranptphelper.NodesToPtpDaemonPods(workerNodesList, ptpDaemonPods)
			for workerNode, ptpDaemonPod := range nodeToPtpDaemonPod {
				By(fmt.Sprintf("killing a ptp4l process on node %s", workerNode.Name))
				// get the ptp4l PID that is not related to the phc2sys
				oldPTP4lPID, err := ranptphelper.GetPTP4lPID(ptpDaemonPod, false)
				Expect(err).NotTo(HaveOccurred())
				oldPhc2sysPid, err := ranptphelper.GetProcessPID(ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())
				err = ranptphelper.KillProcess(ptpDaemonPod, oldPTP4lPID)
				Expect(err).NotTo(HaveOccurred())
				event, err := ranptphelper.GetEventValueFromEndOfEventByKeyValue(ptpDaemonPod,
					map[string]string{"type": "event.sync.ptp-status.ptp-state-change"}, 0)
				Expect(err).NotTo(HaveOccurred())
				newPTP4lPID, err := ranptphelper.GetPTP4lPID(ptpDaemonPod, false)
				Expect(err).NotTo(HaveOccurred())
				newEvent, err := ranptphelper.GetEventValueFromEndOfEventByKeyValue(ptpDaemonPod,
					map[string]string{"type": "event.sync.ptp-status.ptp-state-change"}, 0)
				Expect(err).NotTo(HaveOccurred())
				newPhc2sysPid, err := ranptphelper.GetProcessPID(ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())

				By("validate the event ptp status changed to [FREERUN] after ptp4l process was killed")
				Expect(event).Should(Equal(ranptpparameters.FreeRun))

				By("validate a new ptp4l process reset")
				Expect(newPTP4lPID).ShouldNot(Equal(oldPTP4lPID))

				By("validate the event ptp status changed to [LOCKED] after ptp4l process reset")
				Expect(newEvent).Should(Equal(ranptpparameters.Locked))

				By(fmt.Sprintf("validate the phc2sys process not effected by killing the ptp4l process" +
					"(same PID before and after reset the ptp4l process)"))
				Expect(newPhc2sysPid).Should(Equal(oldPhc2sysPid))
			}
		})

		It("should reset both ptp4l after killing both of them", func() {
			nodeToPtpDaemonPod := ranptphelper.NodesToPtpDaemonPods(workerNodesList, ptpDaemonPods)
			for workerNode, ptpDaemonPod := range nodeToPtpDaemonPod {
				By(fmt.Sprintf("killing the two ptp4l processes on node %s", workerNode.Name))

				// the ptp4l that is related to the phc2sys process
				oldPtp4l1, err := ranptphelper.GetPTP4lPID(ptpDaemonPod, true)
				Expect(err).NotTo(HaveOccurred())
				// the ptp4l that is not related to the phc2sys process
				oldPtp4l2, err := ranptphelper.GetPTP4lPID(ptpDaemonPod, false)
				Expect(err).NotTo(HaveOccurred())

				// kill the ptp4l process that is related to phc2sys process
				err = ranptphelper.KillProcess(ptpDaemonPod, oldPtp4l1)
				Expect(err).NotTo(HaveOccurred())
				oldEvent1, err := ranptphelper.GetEventValueFromEndOfEventByKeyValue(ptpDaemonPod,
					map[string]string{"type": "event.sync.sync-status.os-clock-sync-state-change"}, 1)
				Expect(err).NotTo(HaveOccurred())

				// kill the ptp4l process that is NOT related to phc2sys process
				err = ranptphelper.KillProcess(ptpDaemonPod, oldPtp4l2)
				Expect(err).NotTo(HaveOccurred())
				oldEvent2, err := ranptphelper.GetEventValueFromEndOfEventByKeyValue(ptpDaemonPod,
					map[string]string{"type": "event.sync.ptp-status.ptp-state-change"}, 0)
				Expect(err).NotTo(HaveOccurred())

				// the new ptp4l that is not related to the phc2sys process
				newPtp4l1, err := ranptphelper.GetPTP4lPID(ptpDaemonPod, false)
				Expect(err).NotTo(HaveOccurred())
				// the new ptp4l that is related to the phc2sys process
				newPtp4l2, err := ranptphelper.GetPTP4lPID(ptpDaemonPod, true)
				Expect(err).NotTo(HaveOccurred())

				newEvent1, err := ranptphelper.GetEventValueFromEndOfEventByKeyValue(ptpDaemonPod,
					map[string]string{"type": "event.sync.sync-status.os-clock-sync-state-change"}, 0)
				Expect(err).NotTo(HaveOccurred())
				newEvent2, err := ranptphelper.GetEventValueFromEndOfEventByKeyValue(ptpDaemonPod,
					map[string]string{"type": "event.sync.ptp-status.ptp-state-change"}, 0)
				Expect(err).NotTo(HaveOccurred())

				By("validate the event clock sync status changed to [FREERUN] after ptp4l process " +
					"(related to phc2sys) was killed")
				Expect(oldEvent1).Should(Equal(ranptpparameters.FreeRun))

				By("validate the event ptp status changed to [FREERUN] after ptp4l process " +
					"(NOT related to phc2sys) was killed")
				Expect(oldEvent2).Should(Equal(ranptpparameters.FreeRun))

				By("validate a new ptp4l processes reset")
				Expect(newPtp4l1).ShouldNot(Equal(oldPtp4l1))
				Expect(newPtp4l2).ShouldNot(Equal(oldPtp4l2))

				By("validate the event clock sync status changed to [LOCKED] after ptp4l process " +
					"(related to phc2sys) reset")
				Expect(newEvent1).Should(Equal(ranptpparameters.Locked))

				By("validate the event ptp status changed to [LOCKED] after ptp4l process" +
					" (NOT related to phc2sys) reset")
				Expect(newEvent2).Should(Equal(ranptpparameters.Locked))
			}
		})
	})
	Context("change offset thresholds", func() {
		It("should change the slave clock state to free run after modify the offset threshold", func() {
			nodeToPtpDaemonPod := ranptphelper.NodesToPtpDaemonPods(workerNodesList, ptpDaemonPods)
			for workerNode, ptpDaemonPod := range nodeToPtpDaemonPod {
				By(fmt.Sprintf("save the current ptp thresholds values on node %s", workerNode.Name))
				err := ranptphelper.SaveOriginalValues()
				Expect(err).NotTo(HaveOccurred())

				By("modify the ptp config - ptp clock thresholds value")
				configsList, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).
					List(context.Background(),
						metav1.ListOptions{})
				Expect(err).NotTo(HaveOccurred())
				err = ranptphelper.SetThresholdsValAllConfigs(configsList, &ranptpparameters.ModifiedThresholdsValues)
				Expect(err).NotTo(HaveOccurred())
				timeout := 3 * time.Minute
				log.Printf("waits %s minutes to update the thresholds values\n", timeout.String())
				time.Sleep(timeout)

				By("validate new values")
				ptpConfigs, err := ranptphelper.GetPtpConfigs()
				Expect(err).NotTo(HaveOccurred())
				for _, ptpConfig := range ptpConfigs {
					Expect(ptpConfig.Spec.Profile[0].PtpClockThreshold.HoldOverTimeout).Should(
						Equal(ranptpparameters.ModifiedThresholdsValues.HoldOverTimeout))
					Expect(ptpConfig.Spec.Profile[0].PtpClockThreshold.MaxOffsetThreshold).Should(
						Equal(ranptpparameters.ModifiedThresholdsValues.MaxOffsetThreshold))
					Expect(ptpConfig.Spec.Profile[0].PtpClockThreshold.MinOffsetThreshold).Should(
						Equal(ranptpparameters.ModifiedThresholdsValues.MinOffsetThreshold))
				}

				By("validate ptp thresholds metrics values change to new values")
				for _, thresholdMetrics := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpThreshold] {
					err = thresholdsMetricsValsValidation(thresholdMetrics, ranptpparameters.ModifiedThresholdsValues)
					Expect(err).NotTo(HaveOccurred())
				}

				By("validate slave clock state chang to [FREERUN]")
				err = ranptphelper.GetPTPMetrics(*ptpDaemonPod)
				Expect(err).NotTo(HaveOccurred())
				for _, clockValueState := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpClockState] {
					if clockValueState.Interface != ranptpparameters.Master {
						Expect(clockValueState.ClockStateValue).Should(Equal(ranptpparameters.FreeRunState))
					}
				}

				By("return to original clock thresholds values")
				for _, ptpConfig := range ptpConfigs {
					err = ranptphelper.RestoreThresholdsValues(&ptpConfig)
					Expect(err).NotTo(HaveOccurred())
				}
				log.Printf("waits %s minutes to update the thresholds values\n", timeout.String())
				time.Sleep(timeout)
				ptpConfigs, err = ranptphelper.GetPtpConfigs()
				Expect(err).NotTo(HaveOccurred())

				for name, ptpConfig := range ptpConfigs {
					By(fmt.Sprintf("validate original values inside the configuration %s", name))
					Expect(ptpConfig.Spec.Profile[0].PtpClockThreshold.HoldOverTimeout).Should(
						Equal(ranptpparameters.OriginalThresholdsValues[name].HoldOverTimeout))
					Expect(ptpConfig.Spec.Profile[0].PtpClockThreshold.MaxOffsetThreshold).Should(
						Equal(ranptpparameters.OriginalThresholdsValues[name].MaxOffsetThreshold))
					Expect(ptpConfig.Spec.Profile[0].PtpClockThreshold.MinOffsetThreshold).Should(
						Equal(ranptpparameters.OriginalThresholdsValues[name].MinOffsetThreshold))

					By("validate ptp thresholds metrics values returned to original values")
					for _, thresholdMetrics := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpThreshold] {
						err = thresholdsMetricsValsValidation(thresholdMetrics,
							*ranptpparameters.OriginalThresholdsValues[name])
						Expect(err).NotTo(HaveOccurred())
					}
				}

				By("validate slave clock state chang to [LOCKED]")
				err = ranptphelper.GetPTPMetrics(*ptpDaemonPod)
				Expect(err).NotTo(HaveOccurred())
				for _, clockValueState := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpClockState] {
					if clockValueState.Interface != ranptpparameters.Master {
						Expect(clockValueState.ClockStateValue).Should(Equal(ranptpparameters.LockedState))
					}
				}
			}
		})
	})

})

// thresholdsMetricsValsValidation validates the correct clock threshold values inside the metrics.
// arguments:		"thresholdMetrics"-	a single clock threshold metrics string.
//					"thresholdVals"-	the expected values.
// return value:	an error if the threshold value is not one of the clock threshold parameters.
func thresholdsMetricsValsValidation(thresholdMetrics ranptpparameters.MetricDetails,
	thresholdVals ptpv1.PtpClockThreshold) error {
	switch thresholdMetrics.Threshold {
	case ranptpparameters.HoldOverTimeout:
		Expect(thresholdMetrics.Value).Should(Equal(thresholdVals.HoldOverTimeout))

		return nil
	case ranptpparameters.MaxOffsetThreshold:
		Expect(thresholdMetrics.Value).Should(Equal(thresholdVals.MaxOffsetThreshold))

		return nil
	case ranptpparameters.MinOffsetThreshold:
		Expect(thresholdMetrics.Value).Should(Equal(thresholdVals.MinOffsetThreshold))

		return nil
	default:
		return fmt.Errorf("threshold value %s is undefined", thresholdMetrics.Threshold)
	}
}
