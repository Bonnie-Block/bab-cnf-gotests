package ranhelper

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"time"

	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	performancev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"github.com/openshift/cluster-node-tuning-operator/pkg/performanceprofile/controller/performanceprofile/components"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	podhelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
)

// DeletePodsAndWaitForRemoval deletes given pods and waits until they are removed from the cluster.
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
				return fmt.Errorf("pod is still on system: %s", pod.Name)
			}
			log.Println("pod is deleted:", pod.Name)
		}

		return nil
	}, timeout, 5*time.Second).ShouldNot(HaveOccurred())
}

// RedefineContainerResources redefines a pod with CPU and Memory resources in first container
// Use empty string to skip a resource. e.g., cpuLimit="".
func RedefineContainerResources(
	pod *corev1.Pod, cpuRequest string, cpuLimit string, memoryRequest string, memoryLimit string) *corev1.Pod {
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

// RedefineContainerEnvVars redefines EnvVars of first container in given pod.
func RedefineContainerEnvVars(pod *corev1.Pod, envVars []corev1.EnvVar) *corev1.Pod {
	pod.Spec.Containers[0].Env = envVars

	return pod
}

// RedefineContainer redefines first container in a pod with its name, image, and pull policy.
// Use empty string to skip a config. e.g., imagePullPolicy="".
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

// RedefineWithNewAnnotations appends given annotations to existing pod annotations.
func RedefineWithNewAnnotations(pod *corev1.Pod, annotations map[string]string) *corev1.Pod {
	podAnnotations := pod.Annotations
	if podAnnotations == nil {
		podAnnotations = make(map[string]string)
	}

	for k, v := range annotations {
		podAnnotations[k] = v
	}

	pod.Annotations = podAnnotations

	return pod
}

// RedefineWithRuntimeClass updates pod definition with specified runtime class name.
func RedefineWithRuntimeClass(pod *corev1.Pod, runtimeClass string) *corev1.Pod {
	pod.Spec.RuntimeClassName = &runtimeClass

	return pod
}

// DefineStressPod returns stress-ng pod definition.
func DefineStressPod(nodeName string, cpus int, guaranteed bool) *corev1.Pod {
	stressngImage := helper.Config.Ran.StressngTestImage
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
	podhelper.RedefineWithObjectMeta(pod, "", "stress-ng-", nil)
	podhelper.RedefineWithCommand(
		RedefineContainer(pod, "stress-ng", "", corev1.PullIfNotPresent),
		nil,
		nil,
	)
	RedefineContainerEnvVars(pod, envVars)
	RedefineContainerResources(pod, strconv.Itoa(cpus), cpuLimit, "100M", memoryLimit)

	return pod
}

// DefineOslatPod returns oslat pod definition on given node with given cpu requests.
func DefineOslatPod(profile *performancev2.PerformanceProfile, nodeName string, cpus int, duration string) *corev1.Pod {
	oslatImage := helper.Config.Ran.OslatTestImage

	volumeType := corev1.HostPathCharDev
	pod := podhelper.RedefineAsPrivileged(podhelper.DefinePodOnNode(ran.NamespaceTesting, oslatImage, nodeName))
	RedefineWithRuntimeClass(pod, components.GetComponentName(profile.Name, components.ComponentNamePrefix))
	podhelper.RedefineWithObjectMeta(
		pod,
		"",
		"oslat-",
		map[string]string{
			"irq-load-balancing.crio.io": "disable",
			"cpu-load-balancing.crio.io": "disable",
			"cpu-quota.crio.io":          "disable",
		},
	)
	RedefineContainer(
		podhelper.RedefineWithCommand(
			pod, nil, nil),
		"container-perf-tools",
		"",
		corev1.PullAlways,
	)
	RedefineContainerResources(pod, strconv.Itoa(cpus), strconv.Itoa(cpus), "1Gi", "1Gi")
	podhelper.RedefineWithVolume(pod, "cstate", "/dev/cpu_dma_latency", corev1.VolumeSource{
		HostPath: &corev1.HostPathVolumeSource{Path: "/dev/cpu_dma_latency", Type: &volumeType}}, false)
	RedefineContainerEnvVars(pod, []corev1.EnvVar{
		{Name: "tool", Value: "oslat"},
		{Name: "RUNTIME_SECONDS", Value: duration},
		{Name: "INITIAL_DELAY_SEC", Value: "30"},
		{Name: "RTPRIO", Value: "1"},
		{Name: "delay", Value: "60"},
		{Name: "manual", Value: "n"}})

	return pod
}

// DeleteProcessExporter deletes process exporter daemonset.
func DeleteProcessExporter() {
	configsDir := helper.Config.Ran.ProcessExporterConfigsDir

	daemonset, err := helper.Apiclient.DaemonSets(parameters.PromNamespace).Get(
		context.Background(),
		ran.ProcessExporterPodName,
		metav1.GetOptions{},
	)
	if err != nil {
		return
	}

	name := daemonset.Name
	log.Println("Deleting", name)

	err = DeleteObjects(configsDir)
	Expect(err).ShouldNot(HaveOccurred())
	Eventually(func() error {
		daemonset, err = helper.Apiclient.DaemonSets(parameters.PromNamespace).Get(
			context.Background(),
			ran.ProcessExporterPodName,
			metav1.GetOptions{},
		)
		if err != nil {
			log.Printf("Daemonset %s is removed from cluster\n", name)

			return nil
		}

		return fmt.Errorf("daemonset %s still exists on cluster", daemonset.Name)
	}, 5*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())
}

// DeployWorkloadPods deploy oslat and stress-ng pods to fill up isolated cpus
// stressNg pod will be pinned to roughly 1/3.5 of total isolated cores.
func DeployWorkloadPods(rtProfile *performancev2.PerformanceProfile, node *corev1.Node) []*corev1.Pod {
	// Determine cpu requests for oslat and stress-ng pods.
	// stressNg cpu count is roughly 1/3.5 of total isolated cores
	isolatedCPUSet := cpuset.MustParse(string(*rtProfile.Spec.CPU.Isolated))
	// 1 cpu will be used by other consumer pods, such as process-exporter, cnfgotestpriv
	workloadCPUCount := isolatedCPUSet.Size() - 1
	oslatCPUCount := workloadCPUCount * 100 / 300
	stressNgCPUCount := workloadCPUCount - oslatCPUCount
	oslatMaxPodCount, stressngMaxPodCount := 2, 40
	oslatPodsCPUs := parsePodCountAndCpus(oslatMaxPodCount, oslatCPUCount)
	stressngPodsCPUs := parsePodCountAndCpus(stressngMaxPodCount, stressNgCPUCount)

	var err error
	// Create and wait for oslat pod to be Ready
	log.Printf("Creating up to %d oslat pods with total %d cpus", oslatMaxPodCount, oslatCPUCount)
	// Pick a large duration to ensure workload pod is always running during test
	workloadPods := []*corev1.Pod{}

	for _, cpuReq := range oslatPodsCPUs {
		pod := DefineOslatPod(rtProfile, node.Name, cpuReq, "1440m")
		err = helper.Apiclient.Create(context.Background(), pod)
		Expect(err).ToNot(HaveOccurred())
		workloadPods = append(workloadPods, pod)
	}

	helper.WaitForPodsHealthy(workloadPods, 15*time.Minute)
	log.Printf("%d oslat pods with total %d cpus are created and running", len(workloadPods), oslatCPUCount)

	log.Printf("Creating up to %d stress-ng pods with total %d cpus", stressngMaxPodCount, stressNgCPUCount)

	stressngPods := []*corev1.Pod{}

	for _, cpuReq := range stressngPodsCPUs {
		pod := DefineStressPod(node.Name, cpuReq, false)
		err = helper.Apiclient.Create(context.Background(), pod)
		Expect(err).ToNot(HaveOccurred())
		stressngPods = append(stressngPods, pod)
	}

	helper.WaitForPodsHealthy(stressngPods, 20*time.Minute)
	log.Printf("%d stress-ng pods with total %d cpus are created and running", len(stressngPods), stressNgCPUCount)

	return append(workloadPods, stressngPods...)
}

func parsePodCountAndCpus(maxPodCount, cpuCount int) []int {
	podCount := int(math.Min(float64(cpuCount), float64(maxPodCount)))
	cpuPerPod := cpuCount / podCount
	cpus := []int{}

	for i := 1; i <= podCount-1; i++ {
		cpus = append(cpus, cpuPerPod)
	}
	cpus = append(cpus, cpuCount-cpuPerPod*(podCount-1))

	return cpus
}

// CleanupRanTestResources deletes created test resources.
func CleanupRanTestResources() {
	// Delete process exporter if exists
	DeleteProcessExporter()
	// Delete ran-test namespace if exists
	for _, ns := range []string{ran.NamespaceTesting, parameters.PrivPodNamespace} {
		if namespaces.Exists(ns, helper.Apiclient) {
			log.Println("Deleting test namespace", ns)
			err := namespaces.DeleteAndWait(helper.Apiclient, ns, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred())
		}
	}
}

// DeployProcessExporter deploys process exporter and returns the daemonset and error if any.
func DeployProcessExporter() *appsv1.DaemonSet {
	daemonset, err := helper.Apiclient.DaemonSets(parameters.PromNamespace).Get(
		context.Background(),
		ran.ProcessExporterPodName, metav1.GetOptions{},
	)
	if err != nil {
		err = ApplyObjects(helper.Config.Ran.ProcessExporterConfigsDir)
	} else {
		err = UpdateObjects(helper.Config.Ran.ProcessExporterConfigsDir)
	}

	Expect(err).ShouldNot(HaveOccurred())
	Eventually(func() error {
		daemonset, err = helper.Apiclient.DaemonSets(parameters.PromNamespace).Get(
			context.Background(),
			ran.ProcessExporterPodName,
			metav1.GetOptions{},
		)
		if err != nil {
			log.Println("Failed to retrieve status for daemonset: ", daemonset.Name)

			return err
		}
		if daemonset.Spec.Template.Spec.Containers[0].Image != helper.Config.Ran.ProcessExporterImage {
			// Update dummy helper.Config.Ran.ProcessExporterImage to configured value
			daemonset.Spec.Template.Spec.Containers[0].Image = helper.Config.Ran.ProcessExporterImage
			_, _ = helper.Apiclient.DaemonSets(parameters.PromNamespace).Update(
				context.Background(),
				daemonset,
				metav1.UpdateOptions{},
			)

			return fmt.Errorf("image updated, retrieve updated daemonset in next round")
		}
		if daemonset.Status.NumberReady == daemonset.Status.DesiredNumberScheduled {
			log.Printf("All pods are ready in daemonset: %s\n", daemonset.Name)

			return nil
		}

		return fmt.Errorf("not all daemon pods are ready in daemonset: %s", daemonset.Name)
	}, 5*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

	return daemonset
}
