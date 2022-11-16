package ranptphelper

import (
	"fmt"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"log"
	"strings"
	"time"
)

// GetEventValueFromEnd gets a pod, "pod", and returns the "eventNumFromEnd" before last value of published event,
// if no event was published within the last 24 hours, an error is occurred. todo change description if needed.
func GetEventValueFromEnd(ptpPod *corev1.Pod, eventNumFromEnd int) (string, error) {
	msgs, err := getEventMsg(ptpPod)
	if nil != err {
		return "", err
	}

	numOfMsgs := len(msgs)
	eventIndex := numOfMsgs - eventNumFromEnd - 1

	if 0 > eventIndex {
		return "", fmt.Errorf("no events exisit %d before the last event", eventNumFromEnd)
	}

	return msgs[numOfMsgs-eventNumFromEnd-1].Data.Values[0].Value, nil
}

// GetEventValueFromEndOfEventByKeyValue todo add description.
func GetEventValueFromEndOfEventByKeyValue(ptpPod *corev1.Pod,
	options map[string]string,
	numFromEnd int) (string, error) {
	eventsValue, err := SearchBy(options, ptpPod)
	if nil != err {
		return "", err
	}

	return eventsValue[len(eventsValue)-numFromEnd-1].Data.Values[0].Value, nil
}

// getEventMsgByEventType todo add description.
func getEventMsgByEventType(eventsType []ranptpparameters.EventMsg, typeValue string) []ranptpparameters.EventMsg {
	var typeEventMsg []ranptpparameters.EventMsg

	for _, msg := range eventsType {
		if strings.Contains(msg.EventType, typeValue) {
			typeEventMsg = append(typeEventMsg, msg)
		}
	}

	return typeEventMsg
}

// SearchBy todo add description.
func SearchBy(options map[string]string, ptpPod *corev1.Pod) ([]ranptpparameters.EventMsg, error) {
	var eventMsg []ranptpparameters.EventMsg

	msgs, err := getEventMsg(ptpPod)

	if nil != err {
		return eventMsg, err
	}

	if len(msgs) == 0 {
		return msgs, fmt.Errorf("no messages found")
	}

	for key, value := range options {
		switch key {
		case "id":
			// add when need
		case "type":
			eventMsg = append(eventMsg, getEventMsgByEventType(msgs, value)...)
			// filter the right events by type option
			msgs = eventMsg
		case "source":
			eventMsg = append(eventMsg, getEventMsgByEventType(msgs, value)...)
			// filter the right events by source option
			msgs = eventMsg
		case "dataContentType":
			// add when need
		case "time":
			// add when need
		case "data":
			// add when need
		default:
			return msgs, fmt.Errorf("key: %s isn't exists", key)
		}
	}

	if len(msgs) == 0 {
		return msgs, fmt.Errorf("no events found with givin options")
	}

	return msgs, nil
}

// WaitForLastEvent waits at least 5 seconds and up to 1 minute for the last event.
// The function returns the last events and an error if any occurred.
func WaitForLastEvent(ptpPod *corev1.Pod) (string, error) {
	log.Println("Waiting at least 5 seconds for new event")

	currEventValue, err := GetLastEventValue(ptpPod)
	if nil != err {
		return "", err
	}

	var newEventValue string

	err = wait.Poll(5*time.Second, 1*time.Minute, func() (bool, error) {
		// conditional function that checks if no new event occurred during the last 5 seconds
		newEventValue, err = GetLastEventValue(ptpPod)
		if nil != err {
			return false, err
		}

		if currEventValue != newEventValue {
			currEventValue = newEventValue

			return false, nil
		}

		return true, nil
	})

	if nil != err {
		return "", err
	}

	log.Println("Got the last event")

	return newEventValue, nil
}

// getEventsLogs gets a long string "logs" and an empty array of strings "eventStrings".
// and fills "eventStrings" array with the logs that contain event state value.
func getEventsLogs(logs string, eventStrings []string) []string {
	logsSlice := strings.Split(logs, "\n")
	for _, line := range logsSlice {
		if strings.Contains(line, "id") && (strings.Contains(line, ranptpparameters.FreeRun) ||
			strings.Contains(line, ranptpparameters.Locked) ||
			strings.Contains(line, ranptpparameters.HoldOver)) {
			eventStrings = append(eventStrings, line)
		}
	}

	return eventStrings
}

// getEventMsg todo add description.
func getEventMsg(ptpPod *corev1.Pod) ([]ranptpparameters.EventMsg, error) {
	var eventMsg []ranptpparameters.EventMsg

	logs, err := pod.GetLog(helper.Apiclient, ptpPod, 24*time.Hour, ranptpparameters.ContainerName)
	if nil != err {
		return eventMsg, err
	}

	var eventStrings []string
	eventStrings = getEventsLogs(logs, eventStrings)

	eventStringsLen := len(eventStrings)
	if eventStringsLen == 0 {
		return eventMsg, fmt.Errorf("no events were found")
	}

	for _, line := range eventStrings {
		logStruct := LogStrToLogStrct(line)

		eventMsg = append(eventMsg, EventMsgParser(logStruct.Msg))
	}

	return eventMsg, nil
}
