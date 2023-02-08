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

var _ = Describe("PTP Recovery", Label("ptp-recovery"), func() {
	var (
		errBeforeAll         error
		ptpConfigCount       int
		originPtpConfigSpecs = map[string]ptpv1.PtpConfigSpec{}
	)

	execute.BeforeAll(func() {
		var ptpConfigCounts []int
		originPtpConfigSpecs, ptpConfigCounts, errBeforeAll = ptpPretestValidations()
		ptpConfigCount = ptpConfigCounts[0]
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

	Context("ptp process restart", func() {
		// 49850
		It("should recover the phc2sys process after killing it", func() {
			nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
			Expect(err).NotTo(HaveOccurred())

			for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
				ptpNode, err := ranhelper.GetNodeByName(nodeName)
				Expect(err).NotTo(HaveOccurred())

				By("Kill ph2sys process on node " + ptpNode.Name)
				// get the phc2sys pid before killing it
				oldPID, err := ranptphelper.GetProcessPID(&ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())
				err = ranptphelper.KillPtpProcess(&ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())
				log.Printf("phc2sys PID %s killed", oldPID)

				By(fmt.Sprintf("Validate new phc2sys process is started on node %s", nodeName))
				newPID, err := ranptphelper.WaitForProcess(&ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())
				Expect(newPID).ShouldNot(Equal(oldPID))

				By("Validate all ptp clocks are in LOCKED state in ptp metrics")
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState,
					"", 1*time.Minute, 10*time.Second)
				Expect(err).NotTo(HaveOccurred())

				// test on one node only
				break
			}
		})

		// 57197
		It("should create a new ptp4l process after killing a ptp4l process that is not related to the "+
			"phc2sy process", func() {
			if ptpConfigCount < 2 {
				Skip("Test requires at least two PTP configs")
			}

			nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
			Expect(err).NotTo(HaveOccurred())

			for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
				workerNode, err := ranhelper.GetNodeByName(nodeName)
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("Kill a ptp4l process on node %s", workerNode.Name))
				// get the ptp4l PID that is not related to the phc2sys
				oldPtp4lPids, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, false)
				Expect(err).NotTo(HaveOccurred())
				oldPtp4lPid := oldPtp4lPids[0]

				oldPhc2sysPid, err := ranptphelper.GetProcessPID(&ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())

				err = ranptphelper.KillProcess(&ptpDaemonPod, oldPtp4lPid)
				Expect(err).NotTo(HaveOccurred())

				time.Sleep(1 * time.Second)
				By("Validate the phc2sys process is not affected by killing the ptp4l process")
				newPhc2sysPid, err := ranptphelper.WaitForProcess(&ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())
				Expect(newPhc2sysPid).Should(Equal(oldPhc2sysPid))

				By("Validate a new ptp4l process is started")
				newPtp4lPids, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(newPtp4lPids).ShouldNot(ContainElement(oldPtp4lPid))

				By("Validate all ptp clocks are in LOCKED state in ptp metrics")
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState,
					"", 1*time.Minute, 10*time.Second)
				Expect(err).NotTo(HaveOccurred())

				// test on one node only
				break
			}
		})

		// 49736
		It("should restart both ptp4l processes after killing them", func() {
			if ptpConfigCount < 2 {
				Skip("Test requires at least two PTP configs")
			}

			nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
			Expect(err).NotTo(HaveOccurred())

			for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
				workerNode, err := ranhelper.GetNodeByName(nodeName)
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("Kill the two ptp4l processes on node %s", workerNode.Name))
				// the ptp4l that is related to the phc2sys process
				oldPtp4lPidPhc2sys, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, true)
				Expect(err).NotTo(HaveOccurred())

				// the ptp4l that is not related to the phc2sys process
				oldPtp4lPids, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, false)
				Expect(err).NotTo(HaveOccurred())

				// kill the ptp4l process that is related to phc2sys process
				err = ranptphelper.KillProcess(&ptpDaemonPod, oldPtp4lPidPhc2sys[0])
				Expect(err).NotTo(HaveOccurred())

				// kill the ptp4l process that is NOT related to phc2sys process
				err = ranptphelper.KillProcess(&ptpDaemonPod, oldPtp4lPids[0])
				Expect(err).NotTo(HaveOccurred())

				By("Validate new ptp4l processes are started")
				// the new ptp4l that is not related to the phc2sys process
				newPtp4lPhc2sys, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, true)
				Expect(err).NotTo(HaveOccurred())
				// the new ptp4l that is related to the phc2sys process
				newPtp4lPids, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(newPtp4lPhc2sys[0]).ShouldNot(Equal(oldPtp4lPidPhc2sys[0]))
				Expect(newPtp4lPids).ShouldNot(ContainElement(oldPtp4lPids[0]))

				By("Validate all ptp clocks are in LOCKED state in ptp metrics")
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState,
					"", 1*time.Minute, 10*time.Second)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		// 49737
		It("should recover the ptp4l process after the killing a ptp4l process that is related to phc2sys process", func() {
			nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
			Expect(err).NotTo(HaveOccurred())

			for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
				workerNode, err := ranhelper.GetNodeByName(nodeName)
				Expect(err).NotTo(HaveOccurred())

				oldPtp4lPidsPhc2sys, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, true)
				Expect(err).NotTo(HaveOccurred())
				oldPhc2sysPid, err := ranptphelper.GetProcessPID(&ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("Kill a ptp4l process that is related to phc2sys on node %s", workerNode.Name))
				// get the ptp4l PID that is not related to the phc2sys
				err = ranptphelper.KillProcess(&ptpDaemonPod, oldPtp4lPidsPhc2sys[0])
				Expect(err).NotTo(HaveOccurred())

				By("Validate phc2sys process is not affected")
				time.Sleep(5 * time.Second)
				newPhc2sysPid, err := ranptphelper.WaitForProcess(&ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())
				Expect(newPhc2sysPid).To(Equal(oldPhc2sysPid))

				By("Validate a new ptp4l process is started")
				newPtl4lPidsPhc2sys, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(newPtl4lPidsPhc2sys[0]).ShouldNot(Equal(oldPtp4lPidsPhc2sys[0]))

				By("Validate all ptp clocks are in LOCKED state in ptp metrics")
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState,
					"", 1*time.Minute, 10*time.Second)
				Expect(err).NotTo(HaveOccurred())

				// test on one node only
				break
			}
		})
	})

	// 49738
	It("should recover to stable state after delete PTP daemon pod", func() {
		var ptpNode *corev1.Node
		ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
			metav1.ListOptions{LabelSelector: parameters.PtpDaemonsetLabelSelector})
		Expect(err).NotTo(HaveOccurred())

		ptpDaemonPod := &ptpDaemonPods.Items[0]
		ptpNode, err = ranhelper.GetNodeByName(ptpDaemonPod.Spec.NodeName)
		Expect(err).NotTo(HaveOccurred())

		// kill ptp pod.
		By("Validate event [LOCKED] after killing the publisher pod")
		err = helper.Apiclient.Pods(parameters.PtpOperatorNamespace).Delete(context.Background(),
			ptpDaemonPod.Name,
			metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())

		err = ranhelper.WaitForClusterRecover(ptpNode, []string{parameters.PtpOperatorNamespace})
		Expect(err).NotTo(HaveOccurred())

		ptpDaemonPod, err = ranptphelper.GetPtpDaemonPodFromNode(ptpNode)
		Expect(err).NotTo(HaveOccurred())

		By("Validate all ptp clocks are in LOCKED state in ptp metrics")
		err = ranptphelper.WaitForPtpClockStateMetric(*ptpDaemonPod, ranptpparameters.LockedState,
			"", 1*time.Minute, 10*time.Second)
		Expect(err).NotTo(HaveOccurred())
	})

	// 49743
	It("should return to same stable status after ptp node soft reboot", func() {

		ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
			metav1.ListOptions{LabelSelector: parameters.PtpDaemonsetLabelSelector})
		Expect(err).NotTo(HaveOccurred())
		ptpNode, err := ranhelper.GetNodeByName(ptpDaemonPods.Items[0].Spec.NodeName)
		Expect(err).NotTo(HaveOccurred())

		By("Soft reboot ptp node: " + ptpNode.Name)
		helper.SoftRebootNodeAndWaitForDisconnect(ptpNode)

		startTime := time.Now()
		err = ranhelper.WaitForClusterRecover(ptpNode, []string{parameters.PtpOperatorNamespace})
		Expect(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("Validate ptp clocks are [LOCKED] after node %s recovered", ptpNode.Name))
		ptpDaemonPod, err := ranptphelper.GetPtpDaemonPodFromNode(ptpNode)
		Expect(err).NotTo(HaveOccurred())

		By("Wait for all ptp clocks in LOCKED state in ptp metrics")
		err = ranptphelper.WaitForPtpClockStateMetric(*ptpDaemonPod, ranptpparameters.LockedState,
			"", 5*time.Minute, 10*time.Second)
		Expect(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("Wait for ptp events [LOCKED] for all PTP clocks after node %s recovered", ptpNode.Name))
		err = ranptphelper.WaitForEvent(ptpDaemonPod, "event.sync.ptp-status.ptp-state-change",
			ranptpparameters.EventLocked, "", time.Since(startTime), 1*time.Minute)
		Expect(err).NotTo(HaveOccurred())
	})
})
