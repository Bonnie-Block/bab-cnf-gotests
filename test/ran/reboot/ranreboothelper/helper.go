package ranreboothelper

import (
	"context"
	"fmt"
	"log"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/machineconfigpool"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

// SoftRebootNodeAndWait soft reboots given node and wait for cluster to recover
func SoftRebootNodeAndWait(node *corev1.Node) {
	isSNO, err := nodes.IsSingleNodeCluster(helper.Apiclient)
	Expect(err).ShouldNot(HaveOccurred())
	output, err := ranhelper.ExecCommandOnNodeWithHostBinaries(node, []string{"systemctl", "reboot"})
	Expect(err).ShouldNot(HaveOccurred(), output)

	if isSNO {
		// Wait for cluster to be unreachable, indicating reboot has started.
		waitForClusterUnreachable()
	} else {
		waitForNodeNotReady(node)
	}
	// check cluster is up
	waitForClusterReachable()
	// check mcps are updated
	err = machineconfigpool.WaitForClusterStable(helper.Apiclient, 30 * time.Minute, 10 * time.Second, 2 * time.Minute)
	Expect(err).ToNot(HaveOccurred())
	// check all nodes are Ready
	err = nodes.WaitForNodesReady(helper.Apiclient, 10 * time.Minute, 3 * time.Second)
	Expect(err).ToNot(HaveOccurred())
	// check all pods are recovered
	unhealthyPods := WaitForPodsHealthy(30 * time.Minute)
	Expect(unhealthyPods).To(BeEmpty())
}

// waitForClusterReachable waits for cluster reachable by listing cluster nodes and expecting it to work
func waitForClusterReachable() {
	log.Println("Waiting for cluster to be reachable")
	Eventually(func() error {
		_, err := helper.Apiclient.Nodes().List(context.Background(), metav1.ListOptions{})
		return err
	}, 15*time.Minute, 15*time.Second).ShouldNot(HaveOccurred())
	log.Println("Cluster is reachable")
}

// waitForClusterUnreachable waits for cluster unreachable by listing cluster nodes and expecting error
func waitForClusterUnreachable() {
	timeout := 3 * time.Minute
	Eventually(func() bool {
		_, err := helper.Apiclient.Nodes().List(context.Background(), metav1.ListOptions{})
		return err != nil
	}, timeout, 5*time.Second).Should(BeTrue(), fmt.Sprintf("cluster is still reachable after %s", timeout.String()))
	log.Println("Lost connection to cluster")
}

// waitForNodeNotReady waits for node status to become Not Ready
func waitForNodeNotReady(node *corev1.Node) {
	log.Println("Waiting for node to be reachable")
	timeout := 5 * time.Minute
	Eventually(func() error {
		node, err := helper.Apiclient.Nodes().Get(context.Background(), node.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !nodes.IsNodeInCondition(node, corev1.NodeReady) {
			return nil
		}
		return err
	}, timeout, 10*time.Second).ShouldNot(HaveOccurred())
	log.Printf("Node %s is not Ready\n", node.Name)
}

// WaitForPodsHealthy waits for all pods on cluster to be Completed or Running & Ready
// Return a map of unhealthy pods if any, otherwise empty map
func WaitForPodsHealthy(timeout time.Duration) map[string]map[string]string {
	log.Println("Waiting for all pods to be healthy")
	unhealthyPods := make(map[string]map[string]string)
	err_ := wait.PollImmediate(15*time.Second, timeout, func() (bool, error) {
		namespaces, err := helper.Apiclient.Namespaces().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		unhealthyPods = make(map[string]map[string]string)
		for _, ns := range namespaces.Items {
			podsInNs, err := getUnhealthyPods(ns.Name)
			if err != nil {
				unhealthyPods[ns.Name] = map[string]string{"all pods": "failed to list pods"}
			} else if len(podsInNs) > 0 {
				unhealthyPods[ns.Name] = podsInNs
			}
		}
		return len(unhealthyPods) == 0, nil
	})

	if err_ == nil {
		log.Println("All pods on cluster are healthy")
	}
	return unhealthyPods
}

// isPodInCondition returns true if given pod is in expected condition, otherwise false.
func isPodInCondition(pod *corev1.Pod, condition corev1.PodConditionType) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == condition && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// getUnhealthyPods lists pods in given namespace and checks pods status.
// Returns pods in specified namespace that are neither Completed nor Running & Ready, and an error if lists pods failed
func getUnhealthyPods(namespace string) (map[string]string, error) {
	unhealthyPods := make(map[string]string)
	pods, err := helper.Apiclient.Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return unhealthyPods, err
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			// Check if running pod is ready
			if !isPodInCondition(&pod, corev1.PodReady) {
				unhealthyPods[pod.Name] = fmt.Sprintf("Pod condition is not Ready")
			}
		} else if pod.Status.Phase != corev1.PodSucceeded {
			// Add pods that are not running or succeeded to unhealthy list
			unhealthyPods[pod.Name] = fmt.Sprintf("Pod phase is %s", pod.Status.Phase)
		}
	}
	return unhealthyPods, nil
}
