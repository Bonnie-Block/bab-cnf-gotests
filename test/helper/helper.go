package helper

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	v1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/machineconfigpool"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sigClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var podWaitingTime time.Duration = 5 * time.Minute

// PullTestImage pulls test image on all relevant nodes
func PullTestImage(cnfNodeLabel string, image string) {
	nodesList, err := nodes.GetByLabel(Apiclient, cnfNodeLabel)
	Expect(err).ToNot(HaveOccurred())
	for _, node := range nodesList.Items {
		pullPodDefenition := pod.RedefineWithRestartPolicy(pod.RedefineWithCommand(pod.DefinePodOnNode("default", image, node.Name),
			[]string{"echo", "image pulled Successfully && exit 0"}, []string{}), k8sv1.RestartPolicyNever)
		pullPod, err := Apiclient.Pods("default").Create(context.Background(), pullPodDefenition, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() k8sv1.PodPhase {
			pullPod, _ = Apiclient.Pods("default").Get(context.Background(), pullPod.Name, metav1.GetOptions{})
			return pullPod.Status.Phase
		}, podWaitingTime, time.Second).Should(Equal(k8sv1.PodSucceeded), fmt.Sprint("Invalid pulling image"))
	}
	err = namespaces.CleanPods("default", Apiclient)
	Expect(err).ToNot(HaveOccurred())
}

// CountLinesByMatches returns match count int based on match pattern
func CountLinesByMatches(str string, stringsToGrep ...string) int {
	count := 0
	exists := false
	for _, line := range strings.Split(str, "\n") {
		for _, grep := range stringsToGrep {
			if !strings.Contains(line, grep) {
				exists = false
				break
			}
			exists = true
		}
		if exists {
			count++
		}
	}
	return count
}

// WaitUntilPodCreatedAndRunning waits until pod created and running. Returns running pod
func WaitUntilPodCreatedAndRunning(podStruct *k8sv1.Pod, waitingTime time.Duration) *k8sv1.Pod {
	return waitUntilPodCreatedAndInPhase(podStruct, waitingTime, k8sv1.PodRunning)
}

func waitUntilPodCreatedAndInPhase(podStruct *k8sv1.Pod, waitingTime time.Duration, status k8sv1.PodPhase) *k8sv1.Pod {
	err := Apiclient.Create(context.Background(), podStruct)
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() k8sv1.PodPhase {
		tempPod, _ := Apiclient.Pods(podStruct.Namespace).Get(
			context.Background(),
			podStruct.Name,
			metav1.GetOptions{})
		return tempPod.Status.Phase
	}, waitingTime, time.Second).Should(Equal(status))
	runningPod, err := Apiclient.Pods(podStruct.Namespace).Get(
		context.Background(),
		podStruct.Name,
		metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())
	return runningPod
}

// IsDeploymentInstalled checks if deployment is installed
func IsDeploymentInstalled(cs *client.ClientSet, operatorNamespace string, operatorDeploymentName string) (bool, error) {
	_, err := cs.Deployments(operatorNamespace).Get(context.Background(), operatorDeploymentName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	return true, nil
}

// IsDeploymentReady checks if deployment is ready
func IsDeploymentReady(cs *client.ClientSet, operatorNamespace string, deploymentName string) (bool, error) {
	deployment, err := cs.Deployments(operatorNamespace).Get(context.Background(), deploymentName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	if deployment.Status.ReadyReplicas > 0 {
		if deployment.Status.Replicas == deployment.Status.ReadyReplicas {
			return true, nil
		}
	}
	return false, nil
}

// CountDaemonsets counts total number of Ready and Desired daemonsets and returns int count
func CountDaemonsets(cs *client.ClientSet, operatorNamespace string, daemonsetName string) (countRunningDaemonsets int32, countDesiredDaemonsets int32) {
	daemonSetDiscovery, err := cs.DaemonSets(operatorNamespace).Get(context.Background(), daemonsetName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	countRunningDaemonsets = daemonSetDiscovery.Status.NumberReady
	countDesiredDaemonsets = daemonSetDiscovery.Status.DesiredNumberScheduled
	return countRunningDaemonsets, countDesiredDaemonsets
}

// WaitForClusterToBeStable validates if MCP is stable
func WaitForClusterToBeStable(machineConfigPoolName string, snoTimeoutMultiplier time.Duration) error {
	mcp := &v1.MachineConfigPool{}

	err := Apiclient.Client.Get(context.TODO(), sigClient.ObjectKey{Name: machineConfigPoolName}, mcp)
	if err != nil {
		return err
	}

	err = machineconfigpool.WaitForCondition(
		Apiclient,
		&v1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Name: machineConfigPoolName}},
		v1.MachineConfigPoolUpdating,
		k8sv1.ConditionTrue,
		2*time.Minute)
	if err != nil {
		return err
	}

	// We need to wait a long time here for the node to reboot
	err = machineconfigpool.WaitForCondition(
		Apiclient,
		&v1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Name: machineConfigPoolName}},
		v1.MachineConfigPoolUpdated,
		k8sv1.ConditionTrue,
		time.Duration(20*mcp.Status.MachineCount)*time.Minute*snoTimeoutMultiplier)

	return err
}

// GetNodeListStringByLabel returns node names in list format
func GetNodeListStringByLabel(labelNodeRole string) []string {
	var nodeListString []string
	nodesList, err := nodes.GetByRole(Apiclient, labelNodeRole)
	Expect(err).ToNot(HaveOccurred())

	for _, node := range nodesList {
		nodeListString = append(nodeListString, node.Name)
	}
	return nodeListString
}
