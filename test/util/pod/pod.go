package pod

import (
	"bytes"
	"context"
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
			Containers: []corev1.Container{{Name: parameters.MainContainerName,
				Image:   image,
				Command: parameters.SleepCommand}}}}

	return podObject
}

// DefineWithNodeNetworks defines pod attached to Node network.
func DefineWithNodeNetworks(nodeName string, networks []string, namespace string, image string) *corev1.Pod {
	podObject := getDefinition(namespace, image)
	podObject.Annotations = map[string]string{"k8s.v1.cni.cncf.io/networks": strings.Join(networks, ",")}
	podObject.Spec.NodeSelector = map[string]string{
		parameters.LabelHostname: nodeName,
	}

	return podObject
}

// RedefineOnMaster defines pod attached to Master Node.
func RedefineOnMaster(pod *corev1.Pod) *corev1.Pod {
	pod.Spec.Tolerations = []corev1.Toleration{
		{
			Key:    "node-role.kubernetes.io/master",
			Effect: "NoSchedule",
		},
	}

	return pod
}

// DefineWithHostNetwork  defines pod attached to Host network.
func DefineWithHostNetwork(nodeName string, namespace string, image string) *corev1.Pod {
	podObject := getDefinition(namespace, image)
	podObject.Spec.HostNetwork = true
	podObject.Spec.NodeSelector = map[string]string{
		parameters.LabelHostname: nodeName,
	}

	return podObject
}

// RedefineAsPrivileged uppdates the pod to be privileged.
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

// RedefineWithHostNetwork uppdates the pod definition Spec.HostNetwork to true.
func RedefineWithHostNetwork(pod *corev1.Pod) *corev1.Pod {
	pod.Spec.HostNetwork = true

	return pod
}

// RedefineWithCommand updates the pod definition with a different command.
func RedefineWithCommand(pod *corev1.Pod, command []string, args []string) *corev1.Pod {
	pod.Spec.Containers[0].Command = command
	pod.Spec.Containers[0].Args = args

	return pod
}

// RedefineWithRestartPolicy updates the pod definition with a restart policy.
func RedefineWithRestartPolicy(pod *corev1.Pod, restartPolicy corev1.RestartPolicy) *corev1.Pod {
	pod.Spec.RestartPolicy = restartPolicy

	return pod
}

// RedefineWithInitContainer adds init container to pod's manifest.
func RedefineWithInitContainer(pod *corev1.Pod, command []string) *corev1.Pod {
	pod.Spec.InitContainers = []corev1.Container{{
		Name:    "initcontainer",
		Image:   pod.Spec.Containers[0].Image,
		Command: command,
	}}

	return pod
}

// ExecCommand runs command in the pod and returns buffer output.
func ExecCommand(clientSet *testclient.ClientSet, pod corev1.Pod, command []string,
	containerName ...string) (bytes.Buffer, error) {
	var (
		buffer bytes.Buffer
		cName  string
	)

	if len(containerName) > 0 {
		cName = containerName[0]
	} else {
		cName = pod.Spec.Containers[0].Name
	}

	req := clientSet.CoreV1Interface.RESTClient().
		Post().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(pod.Name).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: cName,
			Command:   command,
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(clientSet.Config, "POST", req.URL())
	if err != nil {
		return buffer, err
	}

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: &buffer,
		Stderr: os.Stderr,
		Tty:    true,
	})
	if err != nil {
		return buffer, err
	}

	return buffer, nil
}

// GetLog connects to a pod and fetches log.
func GetLog(cs *testclient.ClientSet, p *corev1.Pod, s time.Duration, containerName string) (string, error) {
	logStart := int64(s.Seconds())
	req := cs.Pods(p.Namespace).GetLogs(p.Name, &corev1.PodLogOptions{SinceSeconds: &logStart, Container: containerName})
	log, err := req.Stream(context.Background())

	if err != nil {
		return "", err
	}

	defer func() {
		_ = log.Close()
	}()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, log)

	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// RedefinePodWithNetwork updates the pod definition with a network annotation.
func RedefinePodWithNetwork(pod *corev1.Pod, networksSpec string) *corev1.Pod {
	pod.ObjectMeta.Annotations = map[string]string{"k8s.v1.cni.cncf.io/networks": networksSpec}

	return pod
}

// DefinePodOnNode creates the pod definition with a node selector.
func DefinePodOnNode(namespace string, image string, nodeName string) *corev1.Pod {
	pod := getDefinition(namespace, image)
	pod.Spec.NodeSelector = map[string]string{parameters.LabelHostname: nodeName}

	return pod
}

// WaitForDeletion waits until the pod will be removed from the cluster.
func WaitForDeletion(cs *testclient.ClientSet, pod *corev1.Pod, timeout time.Duration) error {
	return wait.PollImmediate(time.Second, timeout, func() (bool, error) {
		_, err := cs.Pods(pod.Namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return true, nil
		}

		return false, nil
	})
}

// DeletePodAndWait removes given pods and waits until it's fully removed.
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

// RedefineWithHostPid allows a pod to have access to the host process ID namespace.
func RedefineWithHostPid(pod *corev1.Pod) *corev1.Pod {
	pod.Spec.HostPID = true

	return pod
}

// RedefineWithVolume redefines a pod with a new volume and volume mount. Given volume/volume mount will be
// appended to existing volumes/volume mounts.
func RedefineWithVolume(
	pod *corev1.Pod,
	volumeMountName string,
	mountPath string,
	volumeSource corev1.VolumeSource,
	readOnly bool) *corev1.Pod {
	volMount := corev1.VolumeMount{Name: volumeMountName, MountPath: mountPath}
	if readOnly {
		volMount.ReadOnly = true
	}

	for index := range pod.Spec.Containers {
		pod.Spec.Containers[index].VolumeMounts = append(pod.Spec.Containers[index].VolumeMounts, volMount)
	}

	if len(pod.Spec.InitContainers) > 0 {
		for index := range pod.Spec.InitContainers {
			pod.Spec.InitContainers[index].VolumeMounts = append(pod.Spec.InitContainers[index].VolumeMounts, volMount)
		}
	}

	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{Name: volumeMountName, VolumeSource: volumeSource})

	return pod
}

// RedefineWithObjectMeta updates pod name/generateName and annotations
// Use empty value "" or nil to skip a config. e.g., annotations=nil.
func RedefineWithObjectMeta(
	pod *corev1.Pod, name string, generateName string, annotations map[string]string) *corev1.Pod {
	if name != "" {
		pod.ObjectMeta.Name = name
		pod.ObjectMeta.GenerateName = ""
	} else if generateName != "" {
		pod.ObjectMeta.GenerateName = generateName
	}

	if annotations != nil {
		pod.ObjectMeta.Annotations = annotations
	}

	return pod
}

// RedefineWithLabel updates DefinePodOnNode() with label.
func RedefineWithLabel(pod *corev1.Pod, labeltype string, labelName string) *corev1.Pod {
	pod.ObjectMeta.Labels = map[string]string{labeltype: labelName}

	return pod
}
