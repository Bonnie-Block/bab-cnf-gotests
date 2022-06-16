package netcnihelper

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
)

// CopyMap returns the new copy of given map.
func CopyMap(originalMap map[string]string) map[string]string {
	newMap := make(map[string]string)
	for key, value := range originalMap {
		newMap[key] = value
	}

	return newMap
}

// MarshalTypeToString returns give struck in json string format.
func MarshalTypeToString(typeToMarshal interface{}) (string, error) {
	marshaledBytes, err := json.Marshal(typeToMarshal)

	if err != nil {
		return "", fmt.Errorf("fail to marshal type due to: %w", err)
	}

	return string(marshaledBytes), err
}

// DefinePodNetworks returns mutation pod function which updates network annotations.
func DefinePodNetworks(annotation map[string]string) func(podManifest *k8sv1.Pod) {
	return func(podManifest *k8sv1.Pod) {
		podManifest.ObjectMeta.Annotations = annotation
	}
}

// GetPodStatus returns pods status.
func GetPodStatus(podDefinition *k8sv1.Pod) k8sv1.PodPhase {
	tempPod, _ := helper.Apiclient.Pods(podDefinition.Namespace).Get(
		context.Background(),
		podDefinition.Name,
		metav1.GetOptions{})

	return tempPod.Status.Phase
}

// IsNamespacedEventListContainsMessage returns true/false in case if event is found/not found.
func IsNamespacedEventListContainsMessage(namespace string, message string) (bool, error) {
	eventsList, err := PullFailedCreatePodSandBoxEvents(netcniparameters.TestNamespace)

	if err != nil {
		return false, fmt.Errorf("error to collect events from the given namespace %s due to %w", namespace, err)
	}

	for _, event := range eventsList.Items {
		if strings.Contains(event.Message, message) {
			return true, nil
		}
	}

	return false, nil
}

// PullFailedCreatePodSandBoxEvents returns list of event messages based on FailedCreatePodSandBox filer.
func PullFailedCreatePodSandBoxEvents(namespace string) (*k8sv1.EventList, error) {
	return pullEventsForFieldSelector(namespace, "reason=FailedCreatePodSandBox")
}

func pullEventsForFieldSelector(namespace string, fieldSelector string) (*k8sv1.EventList, error) {
	return helper.Apiclient.Events(namespace).List(context.Background(),
		metav1.ListOptions{FieldSelector: fieldSelector})
}
