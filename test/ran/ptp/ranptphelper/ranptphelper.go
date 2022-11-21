package ranptphelper

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"time"
)

// GetLastEventValue gets a pod "pod" and returns the last state of the last published event,
// if no event was published within the last 24 hours, an error is occurred.
func GetLastEventValue(ptpPod *corev1.Pod) (string, error) {
	return GetEventValueFromEnd(ptpPod, 0)
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

// GetTimeoutVal gets the timeout value in seconds from the ptp configuration.
// return value:    the holdover timeout duration and an error if any occurred.
func GetTimeoutVal() (time.Duration, error) {
	configList, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{})
	if nil != err {
		return 0, err
	}

	return time.Duration(configList.Items[0].Spec.Profile[0].PtpClockThreshold.HoldOverTimeout) * time.Second, nil
}

// BytesToStrings converts bytes.buffer to an array of strings.
// arguments:		"buff"-	a bytes buffer.
// return value:	an array of strings for each line in the bytes buffer.
func BytesToStrings(buff bytes.Buffer) []string {
	var strs []string
	strs = append(strs, strings.Split(buff.String(), "\n")...)

	return strs
}
