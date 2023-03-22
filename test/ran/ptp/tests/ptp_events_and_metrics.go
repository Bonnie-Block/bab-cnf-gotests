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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"context"
	"fmt"
	"log"
	"time"
)

var _ = Describe("Basic PTP Configs", func() {
	var (
		errBeforeAll         error
		originPtpConfigSpecs = map[string]ptpv1.PtpConfigSpec{}
	)

	execute.BeforeAll(func() {
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

		It("should have [LOCKED] clock state in PTP metrics", func() {
			for _, clockValueState := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpClockState] {
				if clockValueState.Interface != ranptpparameters.Master {
					Expect(clockValueState.ClockStateValue).Should(Equal(ranptpparameters.LockedState))
				}
			}
		})

		It("should have the 'phc2sys' and 'ptp4l' processes in 'UP' state in PTP metrics", func() {
			if !ranhelper.IsVersionStringInRange(ranptpparameters.PtpVersion, "4.11", "") {
				Skip("ptp process metrics is not support in version " + ranptpparameters.PtpVersion)
			}

			for _, processState := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpProcessStatus] {
				if ranptpparameters.PTP4L == processState.Process {
					Expect(processState.ProcessStatusValue).Should(Equal(ranptpparameters.Up),
						"Unexpected ptp4l process_status_value for ptp config: "+processState.Config)
				}
				if ranptpparameters.PHC2SYS == processState.Process {
					Expect(processState.ProcessStatusValue).Should(Equal(ranptpparameters.Up),
						"Unexpected phc2sys process_status_value for ptp config: "+processState.Config)
				}
			}
		})
	})

	Context("change PTP offset thresholds", Ordered, func() {
		// 49741
		It("should change the slave clock state to free run after modify the offset threshold", func() {
			nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
			Expect(err).NotTo(HaveOccurred())

			for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
				workerNode, err := ranhelper.GetNodeByName(nodeName)
				Expect(err).NotTo(HaveOccurred())

				log.Println("Save the current ptp thresholds values on node", workerNode.Name)
				err = ranptphelper.SaveOriginalValues()
				Expect(err).NotTo(HaveOccurred())

				configsList, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).
					List(context.Background(),
						metav1.ListOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("Modify the ptp config - ptp clock thresholds value")
				err = ranptphelper.SetThresholdsValAllConfigs(configsList, &ranptpparameters.ModifiedThresholdsValues)
				Expect(err).NotTo(HaveOccurred())

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

				By("Validate slave clock state changed to [FREERUN] in ptp metrics")
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.FreeRunState, "",
					5*time.Minute, 10*time.Second)
				Expect(err).NotTo(HaveOccurred())

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

		It("should have the 'phc2sys' and 'ptp4l' processes 'UP' after ptp config change", func() {
			for _, processState := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpProcessStatus] {
				if ranptpparameters.PTP4L == processState.Process {
					Expect(processState.ProcessStatusValue).Should(Equal(ranptpparameters.Up),
						"Unexpected ptp4l process_status_value for ptp config: "+processState.Config)
				}
				if ranptpparameters.PHC2SYS == processState.Process {
					Expect(processState.ProcessStatusValue).Should(Equal(ranptpparameters.Up),
						"Unexpected phc2sys process_status_value for ptp config: "+processState.Config)
				}
			}
		})
	})
})

func getPtpConfigCounts(ptpConfigsList ptpv1.PtpConfigList) []int {
	configCount, ocCount, bcCount /*, gmCount*/ := 0, 0, 0 /*, 0*/

	for _, ptpconfig := range ptpConfigsList.Items {
		for _, profile := range ptpconfig.Spec.Profile {
			configCount++

			if ranptphelper.IsOrdinaryClockProfile(profile) {
				ocCount++

				continue
			}

			if ranptphelper.IsBoundaryClockProfile(profile) {
				bcCount++

				continue
			}

			//if ranptphelper.IsGMProfile(profile) {
			//	gmCount++
			//} else {
			//	log.Println("Warning: unrecognized PTP profile type: ", *profile.Name)
			//}
		}
	}

	return []int{configCount, ocCount, bcCount}
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

	ptpConfigCounts := getPtpConfigCounts(*originPtpConfigList)

	return originPtpConfigSpecs, ptpConfigCounts, beforeAllErr
}
