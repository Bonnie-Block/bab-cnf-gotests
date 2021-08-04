package helper

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
