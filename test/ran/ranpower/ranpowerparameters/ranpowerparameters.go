package ranpowerparameters

// RAN Power Measurement metric names/prefixes.
const (
	RanPowerMetricTotalSamples            = "ranmetrics_power_total_samples"
	RanPowerMetricSamplingIntervalSeconds = "ranmetrics_power_sampling_interval_seconds"
	RanPowerMetricMinInstantPower         = "ranmetrics_power_min_instantaneous"
	RanPowerMetricMaxInstantPower         = "ranmetrics_power_max_instantaneous"
	RanPowerMetricMeanInstantPower        = "ranmetrics_power_mean_instantaneous"
	RanPowerMetricStdDevInstantPower      = "ranmetrics_power_standard_deviation_instantaneous"
	RanPowerMetricMedianInstantPower      = "ranmetrics_power_median_instantaneous"
)

// Power State Configurations.
const (
	PowerSavingMode     = "powersaving"
	PerformanceMode     = "performance"
	HighPerformanceMode = "highperformance"
)

// Ipmitool power metrics.
const (
	IpmiDcmiPowerMinimumDuringSampling = "minPower"
	IpmiDcmiPowerMaximumDuringSampling = "maxPower"
	IpmiDcmiPowerAverageDuringSampling = "avgPower"
	IpmiDcmiPowerInstantaneous         = "instantaneousPower"
)

// Default sampling values.
const (
	DefaultRanMetricSamplingInterval = "30s"
	DefaultRanNoWorkloadDuration     = "5m"
	DefaultRanSteadyWorkloadDuration = "10m"
)
