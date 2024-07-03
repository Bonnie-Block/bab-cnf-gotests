package tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/schemes/ptp/ptpv1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"context"
	"fmt"
	"log"
	"time"
)

var _ = Describe("Basic PTP Configs", Ordered, ContinueOnFailure, func() {
	var (
		errBeforeAll         error
		originPtpConfigSpecs = map[string]ptpv1.PtpConfigSpec{}
	)

	BeforeAll(func() {
		originPtpConfigSpecs, _, errBeforeAll = ptpPretestValidations()
	})

	BeforeEach(func() {
		// Any failure in execute.BeforeAll only fails the first test case.
		if errBeforeAll != nil {
			Skip(fmt.Sprintf("Error encountered before the first test started: %v", errBeforeAll.Error()))
		}

		// Ensure ptp clocks are locked before starting any test
		err := checkPtpLockState(5*time.Second, 0)
		if err != nil {
			Skip("PTP clocks are not in locked states")
		}
	})

	AfterEach(func() {
		if CurrentSpecReport().State.String() == "skipped" {
			return
		}

		if CurrentSpecReport().Failed() {
			// Best effort print PTP container logs and metrics
			printPTPInfo()
		} else {
			// Best effort print PTP consumer logs
			printConsumerLog()
		}

		// Always restore ptpconfigs to original values after each test
		log.Println("Restore ptpconfigs to original specs")
		restorePtpConfigs(originPtpConfigSpecs)
		log.Println("Check ptp clocks are in sync")
		err := checkPtpLockState(5*time.Minute, 10*time.Second)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("PTP metrics", func() {
		// In the top level BeforeEach, since the  metric map is already updated when checking the ptp clock state,
		// it does not need to be repeated in here.

		// 66848
		It("should have [LOCKED] clock state in PTP metrics", polarion.ID("66848"), func() {
			for _, clockValueState := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpClockState] {
				if clockValueState.Interface != ranptpparameters.Master {
					Expect(clockValueState.ClockStateValue).Should(Equal(ranptpparameters.LockedState))
				}
			}
		})

		// 66848
		It("should have the 'phc2sys' and 'ptp4l' processes in 'UP' state in PTP metrics", polarion.ID("66848"), func() {
			if !ranhelper.IsVersionStringInRange(ranptpparameters.PtpVersion, "4.11", "") {
				Skip("ptp process metrics is not support in version " + ranptpparameters.PtpVersion)
			}

			for _, processState := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpProcessStatus] {
				if ranptpparameters.ProcessPTP4L == processState.Process {
					Expect(processState.ProcessStatusValue).Should(Equal(ranptpparameters.Up),
						"Unexpected ptp4l process_status_value for ptp config: "+processState.Config)
				}
				if ranptpparameters.ProcessPHC2SYS == processState.Process {
					Expect(processState.ProcessStatusValue).Should(Equal(ranptpparameters.Up),
						"Unexpected phc2sys process_status_value for ptp config: "+processState.Config)
				}
			}
		})
	})

	Context("change PTP offset thresholds", Ordered, func() {
		// 49741
		It("should change the slave clock state to free run after modify the offset threshold", polarion.ID("49741"), func() {
			nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
			Expect(err).NotTo(HaveOccurred())

			for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
				workerNode, err := ranhelper.GetNodeByName(nodeName)
				Expect(err).NotTo(HaveOccurred())

				log.Println("Save the current ptp thresholds values on node", workerNode.Name)
				err = ranptphelper.SaveOriginalValues()
				Expect(err).NotTo(HaveOccurred())

				verifyEventsAndMetricsModifyThresholds(&ptpDaemonPod, &ptpDaemonPod,
					ranptpparameters.CloudEventContainer, 5*time.Minute, false)

				By("Validate new values in ptpconfig")
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

				By("Validate ptp thresholds metrics values changed to new values in ptp metrics")
				for _, thresholdMetrics := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpThreshold] {
					err = ranptphelper.ThresholdsMetricsValsValidation(thresholdMetrics, ranptpparameters.ModifiedThresholdsValues)
					Expect(err).NotTo(HaveOccurred())
				}

				By("Reset to original PTP clock thresholds values")
				ptpConfigs, err = ranptphelper.GetPtpConfigs()
				Expect(err).NotTo(HaveOccurred())
				for _, ptpConfig := range ptpConfigs {
					err = ranptphelper.RestoreThresholdsValues(&ptpConfig)
					Expect(err).NotTo(HaveOccurred())
				}

				By("Validate slave clock state changed to [LOCKED] in ptp metrics")
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState, "",
					5*time.Minute, 30*time.Second)
				Expect(err).NotTo(HaveOccurred())

				// Test one node only
				break
			}
		})

		// 66848
		It("should have the 'phc2sys' and 'ptp4l' processes 'UP' after ptp config change", polarion.ID("66848"), func() {
			for _, processState := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpProcessStatus] {
				if ranptpparameters.ProcessPTP4L == processState.Process {
					Expect(processState.ProcessStatusValue).Should(Equal(ranptpparameters.Up),
						"Unexpected ptp4l process_status_value for ptp config: "+processState.Config)
				}
				if ranptpparameters.ProcessPHC2SYS == processState.Process {
					Expect(processState.ProcessStatusValue).Should(Equal(ranptpparameters.Up),
						"Unexpected phc2sys process_status_value for ptp config: "+processState.Config)
				}
			}
		})
	})
})

// getPtpConfigCounts counts ptpconfig types.
// arguments:		"ptpConfigsList"-	a list of ptpconfigs.
// return value:	an array of int index:
// 0 - total number of configs.
// 1 - OC configs.
// 2 - BC configs.
// 3 - GM configs.
func getPtpConfigCounts(ptpConfigsList ptpv1.PtpConfigList) []int {
	configCount, ocCount, bcCount, gmOneCount, gmTwoCount, haCount := 0, 0, 0, 0, 0, 0

	for _, ptpconfig := range ptpConfigsList.Items {
		for _, profile := range ptpconfig.Spec.Profile {
			configCount++

			if ranptphelper.IsGmOneCardProfile(profile) {
				gmOneCount++

				continue
			}

			if ranptphelper.IsGmTwoCardProfile(profile) {
				gmTwoCount++

				continue
			}

			if ranptphelper.IsOrdinaryClockProfile(profile) {
				ocCount++

				continue
			}

			if ranptphelper.IsHaProfile(profile) {
				haCount++

				continue
			}

			if ranptphelper.IsBoundaryClockProfile(profile) {
				bcCount++
			} else {
				log.Println("Warning: unrecognized PTP profile type: ", *profile.Name)
			}
		}
	}

	return []int{configCount, ocCount, bcCount, gmOneCount, gmTwoCount, haCount}
}

// restore ptp configs on system to original configs.
func restorePtpConfigs(originalPtpSpecs map[string]ptpv1.PtpConfigSpec) {
	err := ranptphelper.UpdatePtpConfigSpecs(originalPtpSpecs)
	Expect(err).ToNot(HaveOccurred())
}

func checkPtpLockState(timeout time.Duration, stableDuration time.Duration) error {
	// Validate that PTP event container is running1
	ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{LabelSelector: parameters.PtpDaemonsetLabelSelector})
	if err != nil {
		log.Println("Failed to get PTP pod list")

		return err
	}

	for _, ptpDaemonPod := range ptpDaemonPods.Items {
		helper.WaitForPodsHealthy([]*corev1.Pod{&ptpDaemonPod}, 3*time.Minute)
		err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState, "",
			timeout, stableDuration)

		if err != nil {
			return err
		}
	}

	return nil
}

// ptpPretestValidations Validate s ptp is healthy before tests begin.
// It returns the original ptpconfig specs, boundary clock configuration count, and error if encountered.
func ptpPretestValidations() (map[string]ptpv1.PtpConfigSpec, []int, error) {
	ptpDaemonPods, beforeAllErr := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{
			LabelSelector: parameters.PtpDaemonsetLabelSelector})
	Expect(beforeAllErr).NotTo(HaveOccurred())

	beforeAllErr = checkPtpLockState(5*time.Second, 0)
	Expect(beforeAllErr).NotTo(HaveOccurred())

	// Get the metrics details before changes_ptp_events_bc
	beforeAllErr = ranptphelper.GetPTPMetrics(ptpDaemonPods.Items[0])
	Expect(beforeAllErr).NotTo(HaveOccurred())

	originPtpConfigList, beforeAllErr := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).
		List(context.Background(), metav1.ListOptions{})
	Expect(beforeAllErr).ToNot(HaveOccurred())

	log.Println("Save ptpconfig specs")

	originPtpConfigSpecs := map[string]ptpv1.PtpConfigSpec{}
	for _, ptpconf := range originPtpConfigList.Items {
		originPtpConfigSpecs[ptpconf.Name] = ptpconf.Spec
	}

	configCounts := getPtpConfigCounts(*originPtpConfigList)

	log.Printf("PTP config counts: %v\n", configCounts)

	return originPtpConfigSpecs, configCounts, beforeAllErr
}

// verifyEventsAndMetricsModifyThresholds verifies ptp metrics and events by changing thresholds for HOLDOVER/FREERUN.
func verifyEventsAndMetricsModifyThresholds(ptpDaemonPod *corev1.Pod, pod *corev1.Pod,
	containerName string, timeout time.Duration, skipMetricCheck bool) {
	startTime := time.Now()
	time.Sleep(1 * time.Second)

	By("Validate no [FREERUN] event received via pod: " + pod.Name)
	err := ranptphelper.WaitForEvent(pod, containerName,
		"event.sync.ptp-status.ptp-state-change",
		ranptpparameters.EventFreeRun, "", "", startTime, 10*time.Second)
	Expect(err).To(HaveOccurred(), "FREERUN event is received before modifying maxoffset threshold")

	// reset start time for FREERUN event test
	startTime = time.Now()
	time.Sleep(1 * time.Second)

	configsList, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).
		List(context.Background(),
			metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	By("Modify the ptp profile ptpClockThresholds values to trigger FREERUN events")

	err = ranptphelper.SetThresholdsValAllConfigs(configsList, &ranptpparameters.ModifiedThresholdsValues)
	Expect(err).NotTo(HaveOccurred())

	By("Validate clock state changed to [FREERUN] in ptp events via pod: " + pod.Name)
	err = ranptphelper.WaitForEvent(pod, containerName,
		"event.sync.ptp-status.ptp-state-change",
		ranptpparameters.EventFreeRun, "", "", startTime, timeout)
	Expect(err).NotTo(HaveOccurred())

	if !skipMetricCheck {
		var excludedProcess []string
		if containsGMProfile(configsList) {
			// If GM is configured and forced to FREERUN,the clock class changes to 248 while downstream slaves'
			// clock_class_threshold set to 7 - This causes slave ports to stuck in Listening without clock_state
			// updates in ptp metric. Thus, when GM is configured, ptp4l clock state check for downstream slaves
			// should be ignored.
			// dpll and ts2phc offsets usually stays at 0, thus offsetThreshold change would not force those
			// clock_states to FREERUN.
			excludedProcess = []string{ranptpparameters.ProcessPTP4L, ranptpparameters.ProcessDPLL,
				ranptpparameters.ProcessTS2PHC}
		}

		By("Validate clock state changed to [FREERUN] in ptp metrics")

		err = ranptphelper.WaitForPtpClockStateMetric(*ptpDaemonPod, ranptpparameters.FreeRunState, "",
			timeout, 0, excludedProcess...)
		Expect(err).NotTo(HaveOccurred())
	}
}

// containsGMProfile returns whether GM is configured.
func containsGMProfile(ptpConfigList *ptpv1.PtpConfigList) bool {
	for _, ptpConfig := range ptpConfigList.Items {
		for _, profile := range ptpConfig.Spec.Profile {
			if ranptphelper.IsGmOneCardProfile(profile) || ranptphelper.IsGmTwoCardProfile(profile) {
				return true
			}
		}
	}

	return false
}

// printPTPInfo prints ptp container logs and metrics with best effort.
func printPTPInfo() {
	var duration time.Duration

	ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{LabelSelector: parameters.PtpDaemonsetLabelSelector})

	if err != nil {
		log.Println("ERROR:", err.Error())

		return
	}

	for _, ptpDaemonPod := range ptpDaemonPods.Items {
		// print ptp container logs
		for _, container := range ptpDaemonPod.Spec.Containers {
			duration = 1 * time.Minute
			if container.Name == parameters.PtpContainerName {
				duration = 1 * time.Second
			}

			ptpContainerLog, err := pod.GetLog(helper.Apiclient, &ptpDaemonPod, duration, container.Name)
			if err == nil {
				log.Printf("Logs from last %s for pod %s container %s:\n%s",
					duration.String(), ptpDaemonPod.Name, container.Name, ptpContainerLog)
			}
		}

		// print ptp metrics
		_ = ranptphelper.GetPTPMetrics(ptpDaemonPod, true)
	}

	printConsumerLog()
}

func printConsumerLog() {
	var duration time.Duration

	// Make sure cloud-event-consumer namespace exists
	if namespaces.Exists(parameters.CloudEventNamespace, helper.Apiclient) {
		log.Println(parameters.CloudEventNamespace, " exists ")

		cloudEventPods, err := helper.Apiclient.Pods(parameters.CloudEventNamespace).List(context.Background(),
			metav1.ListOptions{LabelSelector: parameters.ConsumerLabelSelector})

		if err != nil {
			log.Println("ERROR:", err.Error())

			return
		}

		if len(cloudEventPods.Items) == 0 {
			log.Println("ERROR: no pods were found in ", parameters.CloudEventNamespace)

			return
		}

		duration = 1 * time.Minute

		for _, cloudEventPod := range cloudEventPods.Items {
			if ranhelper.IsContainerExistInPod(cloudEventPod, ranptpparameters.ConsumerContainer) {
				consumerContainerLog, err := pod.GetLog(helper.Apiclient, &cloudEventPod, duration,
					ranptpparameters.ConsumerContainer)

				if err == nil {
					log.Printf("Logs from last %s for pod %s container %s:\n%s",
						duration.String(), cloudEventPod.Name, ranptpparameters.ConsumerContainer,
						consumerContainerLog)
				}
			} else {
				log.Println("ERROR: no", ranptpparameters.ConsumerContainer, "container found in pod",
					cloudEventPod.Name)
			}
		}
	}
}
