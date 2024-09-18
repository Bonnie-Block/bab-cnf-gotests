package ran

const (
	EnvWorkloadDuration       = "RAN_WORKLOAD_DURATION"
	EnvNoWorkloadDuration     = "RAN_NO_WORKLOAD_DURATION"
	EnvMetricSamplingInterval = "RAN_METRIC_SAMPLING_INTERVAL"
	Baseline                  = "BASELINE"
	BaselineVersion           = "BASELINE_VERSION"
	TrendTimeframe            = "TREND_TIMEFRAME"
	TrendThreshold            = "TREND_THRESHOLD"
	TrendAPIThreshold         = "TREND_API_THRESHOLD"
	MillicoreThreshold        = "MILLICORE_THRESHOLD"
	PrometheusURL             = "PROMETHEUS_URL"
)

const (
	// NamespaceTesting contains the name of the testing namespace.
	NamespaceTesting  = "ran-test"
	NamespaceFec      = "vran-acceleration-operators"
	NamespaceBmer     = "openshift-bare-metal-events"
	NamespaceAmq      = "amq-router"
	NamespaceNetdiag  = "openshift-network-diagnostics"
	NamespaceConsole  = "openshift-console"
	NamespaceWorkload = "workload"
)

const (
	ProcessExporterPodName = "process-exporter"
	PromPodName            = "prometheus-k8s-0"
)

const (
	// SnoMgmtCoreLimit Number of cores used for management. Rest should be reserved for customer work load.
	// If system is hyperthreaded, then mgmt cpus would be core count * 2.
	SnoMgmtCoreLimit       = 2
	ThreadSiblingsListPath = "/sys/devices/system/cpu/cpu%v/topology/thread_siblings_list"
)
