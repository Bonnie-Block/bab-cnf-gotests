package tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/schemes/ptp/ptpv1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

var _ = Describe("PTP Recovery", Label("ptp-recovery"), func() {
	var (
		errBeforeAll         error
		ptpConfigCounts      []int
		originPtpConfigSpecs = map[string]ptpv1.PtpConfigSpec{}
	)
	const (
		configsIndx                = 0
		bcConfigIndx               = 2
		gmOneCardConfigIndx        = 3
		gmTwoCardConfigIndx        = 4
		highAvailabilityConfigIndx = 5
	)

	execute.BeforeAll(func() {
		originPtpConfigSpecs, ptpConfigCounts, errBeforeAll = ptpPretestValidations()
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
		if CurrentSpecReport().Failed() {
			// Best effort print PTP container logs and metrics
			printPTPInfo()
		}

		restorePtpInterfaces()
		// Always restore ptpconfigs to original values after each test
		log.Println("Restore ptpconfigs to original specs")
		restorePtpConfigs(originPtpConfigSpecs)
		log.Println("Check ptp clocks are in sync")
		err := checkPtpLockState(5*time.Minute, 10*time.Second)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("ptp process restart", func() {
		// 49850
		It("should recover the phc2sys process after killing it", polarion.ID("49850"), func() {
			nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
			Expect(err).NotTo(HaveOccurred())

			for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
				ptpNode, err := ranhelper.GetNodeByName(nodeName)
				Expect(err).NotTo(HaveOccurred())

				By("Kill ph2sys process on node " + ptpNode.Name)
				// get the phc2sys pid before killing it
				oldPID, err := ranptphelper.GetProcessPID(&ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())

				sinceTime := time.Now()

				err = ranptphelper.KillPtpProcess(&ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())
				log.Printf("phc2sys PID %s killed", oldPID)

				By("validating FREERUN event received after killing phc2sys process")
				err = ranptphelper.WaitForEvent(&ptpDaemonPod, ranptpparameters.CloudEventContainer,
					"event.sync.sync-status.os-clock-sync-state-change", ranptpparameters.EventFreeRun, "",
					time.Since(sinceTime), 3*time.Minute)
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("validate new phc2sys process is started on node %s", nodeName))
				newPID, err := ranptphelper.WaitForProcess(&ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())
				Expect(newPID).ShouldNot(Equal(oldPID))

				By("validating LOCKED event received after killing phc2sys process")
				err = ranptphelper.WaitForEvent(&ptpDaemonPod, ranptpparameters.CloudEventContainer,
					"event.sync.sync-status.os-clock-sync-state-change", ranptpparameters.EventLocked, "",
					time.Since(sinceTime), 3*time.Minute)
				Expect(err).NotTo(HaveOccurred())

				By("validate all ptp clocks are in LOCKED state in ptp metrics")
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState, "", 1*time.Minute,
					10*time.Second)
				Expect(err).NotTo(HaveOccurred())

				// test on one node only
				break
			}
		})

		// 57197
		It("should create a new ptp4l process after killing a ptp4l process that is not related to the "+
			"phc2sy process", polarion.ID("57197"), func() {
			if ptpConfigCounts[configsIndx] < 2 {
				Skip("Test requires at least two PTP configs")
			}

			nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
			Expect(err).NotTo(HaveOccurred())

			for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
				workerNode, err := ranhelper.GetNodeByName(nodeName)
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("Kill a ptp4l process on node %s", workerNode.Name))
				// get the ptp4l PID that is not related to the phc2sys
				oldPtp4lPids, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, "phc2sys", false)
				Expect(err).NotTo(HaveOccurred())
				oldPtp4lPid := oldPtp4lPids[0]

				oldPhc2sysPid, err := ranptphelper.GetProcessPID(&ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())

				sinceTime := time.Now()

				err = ranptphelper.KillProcess(&ptpDaemonPod, oldPtp4lPid)
				Expect(err).NotTo(HaveOccurred())

				By("validating FREERUN event received after killing ptp4l process")
				err = ranptphelper.WaitForEvent(&ptpDaemonPod, ranptpparameters.CloudEventContainer,
					"event.sync.ptp-status.ptp-state-change", ranptpparameters.EventFreeRun, "", time.Since(sinceTime),
					3*time.Minute)
				Expect(err).NotTo(HaveOccurred())

				time.Sleep(1 * time.Second)
				By("validate the phc2sys process is not affected by killing the ptp4l process")
				newPhc2sysPid, err := ranptphelper.WaitForProcess(&ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())
				Expect(newPhc2sysPid).Should(Equal(oldPhc2sysPid))

				By("validate a new ptp4l process is started")
				newPtp4lPids, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, "phc2sys", false)
				Expect(err).NotTo(HaveOccurred())
				Expect(newPtp4lPids).ShouldNot(ContainElement(oldPtp4lPid))

				By("validating LOCKED event received after killing ptp4l process")
				err = ranptphelper.WaitForEvent(&ptpDaemonPod, ranptpparameters.CloudEventContainer,
					"event.sync.ptp-status.ptp-state-change", ranptpparameters.EventLocked, "", time.Since(sinceTime),
					3*time.Minute)
				Expect(err).NotTo(HaveOccurred())

				By("validate all ptp clocks are in LOCKED state in ptp metrics")
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState, "", 1*time.Minute,
					10*time.Second)
				Expect(err).NotTo(HaveOccurred())

				// test on one node only
				break
			}
		})

		// 49736
		It("should restart both ptp4l processes with one related to phc2sys after killing them",
			polarion.ID("49736"), func() {
				if ptpConfigCounts[configsIndx] < 2 || ptpConfigCounts[bcConfigIndx] == 0 {
					Skip("Test requires at least two PTP configs with BC configuration")
				}

				if ptpConfigCounts[highAvailabilityConfigIndx] > 0 {
					Skip("Test requires the phc2sys and ptp4l to be configured in one profile")
				}

				nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
				Expect(err).NotTo(HaveOccurred())

				for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
					workerNode, err := ranhelper.GetNodeByName(nodeName)
					Expect(err).NotTo(HaveOccurred())

					By(fmt.Sprintf("Kill the two ptp4l processes on node %s", workerNode.Name))
					// the ptp4l that is related to the phc2sys process
					oldPtp4lPidPhc2sys, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, "phc2sys", true)
					Expect(err).NotTo(HaveOccurred())

					// the ptp4l that is not related to the phc2sys process
					oldPtp4lPids, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, "phc2sys", false)
					Expect(err).NotTo(HaveOccurred())

					// kill the ptp4l process that is related to phc2sys process
					err = ranptphelper.KillProcess(&ptpDaemonPod, oldPtp4lPidPhc2sys[0])
					Expect(err).NotTo(HaveOccurred())

					// kill the ptp4l process that is NOT related to phc2sys process
					err = ranptphelper.KillProcess(&ptpDaemonPod, oldPtp4lPids[0])
					Expect(err).NotTo(HaveOccurred())

					By("validate new ptp4l processes are started")
					// the new ptp4l that is not related to the phc2sys process
					newPtp4lPhc2sys, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, "phc2sys", true)
					Expect(err).NotTo(HaveOccurred())
					// the new ptp4l that is related to the phc2sys process
					newPtp4lPids, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, "phc2sys", false)
					Expect(err).NotTo(HaveOccurred())

					Expect(newPtp4lPhc2sys[0]).ShouldNot(Equal(oldPtp4lPidPhc2sys[0]))
					Expect(newPtp4lPids).ShouldNot(ContainElement(oldPtp4lPids[0]))

					By("validate all ptp clocks are in LOCKED state in ptp metrics")
					err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState, "", 1*time.Minute,
						10*time.Second)
					Expect(err).NotTo(HaveOccurred())
				}
			})

		// 49737
		It("should recover the ptp4l process after the killing "+
			"a ptp4l process that is related to phc2sys process", polarion.ID("49737"), func() {
			if ptpConfigCounts[highAvailabilityConfigIndx] > 0 {
				Skip("Test requires the phc2sys and ptp4l to be configured in one profile")
			}

			nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
			Expect(err).NotTo(HaveOccurred())

			for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
				workerNode, err := ranhelper.GetNodeByName(nodeName)
				Expect(err).NotTo(HaveOccurred())

				oldPtp4lPidsPhc2sys, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, "phc2sys", true)
				Expect(err).NotTo(HaveOccurred())
				oldPhc2sysPid, err := ranptphelper.GetProcessPID(&ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("Kill a ptp4l process that is related to phc2sys on node %s", workerNode.Name))
				// get the ptp4l PID that is not related to the phc2sys
				err = ranptphelper.KillProcess(&ptpDaemonPod, oldPtp4lPidsPhc2sys[0])
				Expect(err).NotTo(HaveOccurred())

				By("validate phc2sys process is not affected")
				time.Sleep(5 * time.Second)
				newPhc2sysPid, err := ranptphelper.WaitForProcess(&ptpDaemonPod, "phc2sys")
				Expect(err).NotTo(HaveOccurred())
				Expect(newPhc2sysPid).To(Equal(oldPhc2sysPid))

				By("validate a new ptp4l process is started")
				newPtl4lPidsPhc2sys, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, "phc2sys", true)
				Expect(err).NotTo(HaveOccurred())
				Expect(newPtl4lPidsPhc2sys[0]).ShouldNot(Equal(oldPtp4lPidsPhc2sys[0]))

				By("validate all ptp clocks are in LOCKED state in ptp metrics")
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState, "", 1*time.Minute,
					10*time.Second)
				Expect(err).NotTo(HaveOccurred())

				// test on one node only
				break
			}
		})
		// 59863
		It("should recover the ts2phc process after the killing a ts2phc process", polarion.ID("59863"), func() {
			if ptpConfigCounts[gmOneCardConfigIndx] == 0 && ptpConfigCounts[gmTwoCardConfigIndx] == 0 {
				Skip("Test requires grand master configuration")
			}

			nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
			Expect(err).NotTo(HaveOccurred())

			for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
				workerNode, err := ranhelper.GetNodeByName(nodeName)
				Expect(err).NotTo(HaveOccurred())

				log.Println("get ts2phc PID")
				pid, err := ranptphelper.GetProcessPID(&ptpDaemonPod, "ts2phc")
				Expect(err).NotTo(HaveOccurred())

				sinceTime := time.Now()

				By(fmt.Sprintf("Kill a ts2phc process on node %s", workerNode.Name))
				err = ranptphelper.KillProcess(&ptpDaemonPod, pid)
				Expect(err).NotTo(HaveOccurred())

				By("validating FREERUN event received after killing ts2phc process")
				err = ranptphelper.WaitForEvent(&ptpDaemonPod, ranptpparameters.CloudEventContainer,
					"event.sync.ptp-status.ptp-state-change", ranptpparameters.EventFreeRun, "", time.Since(sinceTime),
					3*time.Minute)
				Expect(err).NotTo(HaveOccurred())

				By("validate a new ts2phc process is started")
				log.Println("get new ts2phc PID")
				newPid, err := ranptphelper.GetProcessPID(&ptpDaemonPod, "ts2phc")
				Expect(err).NotTo(HaveOccurred())
				Expect(pid).ShouldNot(Equal(newPid))

				By("validating LOCKED event received after killing ts2phc process")
				err = ranptphelper.WaitForEvent(&ptpDaemonPod, ranptpparameters.CloudEventContainer,
					"event.sync.ptp-status.ptp-state-change", ranptpparameters.EventLocked, "", time.Since(sinceTime),
					3*time.Minute)
				Expect(err).NotTo(HaveOccurred())

				By("validate all ptp clocks are in LOCKED state in ptp metrics")
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState, "", 1*time.Minute,
					10*time.Second)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		// 59863
		It("should recover the ptp4l process after the killing a "+
			"ptp4l process that is related to ts2phc process", polarion.ID("59863"), func() {
			if ptpConfigCounts[gmOneCardConfigIndx] == 0 && ptpConfigCounts[gmTwoCardConfigIndx] == 0 {
				Skip("Test requires grand master configuration")
			}

			nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
			Expect(err).NotTo(HaveOccurred())

			for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
				workerNode, err := ranhelper.GetNodeByName(nodeName)
				Expect(err).NotTo(HaveOccurred())

				log.Println("get ptp4l PID")
				pid, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, "ts2phc", true)
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("Kill a ptp4l process that is related to ts2phc on node %s", workerNode.Name))
				err = ranptphelper.KillProcess(&ptpDaemonPod, pid[0])
				Expect(err).NotTo(HaveOccurred())

				By("validate a new ptp4l process is started")
				log.Println("new get ptp4l PID")
				newPid, err := ranptphelper.GetPtp4lPids(&ptpDaemonPod, "ts2phc", true)
				Expect(err).NotTo(HaveOccurred())
				Expect(pid).ShouldNot(Equal(newPid))

				By("validate all ptp clocks are in LOCKED state in ptp metrics")
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState, "", 1*time.Minute,
					10*time.Second)
				Expect(err).NotTo(HaveOccurred())

			}
		})

		// 64777
		It("should recover gpsd process after killing it on node ", polarion.ID("64777"), func() {
			if ptpConfigCounts[gmOneCardConfigIndx] == 0 && ptpConfigCounts[gmTwoCardConfigIndx] == 0 {
				Skip("Test requires grand master configuration")
			}

			nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
			Expect(err).NotTo(HaveOccurred())

			for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
				workerNode, err := ranhelper.GetNodeByName(nodeName)
				Expect(err).NotTo(HaveOccurred())

				log.Println("get gpsd PID")
				pid, err := ranptphelper.GetProcessPID(&ptpDaemonPod, "gpsd")
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("Kill a gpsd process on node %s", workerNode.Name))
				err = ranptphelper.KillProcess(&ptpDaemonPod, pid)
				Expect(err).NotTo(HaveOccurred())

				By("validate a new gpsd process is started")
				log.Println("get new gpsd PID")
				newPid, err := ranptphelper.GetProcessPID(&ptpDaemonPod, "gpsd")
				Expect(err).NotTo(HaveOccurred())
				Expect(pid).ShouldNot(Equal(newPid))

				By("validate all ptp clocks are in LOCKED state in ptp metrics")
				err = ranptphelper.WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState, "", 1*time.Minute,
					10*time.Second)
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})

	// 49738
	It("should recover to stable state after delete PTP daemon pod", polarion.ID("49738"), func() {
		var ptpNode *corev1.Node
		ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
			metav1.ListOptions{LabelSelector: parameters.PtpDaemonsetLabelSelector})
		Expect(err).NotTo(HaveOccurred())

		ptpDaemonPod := &ptpDaemonPods.Items[0]
		ptpNode, err = ranhelper.GetNodeByName(ptpDaemonPod.Spec.NodeName)
		Expect(err).NotTo(HaveOccurred())

		// kill ptp pod.
		By("validate event [LOCKED] after killing the publisher pod")
		err = helper.Apiclient.Pods(parameters.PtpOperatorNamespace).Delete(context.Background(),
			ptpDaemonPod.Name,
			metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())

		err = ranhelper.WaitForClusterRecover(ptpNode, []string{parameters.PtpOperatorNamespace})
		Expect(err).NotTo(HaveOccurred())

		ptpDaemonPod, err = ranptphelper.GetPtpDaemonPodFromNode(ptpNode)
		Expect(err).NotTo(HaveOccurred())

		By("validate all ptp clocks are in LOCKED state in ptp metrics")
		err = ranptphelper.WaitForPtpClockStateMetric(*ptpDaemonPod, ranptpparameters.LockedState, "", 1*time.Minute,
			10*time.Second)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("HTTP events using consumer", Ordered, func() {
		var (
			consumerNode *corev1.Node
			consumerPod  *corev1.Pod
			ptpDaemonPod *corev1.Pod
		)

		BeforeAll(func() {
			transport, err := ranptphelper.GetPtpTransport()
			Expect(err).NotTo(HaveOccurred())
			if transport != ranparameters.TransportHTTP {
				Skip("This test can only be applied to HTTP transport")
			}

			consumerNode, consumerPod = getConsumerNodeAndPod(parameters.CloudEventNamespace)
			ptpDaemonPods, err := getPtpDaemonPods(consumerNode.Name)
			Expect(err).NotTo(HaveOccurred())
			ptpDaemonPod = &ptpDaemonPods.Items[0]
		})

		AfterAll(func() {
			// Make sure consumer exists or redeployed after destroy consumer test case
			getConsumerNodeAndPod(parameters.CloudEventNamespace)
		})

		// 59992
		It("validates HTTP PTP events via consumer", polarion.ID("59992"), func() {
			// Verify communication between publisher to consumer
			verifyConsumerEvents(consumerNode, consumerPod)
		})

		// 59996
		It("validates the system is fully functional after removing consumer", polarion.ID("59996"), func() {

			By("Remove the consumer")
			destroyErrors := ranhelper.DestroyConsumers(parameters.CloudEventNamespace)
			for _, err := range destroyErrors {
				Expect(err).ShouldNot(HaveOccurred())
			}

			// Validate the PTP events are working after the consumer is removed.
			By("Verify PTP events and metrics after the consumer is removed")
			verifyEventsAndMetricsModifyThresholds(ptpDaemonPod, ptpDaemonPod,
				ranptpparameters.CloudEventContainer, 10*time.Minute, false)

			By("Restore PTP configs and wait for clock locked")
			restorePtpConfigs(originPtpConfigSpecs)
			err := checkPtpLockState(5*time.Minute, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			// Once the configmap is available, the verification of subscriber removal will be re-enabled.
			// By("Verify subscriber is removed")
			// err = verifySubscriberIsRemoved(consumerNode, startTime)
			// Expect(err).NotTo(HaveOccurred())

			// Redeploy consumer and validate the consumer get the events
			By("Redeploy the consumer")
			consumersList, err := ranptphelper.DeployPtpConsumer()
			Expect(err).NotTo(HaveOccurred())
			log.Println("Redeployed consumer:", consumersList.Items[0].Name)
			consumerNode, consumerPod = getConsumerNodeAndPod(parameters.CloudEventNamespace)

			By("Verify consumer events again")
			verifyConsumerEvents(consumerNode, consumerPod)
		})
	})

	Context("ptp node reboot", Ordered, func() {
		// 49743
		It("should return to same stable status after ptp node soft reboot", polarion.ID("49743"), func() {

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

			By(fmt.Sprintf("validate ptp clocks are [LOCKED] after node %s recovered", ptpNode.Name))
			ptpDaemonPod, err := ranptphelper.GetPtpDaemonPodFromNode(ptpNode)
			Expect(err).NotTo(HaveOccurred())

			By("Wait for all ptp clocks in LOCKED state in ptp metrics")
			err = ranptphelper.WaitForPtpClockStateMetric(*ptpDaemonPod, ranptpparameters.LockedState, "", 10*time.Minute,
				10*time.Second)
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("Wait for ptp events [LOCKED] for all PTP clocks after node %s recovered", ptpNode.Name))
			err = ranptphelper.WaitForEvent(ptpDaemonPod, ranptpparameters.CloudEventContainer,
				"event.sync.ptp-status.ptp-state-change",
				ranptpparameters.EventLocked, "", time.Since(startTime), 1*time.Minute)
			Expect(err).NotTo(HaveOccurred())
		})

		// 59995
		It("validates PTP consumer events after ptp node reboot", polarion.ID("59995"), func() {
			By("Workaround for OCPBUGS-12954 - sleep for 5 minutes after reboot.")
			time.Sleep(5 * time.Minute)

			By("Generate events by changing ptp offset thresholds and verify events are received by the consumer")
			// Verify communication between publisher to consumer
			consumerNode, consumerPod := getConsumerNodeAndPod(parameters.CloudEventNamespace)
			verifyConsumerEvents(consumerNode, consumerPod)
		})
	})

	Context("restart GM", func() {
		It("should make nmea lost after GPS cold reboot", polarion.ID("70111"), func() {
			if ptpConfigCounts[gmOneCardConfigIndx] == 0 && ptpConfigCounts[gmTwoCardConfigIndx] == 0 {
				Skip("Test requires Grandmaster configuration")
			}

			ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
				metav1.ListOptions{LabelSelector: parameters.PtpDaemonsetLabelSelector})
			Expect(err).NotTo(HaveOccurred())

			ptpDaemonPod := &ptpDaemonPods.Items[0]

			By("checking the nmea metrics is in available state")
			err = ranptphelper.WaitForMetricValueStatus(*ptpDaemonPod, ranptpparameters.OpenshiftPtpNmeaStatus,
				ranptpparameters.Available, 1*time.Minute, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())

			// Time to start log monitoring for nmea loss - right before GPS cold reboot
			startTime := time.Now()

			By("GPS cold boot via pod:" + ptpDaemonPod.Name)
			err = ranptphelper.GpsColdReboot(ptpDaemonPod)
			Expect(err).NotTo(HaveOccurred())

			// Time to start log monitoring for recovery
			startTimeRecover := time.Now()

			By("checking dpll holdover state via linuxptp-daemon log - 'state is HOLDOVER'")
			err = ranptphelper.WaitForLog(ptpDaemonPod, parameters.PtpContainerName, "state is HOLDOVER",
				time.Since(startTime), 1*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			By("checking nmea loss via linuxptp-daemon log - 'nmea string lost'")
			err = ranptphelper.WaitForLog(ptpDaemonPod, parameters.PtpContainerName, "nmea string lost",
				time.Since(startTime), 1*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			By("checking nmea loss via metrics - openshift_ptp_nmea_status metrics becomes unavailable")
			err = ranptphelper.WaitForMetricValueStatus(*ptpDaemonPod, ranptpparameters.OpenshiftPtpNmeaStatus,
				ranptpparameters.Unavailable, 1*time.Minute, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())

			By("checking recovery via linuxptp-daemon log - 'dpll is locked'")
			err = ranptphelper.WaitForLog(ptpDaemonPod, parameters.PtpContainerName, "dpll is locked",
				time.Since(startTimeRecover), 1*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			By("checking recovery via metrics - openshift_ptp_nmea_status metrics becomes available ")
			err = ranptphelper.WaitForMetricValueStatus(*ptpDaemonPod, ranptpparameters.OpenshiftPtpNmeaStatus,
				ranptpparameters.Available, 1*time.Minute, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})
	Context("disable SMA connection between the two cards", func() {
		var (
			rxInterface  string
			ptpDaemonPod *corev1.Pod
			ptpConfigs   *ptpv1.PtpConfigList
		)

		BeforeEach(func() {
			if ptpConfigCounts[gmTwoCardConfigIndx] == 0 {
				Skip("Test requires two Grandmaster configurations")
			}
			ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
				metav1.ListOptions{LabelSelector: parameters.PtpDaemonsetLabelSelector})
			Expect(err).NotTo(HaveOccurred())
			ptpDaemonPod = &ptpDaemonPods.Items[0]
			log.Printf("get ptpconfig")
			ptpConfigs, err = helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).List(context.Background(),
				metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())

			gmPtpConfiguration, err := ranptphelper.GetGmPtpConfig(*ptpConfigs)
			Expect(err).NotTo(HaveOccurred())

			rxInterface, err = ranptphelper.GetRxIface(*gmPtpConfiguration)
			Expect(err).NotTo(HaveOccurred())
			log.Printf("RX interface plugin %s", rxInterface)
		})

		AfterEach(func() {
			if ptpConfigCounts[gmTwoCardConfigIndx] == 0 {
				Skip("Test requires two Grandmaster configurations")
			}
			// make sure sma connection is up.
			rxSma, err := ranptphelper.GetSma(ptpDaemonPod, rxInterface)
			Expect(err).NotTo(HaveOccurred())
			if rxSma != "1 1" {
				err = ranptphelper.SetSma(ptpDaemonPod, rxInterface, "1 1")
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("checks FREERUN status are generated for dpll process for RX interface and GM process for TX "+
			"interface", polarion.ID("70114"), func() {

			gmPtpConfiguration, err := ranptphelper.GetGmPtpConfig(*ptpConfigs)
			Expect(err).NotTo(HaveOccurred())

			txInterface, err := ranptphelper.GetTxIface(*gmPtpConfiguration)
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("modify SMA1 value for interface %s, in pod  %s to 0 1", rxInterface,
				ptpDaemonPod.Name))
			err = ranptphelper.SetSma(ptpDaemonPod, rxInterface, "0 1")
			Expect(err).NotTo(HaveOccurred())

			readSMA, err := ranptphelper.GetSma(ptpDaemonPod, rxInterface)
			Expect(err).NotTo(HaveOccurred())
			log.Printf("interface %s has sma1 value of: %s, expected result 0 1", rxInterface, readSMA)

			By(fmt.Sprintf("Wait for FREERUN states for dpll process for RX interface %s", rxInterface))
			err = ranptphelper.WaitForPtpClockStateMetric(*ptpDaemonPod, ranptpparameters.FreeRunState, rxInterface,
				1*time.Minute, 5*time.Second, "ts2phc")
			Expect(err).NotTo(HaveOccurred())
			log.Printf("FREERUN state found for RX interface %s in pod %s", rxInterface, ptpDaemonPod.Name)

			By(fmt.Sprintf("Wait for FREERUN states for GM process for TX interface %s", txInterface))
			err = ranptphelper.WaitForPtpClockStateMetric(*ptpDaemonPod, ranptpparameters.FreeRunState, txInterface,
				1*time.Minute, 5*time.Second, "dpll", "gnss", "ts2phc")
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("modify SMA1 value for RX interface %s, in pod  %s to 1 1", rxInterface,
				ptpDaemonPod.Name))
			err = ranptphelper.SetSma(ptpDaemonPod, rxInterface, "1 1")
			Expect(err).NotTo(HaveOccurred())
			readSMA, err = ranptphelper.GetSma(ptpDaemonPod, rxInterface)
			Expect(err).NotTo(HaveOccurred())
			log.Printf("interface %s has sma1 value of: %s, expected result 1 1", rxInterface, readSMA)

			err = ranptphelper.WaitForMetricValueStatus(*ptpDaemonPod, ranptpparameters.OpenshiftPtpPpsStatus,
				ranptpparameters.Available, 1*time.Minute, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

func getConsumerNodeAndPod(namespace string) (*corev1.Node, *corev1.Pod) {
	// Ensure the consumer is running.
	consumersList, err := ranhelper.GetConsumers(namespace)
	if err != nil {
		By("Failed to get consumer, redeploy the consumer")

		consumersList, err = ranptphelper.DeployPtpConsumer()
		Expect(err).NotTo(HaveOccurred())
	}

	consumerPod := &consumersList.Items[0]
	// Get the node object where the consumer pod is running.
	consumerNode, err := ranhelper.GetNodeByName(consumerPod.Spec.NodeName)
	Expect(err).NotTo(HaveOccurred())

	return consumerNode, consumerPod
}

// Verify events are being received the cloud-event-consumer.
func verifyConsumerEvents(consumerNode *corev1.Node, consumerPod *corev1.Pod) {
	// ptp-event-publisher-service is added in 4.12
	if ranhelper.IsVersionStringInRange(ranptpparameters.PtpVersion, "4.12", "") {
		// Validate the ptp-event-publisher-service is running in the required namespace.
		nodeName := strings.Split(consumerNode.Name, ".")[0]
		validatePublisherService(nodeName)
	}

	// Get the ptp daemon pod that runs on the same node as the consumer.
	ptpDaemonPods, err := getPtpDaemonPods(consumerNode.Name)
	Expect(err).NotTo(HaveOccurred())

	// Ensure the consumer is ready for events.
	err = ranptphelper.WaitForConsumerReady(consumerPod)
	Expect(err).NotTo(HaveOccurred())

	verifyEventsAndMetricsModifyThresholds(&ptpDaemonPods.Items[0], consumerPod,
		ranptpparameters.ConsumerContainer, 10*time.Minute, false)
}
