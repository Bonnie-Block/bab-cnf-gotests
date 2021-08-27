package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	performancev2 "github.com/openshift-kni/performance-addon-operators/api/v2"
	corev1 "k8s.io/api/core/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/cpu/rancpuhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/reboot/ranreboothelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/reboot/ranrebootparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

var _ = Describe("SNO Reboot", func() {
	var (
		node         *corev1.Node
		isSNO        bool
		rtProfile    *performancev2.PerformanceProfile
		workloadPods []*corev1.Pod
	)

	execute.BeforeAll(func() {
		isSNO, _ = nodes.IsSingleNodeCluster(helper.Apiclient)
		rtProfile, _ = rancpuhelper.GetRTPerformanceProfile()
		// Get node for testing
		workers, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
		Expect(err).ToNot(HaveOccurred())
		node = &workers[0]
		workloadPods = ranhelper.DeployWorkloadPods(rtProfile, node)
	})

	BeforeEach(func() {
		if !isSNO {
			Skip("Test is only applicable to Single Node cluster")
		}
		if rtProfile == nil {
			Skip("No RT profile found on cluster")
		}
		Expect(node).ToNot(Equal(nil))
		Expect(workloadPods).NotTo(Equal(nil))

		// Skip test suite if not all pods are healthy before reboot.
		unhealthyPods := ranreboothelper.WaitForPodsHealthy(3 * time.Minute)
		if len(unhealthyPods) > 0 {
			Skip(fmt.Sprintln("Some pods are unhealthy before reboot: ", unhealthyPods))
		}
	})

	Context("soft reboot with workloads running", func() {
		// 40896
		It(fmt.Sprintf("cluster and workload pods should be recovered after reboot"), func() {
			startTime := time.Now()
			// Trigger soft reboot and wait for cluster and workload pods to recover.
			ranreboothelper.SoftRebootNodeAndWait(node)
			// Add soft reboot time to ginkgo report for further processing in pipeline.
			writeToGinkgoReport(ranrebootparameters.RanMetricSoftReboot, time.Since(startTime))
		})
	})

	Context("power cycle with workloads running", func() {
		// 40814
		It(fmt.Sprintf("cluster and workload pods should be recovered sno powered on"), func() {
			// TODO: power cycle test to be added.
			// Trigger power off and power on to simulate sno site power outage
			// Calculate total time from power on to full recovery.
			// Test fails if cluster or workload pods did not recover.
		})
	})
})

func writeToGinkgoReport(metric string, duration time.Duration) {
	fmt.Fprintf(GinkgoWriter, "%s: %d\n", metric, int(duration.Minutes()))
}
