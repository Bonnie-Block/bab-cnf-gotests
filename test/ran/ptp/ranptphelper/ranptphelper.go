package ranptphelper

import (
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// GetLastEventValue gets a pod "pod" and returns the last state of the last published event,
// if no event was published within the last 24 hours, an error is occurred.
func GetLastEventValue(ptpPod *corev1.Pod) (string, error) {
	return GetEventValueFromEnd(ptpPod, 0)
}

// GetEventValueFromEnd gets a pod, "pod", and returns the "eventNumFromEnd" before last value of published event,
// if no event was published within the last 24 hours, an error is occurred.
func GetEventValueFromEnd(ptpPod *corev1.Pod, eventNumFromEnd int) (string, error) {
	logs, err := pod.GetLog(helper.Apiclient, ptpPod, 24*time.Hour, ranptpparameters.ContainerName)
	if nil != err {
		return "", err
	}

	var eventStrings []string
	eventStrings = getEventsLogs(logs, eventStrings)

	eventStringsLen := len(eventStrings)
	if eventStringsLen == 0 {
		return "", fmt.Errorf("no events were found")
	}

	eventIndex := eventStringsLen - eventNumFromEnd - 1
	if 0 > eventIndex {
		return "", fmt.Errorf("no events exisit %d before the last event", eventNumFromEnd)
	}

	lastLogEvent := eventStrings[len(eventStrings)-eventNumFromEnd-1]

	logStruct := LogStrToLogStrct(lastLogEvent)
	msg := EventMsgParser(logStruct.Msg)

	return msg.Data.Values[0].Value, nil
}

// NodesToPtpDaemonPods gets a list of nodes "nodesList" and a list of ptp daemon pods "podsList".
// and returns a map which the 'key' is a node and the 'value' is the ptp daemon pod of that specific node.
func NodesToPtpDaemonPods(nodesList []corev1.Node, podsList *corev1.PodList) map[*corev1.Node]*corev1.Pod {
	nodeToPtpDaemonPod := make(map[*corev1.Node]*corev1.Pod)

	podMap := make(map[string]*corev1.Pod)
	for _, podInList := range podsList.Items {
		podMap[podInList.Spec.NodeName] = &podInList
	}

	nodeMap := make(map[*corev1.Node]string)
	for _, node := range nodesList {
		nodeMap[&node] = node.Name
	}

	for key := range nodeMap {
		nodeToPtpDaemonPod[key] = podMap[nodeMap[key]]
	}

	return nodeToPtpDaemonPod
}

// GetPtpDaemonPodFromNode returns a ptp daemon pod from a given node "node".
// returns an error if any occurred, or if a ptp daemon pod is not exists in the node.
func GetPtpDaemonPodFromNode(node *corev1.Node) (*corev1.Pod, error) {
	ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{
			LabelSelector: parameters.PtpDaemonsetLabelSelector})
	if nil != err {
		return nil, err
	}

	for _, daemonPod := range ptpDaemonPods.Items {
		if daemonPod.Spec.NodeName == node.Name {
			return &daemonPod, nil
		}
	}

	return nil, fmt.Errorf("no new ptp daemon pod were created in %s node", node.Name)
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
		// conditional function that checks if no new event accorded during the last 5 seconds
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
		if strings.Contains(line, ranptpparameters.FreeRun) ||
			strings.Contains(line, ranptpparameters.Locked) ||
			strings.Contains(line, ranptpparameters.HoldOver) {
			eventStrings = append(eventStrings, line)
		}
	}

	return eventStrings
}
