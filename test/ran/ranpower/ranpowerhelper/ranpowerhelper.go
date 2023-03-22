package ranpowerhelper

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"sync"
	"time"

	performancev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranpower/ranpowerparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranutil/ranstats"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

// GetHostPowerUsage retrieve host power utilization metrics queried via ipmitool command.
func GetHostPowerUsage() (map[string]float64, error) {
	user, password, hosts := ranhelper.ParseBmcInfo(helper.Config)
	if len(hosts) > 1 {
		log.Printf("multiple hosts detected, only using %s\n", hosts[0])
	}

	subcommands := []string{"dcmi", "power", "reading"}
	args := []string{"-I", "lanplus", "-U", user, "-P", password, "-H", hosts[0]}
	args = append(args, subcommands...)
	output, err := helper.ExecAndLogCommand(true, 1*time.Minute, "ipmitool", args...)

	if err != nil {
		return nil, err
	}

	// Parse the ipmitool string output and return the result.
	return parseIpmiPowerOutput(string(output))
}

// parseIpmiPowerOutput parses the ipmitool host power usage and returns a map of corresponding float values.
func parseIpmiPowerOutput(result string) (map[string]float64, error) {
	powerMeasurements := make(map[string]float64)
	powerMeasurementExpr := make(map[string]string)

	powerMeasurementExpr[ranpowerparameters.IpmiDcmiPowerInstantaneous] =
		`Instantaneous power reading: (\s*[0-9]+) Watts`
	powerMeasurementExpr[ranpowerparameters.IpmiDcmiPowerMinimumDuringSampling] =
		`Minimum during sampling period: (\s*[0-9]+) Watts`
	powerMeasurementExpr[ranpowerparameters.IpmiDcmiPowerMaximumDuringSampling] =
		`Maximum during sampling period: (\s*[0-9]+) Watts`
	powerMeasurementExpr[ranpowerparameters.IpmiDcmiPowerAverageDuringSampling] =
		`Average power reading over sample period: (\s*[0-9]+) Watts`

	// Extract power measurements.
	for key, pattern := range powerMeasurementExpr {
		re := regexp.MustCompile(pattern)
		res := re.FindStringSubmatch(result)

		if len(res) > 0 {
			var value float64
			_, err := fmt.Sscan(res[1], &value)

			if err != nil {
				return nil, err
			}

			powerMeasurements[key] = value
		}
	}

	return powerMeasurements, nil
}

// GetPowerState determines the power state from the workloadHints object of the PerformanceProfile.
func GetPowerState(perfProfile *performancev2.PerformanceProfile) (powerState string, err error) {
	workloadHints := perfProfile.Spec.WorkloadHints

	if workloadHints == nil {
		// No workloadHints object -> default is Performance mode
		powerState = ranpowerparameters.PerformanceMode
	} else {
		realTime := *workloadHints.RealTime
		highPowerConsumption := *workloadHints.HighPowerConsumption
		perPodPowerManagement := *workloadHints.PerPodPowerManagement

		switch {
		case realTime && !highPowerConsumption && !perPodPowerManagement:
			powerState = ranpowerparameters.PerformanceMode
		case realTime && highPowerConsumption && !perPodPowerManagement:
			powerState = ranpowerparameters.HighPerformanceMode
		case realTime && !highPowerConsumption && perPodPowerManagement:
			powerState = ranpowerparameters.PowerSavingMode
		default:
			return "", errors.New("unknown workloadHints power state configuration")
		}

	}

	return powerState, nil
}

func CollectPowerUsageMetrics(duration, samplingInterval time.Duration, scenario,
	tag string) (map[string]string, error) {
	startTime := time.Now()
	expectedEndTime := startTime.Add(duration)
	powerMeasurements := make(map[time.Time]map[string]float64)

	stopSampling := func(t time.Time) bool {
		if t.After(startTime) && t.Before(expectedEndTime) {
			return false
		}

		return true
	}

	var sampleGroup sync.WaitGroup

	err := wait.PollImmediate(samplingInterval, duration, func() (bool, error) {
		sampleGroup.Add(1)
		timestamp := time.Now()
		go func(t time.Time) {
			defer sampleGroup.Done()
			out, err := GetHostPowerUsage()
			if err == nil {
				powerMeasurements[t] = out
			}
		}(timestamp)

		return stopSampling(timestamp.Add(samplingInterval)), nil
	})

	// Wait for all the tasks to complete.
	sampleGroup.Wait()

	if err != nil {
		return nil, err
	}

	log.Printf("Power usage test started: %v\nPower usage test ended: %v\n", startTime, time.Now())

	if len(powerMeasurements) < 1 {
		return nil, errors.New("no power usage metrics were retrieved")
	}

	// Compute power metrics.
	return computePowerUsageStatistics(powerMeasurements, samplingInterval, scenario, tag)
}

// computePowerUsageStatistics computes the power usage summary statistics.
func computePowerUsageStatistics(powerMeasurements map[time.Time]map[string]float64,
	samplingInterval time.Duration, scenario, tag string) (map[string]string, error) {
	/*
		Compute power measurement statistics

		Sample power measurement data:
		map[
		2023-03-08 10:17:46.629599 -0500 EST m=+132.341222733:map[avgPower:251 instantaneousPower:326 maxPower:503 minPower:8]
		2023-03-08 10:18:46.630737 -0500 EST m=+192.341245075:map[avgPower:251 instantaneousPower:324 maxPower:503 minPower:8]
		2023-03-08 10:19:46.563857 -0500 EST m=+252.341201729:map[avgPower:251 instantaneousPower:329 maxPower:503 minPower:8]
		2023-03-08 10:20:46.563313 -0500 EST m=+312.340308977:map[avgPower:251 instantaneousPower:332 maxPower:503 minPower:8]
		2023-03-08 10:21:46.564469 -0500 EST m=+372.341065314:map[avgPower:251 instantaneousPower:329 maxPower:503 minPower:8]
		]

		The following power measurement summary statistics are computed:

		numberSamples: count(powerMeasurements)
		samplingInterval: <samplingInterval>
		minInstantaneousPower: min(instantaneousPower)
		maxInstantaneousPower: max(instantaneousPower)
		meanInstantaneousPower: mean(instantaneousPower)
		stdDevInstantaneousPower: standard-deviation(instantaneousPower)
		medianInstantaneousPower: median(instantaneousPower)
	*/
	log.Printf("Power usage measurements for %s:\n%v\n", scenario, powerMeasurements)

	compMap := make(map[string]string)

	numberSamples := len(powerMeasurements)

	compMap[fmt.Sprintf("%s_%s_%s", ranpowerparameters.RanPowerMetricTotalSamples, scenario, tag)] =
		fmt.Sprintf("%d", numberSamples)
	compMap[fmt.Sprintf("%s_%s_%s", ranpowerparameters.RanPowerMetricSamplingIntervalSeconds, scenario, tag)] =
		fmt.Sprintf("%.0f", samplingInterval.Seconds())

	instantPowerData := make([]float64, numberSamples)

	index := -1
	for _, row := range powerMeasurements {
		index++

		instantPowerData[index] = row[ranpowerparameters.IpmiDcmiPowerInstantaneous]
	}

	minInstantaneousPower, err := ranstats.Min(instantPowerData)
	if err != nil {
		return compMap, err
	}

	compMap[fmt.Sprintf("%s_%s_%s", ranpowerparameters.RanPowerMetricMinInstantPower, scenario, tag)] =
		fmt.Sprintf("%.7f", minInstantaneousPower)

	maxInstantaneousPower, err := ranstats.Max(instantPowerData)
	if err != nil {
		return compMap, err
	}

	compMap[fmt.Sprintf("%s_%s_%s", ranpowerparameters.RanPowerMetricMaxInstantPower, scenario, tag)] =
		fmt.Sprintf("%.7f", maxInstantaneousPower)

	meanInstantaneousPower, err := ranstats.Mean(instantPowerData)
	if err != nil {
		return compMap, err
	}

	compMap[fmt.Sprintf("%s_%s_%s", ranpowerparameters.RanPowerMetricMeanInstantPower, scenario, tag)] =
		fmt.Sprintf("%.7f", meanInstantaneousPower)

	stdDevInstantaneousPower, err := ranstats.StdDev(instantPowerData)
	if err != nil {
		return compMap, err
	}

	compMap[fmt.Sprintf("%s_%s_%s", ranpowerparameters.RanPowerMetricStdDevInstantPower, scenario, tag)] =
		fmt.Sprintf("%.7f", stdDevInstantaneousPower)

	medianInstantaneousPower, err := ranstats.Median(instantPowerData)
	if err != nil {
		return compMap, err
	}

	compMap[fmt.Sprintf("%s_%s_%s", ranpowerparameters.RanPowerMetricMedianInstantPower, scenario, tag)] =
		fmt.Sprintf("%.7f", medianInstantaneousPower)

	return compMap, nil
}

func CollectPowerMetricsWithNoWorkload(duration, samplingInterval time.Duration,
	tag string) (map[string]string, error) {
	scenario := "noworkload"
	log.Printf("Wait for %s for %s scenario\n", duration.String(), scenario)

	return CollectPowerUsageMetrics(duration, samplingInterval, scenario, tag)
}

func CollectPowerMetricsWithSteadyWorkload(duration, samplingInterval time.Duration, tag string,
	perfProfile *performancev2.PerformanceProfile, snoNode *corev1.Node) (map[string]string, error) {
	scenario := "steadyworkload"
	// Create stress-ng workload pods.
	// Determine cpu requests for stress-ng pods.
	// stressNg cpu count is roughly 75% of total isolated cores.
	// 1 cpu will be used by other consumer pods, such as process-exporter, cnfgotestpriv.
	isolatedCPUSet := cpuset.MustParse(string(*perfProfile.Spec.CPU.Isolated))
	stressNgCPUCount := (isolatedCPUSet.Size() - 1) * 300 / 400
	stressngMaxPodCount := 50
	stressNgPods := ranhelper.DeployStressNgPods(stressNgCPUCount, stressngMaxPodCount, snoNode)

	if len(stressNgPods) < 1 {
		return nil, errors.New("not enough stress-ng pods to run test")
	}

	log.Printf("Wait for %s for %s scenario\n", duration.String(), scenario)
	result, err := CollectPowerUsageMetrics(duration, samplingInterval, scenario, tag)
	// Delete stress-ng pods.
	ranhelper.DeletePodsAndWaitForRemoval(stressNgPods, 5*time.Minute)

	return result, err
}
