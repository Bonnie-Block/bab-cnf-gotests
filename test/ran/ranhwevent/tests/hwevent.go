package tests

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventhelper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stmcginnis/gofish/redfish"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventhelper/nodevendor"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventhelper/rfclient"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	statusOk   = "OK"
	statusFail = "FAIL"
)

var _ = Describe("HW event proxy", func() {
	var (
		eventService    *redfish.EventService
		testEvents      []string
		ConsumersList   *corev1.PodList
		LocalNodeVendor string
		err             error
	)
	execute.BeforeAll(func() {
		ranhweventparameters.Redfish.Session, _ = rfclient.GetClient(ranhweventparameters.Redfish)
		By("Query the node under test redfish vendor")
		LocalNodeVendor, err = nodevendor.GetRedfishVendor(ranhweventparameters.Redfish.Session)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("On redfish vendor query, got this error: %v\n", err))
		ConsumersList, _ = ranhweventhelper.GetConsumers()
		eventService, _ = ranhweventparameters.Redfish.Session.Service.EventService()
		if helper.Config.Ran.RanEventTestDebug != "" {
			ranhweventparameters.DebugTest = true
			log.Println("Test debug flag is on")
		}
		By("Get predefined Vendor events for: " + LocalNodeVendor)
		testEvents, err = rfclient.GetVendorTestEvents(LocalNodeVendor, ranhweventparameters.Redfish)
		Expect(err).ShouldNot(HaveOccurred())

	})

	// OCP-47124
	It("Validate single Redfish event", func() {
		By("Send events to redfish and verify them in the consumers")
		err := TestEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)
		Expect(err).ShouldNot(HaveOccurred())
	})
	// OCP-47125
	It("Hw-event-proxy app recovery", func() {
		By("Validate consumer receive events")
		err := TestEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)
		Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("failed to verify expected events due to: %v", err))

		By("List the app pod by label")
		appPods, err := helper.Apiclient.Pods(ranhweventparameters.NamespaceConsumer).List(context.Background(),
			metav1.ListOptions{
				LabelSelector: ranhweventparameters.AppPodLabel})
		Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf(
			"failed to list app pod with label %v due to: %v ", ranhweventparameters.AppPodLabel, err))

		By(fmt.Sprintf("Delete app pod %v and wait for it to restart\n", appPods.Items[0].Name))
		err = ranhweventhelper.RestartPod(ranhweventparameters.AppPodLabel, 5*time.Minute)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf(
			"failed to restart pod by label %v due to: %v", ranhweventparameters.AppPodLabel, err))

		pod, err := ranhweventhelper.GetPodByLabel(ranhweventparameters.AppPodLabel)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf(
			"failed to get pod by label: %v due to: %v", ranhweventparameters.AppPodLabel, err))

		if ranhweventparameters.DebugTest {
			log.Println("Status after application restart:")
			log.Printf("pod.Status.Phase: %v running is: %v\n", pod.Status.Phase, corev1.PodRunning)
			for index, container := range pod.Status.ContainerStatuses {
				log.Printf("container[%v] status: %v\n", index, container.Ready)
			}
		}
		By("Validate again consumer receives events")
		log.Print("Second test")
		err = TestEvents(ConsumersList, testEvents, eventService, LocalNodeVendor)
		Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("failed to verify expected events due to: %v", err))
	})
	// OCP-48698
	It("Validate 10k Redfish event", func() {
		if LocalNodeVendor == ranhweventparameters.ZT {
			Skip("Zt systems found which is too slow in sending many events skipping this test.")
		}
		var manyEvents []string
		for i := 0; i < 10000/len(testEvents); i++ {
			manyEvents = append(manyEvents, testEvents...)
		}

		By("Send events to redfish and verify them in the consumers")
		err := TestEvents(ConsumersList, manyEvents, eventService, LocalNodeVendor)
		Expect(err).ShouldNot(HaveOccurred())
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
		if !ranhweventhelper.Contains(consumersResults, consumer) {
			missingConsumers = append(missingConsumers, consumer)
		}
	}

	log.Printf("No events received from consumers: %v\n", missingConsumers)

	for consumer, r := range results {
		if len(r) < len(testEvents) {
			var missingEvents []string

			for _, resultEvent := range testEvents {
				if !ranhweventhelper.Contains(r, resultEvent) {
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

	req := helper.Apiclient.Pods(ranhweventparameters.NamespaceConsumer).GetLogs(consumerPod.Name,
		&corev1.PodLogOptions{
			Container: ranhweventparameters.ConsumerContainerName,
			Follow:    true,
		})

	LogStream, err := req.Stream(cancelCtx)
	if err != nil {
		log.Printf("failed to open log stream to %v container: %v due to: %v\n",
			consumerPod.Name, ranhweventparameters.ConsumerContainerName, err)

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
				eventJSON := ranhweventhelper.IsEventJSON(line)

				if eventJSON != "" {
					processEvents(eventJSON, localNodeVendor, expectedEvent, consumerPod, verificationReportChannel)

					break
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
	verificationReportChannel chan string) {
	events, err := ranhweventhelper.GetMsgID(eventJSON)
	if err != nil {
		verificationReportChannel <- fmt.Sprintf("%v/%v/%v",
			consumerPod.Name, expectedEvent, statusFail)
	} else {

		for _, event := range events {
			if rfclient.GetSkippedEvents(localNodeVendor)[event.MessageID] {
				if ranhweventparameters.DebugTest {
					log.Printf("Skipping event: %v for consumer: %v\n", event.MessageID, consumerPod.Name)
				}
			} else {
				if event.MessageID != expectedEvent {
					log.Printf("Event verification failed for consumer %v "+
						"expected msgId: %v got: %v at event timestamp: %v\n",
						consumerPod.Name, expectedEvent, event.MessageID, event.EventTimestamp)
					verificationReportChannel <- fmt.Sprintf("%v/%v/%v",
						consumerPod.Name, expectedEvent, statusFail)
				} else {
					if ranhweventparameters.DebugTest {
						log.Printf("Consumer: %v received event: %v\n",
							consumerPod.Name,
							event.MessageID)
					}
					verificationReportChannel <- fmt.Sprintf("%v/%v/%v",
						consumerPod.Name, expectedEvent, statusOk)
				}
			}
		}
	}
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

		if ranhweventparameters.DebugTest {
			log.Printf("Sent: %v\n", sentMsgID)
		}

		for _, consumerPod := range consumersList.Items {
			consumerInChannels[consumerPod.Name] <- sentMsgID
		}
	}

	if VerifyEvents(consumersList, testEvents, consumerOutChannel, ranhweventparameters.EventRxTimeout) != nil {
		endConsumerCheckers()

		return fmt.Errorf("failed in verifying consumer events")
	}

	endConsumerCheckers()

	return nil
}

// PowerSupplyTest uses a PDU to cause a power fault and verify it.
func PowerSupplyTest(consumersList *corev1.PodList, localNodeVendor string) error {
	if !ranhweventparameters.GetPDU() {
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

	if err := ranhweventhelper.Init(); err != nil {
		endConsumerCheckers()

		return err
	}

	socket, err := ranhweventhelper.GetSocket()
	if err != nil {
		endConsumerCheckers()
		ranhweventhelper.Close()

		return fmt.Errorf("failed to get PDU Socket due to: %w", err)
	}

	if !socket {
		log.Printf("Power socket %v is off. Powering it on before testing.", ranhweventhelper.Name)
		err = ChangeSocketState(
			consumersList,
			PowerOnEvents,
			consumerInChannels,
			consumerOutChannel,
			endConsumerCheckers,
			ranhweventhelper.ModeOn)

		if err != nil {
			endConsumerCheckers()
			ranhweventhelper.Close()

			return err
		}
	}

	log.Printf("Power off %v\n", ranhweventhelper.Name)
	// takes 10 s for all events
	err = ChangeSocketState(consumersList,
		PowerOnEvents,
		consumerInChannels,
		consumerOutChannel,
		endConsumerCheckers,
		ranhweventhelper.ModeOff)
	if err != nil {
		endConsumerCheckers()
		ranhweventhelper.Close()

		return err
	}

	err = ChangeSocketState(consumersList,
		PowerOnEvents,
		consumerInChannels,
		consumerOutChannel,
		endConsumerCheckers,
		ranhweventhelper.ModeOn)

	if err != nil {
		endConsumerCheckers()
		ranhweventhelper.Close()

		return err
	}

	endConsumerCheckers()
	ranhweventhelper.Close()

	return nil
}

func ChangeSocketState(consumersList *corev1.PodList,
	powerEvents []string,
	consumerInChannels map[string]chan string,
	consumerOutChannel chan string,
	endConsumerCheckers context.CancelFunc,
	state int) error {
	if _, err := ranhweventhelper.SetSocket(state); err != nil {
		return err
	}

	for _, consumerPod := range consumersList.Items {
		for _, sentMsgID := range powerEvents {
			consumerInChannels[consumerPod.Name] <- sentMsgID
		}
	}

	if VerifyEvents(consumersList, powerEvents, consumerOutChannel, ranhweventparameters.EventRxTimeout) != nil {
		endConsumerCheckers()

		return fmt.Errorf("failed to verify power %v events", state)
	}

	return nil
}

func max(a, b int) int {
	return int(math.Max(float64(a), float64(b)))
}
