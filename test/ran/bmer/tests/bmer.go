package tests

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math"
	"strings"
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

const (
	statusOk   = "OK"
	statusFail = "FAIL"
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
		err := TestEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)
		Expect(err).ShouldNot(HaveOccurred())
	})
	// OCP-47125
	It("recovers from hw-event-proxy app restart", func() {
		By("Validate consumer receive events")
		err := TestEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)
		Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("failed to verify expected events due to: %v", err))

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
		err = TestEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)
		Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("failed to verify expected events due to: %v", err))
	})
	// OCP-47129
	It("recovers from producer cloud-event-sidecar crash", func() {
		By("Validate consumer receive events")
		err := TestEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)
		Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("failed to verify expected events due to: %v", err))

		By("Crash cloud-event-sidecar and wait for it to restart")
		err = ranbmerhelper.RestartSidecar(ranbmerparameters.AppPodLabel, 5*time.Minute)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf(
			"failed to restart sidecar on pod %v due to: %v", ranbmerparameters.AppPodLabel, err))

		By("Validate again consumer receives events")
		err = TestEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)
		Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("failed to verify expected events due to: %v", err))
	})
	// OCP-47130
	It("recovers from consumer app restart", func() {
		By("Validate consumer receive events")
		err := TestEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)
		Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("failed to verify expected events due to: %v", err))

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
		err = TestEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)
		Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("failed to verify expected events due to: %v", err))
	})
	// OCP-47128
	It("recovers from node restart", func() {
		By("Validate consumer receive events")
		err := TestEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)
		Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("failed to verify expected events due to: %v", err))

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
		err = TestEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)
		Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("failed to verify expected events due to: %v", err))
	})
})

// VerifyEvents collects channel messages from the go routines supervising the consumers
// returns error in case of bad consumer event or timeout waiting for events, else ok.
func VerifyEvents(consumersList *corev1.PodList, testEvents []string, consumerOutChannel chan string,
	timeoutDuration time.Duration) error {
	var verificationMsg string

	results := make(map[string][]string)
	ctx := context.Background()
	timeout, cancelTimeout := context.WithTimeout(ctx, timeoutDuration)

	for i := 0; i < (len(consumersList.Items) * len(testEvents)); i++ {
		select {
		case <-timeout.Done():
			log.Printf("Timeout reached while not all the expected events were received by the consumers.")
			log.Printf("Issuing an end to the consumers checkers. They may report an error," +
				" as they were still waiting for incoming events")
			cancelTimeout()
			sumResults(consumersList, testEvents, timeoutDuration, results)

			return fmt.Errorf("timeout reached waiting for consumer events")

		case verificationMsg = <-consumerOutChannel:
			if verificationMsg[len(verificationMsg)-2:] == statusOk {
				verified := strings.Split(verificationMsg, "/")
				results[verified[0]] = append(results[verified[0]], verified[1])
			} else {
				log.Printf("Got faulty event in consumer, test failed. Data: %v\n", verificationMsg)
				cancelTimeout()

				return fmt.Errorf("failed to receive expected event")
			}
		}
	}
	cancelTimeout()

	return nil
}

// sumResults is invoked in case of timeout waiting for the events.
// It prints the consumers that did not get the expected event in time
// and which events where missing.
func sumResults(
	consumersList *corev1.PodList,
	testEvents []string,
	timeoutDuration time.Duration,
	results map[string][]string) {
	log.Printf("Timeout of %v sec expired waiting for events from consumers\n", timeoutDuration.String())
	log.Printf("Missing events for %v/%v consumers \n",
		len(consumersList.Items)-len(results),
		len(consumersList.Items))

	var testedConsumers []string
	for _, result := range consumersList.Items {
		testedConsumers = append(testedConsumers, result.Name)
	}

	var consumersResults []string

	for consumerResult := range results {
		consumersResults = append(consumersResults, consumerResult)
	}

	var missingConsumers []string

	for _, consumer := range testedConsumers {
		if !ranbmerhelper.Contains(consumersResults, consumer) {
			missingConsumers = append(missingConsumers, consumer)
		}
	}

	log.Printf("No events received from consumers: %v\n", missingConsumers)

	for consumer, r := range results {
		if len(r) < len(testEvents) {
			var missingEvents []string

			for _, resultEvent := range testEvents {
				if !ranbmerhelper.Contains(r, resultEvent) {
					missingEvents = append(missingEvents, resultEvent)
				}
			}

			log.Printf("Missing event for consumer: %v %v\n", consumer, missingEvents)
		}
	}
}

// ConsumerVerifyEvents spins of a go routine for each consumer and verifies the received events
// it outputs to a channel if the event is verified or failed.
func ConsumerVerifyEvents(cancelCtx context.Context, consumerPod corev1.Pod, expectedEventIn chan string,
	verificationReportChannel chan string, localNodeVendor string) {
	var (
		expectedEvent string
		line          string
	)

	req := helper.Apiclient.Pods(parameters.BmerNamespace).GetLogs(consumerPod.Name,
		&corev1.PodLogOptions{
			Container: ranbmerparameters.ConsumerContainerName,
			Follow:    true,
		})

	LogStream, err := req.Stream(cancelCtx)
	if err != nil {
		log.Printf("failed to open log stream to %v container: %v due to: %v\n",
			consumerPod.Name, ranbmerparameters.ConsumerContainerName, err)

		return
	}

	scanner := bufio.NewScanner(LogStream)

	for {
		select {
		case <-cancelCtx.Done():
			err = LogStream.Close()
			if err != nil {
				log.Printf("failed to close log stream from consumer pod: %v due to: %v\n", consumerPod.Name, err)
			}

			break
		case expectedEvent = <-expectedEventIn:
			for scanner.Scan() {
				line = scanner.Text()
				eventJSON := ranbmerhelper.IsEventJSON(line)

				if eventJSON != "" {
					if processEvents(eventJSON, localNodeVendor, expectedEvent, consumerPod, verificationReportChannel) {
						// if expected event is found, break to process next expectedEventIn
						break
					}
				}
			}

			// if this is stopped with context, then it is OK
			if scanner.Err() != nil && scanner.Err().Error() != "context canceled" {
				log.Printf("ConsumerVerifyEvents() got error while reading the logs from consumer pod: %v\n",
					consumerPod.Name)
				log.Printf("Last line was: \"%v\"\n", line)
				log.Printf("Then got this error: %v\n", scanner.Err())

				return
			}
		}
	}
}

func processEvents(
	eventJSON string,
	localNodeVendor string,
	expectedEvent string,
	consumerPod corev1.Pod,
	verificationReportChannel chan string) bool {
	events, err := ranbmerhelper.GetMsgID(eventJSON)
	if err != nil {
		verificationReportChannel <- fmt.Sprintf("%v/%v/%v",
			consumerPod.Name, expectedEvent, statusFail)

		return false
	}

	for _, event := range events {
		msgID := ranbmerhelper.SanitizeMsgID(event.MessageID)

		if rfclient.GetSkippedEvents(localNodeVendor)[msgID] {
			if ranbmerparameters.DebugTest {
				log.Printf("Skipping event: %v for consumer: %v\n", event.MessageID, consumerPod.Name)
			}

			return false
		}

		if msgID != expectedEvent {
			log.Printf("Event verification failed for consumer %v "+
				"expected msgID: %v got: %v sanitized msgID: %v  at event timestamp: %v\n",
				consumerPod.Name, expectedEvent, event.MessageID, msgID, event.EventTimestamp)
			verificationReportChannel <- fmt.Sprintf("%v/%v/%v",
				consumerPod.Name, expectedEvent, statusFail)

			return false
		}

		if ranbmerparameters.DebugTest {
			log.Printf("Consumer: %v received event: %v\n",
				consumerPod.Name,
				msgID)
		}
		verificationReportChannel <- fmt.Sprintf("%v/%v/%v",
			consumerPod.Name, expectedEvent, statusOk)
	}

	return true
}

// TestEvents starts the Goroutines and collects the validation results returns which
// error happened during verification
// or ok.
func TestEvents(consumersList *corev1.PodList, testEvents []string, eventService *redfish.EventService,
	localNodeVendor string) error {
	if testEvents == nil || len(testEvents) < 1 {
		return fmt.Errorf("got no vendor events to test")
	}

	ctx := context.Background()
	cancelCtx, endConsumerCheckers := context.WithCancel(ctx)

	consumerInChannels := make(map[string]chan string)
	for _, consumerPod := range consumersList.Items {
		consumerInChannels[consumerPod.Name] = make(chan string, len(testEvents))
	}

	consumerOutChannel := make(chan string, len(consumersList.Items)*len(testEvents))

	for _, consumerPod := range consumersList.Items {
		go ConsumerVerifyEvents(cancelCtx, consumerPod, consumerInChannels[consumerPod.Name], consumerOutChannel,
			localNodeVendor)
	}

	for _, sentMsgID := range testEvents {
		err := rfclient.SendEvent(eventService, sentMsgID, localNodeVendor)
		if err != nil {
			log.Print("error on sending redfish event")
			endConsumerCheckers()

			return fmt.Errorf(fmt.Sprintf("failed to send event: %v due to: %v", sentMsgID, err))
		}

		if ranbmerparameters.DebugTest {
			log.Printf("Sent: %v\n", sentMsgID)
		}

		for _, consumerPod := range consumersList.Items {
			consumerInChannels[consumerPod.Name] <- sentMsgID
		}
	}

	if VerifyEvents(consumersList, testEvents, consumerOutChannel, ranbmerparameters.EventRxTimeout) != nil {
		endConsumerCheckers()

		return fmt.Errorf("failed in verifying consumer events")
	}

	endConsumerCheckers()

	return nil
}

// PowerSupplyTest uses a PDU to cause a power fault and verify it.
func PowerSupplyTest(consumersList *corev1.PodList, localNodeVendor string) error {
	if !ranbmerparameters.GetPDU() {
		return fmt.Errorf("powerSupplyTest skipped due to missing PDU config")
	}

	PowerOnEvents, PowerOffEvents := rfclient.GetPowerEvents(localNodeVendor)
	ctx := context.Background()
	cancelCtx, endConsumerCheckers := context.WithCancel(ctx)

	consumerInChannels := make(map[string]chan string)
	for _, consumerPod := range consumersList.Items {
		consumerInChannels[consumerPod.Name] = make(chan string, max(len(PowerOnEvents), len(PowerOffEvents)))
	}

	consumerOutChannel := make(chan string, len(consumersList.Items)*max(len(PowerOnEvents), len(PowerOffEvents)))

	for _, consumerPod := range consumersList.Items {
		go ConsumerVerifyEvents(cancelCtx, consumerPod, consumerInChannels[consumerPod.Name], consumerOutChannel,
			localNodeVendor)
	}

	if err := ranbmerhelper.Init(); err != nil {
		endConsumerCheckers()

		return err
	}

	socket, err := ranbmerhelper.GetSocket()
	if err != nil {
		endConsumerCheckers()
		ranbmerhelper.Close()

		return fmt.Errorf("failed to get PDU Socket due to: %w", err)
	}

	if !socket {
		log.Printf("Power socket %v is off. Powering it on before testing.", ranbmerhelper.Name)
		err = ChangeSocketState(
			consumersList,
			PowerOnEvents,
			consumerInChannels,
			consumerOutChannel,
			endConsumerCheckers,
			ranbmerhelper.ModeOn)

		if err != nil {
			endConsumerCheckers()
			ranbmerhelper.Close()

			return err
		}
	}

	log.Printf("Power off %v\n", ranbmerhelper.Name)
	// takes 10 s for all events
	err = ChangeSocketState(consumersList,
		PowerOnEvents,
		consumerInChannels,
		consumerOutChannel,
		endConsumerCheckers,
		ranbmerhelper.ModeOff)
	if err != nil {
		endConsumerCheckers()
		ranbmerhelper.Close()

		return err
	}

	err = ChangeSocketState(consumersList,
		PowerOnEvents,
		consumerInChannels,
		consumerOutChannel,
		endConsumerCheckers,
		ranbmerhelper.ModeOn)

	if err != nil {
		endConsumerCheckers()
		ranbmerhelper.Close()

		return err
	}

	endConsumerCheckers()
	ranbmerhelper.Close()

	return nil
}

func ChangeSocketState(consumersList *corev1.PodList,
	powerEvents []string,
	consumerInChannels map[string]chan string,
	consumerOutChannel chan string,
	endConsumerCheckers context.CancelFunc,
	state int) error {
	if _, err := ranbmerhelper.SetSocket(state); err != nil {
		return err
	}

	for _, consumerPod := range consumersList.Items {
		for _, sentMsgID := range powerEvents {
			consumerInChannels[consumerPod.Name] <- sentMsgID
		}
	}

	if VerifyEvents(consumersList, powerEvents, consumerOutChannel, ranbmerparameters.EventRxTimeout) != nil {
		endConsumerCheckers()

		return fmt.Errorf("failed to verify power %v events", state)
	}

	return nil
}

func max(a, b int) int {
	return int(math.Max(float64(a), float64(b)))
}
