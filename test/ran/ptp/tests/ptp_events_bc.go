package tests

import (
	"context"
	"fmt"
	"log"
	"time"

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
)

var _ = Describe("PTP Events", Ordered, func() {
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

	AfterAll(func() {
		var allIfaces []string
		slaveIfaces, err := ranptphelper.GetInterfaces(ptpv1.Slave)
		Expect(err).NotTo(HaveOccurred())
		masterIfaces, err := ranptphelper.GetInterfaces(ptpv1.Master)
		Expect(err).NotTo(HaveOccurred())
		allIfaces = append(allIfaces, slaveIfaces...)
		allIfaces = append(allIfaces, masterIfaces...)
		for _, iface := range allIfaces {
			err = ranptphelper.SetInterfaceStatus(&ptpDaemonPods.Items[0],
				parameters.PtpContainerName,
				iface,
				ranptpparameters.On)
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
					err := ranptphelper.GetPTPMetrics(ptpDaemonPods.Items[0])
					Expect(err).NotTo(HaveOccurred())
					for _, processState := range ranptpparameters.MetricMap["openshift_ptp_clock_state"] {
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
					err = ranptphelper.GetPTPMetrics(ptpDaemonPods.Items[0])
					Expect(err).NotTo(HaveOccurred())
					for _, processState := range ranptpparameters.MetricMap["openshift_ptp_clock_state"] {
						if iface == processState.Interface {
							Expect(processState.ClockStateValue).Should(Equal(ranptpparameters.HoldOverState))
						}
					}

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
					err = ranptphelper.GetPTPMetrics(ptpDaemonPods.Items[0])
					Expect(err).NotTo(HaveOccurred())
					for _, processState := range ranptpparameters.MetricMap["openshift_ptp_clock_state"] {
						if iface == processState.Interface {
							Expect(processState.ClockStateValue).Should(Equal(ranptpparameters.FreeRunState))
						}
					}

					By(fmt.Sprintf("verify event entering to [LOCKED] state after salve interface"+
						" %s goes up on node %s", iface, workerNode.Name))
					err = ranptphelper.SetInterfaceStatus(&ptpDaemonPods.Items[0],
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
					err = ranptphelper.GetPTPMetrics(ptpDaemonPods.Items[0])
					Expect(err).NotTo(HaveOccurred())
					for _, processState := range ranptpparameters.MetricMap["openshift_ptp_clock_state"] {
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

					By(fmt.Sprintf("verify event is still on [LOCKED] state after master interface %s goes up",
						iface))
					err = ranptphelper.SetInterfaceStatus(&ptpDaemonPods.Items[0],
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

	Context("resets process", func() {
		It("should recover the phc2sys process after killing it", func() {
			nodeToPtpDaemonPod := ranptphelper.NodesToPtpDaemonPods(workerNodesList, ptpDaemonPods)
			for workerNode, ptpDaemonPod := range nodeToPtpDaemonPod {
				// get the phc2sys pid before killing it
				pid, err := ranptphelper.GetProcessPID("phc2sys", ptpDaemonPod)
				Expect(err).NotTo(HaveOccurred())
				err = ranptphelper.KillPtpProcess("phc2sys", ptpDaemonPod)
				Expect(err).NotTo(HaveOccurred())
				By(fmt.Sprintf("verify a new phc2sys process is running after kill phc2sys process"+
					" on node %s", workerNode.Name))
				newPID, err := ranptphelper.GetProcessPID("phc2sys", ptpDaemonPod)
				Expect(err).NotTo(HaveOccurred())
				Expect(newPID).ShouldNot(Equal(pid))

				eventAfterDeath, err := ranptphelper.GetEventValueFromEnd(ptpDaemonPod, 1)
				Expect(err).NotTo(HaveOccurred())
				Expect(eventAfterDeath).Should(Equal(ranptpparameters.FreeRun))
				lastValue, err := ranptphelper.GetLastEventValue(ptpDaemonPod)
				Expect(err).NotTo(HaveOccurred())
				Expect(lastValue).Should(Equal(ranptpparameters.Locked))
			}
		})
	})

})
