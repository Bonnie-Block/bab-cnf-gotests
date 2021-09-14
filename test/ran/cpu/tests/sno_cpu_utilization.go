package tests

import (
	"fmt"
	"log"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	performancev2 "github.com/openshift-kni/performance-addon-operators/api/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/cpu/rancpuhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/cpu/rancpuparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

const (
	// Prom query statistic representation for management cpu overhead
	cpuOverheadStat = "namedprocess_namegroup_cpu_rate{groupname!~\"conmon\"}"
	// Prom query statistic representation for infra pods. Assuming only oslat and stress-ng user pods are running
	cpuInfraPodsStat      = "pod:container_cpu_usage:sum{pod!~\"process-exp.*\",pod!~\"oslat.*\",pod!~\"stress.*\",pod!~\"ranpriv.*\"}"
	testCountWithWorkload = 4
)

var _ = Describe("SNO core reduction", func() {
	var (
		node         *corev1.Node
		isSNO        bool
		mgmtCpuLimit int
		rtProfile    *performancev2.PerformanceProfile
		mgmtCpuSet   cpuset.CPUSet
	)

	execute.BeforeAll(func() {
		isSNO, _ = nodes.IsSingleNodeCluster(helper.Apiclient)
		rtProfile, _ = rancpuhelper.GetRTPerformanceProfile()
		// Get node for testing
		workers, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
		Expect(err).ToNot(HaveOccurred())
		node = &workers[0]
	})

	BeforeEach(func() {
		if !isSNO {
			Skip("Test is only applicable to Single Node cluster")
		}
		if rtProfile == nil {
			Skip("No RT profile found on cluster")
		}
		Expect(node).ToNot(Equal(nil))
		mgmtCpuSet = cpuset.MustParse(string(*rtProfile.Spec.CPU.Reserved))
		mgmtCpuLimit = mgmtCpuSet.Size()
	})

	Context("Reserved CPUs configured in RT performance profile", func() {
		It(fmt.Sprintf("should be limited to %d cores", ran.SnoMgmtCoreLimit), func() {
			// Management CPU list is determined by product requirement on mgmt core count * thread count
			siblingList, err := rancpuhelper.GetThreadSiblingsList(0, node)
			Expect(err).ToNot(HaveOccurred())
			exptMgmtCpuLimit := ran.SnoMgmtCoreLimit * len(siblingList)
			Expect(mgmtCpuLimit).To(Equal(exptMgmtCpuLimit),
				"Check Reserved Cpus in RT performance profile configures mgmt cpus as per core reduction requirement")
		})
	})

	Context("Management CPU utilization in idle state", func() {
		It(fmt.Sprintf("should use less than %d core(s)", ran.SnoMgmtCoreLimit), func() {
			duration := 10 * time.Minute
			log.Printf("Wait for %s in idle...\n", duration.String())
			time.Sleep(duration)
			checkCpuUsage(duration, mgmtCpuLimit, "idle")
		})
	})

	Context("Management CPU utilization with workload pods running", func() {
		var (
			workloadPods []*corev1.Pod
			testExecCount     = 0
			workloadStartTime time.Time
		)

		BeforeEach(func() {
			// Create pod before first test under workload context is started.
			if testExecCount == 0 || workloadPods == nil || len(workloadPods) == 0 {
				workloadStartTime = time.Now().UTC()
				time.Sleep(20 * time.Second)
				// stressNg cpu count is roughly 1/3.5 of total isolated cores
				workloadPods = ranhelper.DeployWorkloadPods(rtProfile, node)
			} else {
				Expect(workloadPods).ToNot(BeEmpty())
			}
			testExecCount += 1
		})

		AfterEach(func() {
			// Delete workload pods after last test under workload context is completed.
			// In case less than testCountWithWorkload tests are executed, cleanup will still be done at suite level.
			if testExecCount >= testCountWithWorkload {
				// Delete oslat pod and wait for deletion completes
				if workloadPods != nil && len(workloadPods) > 0 {
					ranhelper.DeletePodsAndWaitForRemoval(workloadPods, 5*time.Minute)
				}
			}
		})

		// 40809
		It(fmt.Sprintf("should use less than %d core(s) during and post pod launch", ran.SnoMgmtCoreLimit), func() {
			postLaunchDuration := 15 * time.Minute
			log.Printf("Wait for %s with workload pods running...\n", postLaunchDuration.String())
			time.Sleep(postLaunchDuration)
			duration := time.Since(workloadStartTime)
			checkCpuUsage(duration, mgmtCpuLimit, "workloadlaunch")
		})

		Context("with must-gather running", func() {
			BeforeEach(func() {
				if !ranhelper.IsOcExist() {
					Skip("oc does not exist. Cannot run must-gather command.")
				}
			})

			// 40816
			It(fmt.Sprintf("should use less than %d core(s)", ran.SnoMgmtCoreLimit), func() {
				startTime := time.Now().UTC()
				// Sleep for 10s to account for the time between each query.
				time.Sleep(10 * time.Second)
				mustGatherExecDir, _, err := ranhelper.RunMustGather()
				defer ranhelper.DeleteMustGathers(mustGatherExecDir)

				Expect(err).ToNot(HaveOccurred())
				// Sleep for 31 seconds to ensure prometheus does not miss the last 1-30 seconds of must-gather run.
				time.Sleep(31 * time.Second)
				checkCpuUsage(time.Since(startTime), mgmtCpuLimit, "mustgather")
			})
		})

		Context("with Prometheus queries running", func() {
			// 40815
			It(fmt.Sprintf("should use less than %d core(s)", ran.SnoMgmtCoreLimit), func() {
				duration := 10 * time.Minute
				startTime, _ := repeatPromQuery(duration)
				time.Sleep(31 * time.Second)
				checkCpuUsage(time.Since(startTime), mgmtCpuLimit, "promquery")
			})
		})

		workloadDuration := ranhelper.GetEnv(ran.EnvWorkloadDuration, "8h")
		Context(fmt.Sprintf("with workload running for %s", workloadDuration), func() {
			// 40810
			It(fmt.Sprintf("should use less than %d core(s)", ran.SnoMgmtCoreLimit), func() {
				duration, err := time.ParseDuration(workloadDuration)
				Expect(err).ToNot(HaveOccurred())

				log.Printf("Wait for %s with workload pods running...\n", duration.String())
				time.Sleep(duration)
				checkCpuUsage(duration, mgmtCpuLimit, "steadyworkload")
			})
		})
	})
})

// Check mgmt cpu utilization is limited to given number of vCPUs for the last given duration.
// Fail test and print top 5 consumers if exceeded given vCPU limit.
func checkCpuUsage(duration time.Duration, mgmtCpuLimit int, scenario string) {
	// This is the timestamp to send the query. If the time difference is more than 1s, then use offset to ensure the
	// same duration was verified.
	timestamp := time.Now().UTC()

	// Ensure it can be converted to string without floating number in seconds
	duration = time.Duration(int(duration.Seconds())) * time.Second

	log.Println("Query max over time total mgmt cpu usage")
	query := fmt.Sprintf("max_over_time((sum(%s)+sum(%s))[%s:30s])", cpuOverheadStat, cpuInfraPodsStat,
		duration.String())
	totalResult, err := ranhelper.ExecPromQuery(query, true)
	Expect(err).ShouldNot(HaveOccurred())
	resTotal, err := strconv.ParseFloat(reflect.ValueOf(totalResult[0].Value[1]).String(), 64)
	Expect(err).ToNot(HaveOccurred())

	log.Println("Query max over time cpu usage for OS daemon")
	query = fmt.Sprintf("max_over_time(sum(%s)[%s:30s]%s)", cpuOverheadStat, duration.String(), getOffset(timestamp))
	nonPodResult, err := ranhelper.ExecPromQuery(query, true)
	Expect(err).ShouldNot(HaveOccurred())
	nonPodCpuUsage := nonPodResult[0].Value[1]
	resOS, err := strconv.ParseFloat(reflect.ValueOf(nonPodCpuUsage).String(), 64)
	Expect(err).ToNot(HaveOccurred())

	log.Println("Query max over time cpu usage for infra pods")
	query = fmt.Sprintf("max_over_time(sum(%s)[%s:30s]%s)", cpuInfraPodsStat, duration.String(), getOffset(timestamp))
	podResult, err := ranhelper.ExecPromQuery(query, true)
	Expect(err).ShouldNot(HaveOccurred())
	podCpuUsage := podResult[0].Value[1]
	resPods, err := strconv.ParseFloat(reflect.ValueOf(podCpuUsage).String(), 64)
	Expect(err).ToNot(HaveOccurred())

	log.Printf("Mgmt CPU usage for the last %s - Non-pod mgmt overhead, mgmt pods, total: %.4f,%.4f,%.4f\n",
		duration.String(), resOS, resPods, resTotal)
	// Add cpu util values to ginkgo report for further processing in pipeline.
	fmt.Fprintf(GinkgoWriter, "%s_%s_%s: %.7F\n", rancpuparameters.RanCpuMetricTotal, scenario, "max", resTotal)
	fmt.Fprintf(GinkgoWriter, "%s_%s_%s: %.7F\n", rancpuparameters.RanCpuMetricOsDaemon, scenario, "max", resOS)
	fmt.Fprintf(GinkgoWriter, "%s_%s_%s: %.7F\n", rancpuparameters.RanCpuMetricInfraPods, scenario, "max", resPods)

	if scenario == "steadyworkload" {
		log.Println("Query avg over time cpu usage for each infra pod")
		query = fmt.Sprintf("avg_over_time(%s[%s:30s]%s)", cpuInfraPodsStat, duration.String(), getOffset(timestamp))
		podBreakdown, err := ranhelper.ExecPromQuery(query, true)
		Expect(err).ShouldNot(HaveOccurred())
		sortAndWriteToReport(rancpuparameters.RanCpuMetricInfraPods, podBreakdown, "avg", scenario)

		log.Println("Query avg over time cpu usage for each os daemon")
		query = fmt.Sprintf("avg_over_time(%s[%s:30s]%s)", cpuOverheadStat, duration.String(), getOffset(timestamp))
		osBreakdown, err := ranhelper.ExecPromQuery(query, true)
		Expect(err).ShouldNot(HaveOccurred())
		sortAndWriteToReport(rancpuparameters.RanCpuMetricOsDaemon, osBreakdown, "avg", scenario)
	}

	// Defer the check until top 5 consumers were printed in case of failure.
	defer Expect(resTotal).ToNot(BeNumerically(">", float64(mgmtCpuLimit)))
	if resOS+resPods > float64(mgmtCpuLimit) {
		log.Println("Query top 5 non-pod CPU consumers")
		query = fmt.Sprintf("topk(5, max_over_time(%s[%s:30s] offset %s))", cpuOverheadStat, duration.String(),
			(time.Duration(int(time.Since(timestamp).Seconds())) * time.Second).String())
		_, err = ranhelper.ExecPromQuery(query, true)
		Expect(err).ShouldNot(HaveOccurred())

		log.Println("Query top 5 pod CPU consumers")
		query = fmt.Sprintf("topk(5, max_over_time(%s[%s:30s] offset %s))", cpuInfraPodsStat, duration.String(),
			(time.Duration(int(time.Since(timestamp).Seconds())) * time.Second).String())
		_, err = ranhelper.ExecPromQuery(query, true)
		Expect(err).ShouldNot(HaveOccurred())
	}
}

func repeatPromQuery(duration time.Duration) (startTime time.Time, err error) {
	query := fmt.Sprintf("max_over_time((sum(%s)+sum(%s))[12h:30s])", cpuOverheadStat, cpuInfraPodsStat)
	log.Printf("Repeatedly run prom query for %s: %s\n", duration.String(), query)
	startTime = time.Now().UTC()
	for time.Since(startTime) < duration {
		_, err = ranhelper.ExecPromQuery(query, false)
	}
	return startTime, err
}

// getOffset returns offset string if it's more than 1s, otherwise returns empty string. The offset will be added to prom query.
func getOffset(starTime time.Time) string {
	offset := " offset " + (time.Duration(int(time.Since(starTime).Seconds())) * time.Second).String()
	if offset == " offset 0s" {
		offset = ""
	}
	return offset
}

// parseTag parses a prom Metric map to a string in this format: key1="val1",key2="val2",...
func parseTag(tag map[string]string) (string, string) {
	tagString := ""
	component := ""
	for key, value := range tag {
		// Do not include node name (key=instance) in tag string
		if key != "instance" {
			if key == "pod" {
				// use <namespace>-<fullpodname> as component
				component = fmt.Sprintf("%s %s", tag["namespace"], value)
				// use parsed pod name as value without randomly generated string
				re := regexp.MustCompile(`-[a-f0-9]{8,10}-[a-z0-9]{5}\z`)
				value = re.ReplaceAllString(value, "-")
				re = regexp.MustCompile(`-[a-z0-9]{5}\z`)
				value = re.ReplaceAllString(value, "-")
			} else if key == "groupname" {
				component = value
			}
			tagString += fmt.Sprintf("%s=\"%s\",", key, value)
		}
	}
	tagString = strings.TrimRight(tagString, ",")
	return component, tagString
}

// sortAndWriteToReport sorts the metrics by podname or groupname, and write them to ginkgo report.
func sortAndWriteToReport(metricName string, metricVals []ranhelper.PromMetric, metricType string, scenario string) {
	compMap := make(map[string]string)
	var components []string
	for _, item := range metricVals {
		component, tag := parseTag(item.Metric)
		Expect(component).ToNot(Equal(""))
		resPod, err := strconv.ParseFloat(reflect.ValueOf(item.Value[1]).String(), 64)
		Expect(err).ToNot(HaveOccurred())
		metricString := fmt.Sprintf("%s_%s_%s(%s): %.7F", metricName, scenario, metricType, tag, resPod)
		compMap[component] = metricString
		components = append(components, component)
	}

	// Sort the metrics by component: "<namespace> <podname>" for mgmt pods or <groupname> for os daemon
	sort.Strings(components)
	for _, comp := range components {
		fmt.Fprintln(GinkgoWriter, compMap[comp])
	}
}
