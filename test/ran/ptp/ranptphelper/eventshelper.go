package ranptphelper

import (
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/strings/slices"

	"log"
	"strings"
	"time"
)

// getEventsLogs gets a long string "logs" and an empty array of strings "eventStrings".
// and fills "eventStrings" array with the logs that contain event state value.
func getEventsLogs(logs string, eventStrings []string) []string {
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
func WaitForEvent(ptpPod *corev1.Pod, eventType string, eventValue string, iface string, since time.Duration,
	timeout time.Duration) error {
	if since < 1*time.Second {
		since = 1 * time.Second
	}

	startTime := time.Now()

	logs, err := pod.GetLog(helper.Apiclient, ptpPod, since, ranptpparameters.CloudEventContainer)
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
			ranptpparameters.CloudEventContainer)
		if err != nil {
			return false, nil
		}

		eventMsgs = getEvents(logs)
		if containsEvent(eventMsgs, eventType, eventValue, iface) {
			log.Printf("%s event %s found for %s\n", eventType, eventValue, iface)

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

	eventStrings = getEventsLogs(eventLogs, eventStrings)

	for _, line := range eventStrings {
		logStruct := LogStrToLogStrct(line)

		eventMsg = append(eventMsg, EventMsgParser(logStruct.Msg))
	}

	return eventMsg
}

// containsEvent returns true when specified event type, value and resource is found in given event messages.
func containsEvent(eventMsgs []ranptpparameters.EventMsg, eventType string, value string, iface string) bool {
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
				if slices.Contains(checkedResources, val.Resource) {
					continue
				}

				if strings.Contains(val.Value, value) {
					if iface == "" {
						checkedResources = append(checkedResources, val.Resource)
					} else if strings.Contains(val.Resource, iface) {
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

	// Event for specific interface is not found
	if iface != "" {
		return false
	}

	// When events are expected for all interfaces, we return true as long as one expected event is found
	for _, res := range failedResources {
		if !slices.Contains(checkedResources, res) {
			log.Printf("Warning: %s %s not found for resource %s\n", eventType, value, res)

			return false
		}
	}

	return true
}
