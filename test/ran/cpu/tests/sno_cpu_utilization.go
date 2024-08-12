package tests

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	performancev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/cpu/rancpuhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/cpu/rancpuparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
)

const (
	cpuOverheadStat       = rancpuparameters.CPUOverheadStat
	cpuInfraPodsStat      = rancpuparameters.CPUInfraPodsStat
	testCountWithWorkload = 6
	idleDuration          = 10 * time.Minute
)

var _ = Describe("SNO core reduction", func() {
	var (
		node         *corev1.Node
		isSNO        bool
		mgmtCPULimit int
		perfProfile  *performancev2.PerformanceProfile
		mgmtCPUSet   cpuset.CPUSet
	)

	execute.BeforeAll(func() {
		isSNO, _ = nodes.IsSingleNodeCluster(helper.Apiclient)
		perfProfile, _ = rancpuhelper.GetPerformanceProfileWithCPUSet(nil)
		// Get node for testing
		var err error
		node, err = ranhelper.GetWorker(true)
		Expect(err).ToNot(HaveOccurred())

		// Print out kernel version with best effort
		output, err := helper.ExecCommandOnNode(node, []string{"uname", "-r"})
		if err == nil {
			log.Println("Kernel version: " + output)
		}
	})

	BeforeEach(func() {
		if !isSNO {
			Skip("Test is only applicable to Single Node cluster")
		}
		if perfProfile == nil {
			Skip("No performance profile with reserved and isolated cpu set configuration found on cluster")
		}
		Expect(node).ToNot(BeNil())
		mgmtCPUSet = cpuset.MustParse(string(*perfProfile.Spec.CPU.Reserved))
		mgmtCPULimit = mgmtCPUSet.Size()
	})

	Context("Reserved CPUs configured in RT performance profile", func() {
		It(fmt.Sprintf("should be limited to %d cores", ran.SnoMgmtCoreLimit), func() {
			// Management CPU list is determined by product requirement on mgmt core count * thread count
			siblingList, err := rancpuhelper.GetThreadSiblingsList(0, node)
			Expect(err).ToNot(HaveOccurred())
			exptMgmtCPULimit := ran.SnoMgmtCoreLimit * len(siblingList)
			Expect(mgmtCPULimit).To(Equal(exptMgmtCPULimit),
				"Check Reserved Cpus in RT performance profile configures mgmt cpus as per core reduction requirement")
		})
	})

	Context("Management CPU utilization in idle state", func() {
		It(fmt.Sprintf("should use less than %d core(s)", ran.SnoMgmtCoreLimit), func() {
			log.Printf("Wait for %s in idle...\n", idleDuration.String())
			time.Sleep(idleDuration)
			// This is the timestamp to send the query. If the time difference is more than 1s,
			// then use offset to ensure the same duration was verified.
			endTime := time.Now().UTC()

			checkCPUUsage(idleDuration, endTime, mgmtCPULimit, "idle")
		})
	})

	baseline, _ := strconv.ParseBool(rancpuhelper.GetEnv(ran.Baseline, "false"))
	baselineVersion := os.Getenv(ran.BaselineVersion)
	trendTimeframe, _ := strconv.Atoi(rancpuhelper.GetEnv(ran.TrendTimeframe, "4"))
	trendThreshold, _ := strconv.ParseFloat(rancpuhelper.GetEnv(ran.TrendThreshold, "50"), 64)
	log.Println("Baseline run:", baseline, "Baseline version:", baselineVersion,
		"Trend Timeframe:", trendTimeframe, "Trend Threshold:", trendThreshold)

	Context("Kube API Server utilization trend", func() {
		It("grouped by resource,verb,namespace", func() {
			endTime := time.Now().UTC()
			checkAPIUsage(idleDuration, endTime, "idle", !baseline, trendTimeframe)

		})
	})

	Context("Management CPU utilization with workload pods running", func() {
		var (
			// workloadPods      []*corev1.Pod
			testExecCount     = 0
			workloadStartTime time.Time
			steadyEndTime     time.Time
			workloadDir       string
		)

		BeforeEach(func() {
			// Deploy workloads using kube-burner under workload namespace
			if testExecCount == 0 {
				workloadStartTime = time.Now().UTC()
				time.Sleep(20 * time.Second)

				workloadDir, _ = helper.GitCloneToTemp(helper.Config.Ran.WorkloadRepository, helper.Config.Ran.WorkloadVersion)
				log.Println("Setting registry to", helper.Config.Ran.WorkloadRegistry)
				os.Setenv("REGISTRY", helper.Config.Ran.WorkloadRegistry)
				_, err := rancpuhelper.ExecKubeBurnerTemplate(workloadDir,
					helper.Config.Ran.WorkloadCreate)
				if err != nil {
					log.Println("Failed to deploy workload templates")
					_, err := rancpuhelper.ExecKubeBurnerTemplate(workloadDir, helper.Config.Ran.WorkloadRemove)
					if err == nil {
						log.Println("Retry after stale workload purge")
						_, err := rancpuhelper.ExecKubeBurnerTemplate(workloadDir,
							helper.Config.Ran.WorkloadCreate)
						if err != nil {
							log.Fatal("Failed to deploy workload templates from ",
								helper.Config.Ran.WorkloadRepository)
						}
					} else {
						log.Fatal("Test cannot continue due to residual workload artifacts")
					}
				}
				log.Println("\nKube-burner workload deployed with templates from", helper.Config.Ran.WorkloadRepository)

			}
			testExecCount++
		})

		AfterEach(func() {
			// Delete workload pods after last test under workload context is completed.
			// In case less than testCountWithWorkload tests are executed, cleanup will still be done at suite level.
			log.Println("Test Exec Count:", testExecCount, testCountWithWorkload)
			if testExecCount >= testCountWithWorkload {
				// Run Kube-burner delete job and wait for deletion.
				log.Println("Removing Kube-burner workload artifacts")
				_, err := rancpuhelper.ExecKubeBurnerTemplate(workloadDir, helper.Config.Ran.WorkloadRemove)
				if err != nil {
					log.Println("Some Kube-burner workload artifacts might not be removed")
				}
			}
		})

		// 40809
		It(fmt.Sprintf("should use less than %d "+
			"core(s) during and post pod launch", ran.SnoMgmtCoreLimit), polarion.ID("40809"), func() {
			postLaunchDuration := 15 * time.Minute
			log.Printf("Wait for %s with workload pods running...\n", postLaunchDuration.String())
			time.Sleep(postLaunchDuration)
			duration := time.Since(workloadStartTime)
			// This is the timestamp to send the query. If the time difference is more than 1s,
			// then use offset to ensure the same duration was verified.
			endTime := time.Now().UTC()
			checkCPUUsage(duration, endTime, mgmtCPULimit, "workloadlaunch")
		})

		Context("with must-gather running", func() {
			BeforeEach(func() {
				if !rancpuhelper.IsOcExist() {
					Skip("oc does not exist. Cannot run must-gather command.")
				}
			})

			// 40816
			It(fmt.Sprintf("should use less than %d core(s)", ran.SnoMgmtCoreLimit), polarion.ID("40816"), func() {
				startTime := time.Now().UTC()
				// Sleep for 10s to account for the time between each query.
				time.Sleep(10 * time.Second)
				mustGatherExecDir, _, err := rancpuhelper.RunMustGather()
				defer func() {
					_ = rancpuhelper.DeleteMustGathers(mustGatherExecDir)
				}()

				Expect(err).ToNot(HaveOccurred())
				// Sleep for 31 seconds to ensure prometheus does not miss the last 1-30 seconds of must-gather run.
				time.Sleep(31 * time.Second)
				// This is the timestamp to send the query. If the time difference is more than 1s,
				// then use offset to ensure the same duration was verified.
				endTime := time.Now().UTC()
				checkCPUUsage(time.Since(startTime), endTime, mgmtCPULimit, "mustgather")
			})
		})

		Context("with Prometheus queries running", func() {
			// 40815
			It(fmt.Sprintf("should use less than %d core(s)", ran.SnoMgmtCoreLimit), polarion.ID("40815"), func() {
				duration := 10 * time.Minute
				startTime, _ := repeatPromQuery(duration)
				time.Sleep(31 * time.Second)
				endTime := time.Now().UTC()
				checkCPUUsage(time.Since(startTime), endTime, mgmtCPULimit, "promquery")
			})
		})

		workloadDuration := rancpuhelper.GetEnv(ran.EnvWorkloadDuration, "8h")
		Context(fmt.Sprintf("with workload running for %s", workloadDuration), func() {
			// 40810
			It(fmt.Sprintf("should use less than %d core(s)", ran.SnoMgmtCoreLimit), polarion.ID("40810"), func() {
				duration, err := time.ParseDuration(workloadDuration)
				Expect(err).ToNot(HaveOccurred())
				log.Printf("Wait for %s with workload pods running...\n", duration.String())
				time.Sleep(duration)
				endTime := time.Now().UTC()
				steadyEndTime = endTime
				checkCPUUsage(duration, endTime, mgmtCPULimit, "steadyworkload")
			})
		})

		millicoreThreshold, _ := strconv.ParseFloat(rancpuhelper.GetEnv(ran.MillicoreThreshold, "10"), 64)
		log.Println("Minimum Millicore usage Threshold:", millicoreThreshold)
		Context("with Trend Test for Infra Pods CPU Usage", func() {
			// Polarion ID??
			It(fmt.Sprintf("should not deviate from trend by %f", trendThreshold), func() {
				duration, err := time.ParseDuration(workloadDuration)
				Expect(err).ToNot(HaveOccurred())
				if !baseline {
					checkCPUTrend(duration, steadyEndTime, workloadDuration, trendTimeframe,
						trendThreshold, millicoreThreshold, baselineVersion, "cputrendpod")
				} else {
					log.Println("Skipping podtrend check on baseline run")
				}

			})
		})

		Context("with Trend Test for Os Daemon CPU Usage", func() {
			// Polarion ID??
			It(fmt.Sprintf("should not deviate from trend by %f", trendThreshold), func() {
				duration, err := time.ParseDuration(workloadDuration)
				Expect(err).ToNot(HaveOccurred())
				if !baseline {
					checkCPUTrend(duration, steadyEndTime, workloadDuration, trendTimeframe,
						trendThreshold, millicoreThreshold, baselineVersion, "cputrendos")
				} else {
					log.Println("Skipping ostrend check on baseline run")
				}

			})
		})

	})
})

// Check mgmt cpu utilization is limited to given number of vCPUs for the last given duration.
// Fail test and print top 5 consumers if exceeded given vCPU limit.
func checkCPUUsage(duration time.Duration, endTime time.Time, mgmtCPULimit int, scenario string) {
	timestamp := endTime

	// Ensure it can be converted to string without floating number in seconds
	duration = time.Duration(int(duration.Seconds())) * time.Second
	query := fmt.Sprintf("max_over_time((sum(%s)+sum(%s))[%s:30s])", cpuOverheadStat, cpuInfraPodsStat,
		duration.String())
	totalResult, err := rancpuhelper.ExecPromQuery(query, true)

	log.Println("Query max over time total mgmt cpu usage")
	Expect(err).ShouldNot(HaveOccurred())
	resTotal, err := strconv.ParseFloat(reflect.ValueOf(totalResult[0].Value[1]).String(), 64)
	Expect(err).ToNot(HaveOccurred())
	log.Println("Query max over time cpu usage for OS daemon")

	query = fmt.Sprintf("max_over_time(sum(%s)[%s:30s]%s)", cpuOverheadStat, duration.String(), getOffset(timestamp))
	nonPodResult, err := rancpuhelper.ExecPromQuery(query, true)
	Expect(err).ShouldNot(HaveOccurred())

	nonPodCPUUsage := nonPodResult[0].Value[1]
	resOS, err := strconv.ParseFloat(reflect.ValueOf(nonPodCPUUsage).String(), 64)
	Expect(err).ToNot(HaveOccurred())
	log.Println("Query max over time cpu usage for infra pods")

	query = fmt.Sprintf("max_over_time(sum(%s)[%s:30s]%s)", cpuInfraPodsStat, duration.String(), getOffset(timestamp))
	podResult, err := rancpuhelper.ExecPromQuery(query, true)
	Expect(err).ShouldNot(HaveOccurred())

	podCPUUsage := podResult[0].Value[1]
	resPods, err := strconv.ParseFloat(reflect.ValueOf(podCPUUsage).String(), 64)
	Expect(err).ToNot(HaveOccurred())

	log.Printf("Mgmt CPU usage for the last %s - Non-pod mgmt overhead, mgmt pods, total: %.4f,%.4f,%.4f\n",
		duration.String(), resOS, resPods, resTotal)
	// Add cpu util values to ginkgo report for further processing in pipeline.
	fmt.Fprintf(GinkgoWriter, "%s_%s_%s: %.7F\n", rancpuparameters.RanCPUMetricTotal, scenario, "max", resTotal)
	fmt.Fprintf(GinkgoWriter, "%s_%s_%s: %.7F\n", rancpuparameters.RanCPUMetricOsDaemon, scenario, "max", resOS)
	fmt.Fprintf(GinkgoWriter, "%s_%s_%s: %.7F\n", rancpuparameters.RanCPUMetricInfraPods, scenario, "max", resPods)

	if scenario == "steadyworkload" {
		log.Println("Query avg over time cpu usage for each infra pod")

		query = fmt.Sprintf("avg_over_time(%s[%s:30s]%s)", cpuInfraPodsStat, duration.String(), getOffset(timestamp))
		podBreakdown, err := rancpuhelper.ExecPromQuery(query, true)
		Expect(err).ShouldNot(HaveOccurred())
		sortAndWriteToReport(rancpuparameters.RanCPUMetricInfraPods, podBreakdown, "avg", scenario)

		log.Println("Query avg over time cpu usage for each os daemon")

		query = fmt.Sprintf("avg_over_time(%s[%s:30s]%s)", cpuOverheadStat, duration.String(), getOffset(timestamp))
		osBreakdown, err := rancpuhelper.ExecPromQuery(query, true)
		Expect(err).ShouldNot(HaveOccurred())
		sortAndWriteToReport(rancpuparameters.RanCPUMetricOsDaemon, osBreakdown, "avg", scenario)
	}
	// Defer the check until top 5 consumers were printed in case of failure.
	defer Expect(resTotal).ToNot(BeNumerically(">", float64(mgmtCPULimit)))

	if resOS+resPods > float64(mgmtCPULimit) {
		log.Println("Query top 5 non-pod CPU consumers")

		query = fmt.Sprintf("topk(5, max_over_time(%s[%s:30s] offset %s))", cpuOverheadStat, duration.String(),
			(time.Duration(int(time.Since(timestamp).Seconds())) * time.Second).String())
		_, err = rancpuhelper.ExecPromQuery(query, true)
		Expect(err).ShouldNot(HaveOccurred())
		log.Println("Query top 5 pod CPU consumers")

		query = fmt.Sprintf("topk(5, max_over_time(%s[%s:30s] offset %s))", cpuInfraPodsStat, duration.String(),
			(time.Duration(int(time.Since(timestamp).Seconds())) * time.Second).String())
		_, err = rancpuhelper.ExecPromQuery(query, true)
		Expect(err).ShouldNot(HaveOccurred())
	}
}

func repeatPromQuery(duration time.Duration) (startTime time.Time, err error) {
	query := fmt.Sprintf("max_over_time((sum(%s)+sum(%s))[12h:30s])", cpuOverheadStat, cpuInfraPodsStat)
	log.Printf("Repeatedly run prom query for %s: %s\n", duration.String(), query)

	startTime = time.Now().UTC()

	for time.Since(startTime) < duration {
		_, err = rancpuhelper.ExecPromQuery(query, false)
	}

	return startTime, err
}

// getOffset returns offset string if it's more than 1s, otherwise returns empty string. The offset will be
// added to prom query.
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
				verb, ok := tag["verb"]

				if ok {
					component = fmt.Sprintf("%s %s %s", tag["namespace"], value, verb)
				}
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
func sortAndWriteToReport(
	metricName string, metricVals []rancpuparameters.PromMetric, metricType string, scenario string) {
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

// Check cpu utilization against last known baseline data for deviations.
// Currently does not fail test and only prints all violations.
func checkCPUTrend(duration time.Duration, endTime time.Time, workloadDuration string, trendTimeframe int,
	trendThreshold float64, millicoreThreshold float64, baselineVersion string, scenario string) {
	timestamp := endTime
	duration = time.Duration(int(duration.Seconds())) * time.Second

	log.Println("Trend test\nBaseline duration->", workloadDuration)

	// Function to compare between baseline and current breakdown.
	getBreakdown := func(breakdown []rancpuparameters.PromMetric, baseline map[string]map[string][]interface{},
		name string, pod string) string {
		var trendString = ""

		for _, metric := range breakdown {
			groupname := metric.Metric[name]

			// For infra pods, fetching each pod name from metric map for baseline[namespace][pod].
			// For os daemons, its expected to stay as a constant in baseline[groupname]['daemon'].
			if pod != "daemon" {
				pod = metric.Metric["pod"]
				// Using parsed pod name without randomly generated string.
				re := regexp.MustCompile(`-[a-f0-9]{8,10}-[a-z0-9]{5}\z`)
				pod = re.ReplaceAllString(pod, "-")
				re = regexp.MustCompile(`-[a-z0-9]{5}\z`)
				pod = re.ReplaceAllString(pod, "-")
			}

			if basePod, ok := baseline[groupname][pod]; ok {
				currentValue, _ := strconv.ParseFloat(metric.Value[1].(string), 64)
				historicValue, _ := strconv.ParseFloat(basePod[1].(string), 64)
				deviation := ((currentValue - historicValue) / historicValue) * 100

				if deviation > trendThreshold && (currentValue*1000) >= millicoreThreshold {
					log.Println("[Fail] Threshold deviated for ", groupname, "_", pod)
					log.Printf("%f %% devation for current: %f vs baseline: %f", deviation, currentValue, historicValue)
					trendString = trendString + groupname + " _ " + pod + "-> current: " +
						strconv.FormatFloat(currentValue, 'f', 7, 64) + " baseline: " + strconv.FormatFloat(historicValue, 'f', 7, 64) +
						" deviation: " + strconv.FormatFloat(deviation, 'f', 2, 64) + "%\n"
				} else {
					log.Println("[Pass] Atleast one threshold observed for ", groupname, "_", pod)
					log.Printf("%f %% devation for current: %f vs baseline: %f", deviation, currentValue, historicValue)
				}
			} else {
				log.Println("[Fail] unknown component not in baseline", pod)
			}
		}

		return trendString
	}

	switch test := scenario; test {
	case "cputrendpod":
		query := fmt.Sprintf("avg_over_time(%s[%s:30s]%s)", cpuInfraPodsStat, duration.String(), getOffset(timestamp))
		podBreakdown, _ := rancpuhelper.ExecPromQuery(query, true)

		log.Println("Infra Pods Trend Test")

		podBaseline, _ := rancpuhelper.GetCPUBaseline(rancpuparameters.PodTrendQuery, baselineVersion,
			workloadDuration, trendTimeframe, "namespace")

		var podTrendString = getBreakdown(podBreakdown, podBaseline, "namespace", "pod")

		log.Println("-----------------------------------------------------------------------------")
		log.Print("Infra Pods Trend Violations current vs baseline deviation\n", podTrendString)
		log.Println("-----------------------------------------------------------------------------")
	case "cputrendos":
		query := fmt.Sprintf("avg_over_time(%s[%s:30s]%s)", cpuOverheadStat, duration.String(), getOffset(timestamp))
		osBreakdown, _ := rancpuhelper.ExecPromQuery(query, true)

		log.Println("Os Daemons Trend Test")

		osBaseline, _ := rancpuhelper.GetCPUBaseline(rancpuparameters.OsTrendQuery, baselineVersion,
			workloadDuration, trendTimeframe, "groupname")

		var osTrendString = getBreakdown(osBreakdown, osBaseline, "groupname", "daemon")

		log.Println("-----------------------------------------------------------------------------")
		log.Print("Os Daemons Trend Violations current vs baseline deviation\n", osTrendString)
		log.Println("-----------------------------------------------------------------------------")
	}
}

func checkAPIUsage(duration time.Duration, startTime time.Time, scenario string, checkTrend bool, trendTimeframe int) {
	timestamp := startTime
	// ApiServer total over the duration.
	query := fmt.Sprintf(rancpuparameters.APITotalQuery, duration.String(), getOffset(timestamp))
	apiServerTotalBreakdown, err := rancpuhelper.ExecPromQuery(query, true)

	for _, metric := range apiServerTotalBreakdown {
		if metric.Metric["namespace"] == "default" {
			metric.Metric["pod"] = "default"
		}
	}

	if err != nil {
		log.Println(err)
	}

	log.Println(apiServerTotalBreakdown)
	sortAndWriteToReport(rancpuparameters.RanAPIServerTotal, apiServerTotalBreakdown, "max", scenario)

	if checkTrend {
		checkAPITrend(apiServerTotalBreakdown, rancpuparameters.APITotalTrendQuery, trendTimeframe)
	}
}

func checkAPITrend(apiResults []rancpuparameters.PromMetric, query string, trendTimeframe int) {
	log.Println("Baseline comparison for API Server")

	log.Println("CURRENT RESULTS ", apiResults)

	baseline := rancpuhelper.GetAPIBaseline(query, trendTimeframe)

	for _, metric := range apiResults {
		namespace := metric.Metric["namespace"]
		pod := metric.Metric["pod"]
		// Using parsed pod name without randomly generated string.
		re := regexp.MustCompile(`-[a-f0-9]{8,10}-[a-z0-9]{5}\z`)
		pod = re.ReplaceAllString(pod, "-")
		re = regexp.MustCompile(`-[a-z0-9]{5}\z`)
		pod = re.ReplaceAllString(pod, "-")
		verb := metric.Metric["verb"]

		if len(metric.Value) > 1 && len(baseline[namespace][pod][verb]) > 1 {
			currentValue, err1 := strconv.ParseFloat(metric.Value[1].(string), 64)
			historicValue, err2 := strconv.ParseFloat(baseline[namespace][pod][verb][1].(string), 64)

			if err1 == nil && err2 == nil {
				if currentValue > historicValue {
					log.Println("[Trend Violated]", namespace, pod, verb, "baseline", historicValue, "current", currentValue)
				} else {
					log.Println("[Trend Observed]", namespace, pod, verb, "baseline", historicValue, "current", currentValue)
				}
			} else {
				log.Println("[Parsing error]", namespace, pod, verb)
			}
		} else {
			log.Println("[Baseline unknown]", namespace, pod, verb)
		}
	}
}
