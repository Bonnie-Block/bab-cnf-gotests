package ranhelper

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/wait"

	"log"
	"strings"
	"time"

	"github.com/k8snetworkplumbingwg/sriov-network-operator/pkg/render"
	"github.com/pkg/errors"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

const (
	createMode = "create"
	deleteMode = "delete"
	updateMode = "update"
)

// ApplyObjects adds manifests from given directory to cluster.
func ApplyObjects(resDir string) error {
	return modifyObjects(createMode, resDir)
}

// DeleteObjects removes manifests from given directory from cluster.
func DeleteObjects(resDir string) error {
	return modifyObjects(deleteMode, resDir)
}

// UpdateObjects updates existing resurces based on manifests from given directory.
func UpdateObjects(resDir string) error {
	return modifyObjects(updateMode, resDir)
}

func modifyObjects(mode string, resDir string) error {
	data := render.MakeRenderData()
	objs, err := render.RenderDir(resDir, &data)

	if err != nil {
		return err
	}

	var errorList []error

	for _, obj := range objs {
		switch mode {
		case createMode:
			err = helper.Apiclient.Client.Create(context.TODO(), obj)
		case deleteMode:
			err = helper.Apiclient.Client.Delete(context.TODO(), obj)
		case updateMode:
			err = updateObject(obj)
		}

		if err != nil {
			errorList = append(errorList, err)
		}
	}

	if len(errorList) > 0 {
		return fmt.Errorf(
			"one or more errors occurred while processing resources from dir %s \n%v errors",
			resDir, errorList)
	}

	return nil
}

func updateObject(obj *unstructured.Unstructured) error {
	if obj.GetName() == "" {
		return errors.Errorf("Object %s has no name", obj.GroupVersionKind().String())
	}

	gvk := obj.GroupVersionKind()
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(gvk)
	err := helper.Apiclient.Client.Get(
		context.TODO(),
		types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()},
		existing)

	if err != nil {
		return err
	}

	obj.SetCreationTimestamp(existing.GetCreationTimestamp())
	obj.SetResourceVersion(existing.GetResourceVersion())
	obj.SetUID(existing.GetUID())
	obj.SetGeneration(existing.GetGeneration())
	obj.SetManagedFields(existing.GetManagedFields())
	obj.SetFinalizers(existing.GetFinalizers())

	return helper.Apiclient.Client.Update(context.TODO(), obj)
}

// WaitForClusterReachable waits for cluster reachable by listing cluster nodes and expecting it to work.
func WaitForClusterReachable() error {
	log.Println("Waiting for cluster to be reachable")

	cond := conditionFunc
	err := wait.Poll(3*time.Second, 15*time.Minute, cond)

	if nil != err {
		return err
	}

	log.Println("Cluster is reachable")

	return nil
}

func conditionFunc() (bool, error) {
	apiTimeout := int64(30)

	_, err := helper.Apiclient.Nodes().List(context.Background(), metav1.ListOptions{TimeoutSeconds: &apiTimeout})
	if nil == err {
		return true, nil
	}

	return false, nil
}

// WaitForAllPodsHealthy waits for all pods on cluster or in given namespaces to be Completed or Running & Ready
// Returns a map of unhealthy pods if any, otherwise empty map
// When namespaces is an empty list or nil, ALL namespaces on cluster will be checked.
func WaitForAllPodsHealthy(
	namespaces []string, timeout, interval, stableDuration time.Duration) map[string]map[string]string {
	var (
		msgNs     = "in cluster"
		msgStable = ""
	)

	if len(namespaces) > 0 {
		msgNs = fmt.Sprintf("in namespaces %v", namespaces)
	}

	if stableDuration > 0 {
		msgStable = fmt.Sprintf(" for %s", stableDuration.String())
	}

	log.Printf("Waiting up to %s for all pods %s to be healthy%s\n", timeout.String(), msgNs, msgStable)

	unhealthyPods := make(map[string]map[string]string)
	apiTimeout := int64(10)
	startTime := time.Now()
	errPullInterval := wait.PollImmediate(interval, timeout, func() (bool, error) {
		var namespacesToCheck []string
		if len(namespaces) == 0 {
			namespaceList, err := helper.Apiclient.Namespaces().List(
				context.Background(),
				metav1.ListOptions{TimeoutSeconds: &apiTimeout},
			)
			if err != nil {
				startTime = time.Now()

				return false, nil
			}
			for _, ns := range namespaceList.Items {
				namespacesToCheck = append(namespacesToCheck, ns.Name)
			}
		} else {
			namespacesToCheck = append(namespacesToCheck, namespaces...)
		}
		unhealthyPods = make(map[string]map[string]string)
		for _, nsName := range namespacesToCheck {
			podsInNs, err := getUnhealthyPods(nsName)
			if err != nil {
				unhealthyPods[nsName] = map[string]string{"all pods": "failed to list pods"}
			} else if len(podsInNs) > 0 {
				unhealthyPods[nsName] = podsInNs
			}
		}
		if len(unhealthyPods) > 0 {
			startTime = time.Now()

			return false, nil
		}
		// All pods are in expected state. Check for stable duration if it's larger than zero.
		if stableDuration > 0 {
			actualStableDuration := time.Since(startTime)
			// Add an interval because the timer started before mcp became updated
			if actualStableDuration < stableDuration+interval {
				return false, nil
			}
		}

		return true, nil
	})

	if errPullInterval == nil {
		log.Printf("All pods %s are healthy%s", msgNs, msgStable)
	} else {
		log.Println(errPullInterval.Error())
	}

	return unhealthyPods
}

// getUnhealthyPods lists pods in given namespace and checks pods status.
// Returns pods in specified namespace that are neither Completed nor Running & Ready, and an error if lists pods
// failed.
func getUnhealthyPods(namespace string) (map[string]string, error) {
	unhealthyPods := make(map[string]string)
	pods, err := helper.Apiclient.Pods(namespace).List(context.Background(), metav1.ListOptions{})

	if err != nil {
		return unhealthyPods, err
	}

	for _, pod := range pods.Items {
		err = helper.IsPodHealthy(&pod)
		if err != nil {
			// Ignore failed pod with restart policy never. This could happen in image pruner or installer pods that
			// will never restart. For those pods, instead of restarting the same pod, a new pod will be created
			// to complete the task.
			// Temp: Also excludes collector pods under logging namespace. As we don't have a valid logging server
			// configured, the pod gets stuck in Crashloopback. Remove this after RAN team figures out a workaround.
			if !((pod.Status.Phase == corev1.PodFailed && pod.Spec.RestartPolicy == corev1.RestartPolicyNever) ||
				pod.Namespace == "openshift-logging" && strings.HasPrefix(pod.Name, "collector")) {
				unhealthyPods[pod.Name] = err.Error()
			}
		}
	}

	return unhealthyPods, nil
}
