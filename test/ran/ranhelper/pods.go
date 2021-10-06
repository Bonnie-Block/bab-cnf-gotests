package ranhelper

import (
	"context"
	"fmt"
	"log"
	"math"
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

// DeletePodsAndWaitForRemoval deletes given pods and waits until they are removed from the cluster
func DeletePodsAndWaitForRemoval(pods []*corev1.Pod, timeout time.Duration) {
	log.Println("Delete and wait for pods to be removed")
	for _, pod := range pods {
		err := helper.Apiclient.Pods(pod.Namespace).Delete(context.Background(), pod.Name,
			metav1.DeleteOptions{})
		Expect(err).ToNot(HaveOccurred())
	}

	Eventually(func() error {
		for _, pod := range pods {
			_, err := helper.Apiclient.Pods(pod.Namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
			if !errors.IsNotFound(err) {
				return fmt.Errorf("Pod is still on system: %s", pod.Name)
			}
			log.Println("Pod is deleted:", pod.Name)
		}
		return nil
	}, timeout, 5*time.Second).ShouldNot(HaveOccurred())
}

// waitForPodsHealthy waits for given pods to appear and healthy
func waitForPodsHealthy(pods []*corev1.Pod, timeout time.Duration) {
	Eventually(func() error {
		for _, pod := range pods {
			tempPod, err := helper.Apiclient.Pods(pod.Namespace).Get(
				context.Background(),
				pod.Name,
				metav1.GetOptions{})
			if err != nil {
				return err
			}
			err = IsPodHealthy(tempPod)
			if err != nil {
				return err
			}
		}
		return nil
	}, timeout, 3*time.Second).ShouldNot(HaveOccurred())
}

// IsPodHealthy returns nil if given pod is healthy, otherwise an error.
func IsPodHealthy(pod *corev1.Pod) error {
	if pod.Status.Phase == corev1.PodRunning {
		// Check if running pod is ready
		if !isPodInCondition(pod, corev1.PodReady) {
			return fmt.Errorf("Pod condition is not Ready. Message: %s", pod.Status.Message)
		}
	} else if pod.Status.Phase != corev1.PodSucceeded {
		// Add pods that are not running or succeeded to unhealthy list
		return fmt.Errorf("Pod phase is %s. Message: %s", pod.Status.Phase, pod.Status.Message)
	}
	return nil
}

// isPodInCondition returns true if given pod is in expected condition, otherwise false.
func isPodInCondition(pod *corev1.Pod, condition corev1.PodConditionType) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == condition && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
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
func DefineStressPod(nodeName string, cpus int, guaranteed bool) *corev1.Pod {
	config_, err := config.NewConfig()
	Expect(err).ShouldNot(HaveOccurred())
	stressngImage := config_.Ran.StressngTestImage
	envVars := []corev1.EnvVar{{Name: "INITIAL_DELAY_SEC", Value: "60"}}
	cpuLimit := strconv.Itoa(cpus)
	memoryLimit := "100M"
	if !guaranteed {
		// Override CMDLINE for non-guaranteed pod to avoid specifying taskset
		envVars = append(envVars, corev1.EnvVar{Name: "CMDLINE", Value: fmt.Sprintf("--cpu %d --cpu-load 50", cpus)})
		cpuLimit = fmt.Sprintf("%dm", cpus*1200)
		memoryLimit = "200M"
	}
	pod := podhelper.DefinePodOnNode(ran.NamespaceTesting, stressngImage, nodeName)
	RedefineWithObjectMeta(pod, "", "stress-ng-", nil)
	podhelper.RedefineWithCommand(RedefineContainer(pod, "stress-ng", "", corev1.PullIfNotPresent), nil, nil)
	RedefineContainerEnvVars(pod, envVars)
	RedefineContainerResources(pod, cpuLimit, strconv.Itoa(cpus), memoryLimit, "100M")
	return pod
}

// DefineOslatPod returns oslat pod definition on given node with given cpu requests
func DefineOslatPod(profile *performancev2.PerformanceProfile, nodeName string, cpus int, duration string) *corev1.Pod {
	config_, err := config.NewConfig()
	Expect(err).ShouldNot(HaveOccurred())
	oslatImage := config_.Ran.OslatTestImage

	volumeType := corev1.HostPathCharDev
	pod := podhelper.RedefineAsPrivileged(podhelper.DefinePodOnNode(ran.NamespaceTesting, oslatImage, nodeName))
	RedefineWithRuntimeClass(pod, components.GetComponentName(profile.Name, components.ComponentNamePrefix))
	RedefineWithObjectMeta(pod, "", "oslat-", map[string]string{"cpu-load-balancing.crio.io": "true", "cpu-quota.crio.io": "true"})
	RedefineContainer(podhelper.RedefineWithCommand(pod, nil, nil), "container-perf-tools", "", corev1.PullAlways)
	RedefineContainerResources(pod, strconv.Itoa(cpus), strconv.Itoa(cpus), "1Gi", "1Gi")
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
	image := config_.Ran.ProcessExporterImage

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
		if daemonset.Spec.Template.Spec.Containers[0].Image != image {
			// Update dummy image to configured value
			daemonset.Spec.Template.Spec.Containers[0].Image = image
			helper.Apiclient.DaemonSets(ran.PromNamespace).Update(context.Background(), daemonset, metav1.UpdateOptions{})
			return fmt.Errorf("image updated, retrieve updated daemonset in next round")
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
	// Determine cpu requests for oslat and stress-ng pods.
	// stressNg cpu count is roughly 1/3.5 of total isolated cores
	isolatedCpuSet := cpuset.MustParse(string(*rtProfile.Spec.CPU.Isolated))
	// 1 cpu will be used by other consumer pods, such as process-exporter, ranpriv
	workloadCpuCount := isolatedCpuSet.Size() - 1
	oslatCpuCount := workloadCpuCount * 100 / 300
	stressNgCpuCount := workloadCpuCount - oslatCpuCount
	oslatMaxPodCount, stressngMaxPodCount := 2, 40
	oslatPodsCpus := parsePodCountAndCpus(oslatMaxPodCount, oslatCpuCount)
	stressngPodsCpus := parsePodCountAndCpus(stressngMaxPodCount, stressNgCpuCount)

	var err error
	// Create and wait for oslat pod to be Ready
	log.Printf("Creating up to %d oslat pods with total %d cpus", oslatMaxPodCount, oslatCpuCount)
	// Pick a large duration to ensure workload pod is always running during test
	workloadPods := []*corev1.Pod{}
	for _, cpuReq := range oslatPodsCpus {
		pod := DefineOslatPod(rtProfile, node.Name, cpuReq, "1440m")
		err = helper.Apiclient.Create(context.Background(), pod)
		Expect(err).ToNot(HaveOccurred())
		workloadPods = append(workloadPods, pod)
	}
	waitForPodsHealthy(workloadPods, 10*time.Minute)
	log.Printf("%d oslat pods with total %d cpus are created and running", len(workloadPods), oslatCpuCount)

	log.Printf("Creating up to %d stress-ng pods with total %d cpus", stressngMaxPodCount, stressNgCpuCount)
	stressngPods := []*corev1.Pod{}
	for _, cpuReq := range stressngPodsCpus {
		pod := DefineStressPod(node.Name, cpuReq, false)
		err = helper.Apiclient.Create(context.Background(), pod)
		Expect(err).ToNot(HaveOccurred())
		stressngPods = append(stressngPods, pod)
	}
	waitForPodsHealthy(stressngPods, 10*time.Minute)
	log.Printf("%d stress-ng pods with total %d cpus are created and running", len(stressngPods), stressNgCpuCount)
	return append(workloadPods, stressngPods...)
}

func parsePodCountAndCpus(maxPodCount, cpuCount int) []int {
	podCount := int(math.Min(float64(cpuCount), float64(maxPodCount)))
	cpuPerPod := int(cpuCount / podCount)
	cpus := []int{}
	for i := 1; i <= podCount-1; i++ {
		cpus = append(cpus, cpuPerPod)
	}
	cpus = append(cpus, cpuCount-cpuPerPod*(podCount-1))
	return cpus
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
		waitForPodsHealthy([]*corev1.Pod{privilegedPod}, 5*time.Minute)
	}
	return privPods
}
