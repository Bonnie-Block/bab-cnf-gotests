package ranptphelper

import (
	"context"
	"fmt"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"strings"
	"time"
)

// GetLastEventValue gets a pod "pod" and returns the last state of the last published event,
// if no event was published within the last 24 hours, an error is occurred.
func GetLastEventValue(ptpPod *corev1.Pod) (string, error) {
	logs, err := pod.GetLog(helper.Apiclient, ptpPod, 24*time.Hour, ranptpparameters.ContainerName)
	if nil != err {
		return "", err
	}

	var eventStrings []string
	eventStrings = getEventsLogs(logs, eventStrings)

	if len(eventStrings) == 0 {
		return "", fmt.Errorf("no events were found")
	}

	lastLogEvent := eventStrings[len(eventStrings)-1]
	log := LogStrToLogStrct(lastLogEvent)
	msg := EventMsgParser(log.Msg)

	return msg.Data.Values[0].Value, nil
}

// WaitForClusterRecover waits up to 45 minutes for all clusters in a given node " node"to recover,
// if at list one cluster is not recovers, an error is occurred.
func WaitForClusterRecover(node *corev1.Node) error {
	// Wait for linux to be reachable via ping and record time
	interval := 5 * time.Second

	helper.WaitForNodeReachable(node)

	err := ranhelper.WaitForClusterReachable()
	if nil != err {
		return err
	}

	workloadStableDuration := 40 * time.Second

	unhealthyWorkloadPods := ranhelper.WaitForAllPodsHealthy(
		[]string{parameters.PtpOperatorNamespace},
		45*time.Minute,
		interval,
		workloadStableDuration,
	)

	if len(unhealthyWorkloadPods) != 0 {
		return fmt.Errorf("at least one pod was not recovered after 45 minutes")
	}

	return nil
}

// NodesToPtpDaemonPods gets a list of nodes "nodesList" and a list of ptp daemon pods "podsList".
// and returns a map which the 'key' is a node and the 'value' is the ptp daemon pod of that specific node.
func NodesToPtpDaemonPods(nodesList []corev1.Node, podsList *corev1.PodList) map[*corev1.Node]*corev1.Pod {
	nodeToPtpDaemonPod := make(map[*corev1.Node]*corev1.Pod)

	podMap := make(map[string]*corev1.Pod)
	for _, pod := range podsList.Items {
		podMap[pod.Spec.NodeName] = &pod
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

	for _, pod := range ptpDaemonPods.Items {
		if pod.Spec.NodeName == node.Name {
			return &pod, nil
		}
	}

	return nil, fmt.Errorf("no new ptp daemon pod were created in %s node", node.Name)
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
