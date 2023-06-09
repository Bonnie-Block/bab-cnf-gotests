package tests

import (
	"fmt"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/bmer/ranbmerhelper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stmcginnis/gofish/redfish"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/bmer/ranbmerhelper/nodevendor"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/bmer/ranbmerhelper/rfclient"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/bmer/ranbmerparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("BMER", func() {
	var (
		eventService    *redfish.EventService
		testEvents      []string
		ConsumersList   *corev1.PodList
		LocalNodeVendor string
		err             error
	)
	execute.BeforeAll(func() {
		ranbmerparameters.Redfish.Session, _ = rfclient.GetClient(ranbmerparameters.Redfish)
		By("Query the node under test redfish vendor")
		LocalNodeVendor, err = nodevendor.GetRedfishVendor(ranbmerparameters.Redfish.Session)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("On redfish vendor query, got this error: %v\n", err))
		ConsumersList, _ = ranhelper.GetConsumers(parameters.BmerNamespace)
		eventService, _ = ranbmerparameters.Redfish.Session.Service.EventService()
		By("Get predefined Vendor events for: " + LocalNodeVendor)
		testEvents, err = rfclient.GetVendorTestEvents(LocalNodeVendor, ranbmerparameters.Redfish)
		Expect(err).ShouldNot(HaveOccurred())

	})

	// OCP-47124
	It("delivers Redfish events", func() {
		By("Request test events from BMC Redfish API and verify events are received in consumer")
		VerifyEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)
		By("Wait 20 seconds for all events to be completed")
		time.Sleep(20 * time.Second)
	})
	// OCP-47125
	It("recovers from hw-event-proxy app restart", func() {
		By("Validate consumer receive events")
		VerifyEvents(ConsumersList, testEvents[:1], eventService, LocalNodeVendor)

		oldPod, err := ranbmerhelper.GetPodByLabel(ranbmerparameters.AppPodLabel)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf(
			"failed to get pod by label: %v due to: %v", ranbmerparameters.AppPodLabel, err))

		By(fmt.Sprintf("Delete app pod %v and wait for it to restart\n", oldPod.Name))
		err = ranbmerhelper.RestartPod(ranbmerparameters.AppPodLabel, 5*time.Minute)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf(
			"failed to restart pod by label %v due to: %v", ranbmerparameters.AppPodLabel, err))

		newPod, err := ranbmerhelper.GetPodByLabel(ranbmerparameters.AppPodLabel)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf(
			"failed to get pod by label: %v due to: %v", ranbmerparameters.AppPodLabel, err))

		By(fmt.Sprintf("New app pod %v is running\n", newPod.Name))
		Expect(newPod.Name).NotTo(Equal(oldPod.Name), fmt.Sprintf(
			"failed to restart pod %v", oldPod.Name))

		By("Validate again consumer receives events")
		VerifyEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)

		By("Wait 20 seconds for all events to be completed")
		time.Sleep(20 * time.Second)
	})
	// OCP-47129
	It("recovers from producer cloud-event-sidecar crash", func() {
		By("Validate consumer receive events")
		VerifyEvents(ConsumersList, testEvents[:1], eventService, LocalNodeVendor)

		By("Crash cloud-event-sidecar and wait for it to restart")
		err = ranbmerhelper.RestartSidecar(ranbmerparameters.AppPodLabel, 5*time.Minute)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf(
			"failed to restart sidecar on pod %v due to: %v", ranbmerparameters.AppPodLabel, err))

		By("Validate again consumer receives events")
		VerifyEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)

		By("Wait 20 seconds for all events to be completed")
		time.Sleep(20 * time.Second)
	})
	// OCP-47130
	It("recovers from consumer app restart", func() {
		By("Validate consumer receive events")
		VerifyEvents(ConsumersList, testEvents[:1], eventService, LocalNodeVendor)

		oldPod, err := ranbmerhelper.GetPodByLabel(ranparameters.ConsumerPodLabel)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf(
			"failed to get pod by label: %v due to: %v", ranparameters.ConsumerPodLabel, err))

		By(fmt.Sprintf("Delete consumer pod %v and wait for it to restart\n", oldPod.Name))
		err = ranbmerhelper.RestartPod(ranparameters.ConsumerPodLabel, 5*time.Minute)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf(
			"failed to restart pod by label %v due to: %v", ranparameters.ConsumerPodLabel, err))

		newPod, err := ranbmerhelper.GetPodByLabel(ranparameters.ConsumerPodLabel)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf(
			"failed to get pod by label: %v due to: %v", ranparameters.ConsumerPodLabel, err))

		By(fmt.Sprintf("New app pod %v is running\n", newPod.Name))
		Expect(newPod.Name).NotTo(Equal(oldPod.Name), fmt.Sprintf(
			"failed to restart pod %v", oldPod.Name))

		By("Wait 30 seconds for consumer to be ready")
		time.Sleep(30 * time.Second)
		ConsumersList, _ = ranhelper.GetConsumers(parameters.BmerNamespace)

		By("Validate again consumer receives events")
		VerifyEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)

		By("Wait 20 seconds for all events to be completed")
		time.Sleep(20 * time.Second)
	})
	// OCP-47128
	It("recovers from node restart", func() {
		By("Validate consumer receive events")
		VerifyEvents(ConsumersList, testEvents[:1], eventService, LocalNodeVendor)

		workerNode, err := ranbmerhelper.GetWorkerNode()
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf(
			"failed to get worker nodedue to: %v", err))

		helper.SoftRebootNodeAndWaitForDisconnect(workerNode)
		if ranparameters.TransportType == ranparameters.TransportHTTP {
			err = ranhelper.WaitForClusterRecover(workerNode, []string{parameters.BmerNamespace})
		} else {
			err = ranhelper.WaitForClusterRecover(workerNode, []string{parameters.AmqNamespace, parameters.BmerNamespace})
		}
		Expect(err).NotTo(HaveOccurred())

		By("Validate again consumer receives events")
		VerifyEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)
	})
})

// VerifyEvents sends givens events to given consumers and verify consumers received the events.
func VerifyEvents(consumersList *corev1.PodList, testMsgIds []string, eventService *redfish.EventService,
	localNodeVendor string) {
	Expect(testMsgIds).ToNot(BeEmpty())

	startTime := time.Now()
	time.Sleep(1 * time.Second)

	for _, testMsgID := range testMsgIds {
		err := rfclient.SendEvent(eventService, testMsgID, localNodeVendor)
		Expect(err).ToNot(HaveOccurred())
	}

	for _, consumer := range consumersList.Items {
		for _, testMsgID := range testMsgIds {
			err := ranbmerhelper.WaitForEvent(&consumer, testMsgID, startTime, 1*time.Minute)
			Expect(err).NotTo(HaveOccurred())
		}
	}
}
