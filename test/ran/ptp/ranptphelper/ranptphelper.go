package ranptphelper

import (
	"context"
	"fmt"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"time"
)

// GetLastEventValue gets a pod "pod" and returns the last state of the last published event,
// if no event was published within the last 24 hours, an error is occurred.
func GetLastEventValue(ptpPod *corev1.Pod) (string, error) {
	return GetEventValueFromEnd(ptpPod, 0)
}

// WaitForClusterRecover waits up to 45 minutes for all clusters in a given node "node" to recover,
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

	unhealthyWorkloadPods := helper.WaitForAllPodsHealthy(
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
