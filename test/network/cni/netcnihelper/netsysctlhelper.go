package netcnihelper

import (
	"context"
	"encoding/json"
	"fmt"

	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
)

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
