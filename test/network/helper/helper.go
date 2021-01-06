package helper

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/gomega"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
)

// WaitUntilPodCreatedAndRunning waits until pod created and running. Returns running pod
func WaitUntilPodCreatedAndRunning(cs *client.ClientSet, podStruct *k8sv1.Pod, namespace string, waitingTime time.Duration) *k8sv1.Pod {
	err := cs.Create(context.Background(), podStruct)
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() k8sv1.PodPhase {
		tempPod, _ := cs.Pods(namespace).Get(context.Background(), podStruct.Name, metav1.GetOptions{})
		return tempPod.Status.Phase
	}, waitingTime, time.Second).Should(Equal(k8sv1.PodRunning))
	runningPod, err := cs.Pods(namespace).Get(context.Background(), podStruct.Name, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())
	return runningPod
}

// StrParamInListOfParams validates if specific sting parameter is valid
func StrParamInListOfParams(param string, paramRange []string) error {
	for _, parameter := range paramRange {
		if param == parameter {
			return nil
		}
	}
	return fmt.Errorf("error: wrong parameter %v", param)
}
