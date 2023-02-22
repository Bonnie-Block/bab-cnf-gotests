package tests

import (
	"fmt"
	"log"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/machineconfigpool"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	performancev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	corev1 "k8s.io/api/core/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/cpu/rancpuhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/reboot/ranrebootparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

var _ = Describe("SNO Reboot", Ordered, func() {
	var (
		node         *corev1.Node
		perfProfile  *performancev2.PerformanceProfile
		workloadPods []*corev1.Pod
	)

	BeforeAll(func() {
		snoNodes, err := ranhelper.GetSnoNodes()
		Expect(err).ToNot(HaveOccurred())
		if len(snoNodes) == 0 {
			Skip("Test is only applicable to SNO")
		}

		perfProfile, _ = rancpuhelper.GetPerformanceProfileWithCPUSet(nil)
		if perfProfile == nil {
			Skip("No performance profile with reserved and isolated cpu set configuration found on cluster")
		}

		node = snoNodes[0]
		// Print out kernel version with best effort
		output, err := helper.ExecCommandOnNode(node, []string{"uname", "-r"})
		if err == nil {
			log.Println("Kernel version: " + output)
		}

		workloadPods = ranhelper.DeployWorkloadPods(perfProfile, node)
		Expect(workloadPods).NotTo(Equal(nil))
	})

	BeforeEach(func() {
		// Skip test if not all pods are healthy before reboot.
		unhealthyPods := helper.WaitForAllPodsHealthy(nil, 3*time.Minute, 5*time.Second, 0)
		if len(unhealthyPods) > 0 {
			Skip(fmt.Sprintln("Some pods are unhealthy before reboot: ", unhealthyPods))
		}
	})

	Context("soft reboot with workloads running", func() {
		// 40896
		It("cluster and workload pods should be recovered after reboot", func() {
			startTime := time.Now()
			// Trigger soft reboot and wait for cluster and workload pods to recover.
			helper.SoftRebootNodeAndWaitForDisconnect(node)
			// Wait for cluster recover and log soft reboot times to ginkgo report for further processing in pipeline.
			waitForClusterRecoverAndLogTime(startTime, node, ranrebootparameters.RanMetricSoftReboot)
		})
	})

	Context("power cycle with workloads running", func() {
		BeforeEach(func() {
			if !ranhelper.IsIpmitoolExist() {
				Skip("ipmitool is not installed on test executor. Skip power cycle test.")
			}
		})
		// 40814
		It("cluster and workload pods should be recovered after power comes back", func() {
			powerOnTime := ranhelper.PowerOffAndOnSno()
			waitForClusterRecoverAndLogTime(powerOnTime, node, ranrebootparameters.RanMetricPowerCycle)
		})
	})
})

// writeToGinkgoReport writes reboot recovery time to ginkgo report.
func writeToGinkgoReport(metric string, duration time.Duration) {
	content := fmt.Sprintf("%s: %d", metric, int(duration.Seconds()))
	_, err := fmt.Fprintln(GinkgoWriter, content)

	if err != nil {
		log.Println("Failed to write content to ginkgo report:", content)
	}
}

// waitForClusterRecoverAndLogTime waits for sno node to be pingable, ocp reachable, then all pods to recover
// Record the timing for each stage in ginkgo report.
func waitForClusterRecoverAndLogTime(rebootStartTime time.Time, node *corev1.Node, ranmetric string) {
	// Wait for linux to be reachable via ping and record time
	metricStartTime, metricCount := rebootStartTime, 1

	helper.WaitForNodeReachable(node)
	writeToGinkgoReport(fmt.Sprintf("%s_%d_node_reachable", ranmetric, metricCount), time.Since(metricStartTime))

	// Wait for openshift to be reachable and record time
	metricStartTime, metricCount = time.Now(), metricCount+1

	err := ranhelper.WaitForClusterReachable()
	Expect(err).ToNot(HaveOccurred())

	writeToGinkgoReport(fmt.Sprintf("%s_%d_cluster_reachable", ranmetric, metricCount), time.Since(metricStartTime))

	// Wait for workload pods to recover and record time
	metricStartTime, metricCount = time.Now(), metricCount+1
	interval := 5 * time.Second
	// check mcps are updated
	err = machineconfigpool.WaitForClusterStable(helper.Apiclient, 30*time.Minute, interval, 0)
	Expect(err).ToNot(HaveOccurred())
	// check all nodes are Ready
	err = nodes.WaitForNodesReady(helper.Apiclient, 10*time.Minute, interval)
	Expect(err).ToNot(HaveOccurred())
	// check all workload pods are recovered and stable
	workloadStableDuration := 30 * time.Second
	unhealthyWorkloadPods := helper.WaitForAllPodsHealthy(
		[]string{ran.NamespaceTesting},
		45*time.Minute,
		interval,
		workloadStableDuration,
	)
	Expect(unhealthyWorkloadPods).To(BeEmpty())
	writeToGinkgoReport(
		fmt.Sprintf("%s_%d_workload_recover", ranmetric, metricCount),
		time.Since(metricStartTime)-workloadStableDuration,
	)

	// Wait for all pods on cluster to recover and record time
	metricStartTime, metricCount = time.Now().Add(-workloadStableDuration), metricCount+1
	clusterStableDuration := 1 * time.Minute
	unhealthyPods := helper.WaitForAllPodsHealthy(
		nil, 30*time.Minute,
		interval, clusterStableDuration,
	)
	Expect(unhealthyPods).To(BeEmpty())
	writeToGinkgoReport(
		fmt.Sprintf("%s_%d_cluster_recover", ranmetric, metricCount),
		time.Since(metricStartTime)-clusterStableDuration,
	)
	writeToGinkgoReport(fmt.Sprintf("%s_total", ranmetric), time.Since(rebootStartTime)-clusterStableDuration)

	log.Println("Sleep for 5 minutes after reboot - quiet time")
	time.Sleep(5 * time.Minute)
}
