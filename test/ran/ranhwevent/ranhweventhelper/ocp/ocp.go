package ocp

import (
	"context"
	"log"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// GetWorkerNode get the node that supplies the redfish for this test.
func GetWorkerNode() (*corev1.Node, error) {
	workers, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
	if err != nil {
		log.Printf("Error when attempting to get worker node due to %v\n", err)

		return nil, err
	}

	return &workers[0], nil
}

// PurgeNamespace remove pods and namespace for a given namespace.
func PurgeNamespace(namespace string, clientSet *client.ClientSet) error {
	err := namespaces.CleanPods(namespace, clientSet)
	if err != nil {
		log.Printf("Failed to purge pods in namespace: %v due to: %v\n", namespace, err)

		return err
	}

	err = namespaces.DeleteAndWait(clientSet, parameters.PrivPodNamespace, 5*time.Minute)

	if err != nil {
		log.Printf("Failed to remove namespace: %v due to: %v\n",
			namespace, err)

		return err
	}

	return nil
}

// PurgePrivPodNamespace Remove privileged pods used to get the node vendor.
func PurgePrivPodNamespace() error {
	log.Println("Remove privileged pods used to get the node vendor")

	return PurgeNamespace(parameters.PrivPodNamespace, helper.Apiclient)
}

// RestartPod deletes a pod marked by a given label
// it then waits for a given timeout for the pod to resume running state.
func RestartPod(label string, timeout time.Duration) error {
	pod, err := GetPodByLabel(label)

	if err != nil {
		return err
	}

	podUID := pod.UID
	err = helper.Apiclient.Pods(pod.Namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})

	if err != nil {
		return err
	}

	return wait.PollImmediate(5*time.Second, timeout, func() (done bool, err error) {
		pod, err := GetPodByLabel(label)
		if pod.UID != podUID && err != nil {
			return false, nil
		}

		return pod.Status.Phase == corev1.PodRunning && pod.Status.ContainerStatuses[0].Ready, nil
	})
}

// GetPodByLabel get all pods in the ranhweventparameters.NamespaceConsumer
// that o have a given label.
func GetPodByLabel(label string) (corev1.Pod, error) {
	Pods, err := helper.Apiclient.Pods(ranhweventparameters.NamespaceConsumer).List(context.Background(),
		metav1.ListOptions{
			LabelSelector: label})

	if err != nil {
		return corev1.Pod{}, err
	}

	pod := Pods.Items[0]

	return pod, nil
}
