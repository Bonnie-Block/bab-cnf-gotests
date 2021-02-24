package helper

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/gomega"

	fpgav1 "github.com/open-ness/openshift-operator/N3000/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/n3000/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
)

// CreateN3000ClusterConfig creates N3000Cluster resource
func CreateN3000ClusterConfig(cs *client.ClientSet, nodeName string, image string, bitstreamStore string, checksum string, pciAddr string) error {
	clusterConfig := &fpgav1.N3000Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "n3000",
			Namespace: parameters.OperatorNamespace,
		},
		Spec: fpgav1.N3000ClusterSpec{
			Nodes: []fpgav1.N3000ClusterNode{{
				NodeName: nodeName,
				FPGA: []fpgav1.N3000Fpga{{
					UserImageURL: "http://" + bitstreamStore + "/" + image,
					PCIAddr:      pciAddr,
					CheckSum:     checksum,
				}},
			}},
		},
	}
	err := cs.Create(context.Background(), clusterConfig)
	if err != nil {
		return err
	}

	Eventually(func() string {
		condition, err := GetN3000NodeCondition(cs)
		Expect(err).ToNot(HaveOccurred())
		return condition.Reason
	}, 1*time.Minute, 5*time.Second).Should(Equal("InProgress"), "N3000Cluster resource is not applied")
	return nil
}

// CreatePodWithPort creates a pod with configured port
func CreatePodWithPort(cs *client.ClientSet, namespace string, image string, port int32) *corev1.Pod {
	podDefenition := pod.GetPodDefinitionWithPortAndLabel(namespace, image, port, parameters.TestLabel)
	pod, err := cs.Pods(namespace).Create(context.Background(), podDefenition, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
	Eventually(func() corev1.PodPhase {
		pod, _ = cs.Pods(namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
		return pod.Status.Phase
	}, 1*time.Minute, time.Second).Should(Equal(corev1.PodRunning))
	return pod
}

// CleanAllN3000Cluster removes all N3000Cluster resources
func CleanAllN3000Cluster(cs *client.ClientSet) {
	n3000ClusterConfigList := &fpgav1.N3000ClusterList{}
	err := cs.List(context.Background(), n3000ClusterConfigList)
	Expect(err).ToNot(HaveOccurred())
	if len(n3000ClusterConfigList.Items) > 0 {
		for _, n3000ClusterConfig := range n3000ClusterConfigList.Items {
			err = cs.Delete(context.Background(), &n3000ClusterConfig)
			Expect(err).ToNot(HaveOccurred())
		}
	}
}

// CountN3000Daemonsets counts total number of  Ready  and Desired n3000 daemonsets
func CountN3000Daemonsets(cs *client.ClientSet, operatorNamespace string) (countRunningDaemonsets int32, countDesiredDaemonsets int32) {
	daemonsetDriver, err := cs.DaemonSets(operatorNamespace).Get(context.Background(), parameters.DaemonsetDriverName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	daemonsetTelemetry, err := cs.DaemonSets(operatorNamespace).Get(context.Background(), parameters.DaemonsetTelemetryName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	daemonsetN3000Daemon, err := cs.DaemonSets(operatorNamespace).Get(context.Background(), parameters.DaemonsetN3000DaemonName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	countRunningDaemonsets = daemonsetDriver.Status.NumberReady +
		daemonsetTelemetry.Status.NumberReady +
		daemonsetN3000Daemon.Status.NumberReady
	countDesiredDaemonsets = daemonsetDriver.Status.DesiredNumberScheduled +
		daemonsetTelemetry.Status.DesiredNumberScheduled +
		daemonsetN3000Daemon.Status.DesiredNumberScheduled
	return countRunningDaemonsets, countDesiredDaemonsets
}

// GetN3000NodeList retrieves n3000NodeList
func GetN3000NodeList(cs *client.ClientSet) (*fpgav1.N3000NodeList, error) {
	n3000NodeList := &fpgav1.N3000NodeList{}
	err := cs.List(context.Background(), n3000NodeList)
	if err != nil {
		return nil, err
	}
	n3000NodeList.Items, err = MatchingOptionalSelectorN3000(cs, n3000NodeList.Items)
	if err != nil {
		return nil, err
	}
	return n3000NodeList, nil
}

// GetN3000FpgaStatus retrieves N3000FpgaStatus of N3000Node
func GetN3000FpgaStatus(cs *client.ClientSet) (*fpgav1.N3000FpgaStatus, error) {
	n3000NodeList, err := GetN3000NodeList(cs)
	if err != nil {
		return nil, err
	}
	if n3000NodeList.Items[0].Status.FPGA == nil || len(n3000NodeList.Items[0].Status.FPGA) < 1 {
		return nil, fmt.Errorf("N3000Node %s doesn`t have FPGA configuration", n3000NodeList.Items[0].Name)
	}
	fpgaStatus := n3000NodeList.Items[0].Status.FPGA[0]
	return &fpgaStatus, nil
}

// GetN3000NodeCondition retrieves condition of N3000Node
func GetN3000NodeCondition(cs *client.ClientSet) (*metav1.Condition, error) {
	n3000NodeList, err := GetN3000NodeList(cs)
	if err != nil {
		return nil, err
	}
	if n3000NodeList.Items[0].Status.Conditions == nil || len(n3000NodeList.Items[0].Status.Conditions) < 1 {
		return nil, fmt.Errorf("N3000Node %s doesn`t have Condition configuration", n3000NodeList.Items[0].Name)
	}
	n3000NodeCondition := n3000NodeList.Items[0].Status.Conditions[0]
	return &n3000NodeCondition, nil
}

// MatchingOptionalSelectorN3000 filter the given slice with only the nodes matching the optional selector.
// If no selector is set, it returns the same list.
// The NODES_SELECTOR must be set with a labelselector expression.
// For example: NODES_SELECTOR="sctp=true"
func MatchingOptionalSelectorN3000(cs *client.ClientSet, toFilter []fpgav1.N3000Node) ([]fpgav1.N3000Node, error) {
	if nodes.NodesSelector == "" {
		return toFilter, nil
	}
	toMatch, err := nodes.GetByLabel(cs, nodes.NodesSelector)
	if err != nil {
		return nil, fmt.Errorf("Error in getting nodes matching the %s label selector, %v", nodes.NodesSelector, err)
	}
	if len(toMatch.Items) == 0 {
		return nil, fmt.Errorf("Failed to get nodes matching %s label selector", nodes.NodesSelector)
	}

	res := make([]fpgav1.N3000Node, 0)
	for _, n := range toFilter {
		for _, m := range toMatch.Items {
			if n.Name == m.Name {
				res = append(res, n)
				break
			}
		}
	}
	if len(res) == 0 {
		return nil, fmt.Errorf("Failed to find matching nodes with %s label selector", nodes.NodesSelector)
	}
	return res, nil
}

// CreateService creates Service resource
func CreateService(cs *client.ClientSet, namespace string) *corev1.Service {
	serviceDefinition := getServiceDefinition(parameters.TestLabel)
	service, err := cs.Services(namespace).Create(context.Background(), serviceDefinition, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	return service
}

// getServiceDefinition retrieves definition for Service with specific selector
func getServiceDefinition(selector map[string]string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "testservice-",
		},
		Spec: corev1.ServiceSpec{
			Selector: selector,
			Ports: []corev1.ServicePort{{
				NodePort: 0,
				Port:     80,
				Protocol: "TCP",
				TargetPort: intstr.IntOrString{
					IntVal: 80,
				},
			}},
		},
	}
	return service
}
