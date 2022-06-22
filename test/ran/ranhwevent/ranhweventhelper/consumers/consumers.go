package consumers

import (
	"context"
	"fmt"
	"log"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventparameters"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConsumerPod get a single consumer pod.
func GetConsumerPod() (*corev1.Pod, error) {
	consumerPods, err := GetConsumers()
	if err != nil {
		return nil, err
	}

	if len(consumerPods.Items) == 0 {
		return nil, fmt.Errorf("missing consumers")
	}

	return &consumerPods.Items[0], nil
}

// GetConsumers get all consumer pods that are deployed.
func GetConsumers() (*corev1.PodList, error) {
	consumerPods, err := helper.Apiclient.Pods(ranhweventparameters.NamespaceConsumer).List(context.Background(),
		metav1.ListOptions{
			LabelSelector: ranhweventparameters.ConsumerPodLabel})

	if err != nil {
		return nil, fmt.Errorf("failed to get consumer pods from namespace: %v due to: %w",
			ranhweventparameters.NamespaceConsumer, err)
	}

	if len(consumerPods.Items) == 0 {
		log.Printf("Failed to find pods with label: app=consumer in name space: %v\n",
			ranhweventparameters.NamespaceConsumer)
	}

	return consumerPods, nil
}
