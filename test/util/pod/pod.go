package pod

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/utils/pointer"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	testclient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
)

func getDefinition(namespace string, image string) *corev1.Pod {
	podObject := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "testpod-",
			Namespace:    namespace},
		Spec: corev1.PodSpec{
			TerminationGracePeriodSeconds: pointer.Int64Ptr(0),
			Containers: []corev1.Container{{Name: "test",
				Image:   image,
				Command: []string{"/bin/bash", "-c", "sleep INF"}}}}}

	return podObject
}

// DefineWithNetworks defines pod attached to network
func DefineWithNetworks(networks []string, namespace string, image string) *corev1.Pod {
	podObject := getDefinition(namespace, image)
	podObject.Annotations = map[string]string{"k8s.v1.cni.cncf.io/networks": strings.Join(networks, ",")}

	return podObject
}

// DefineWithNodeNetworks defines pod attached to Node network
func DefineWithNodeNetworks(nodeName string, networks []string, namespace string, image string) *corev1.Pod {
	podObject := getDefinition(namespace, image)
	podObject.Annotations = map[string]string{"k8s.v1.cni.cncf.io/networks": strings.Join(networks, ",")}
	podObject.Spec.NodeSelector = map[string]string{
		parameters.LabelHostname: nodeName,
	}
	return podObject
}

// DefineWithHostNetwork  defines pod attached to Host network
func DefineWithHostNetwork(nodeName string, namespace string, image string) *corev1.Pod {
	podObject := getDefinition(namespace, image)
	podObject.Spec.HostNetwork = true
	podObject.Spec.NodeSelector = map[string]string{
		parameters.LabelHostname: nodeName,
	}

	return podObject
}

// RedefineAsPrivileged uppdates the pod to be privileged
func RedefineAsPrivileged(pod *corev1.Pod) *corev1.Pod {
	pod.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{}
	b := true
	pod.Spec.Containers[0].SecurityContext.Privileged = &b
	return pod
}

func RedefineAsNetRaw(pod *corev1.Pod) *corev1.Pod {
	pod.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{}
	pod.Spec.Containers[0].SecurityContext.Capabilities = &corev1.Capabilities{
		Add: []corev1.Capability{"NET_RAW"},
	}
	return pod
}

// RedefineWithHostNetwork uppdates the pod definition Spec.HostNetwork to true
func RedefineWithHostNetwork(pod *corev1.Pod) *corev1.Pod {
	pod.Spec.HostNetwork = true
	return pod
}

// RedefineWithNodeSelector uppdates the pod definition with a node selector
func RedefineWithNodeSelector(pod *corev1.Pod, node string) *corev1.Pod {
	pod.Spec.NodeSelector = map[string]string{
		parameters.LabelHostname: node,
	}
	return pod
}

// RedefineWithCommand updates the pod defintion with a different command
func RedefineWithCommand(pod *corev1.Pod, command []string, args []string) *corev1.Pod {
	pod.Spec.Containers[0].Command = command
	pod.Spec.Containers[0].Args = args
	return pod
}

// RedefineWithRestartPolicy updates the pod defintion with a restart policy
func RedefineWithRestartPolicy(pod *corev1.Pod, restartPolicy corev1.RestartPolicy) *corev1.Pod {
	pod.Spec.RestartPolicy = restartPolicy
	return pod
}

// ExecCommand runs command in the pod and returns buffer output
func ExecCommand(cs *testclient.ClientSet, pod corev1.Pod, command []string) (bytes.Buffer, error) {
	var buf bytes.Buffer
	req := cs.CoreV1Interface.RESTClient().
		Post().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(pod.Name).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: pod.Spec.Containers[0].Name,
			Command:   command,
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(cs.Config, "POST", req.URL())
	if err != nil {
		return buf, err
	}

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: &buf,
		Stderr: os.Stderr,
		Tty:    true,
	})
	if err != nil {
		return buf, err
	}

	return buf, nil
}

// GetLog connects to a pod and fetches log
func GetLog(cs *testclient.ClientSet, p *corev1.Pod, s time.Duration, containerName string) (string, error) {
	logStart := int64(s.Seconds())
	req := cs.Pods(p.Namespace).GetLogs(p.Name, &corev1.PodLogOptions{SinceSeconds: &logStart, Container: containerName})
	log, err := req.Stream(context.Background())
	if err != nil {
		return "", err
	}
	defer log.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, log)

	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// RedefinePodWithNetwork updates the pod defintion with a network annotation
func RedefinePodWithNetwork(pod *corev1.Pod, networksSpec string) *corev1.Pod {
	pod.ObjectMeta.Annotations = map[string]string{"k8s.v1.cni.cncf.io/networks": networksSpec}
	return pod
}

// DefinePodOnNode creates the pod defintion with a node selector
func DefinePodOnNode(namespace string, image string, nodeName string) *corev1.Pod {
	pod := getDefinition(namespace, image)
	pod.Spec.NodeSelector = map[string]string{parameters.LabelHostname: nodeName}
	return pod
}

// DefinePodOnHostNetwork updates the pod defintion with a host network flag
func DefinePodOnHostNetwork(namespace string, image string, nodeName string) *corev1.Pod {
	pod := DefinePodOnNode(namespace, image, nodeName)
	pod.Spec.HostNetwork = true
	return pod
}

// DefinePodWithStaticIpamStatiMac sets pod's network with static IP+mac address config
func DefinePodWithStaticIpamStatiMac(pod *corev1.Pod, networkName string, ipAddress string, macAddress string) *corev1.Pod {
	pod.Annotations = map[string]string{"k8s.v1.cni.cncf.io/networks": fmt.Sprintf(`[
		{
			"name": "%s", 
			"mac": "%s",
			"ips": ["%s/24"]
		}
	]`, networkName, macAddress, ipAddress)}
	return pod
}

// GetPodDefinitionWithPort retrieves pod with port configuration
func GetPodDefinitionWithPort(namespace string, image string, port int32) *corev1.Pod {
	podObject := getDefinition(namespace, image)
	podObject.Spec.Containers[0].Command = []string{"/bin/sleep", "3650d"}
	podObject.Spec.Containers[0].ImagePullPolicy = "IfNotPresent"
	podObject.Spec.Containers[0].Ports = []corev1.ContainerPort{{
		ContainerPort: port,
		Protocol:      "TCP"}}
	return podObject
}

// GetPodDefinitionWithPortAndLabel retrieves pod with port configuration
func GetPodDefinitionWithPortAndLabel(namespace string, image string, port int32, labels map[string]string) *corev1.Pod {
	podObject := GetPodDefinitionWithPort(namespace, image, port)
	podObject.Labels = labels
	return podObject
}

// WaitForDeletion waits until the pod will be removed from the cluster
func WaitForDeletion(cs *testclient.ClientSet, pod *corev1.Pod, timeout time.Duration) error {
	return wait.PollImmediate(time.Second, timeout, func() (bool, error) {
		_, err := cs.Pods(pod.Namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return true, nil
		}
		return false, nil
	})
}

// DeletePodAndWait removes given pods and waits until it's fully removed
func DeletePodAndWait(apiClient *testclient.ClientSet, podToDelete *corev1.Pod) error {
	err := apiClient.Pods(podToDelete.Namespace).Delete(
		context.Background(),
		podToDelete.Name,
		metav1.DeleteOptions{GracePeriodSeconds: pointer.Int64Ptr(0)})
	if err != nil {
		return err
	}
	return WaitForDeletion(apiClient, podToDelete, 3*time.Minute)
}
