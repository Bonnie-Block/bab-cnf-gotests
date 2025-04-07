package tests

import (
	"context"
	"fmt"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("PTP leap testing", Label("ptp-leap"), func() {
	var (
		ptpConfigCounts    ranptpparameters.PtpConfigTypeCounter
		nodeToPtpDaemonPod map[string]corev1.Pod
	)

	BeforeEach(func() {
		originPtpConfigList, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).
			List(context.Background(), metav1.ListOptions{})
		Expect(err).ToNot(HaveOccurred())

		ptpConfigCounts = ranptphelper.GetPtpConfigCounts(*originPtpConfigList)

		err = ranptphelper.CheckPtpLockState(5*time.Second, 0)
		Expect(err).ToNot(HaveOccurred())

		if ptpConfigCounts.GMOneNIC == 0 && ptpConfigCounts.GMMultiNIC == 0 {
			Skip("Test requires Grandmaster configurations")
		}

		nodeToPtpDaemonPod, err = ranptphelper.NodesToPtpDaemonPods()
		Expect(err).NotTo(HaveOccurred())
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

		leapCM, err := helper.Apiclient.ConfigMaps(parameters.PtpOperatorNamespace).Get(context.Background(),
			ranptpparameters.LeapConfigMap, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		nodeToPtpDaemonPod, err := ranptphelper.NodesToPtpDaemonPods()
		Expect(err).NotTo(HaveOccurred())

		for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {
			By("Restoring configmap to original")
			emptyCM, err := ranptphelper.ClearLeapCmData(leapCM, nodeName)
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("Deleting linuxptp-daemon pod %s", ptpDaemonPod.Name))
			err = helper.Apiclient.Pods(parameters.PtpOperatorNamespace).Delete(context.Background(), ptpDaemonPod.Name,
				metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			unhealthyPods := helper.WaitForAllPodsHealthy([]string{parameters.PtpOperatorNamespace}, 2*time.Minute,
				3*time.Second, 10*time.Second)

			Expect(unhealthyPods).Should(Or(BeEmpty(), BeNil()))

			By("Validating last announcement of leap event is different from previous announcement")
			newLeapCM, err := helper.Apiclient.ConfigMaps(parameters.PtpOperatorNamespace).Get(context.Background(),
				ranptpparameters.LeapConfigMap, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			emptyCMLastAnnouncement, err := ranptphelper.GetLastAnnouncement(emptyCM, nodeName)
			Expect(err).NotTo(HaveOccurred())
			newLeapCMLastAnnouncement, err := ranptphelper.GetLastAnnouncement(newLeapCM, nodeName)
			Expect(err).NotTo(HaveOccurred())

			Expect(emptyCMLastAnnouncement).NotTo(Equal(newLeapCMLastAnnouncement))
		}

		log.Println("Check ptp clocks are in sync")
		err = ranptphelper.CheckPtpLockState(5*time.Minute, 10*time.Second)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should add leap event announcement in leap configmap when removing the last announcement",
		polarion.ID("75325"), func() {
			By("Getting the current leap-configmap")
			leapCM, err := helper.Apiclient.ConfigMaps(parameters.PtpOperatorNamespace).Get(context.Background(),
				ranptpparameters.LeapConfigMap, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			for nodeName, ptpDaemonPod := range nodeToPtpDaemonPod {

				By("Removing the last leap announcement from the leap-configmap")
				updatedLeapCM, err := ranptphelper.RemoveLastLeapAnnouncement(leapCM, nodeName)
				Expect(err).NotTo(HaveOccurred())

				log.Printf("Deleting linuxptp-daemon pod %s", ptpDaemonPod.Name)
				err = helper.Apiclient.Pods(parameters.PtpOperatorNamespace).Delete(context.Background(),
					ptpDaemonPod.Name, metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				Expect(helper.WaitForAllPodsHealthy([]string{parameters.PtpOperatorNamespace}, 2*time.Minute,
					3*time.Second, 10*time.Second)).Should(Or(BeEmpty(), BeNil()))

				By("Validating the leap-configmap update with today's date")
				err = ranptphelper.WaitForLeapCMUpdate(nodeName)
				Expect(err).NotTo(HaveOccurred())

				By("Validating last announcement of leap event is different from previous announcement")
				newLeapCM, err := helper.Apiclient.ConfigMaps(parameters.PtpOperatorNamespace).Get(context.Background(),
					ranptpparameters.LeapConfigMap, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())

				newLeapCMLastAnnouncement, err := ranptphelper.GetLastAnnouncement(newLeapCM, nodeName)
				Expect(err).NotTo(HaveOccurred())

				updatedLeapCMLastAnnouncement, err := ranptphelper.GetLastAnnouncement(updatedLeapCM, nodeName)
				Expect(err).NotTo(HaveOccurred())

				Expect(newLeapCMLastAnnouncement).NotTo(Equal(updatedLeapCMLastAnnouncement))
			}
		})
})
