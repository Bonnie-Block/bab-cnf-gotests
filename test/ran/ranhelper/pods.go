package ranhelper

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	. "github.com/onsi/gomega"
	performancev2 "github.com/openshift-kni/performance-addon-operators/api/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/openshift-kni/performance-addon-operators/pkg/controller/performanceprofile/components"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	podhelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
)

// DeletePodAndWaitForRemoval deletes given pod and waits until it is removed from the cluster
func DeletePodAndWaitForRemoval(pod *corev1.Pod, timeout time.Duration) {
	log.Println("Delete pod and waiting for it to be removed:", pod.Name)
	err := helper.Apiclient.Pods(pod.Namespace).Delete(context.Background(), pod.Name,
		metav1.DeleteOptions{})
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() error {
		_, err := helper.Apiclient.Pods(pod.Namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			log.Println("Pod is deleted:", pod.Name)
			return nil
		}
		return fmt.Errorf("Pod is still on system: %s", pod.Name)
	}, timeout, 5*time.Second).ShouldNot(HaveOccurred())
}

// WaitForCondition waits until the pod will have specified condition type with the expected status
func WaitForCondition(pod *corev1.Pod, conditionType corev1.PodConditionType, conditionStatus corev1.ConditionStatus, timeout time.Duration) {
	Eventually(func() error {
		updatePod, err := helper.Apiclient.Pods(pod.Namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
		if err != nil {
			log.Println("Failed to retrieve pod conditions for pod: ", pod.Name)
			return err
		}
		for _, c := range updatePod.Status.Conditions {
			if c.Type == conditionType && c.Status == conditionStatus {
				log.Printf("Pod %s reached %v condition\n", pod.Name, conditionType)
				return nil
			}
		}
		return fmt.Errorf("Pod %s did not reach %v condition", pod.Name, conditionType)
	}, timeout, 5*time.Second).ShouldNot(HaveOccurred())
}

// WaitForPhases waits until the pod is in any of the specified phases
// Returns actual phase of the pod
func WaitForPhases(pod *corev1.Pod, phaseTypes []corev1.PodPhase, timeout time.Duration) corev1.PodPhase {
	log.Printf("Waiting for pod %s to be in any of these phases: %v", pod.Name, phaseTypes)
	var podPhase corev1.PodPhase
	Eventually(func() error {
		updatePod, err := helper.Apiclient.Pods(pod.Namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
		if err != nil {
			log.Println("Failed to retrieve pod phase for pod: ", pod.Name)
			return err
		}
		podPhase = updatePod.Status.Phase
		for _, expectedPhase := range phaseTypes {
			if podPhase == expectedPhase {
				log.Printf("Pod %s reached %v phase\n", pod.Name, expectedPhase)
				return nil
			}
		}
		return fmt.Errorf("Pod %s did not reach phase(s): %s. It is in %s phase", pod.Name, phaseTypes, updatePod.Status.Phase)
	}, timeout, 5*time.Second).ShouldNot(HaveOccurred())
	return podPhase
}

// WaitForPodHealthy waits until the pod is Completed or Running & Ready
func WaitForPodHealthy(pod *corev1.Pod, timeout time.Duration) {
	// First wait for pod to be Running or Succeeded
	podPhase := WaitForPhases(pod, []corev1.PodPhase{corev1.PodRunning, corev1.PodSucceeded}, timeout)

	// Then wait for Running pod to be Ready
	if podPhase == corev1.PodRunning {
		WaitForCondition(pod, corev1.PodReady, corev1.ConditionTrue, timeout)
	}
}

// RedefineContainerResources redefines a pod with CPU and Memory resources in first container
// Use empty string to skip a resource. e.g., cpuLimit=""
func RedefineContainerResources(pod *corev1.Pod, cpuLimit string, cpuRequest string, memoryLimit string, memoryRequest string) *corev1.Pod {
	pod.Spec.Containers[0].Resources.Requests = corev1.ResourceList{}
	pod.Spec.Containers[0].Resources.Limits = corev1.ResourceList{}
	if cpuLimit != "" {
		pod.Spec.Containers[0].Resources.Limits["cpu"] = resource.MustParse(cpuLimit)
	}
	if cpuRequest != "" {
		pod.Spec.Containers[0].Resources.Requests["cpu"] = resource.MustParse(cpuRequest)
	}
	if memoryLimit != "" {
		pod.Spec.Containers[0].Resources.Limits["memory"] = resource.MustParse(memoryLimit)
	}
	if memoryRequest != "" {
		pod.Spec.Containers[0].Resources.Requests["memory"] = resource.MustParse(memoryRequest)
	}
	return pod
}

// RedefineContainerEnvVars redefines EnvVars of first container in given pod
func RedefineContainerEnvVars(pod *corev1.Pod, EnvVars []corev1.EnvVar) *corev1.Pod {
	pod.Spec.Containers[0].Env = EnvVars
	return pod
}

// RedefineWithVolume redefines a pod with a new volume and volume mount. Given volume/volume mount will be appended to existing volumes/volume mounts.
func RedefineWithVolume(pod *corev1.Pod, volumeMountName string, mountPath string, volumeName string, volumeSource corev1.VolumeSource) *corev1.Pod {
	pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{Name: volumeMountName, MountPath: mountPath})
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{Name: volumeName, VolumeSource: volumeSource})
	return pod
}

// RedefineContainer redefines first container in a pod with its name, image, and pull policy.
// Use empty string to skip a config. e.g., imagePullPolicy=""
func RedefineContainer(pod *corev1.Pod, name string, image string, imagePullPolicy corev1.PullPolicy) *corev1.Pod {
	if name != "" {
		pod.Spec.Containers[0].Name = name
	}
	if image != "" {
		pod.Spec.Containers[0].Image = image
	}
	if imagePullPolicy != "" {
		pod.Spec.Containers[0].ImagePullPolicy = imagePullPolicy
	}
	return pod
}

// RedefineWithObjectMeta updates pod name/generateName and annotations
// Use empty value "" or nil to skip a config. e.g., annotations=nil
func RedefineWithObjectMeta(pod *corev1.Pod, name string, generateName string, annotations map[string]string) *corev1.Pod {
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

// RedefineWithRuntimeClass updates pod definition with specified runtime class name.
func RedefineWithRuntimeClass(pod *corev1.Pod, runtimeClass string) *corev1.Pod {
	pod.Spec.RuntimeClassName = &runtimeClass
	return pod
}

// DefineStressPod returns stress-ng pod definition.
func DefineStressPod(nodeName string, cpus int) *corev1.Pod {
	// TODO: Use config.Ran.StressngTestImage after CNF-2570 is done.
	// config_, err := config.NewConfig()
	// Expect(err).ShouldNot(HaveOccurred())
	// stressngImage := config_.Ran.StressngTestImage
	stressngImage := "quay.io/imiller/stress-ng:2.0"

	pod := podhelper.DefinePodOnNode(ran.NamespaceTesting, stressngImage, nodeName)
	RedefineWithObjectMeta(pod, "", "stress-ng-", nil)
	podhelper.RedefineWithCommand(RedefineContainer(pod, "stress-ng", "", corev1.PullIfNotPresent), nil, nil)
	RedefineContainerResources(pod, strconv.Itoa(cpus), strconv.Itoa(cpus), "100M", "100M")
	RedefineContainerEnvVars(pod, []corev1.EnvVar{{Name: "INITIAL_DELAY_SEC", Value: "60"}})
	return pod
}

// DefineOslatPod returns oslat pod definition on given node with given cpu requests
func DefineOslatPod(profile *performancev2.PerformanceProfile, nodeName string, cpus int, duration string) *corev1.Pod {

	// TODO: Use config.Ran.OslatTestImage after CNF-2570 is done.
	// config_, err := config.NewConfig()
	// Expect(err).ShouldNot(HaveOccurred())
	// oslatImage := config_.Ran.OslatTestImage
	oslatImage := "quay.io/jianzzha/oslat"

	volumeType := corev1.HostPathCharDev
	pod := podhelper.RedefineAsPrivileged(podhelper.DefinePodOnNode(ran.NamespaceTesting, oslatImage, nodeName))
	RedefineWithRuntimeClass(pod, components.GetComponentName(profile.Name, components.ComponentNamePrefix))
	RedefineWithObjectMeta(pod, "", "oslat-", map[string]string{"cpu-load-balancing.crio.io": "true", "cpu-quota.crio.io": "true"})
	RedefineContainer(podhelper.RedefineWithCommand(pod, nil, nil), "container-perf-tools", "", corev1.PullAlways)
	RedefineContainerResources(pod, strconv.Itoa(cpus), strconv.Itoa(cpus), "2Gi", "2Gi")
	RedefineWithVolume(pod, "cstate", "/dev/cpu_dma_latency", "cstate", corev1.VolumeSource{
		HostPath: &corev1.HostPathVolumeSource{Path: "/dev/cpu_dma_latency", Type: &volumeType}})
	RedefineContainerEnvVars(pod, []corev1.EnvVar{
		{Name: "tool", Value: "oslat"},
		{Name: "RUNTIME_SECONDS", Value: duration},
		{Name: "INITIAL_DELAY_SEC", Value: "30"},
		{Name: "RTPRIO", Value: "1"},
		{Name: "delay", Value: "60"},
		{Name: "manual", Value: "n"}})
	return pod
}

// DeployProcessExporter deploys process exporter and returns the daemonset and error if any
func DeployProcessExporter() *appsv1.DaemonSet {
	config_, err := config.NewConfig()
	Expect(err).ShouldNot(HaveOccurred())
	configsDir := config_.Ran.ProcessExporterConfigsDir

	daemonset, err := helper.Apiclient.DaemonSets(ran.PromNamespace).Get(context.Background(), ran.ProcessExporterPodName, metav1.GetOptions{})
	if err != nil {
		err = ApplyObjects(configsDir)
	} else {
		err = UpdateObjects(configsDir)
	}
	Expect(err).ShouldNot(HaveOccurred())

	Eventually(func() error {
		daemonset, err = helper.Apiclient.DaemonSets(ran.PromNamespace).Get(context.Background(), ran.ProcessExporterPodName, metav1.GetOptions{})
		if err != nil {
			log.Println("Failed to retrieve status for daemonset: ", daemonset.Name)
			return err
		}
		if daemonset.Status.NumberReady == daemonset.Status.DesiredNumberScheduled {
			log.Printf("All pods are ready in daemonset: %s\n", daemonset.Name)
			return nil
		} else {
			return fmt.Errorf("Not all daemon pods are ready in daemonset: %s", daemonset.Name)
		}
	}, 5*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

	return daemonset
}

// DeployWorkloadPods deploy oslat and stress-ng pods to fill up isolated cpus
// stressNg pod will be pinned to roughly 1/3.5 of total isolated cores
func DeployWorkloadPods(rtProfile *performancev2.PerformanceProfile, node *corev1.Node) []*corev1.Pod {
	var worklodPods = []*corev1.Pod{nil, nil}

	// Determine cpu requests for oslat and stress-ng pods.
	// stressNg cpu count is roughly 1/3.5 of total isolated cores
	isolatedCpuSet := cpuset.MustParse(string(*rtProfile.Spec.CPU.Isolated))
	// 1 cpu will be used by other consumer pods, such as process-exporter
	workloadCpuCount := isolatedCpuSet.Size() - 1
	stressNgCpuCount := workloadCpuCount * 100 / 350
	if stressNgCpuCount%2 != 0 {
		stressNgCpuCount -= 1
	}
	oslatCpuCount := workloadCpuCount - stressNgCpuCount

	// Create and wait for oslat pod to be Ready
	log.Printf("Creating oslat pod with %d cpus requested", oslatCpuCount)
	// Pick a large duration to ensure workload pod is always running during test
	oslatPod := helper.WaitUntilPodCreatedAndRunning(DefineOslatPod(rtProfile, node.Name, oslatCpuCount, "1440m"), 10*time.Minute)
	worklodPods[0] = oslatPod
	WaitForPodHealthy(oslatPod, 10*time.Minute)

	// Create and wait for stress-ng pod to be Ready
	log.Printf("Creating stress-ng pod with %d cpus requested", stressNgCpuCount)
	stressNgPod := helper.WaitUntilPodCreatedAndRunning(DefineStressPod(node.Name, stressNgCpuCount), 10*time.Minute)
	worklodPods[1] = stressNgPod
	WaitForPodHealthy(stressNgPod, 10*time.Minute)
	return worklodPods
}

// CreatePrivilegedPods creates privileged test pods on all nodes to assist testing
// Returns a map with nodeName as key and pod pointer as value, and error if occurred.
func CreatePrivilegedPods(image string) map[string]*corev1.Pod {
	if image == "" {
		config_, err := config.NewConfig()
		Expect(err).ShouldNot(HaveOccurred())
		image = config_.Ran.CnfTestImage
	}
	// Create ranpriv namespace if not alrady created
	if !namespaces.Exists(ran.PrivPodNamespace, helper.Apiclient) {
		log.Println("Creating namespace:", ran.PrivPodNamespace)
		err := namespaces.Create(ran.PrivPodNamespace, helper.Apiclient)
		Expect(err).ShouldNot(HaveOccurred())
	}
	nodes, err := helper.Apiclient.Nodes().List(context.Background(), metav1.ListOptions{})
	Expect(err).ShouldNot(HaveOccurred())
	privPods := make(map[string]*corev1.Pod)
	volumeType := corev1.HostPathUnset
	volSource := corev1.VolumeSource{
		HostPath: &corev1.HostPathVolumeSource{Path: "/", Type: &volumeType}}

	for _, node := range nodes.Items {
		podName := fmt.Sprintf("%s-%s", ran.PrivPodNamespace, node.Name)
		privilegedPod, err := helper.Apiclient.Pods(ran.PrivPodNamespace).Get(context.Background(), podName, metav1.GetOptions{})
		if err != nil {
			privilegedPod = podhelper.RedefineAsPrivileged(podhelper.DefinePodOnNode(ran.PrivPodNamespace, image, node.Name))
			privilegedPod = RedefineWithVolume(privilegedPod, "rootfs", "/rootfs", "rootfs", volSource)
			privilegedPod = helper.WaitUntilPodCreatedAndRunning(RedefineWithObjectMeta(privilegedPod, podName, "", nil), 10*time.Minute)
		}
		privPods[node.Name] = privilegedPod
		WaitForPodHealthy(privilegedPod, 5*time.Minute)
	}
	return privPods
}
