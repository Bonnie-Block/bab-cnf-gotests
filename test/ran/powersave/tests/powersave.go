package tests

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strings"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranpower/ranpowerparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/workloadpartitioning/ranwphelper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	performancev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"github.com/openshift/cluster-node-tuning-operator/pkg/performanceprofile/controller/performanceprofile/components"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/cpu/rancpuhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranpower/ranpowerhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
	"k8s.io/utils/pointer"
)

var _ = Describe("Per-Core Runtime Tuning of power states - CRI-O", Ordered, func() {
	var (
		nodeList                     *corev1.NodeList
		snoNode                      corev1.Node
		err                          error
		isSNO                        bool
		perfProfile                  *performancev2.PerformanceProfile
		workloadHints                *performancev2.WorkloadHints
		output                       string
		originPerformanceProfileSpec performancev2.PerformanceProfileSpec
	)

	BeforeAll(func() {

		// Get nodes for connection host
		nodeList, err = helper.Apiclient.Nodes().List(context.Background(), metav1.ListOptions{})
		Expect(err).ToNot(HaveOccurred())
		isSNO, _ = nodes.IsSingleNodeCluster(helper.Apiclient)
		Expect(isSNO).Should(BeTrue(), "Currently only SNO nodes are supported by this test")
		perfProfile, err = rancpuhelper.GetPerformanceProfileWithCPUSet(nil)
		Expect(err).ToNot(HaveOccurred())
		snoNode = nodeList.Items[0]
		originPerformanceProfileSpec = perfProfile.Spec
		Expect(err).ToNot(HaveOccurred())
	})

	AfterAll(func() {
		// Restore performance profile to original spec after each test
		log.Println("Restore performance profile to original specs")
		err = updatePerformanceProfileSpecs(perfProfile, originPerformanceProfileSpec)
		Expect(err).ToNot(HaveOccurred())
		err = ranhelper.WaitForSnoRebootAndMcpUpdated(&snoNode)
	})

	// OCP-54571 - Install SNO node with standard DU profile that does not include WorkloadHints
	It("verifies expected kernel parameters with no workload hints specified in PerformanceProfile", func() {

		// Verify no workload hints in performanceprofile
		workloadHints = perfProfile.Spec.WorkloadHints
		// By(fmt.Sprintf("DEBUG WorkloadHints = %v\n\n%+v", workloadHints, workloadHints))
		if workloadHints != nil {
			Skip("WorkloadHints already present in perfProfile.Spec")
		}

		By("Checking for expected kernel parameters")

		// Expected default set of kernel parameters when no WorkloadHints are specified in PerformanceProfile
		requiredKernelParms := []string{
			"nohz_full=[0-9,-]+",
			"tsc=nowatchdog",
			"nosoftlockup",
			"nmi_watchdog=0",
			"mce=off",
			"skew_tick=1",
			"intel_pstate=disable",
		}
		output, err = helper.ExecCommandOnNodeWithHostBinaries(&snoNode, []string{"cat", "/proc/cmdline"})
		Expect(err).ToNot(HaveOccurred(), "Unable to cat /proc/cmdline")
		// By(fmt.Sprintf("DEBUG output=%s", output))
		for _, parameter := range requiredKernelParms {
			By(fmt.Sprintf("Checking /proc/cmdline for %s", parameter))
			rePattern := regexp.MustCompile(parameter)
			Expect(rePattern.FindStringIndex(output)).
				ToNot(BeNil(), fmt.Sprintf("Kernel parameter %s is missing from cmdline", parameter))
		}

	})

	// OCP-54572 - Enable powersave at node level and then enable performance at node level
	It("Enable powersave at node level and then enable performance at node level", func() {
		By("Patching the performance profile with the workload hints")
		err := setPowerMode(perfProfile, snoNode.Name, true, false, true)
		Expect(err).ToNot(HaveOccurred(), "Unable to set power mode")

		cmdline, err := helper.ExecCommandOnNodeWithHostBinaries(&snoNode, []string{"cat", "/proc/cmdline"})
		Expect(err).ToNot(HaveOccurred(), "Unable to cat /proc/cmdline")
		Expect(cmdline).To(ContainSubstring("intel_pstate=passive"))
		Expect(cmdline).ToNot(ContainSubstring("intel_pstate=disable"))
	})

	// OCP-54573 - Enable per pod powersave and start high performance pods and powersave pods
	PIt("Enable per pod powersave and start high performance pods and powersave pods", func() {

	})

	// OCP-54574 - Telco_Case: Enable powersave at node level and then enable high performance
	// at node level, check power consumption with no workload pods.
	It("Enable powersave, and then enable high performance at node level, "+
		"check power consumption with no workload pods.", func() {

		testPodAnnotations := map[string]string{
			"cpu-load-balancing.crio.io": "disable",
			"cpu-quota.crio.io":          "disable",
			"irq-load-balancing.crio.io": "disable",
			"cpu-c-states.crio.io":       "disable",
			"cpu-freq-governor.crio.io":  "performance",
		}

		cpuLimit := resource.MustParse("2")
		memLimit := resource.MustParse("100Mi")

		By("Patching the performance profile with the workload hints")
		err := setPowerMode(perfProfile, snoNode.Name, true, false, true)
		Expect(err).ToNot(HaveOccurred(), "Unable to set power mode")

		By("Define test pod")
		testpod := ranwphelper.DefineQoSTestPod(snoNode.Name, parameters.PrivPodNamespace, cpuLimit.String(),
			cpuLimit.String(), memLimit.String(), memLimit.String())
		testpod.Annotations = testPodAnnotations
		runtimeClass := components.GetComponentName(perfProfile.Name, components.ComponentNamePrefix)
		testpod.Spec.RuntimeClassName = &runtimeClass

		By("Create test pod")
		testpod = helper.WaitUntilPodCreatedAndRunning(testpod, 10*time.Minute)
		Expect(testpod.Status.QOSClass).To(Equal(corev1.PodQOSGuaranteed), "Test pod does not have QoS class of Guaranteed")

		defer func() {
			// delete the pod only if the tc had failed and the pod still exists
			By("Delete pod in case of a failure")
			podExists, err := helper.Apiclient.Pods(testpod.Namespace).Get(context.Background(),
				testpod.Name, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			if podExists != nil {
				err = pod.DeletePodAndWait(helper.Apiclient, testpod)
				Expect(err).ToNot(HaveOccurred())
			}
		}()

		output, err := pod.ExecCommand(helper.Apiclient, *testpod,
			[]string{"cat", "/sys/fs/cgroup/cpuset/cpuset.cpus"}, "test")
		Expect(err).ToNot(HaveOccurred())

		By("Verify powersetting of cpus used by the pod")
		trimmedOutput := strings.Trim(output.String(), "\r\n")
		cpusUsed, err := cpuset.Parse(trimmedOutput)
		Expect(err).ToNot(HaveOccurred())

		targetCpus := cpusUsed.ToSlice()
		err = checkCPUGovernorsAndResumeLatency(targetCpus, &snoNode, "n/a", "performance")
		Expect(err).ToNot(HaveOccurred())

		By("Verify the rest of cpus have default power setting")
		allCpus := snoNode.Status.Capacity.Cpu()
		cpus := cpuset.MustParse(fmt.Sprintf("0-%d", allCpus.Value()-1))

		otherCPUs := cpus.Difference(cpusUsed)
		// Verify cpus not assigned to the pod have default power settings.
		err = checkCPUGovernorsAndResumeLatency(otherCPUs.ToSlice(), &snoNode, "0", "performance")
		Expect(err).ToNot(HaveOccurred())

		By("Delete the pod")
		err = pod.DeletePodAndWait(helper.Apiclient, testpod)
		Expect(err).ToNot(HaveOccurred())

		By("Verify after pod was deleted cpus assigned to container have default powersave settings")
		err = checkCPUGovernorsAndResumeLatency(targetCpus, &snoNode, "0", "performance")
		Expect(err).ToNot(HaveOccurred())

	})

	Context("Collect power usage metrics", func() {

		When("ipmitool exists", func() {

			var (
				samplingInterval time.Duration
				powerState       string
			)

			BeforeAll(func() {
				if !ranhelper.IsIpmitoolExist() {
					Skip("ipmitool is not installed on test executor. Skip retrieving power metrics.")
				}
			})

			BeforeEach(func() {
				metricSamplingInterval := rancpuhelper.GetEnv(ran.EnvMetricSamplingInterval,
					ranpowerparameters.DefaultRanMetricSamplingInterval)
				samplingInterval, err = time.ParseDuration(metricSamplingInterval)
				Expect(err).ToNot(HaveOccurred())

				// Determine power state to be used as a tag for the metric
				powerState, err = ranpowerhelper.GetPowerState(perfProfile)
				Expect(err).ToNot(HaveOccurred())
			})

			It("Check power usage for 'noworkload' scenario", func() {
				noWorkloadDuration := rancpuhelper.GetEnv(ran.EnvNoWorkloadDuration,
					ranpowerparameters.DefaultRanNoWorkloadDuration)
				duration, err := time.ParseDuration(noWorkloadDuration)
				Expect(err).ToNot(HaveOccurred())
				compMap, err := ranpowerhelper.CollectPowerMetricsWithNoWorkload(duration, samplingInterval, powerState)
				Expect(err).ToNot(HaveOccurred())
				// Persist power usage metric to ginkgo report for further processing in pipeline.
				for metricName, metricValue := range compMap {
					_, err := fmt.Fprintf(GinkgoWriter, "%s: %s\n", metricName, metricValue)
					Expect(err).ToNot(HaveOccurred())
				}
			})

			It("Check power usage for 'steadyworkload' scenario", func() {
				workloadDuration := rancpuhelper.GetEnv(ran.EnvWorkloadDuration,
					ranpowerparameters.DefaultRanSteadyWorkloadDuration)
				duration, err := time.ParseDuration(workloadDuration)
				Expect(err).ToNot(HaveOccurred())
				compMap, err := ranpowerhelper.CollectPowerMetricsWithSteadyWorkload(duration, samplingInterval,
					powerState, perfProfile, &snoNode)
				Expect(err).ToNot(HaveOccurred())
				// Persist power usage metric to ginkgo report for further processing in pipeline.
				for metricName, metricValue := range compMap {
					_, err := fmt.Fprintf(GinkgoWriter, "%s: %s\n", metricName, metricValue)
					Expect(err).ToNot(HaveOccurred())
				}
			})
		})
	})
})

// setPowerMode updates the performance profile with the given workload hints, and waits for the mcp update.
func setPowerMode(perfProfile *performancev2.PerformanceProfile,
	nodeName string, perPodPowerManagement, highPowerConsumption, realTime bool) error {
	log.Println("Set powersave mode on performance profile")

	perfProfile.Spec.WorkloadHints = &performancev2.WorkloadHints{
		PerPodPowerManagement: pointer.Bool(perPodPowerManagement),
		HighPowerConsumption:  pointer.Bool(highPowerConsumption),
		RealTime:              pointer.Bool(realTime),
	}

	err := helper.Apiclient.Update(context.TODO(), perfProfile)
	if err != nil {
		return err
	}

	node, err := ranhelper.GetNodeByName(nodeName)
	if err != nil {
		return err
	}

	err = ranhelper.WaitForSnoRebootAndMcpUpdated(node)

	return err
}

// updatePerformanceProfileSpecs updates given performance profiles with a given spec.
func updatePerformanceProfileSpecs(perfProfile *performancev2.PerformanceProfile,
	newSpec performancev2.PerformanceProfileSpec) error {
	if reflect.DeepEqual(perfProfile.Spec, newSpec) {
		log.Println("No change was made to the performance profile")

		return nil
	}

	perfProfile.Spec = newSpec

	return helper.Apiclient.Update(context.TODO(), perfProfile)
}

// checkCPUGovernorsAndResumeLatency  Checks power and latency settings of the cpus.
func checkCPUGovernorsAndResumeLatency(cpus []int, targetNode *corev1.Node, pmQos string, governor string) error {
	for _, cpu := range cpus {
		cmd := []string{"/bin/bash", "-c", fmt.Sprintf(
			"cat /sys/devices/system/cpu/cpu%d/power/pm_qos_resume_latency_us", cpu)}

		output, err := helper.ExecCommandOnNode(targetNode, cmd)
		if err != nil {
			return err
		}

		Expect(strings.Trim(output, "\r")).To(Equal(pmQos))

		cmd = []string{"/bin/bash", "-c", fmt.Sprintf(
			"cat /sys/devices/system/cpu/cpu%d/cpufreq/scaling_governor", cpu)}

		output, err = helper.ExecCommandOnNode(targetNode, cmd)
		if err != nil {
			return err
		}

		Expect(strings.Trim(output, "\r")).To(Equal(governor))
	}

	return nil
}
