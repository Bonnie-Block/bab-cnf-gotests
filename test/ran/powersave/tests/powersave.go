package tests

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranpower/ranpowerparameters"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	performancev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/cpu/rancpuhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranpower/ranpowerhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Per-Core Runtime Tuning of power states - CRI-O", Ordered, func() {
	var (
		nodeList      *corev1.NodeList
		snoNode       corev1.Node
		err           error
		isSNO         bool
		perfProfile   *performancev2.PerformanceProfile
		workloadHints *performancev2.WorkloadHints
		output        string
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
		// SNOHost = snoNode.Name

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
	PIt("Enable powersave at node level and then enable performance at node level", func() {

	})
	// OCP-54573 - Enable per pod powersave and start high performance pods and powersave pods
	PIt("Enable per pod powersave and start high performance pods and powersave pods", func() {

	})
	// OCP-54574 - Telco_Case: Enable powersave at node level and then enable high performance
	// at node level, check power consumption with no workload pods.
	PIt("Enable powersave, and then enable high performance at node level, "+
		"check power consumption with no workload pods.", func() {
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
