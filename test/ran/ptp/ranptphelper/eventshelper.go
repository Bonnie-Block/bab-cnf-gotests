package ranptphelper

import (
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/strings/slices"

	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
)

// getClockStateEventsLogs gets a long string "logs" and an empty array of strings "eventStrings".
// and fills "eventStrings" array with the logs that contain event state value.
func getClockStateEventsLogs(logs string, eventStrings []string) []string {
	logs = strings.ReplaceAll(logs, "\\n", "")
	logsSlice := strings.Split(logs, "\n")

	for _, line := range logsSlice {
		if strings.Contains(line, "id") && (strings.Contains(line, ranptpparameters.EventFreeRun) ||
			strings.Contains(line, ranptpparameters.EventLocked) ||
			strings.Contains(line, ranptpparameters.EventHoldOver)) {
			eventStrings = append(eventStrings, line)
		}
	}

	return eventStrings
}

// WaitForEvent waits for specified event to appear in ptp cloud event proxy log.
func WaitForEvent(ptpPod *corev1.Pod, container string, eventType string, eventValue string,
	iface string, since time.Duration, timeout time.Duration) error {
	if since < 1*time.Second {
		since = 1 * time.Second
	}

	startTime := time.Now()

	logs, err := pod.GetLog(helper.Apiclient, ptpPod, since, container)
	if err != nil {
		return err
	}

	eventMsgs := getEvents(logs)
	if containsEvent(eventMsgs, eventType, eventValue, iface) {
		return nil
	}

	interval := 5 * time.Second

	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		time.Sleep(interval)
		logs, err = pod.GetLog(helper.Apiclient, ptpPod, time.Since(startTime)+time.Second,
			container)
		if err != nil {
			return false, nil
		}

		eventMsgs = getEvents(logs)
		if containsEvent(eventMsgs, eventType, eventValue, iface) {
			return true, nil
		}

		startTime = time.Now()

		return false, nil
	})
}

// getEvents parses ptp cloud event logs and returns EvenMsg structs.
func getEvents(eventLogs string) []ranptpparameters.EventMsg {
	var (
		eventMsg     []ranptpparameters.EventMsg
		eventStrings []string
	)

	eventStrings = getClockStateEventsLogs(eventLogs, eventStrings)

	for _, line := range eventStrings {
		eventJSON := getEventJSON(line)
		if eventJSON != "" {
			var event ranptpparameters.EventMsg

			eventUnquote := strings.ReplaceAll(eventJSON, "\\", "")
			err := json.Unmarshal([]byte(eventUnquote), &event)

			if err != nil {
				log.Printf("Error when parsing event log: %s", err)
			} else {
				eventMsg = append(eventMsg, event)
			}
		}
	}

	return eventMsg
}

// containsEvent returns true when specified event type, value and resource is found in given event messages.
func containsEvent(eventMsgs []ranptpparameters.EventMsg, eventType string, value string, iface string) bool {
	if len(eventMsgs) == 0 {
		return false
	}

	// NIC info is added to ptp event since 4.11
	if ranhelper.IsVersionStringInRange(ranptpparameters.PtpVersion, "", "4.11") {
		iface = ""
	}

	if iface != "" && !strings.HasSuffix(iface, "x") {
		iface = iface[:len(iface)-1] + "x"
	}

	var (
		checkedResources []string
		failedResources  []string
	)

	for _, event := range eventMsgs {
		if event.EventType == eventType {
			for _, val := range event.Data.Values {
				if !strings.Contains(val.DataType, "notification") || slices.Contains(checkedResources, val.Resource) {
					continue
				}

				if strings.Contains(val.Value, value) {
					if iface == "" {
						checkedResources = append(checkedResources, val.Resource)
					} else if strings.Contains(val.Resource, iface) {
						log.Printf("Info: %s %s is found for resource(s): %v\n", eventType, value, iface)

						return true
					}
				} else if iface == "" {
					log.Printf("Info: %s has value %s for %s, expected value: %s\n",
						eventType, val.Value, val.Resource, value)
					failedResources = append(failedResources, val.Resource)
				}
			}
		}
	}

	// Event for specific interface is not found or no expect event found at all
	if iface != "" || len(checkedResources) == 0 {
		return false
	}

	// When events are expected for all interfaces, we return true as long as one expected event is found
	for _, res := range failedResources {
		if !slices.Contains(checkedResources, res) {
			log.Printf("Warning: %s %s not found for resource %s\n", eventType, value, res)

			return false
		}
	}

	log.Printf("Info: %s %s is found for resources: %v\n", eventType, value, checkedResources)

	return true
}

// Return the json body of the event.
func getEventJSON(line string) string {
	r := regexp.MustCompile(`[(received event|event sent)]\{(.*)\}`)
	eventJSON := r.FindString(line)

	return eventJSON
}

// GetPtpTransport gets the ptp operator configured transport host.
func GetPtpTransport() (string, error) {
	// get the ptp operator configured transport host.
	ptpOperatorConfigs, err := helper.Apiclient.PtpOperatorConfigs(parameters.PtpOperatorNamespace).
		List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to ptp operator config due to: %w", err)
	}

	transportHost := ptpOperatorConfigs.Items[0].Spec.EventConfig.TransportHost

	switch {
	case transportHost == "":
		return ranparameters.TransportHTTP, nil
	case strings.Contains(transportHost, "ptp-event-publisher-service"):
		return ranparameters.TransportHTTP, nil
	case strings.Contains(transportHost, "amqp"):
		return ranparameters.TransportAMQP, nil
	default:
		return "", fmt.Errorf("unexpected ptp operator transport host: %v", transportHost)
	}
}

// DeployPtpConsumer deploys an event consumer using the transport protocol from the ptp operator.
func DeployPtpConsumer() (*corev1.PodList, error) {
	log.Println("Check consumer image is defined")

	if helper.Config.Ran.ConsumerImage == "" {
		return nil, fmt.Errorf("CLOUD_EVENT_CONSUMER_IMAGE environment is missing")
	}

	log.Println("Configure the cluster objects for cloud event proxy")

	err := ranhelper.ConfigEventProxyObjects(parameters.CloudEventNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to config app due to: %w", err)
	}

	mirroredImages, err := ranhelper.GetDeployImages(parameters.PtpOperatorNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get images to be used from ClusterServiceVersions due to: %w", err)
	}

	log.Println("Deploy cloud event consumers")

	err = ranhelper.DeployConsumers(mirroredImages, ranparameters.TransportType, parameters.CloudEventNamespace)

	// In case the consumer already exist on the cluster an error with the string skip will be returned.
	if err != nil && err.Error() == "consumers already deployed in cluster. skipping creating them" {
		log.Printf("Consumers creating skipped: %v", err)
	} else if err != nil {
		return nil, fmt.Errorf("failed to deploy consumers due to: %w", err)
	}

	log.Println("Check consumers exist")

	var consumersList *corev1.PodList
	consumersList, err = ranhelper.GetConsumers(parameters.CloudEventNamespace)

	if err != nil {
		return nil, fmt.Errorf("failed to check consumers exist due to: %w", err)
	}

	return consumersList, nil
}

// WaitForConsumerReady waits for the consumer to be ready for events.
func WaitForConsumerReady(consumerPod *corev1.Pod) error {
	err := wait.PollImmediate(5*time.Second, 1*time.Minute, func() (bool, error) {
		logs, err := pod.GetLog(helper.Apiclient, consumerPod, 1*time.Hour,
			ranptpparameters.ConsumerContainer)

		if err != nil {
			return false, nil
		}

		if strings.Contains(logs, "waiting for events") {
			return true, nil
		}

		return false, nil
	})

	return err
}
