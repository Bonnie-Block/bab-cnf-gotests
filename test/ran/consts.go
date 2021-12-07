package ran

const (
	EnvWorkloadDuration = "RAN_WORKLOAD_DURATION"
)

const (
	// NamespaceTesting contains the name of the testing namespace.
	NamespaceTesting = "ran-test"
)

const (
	ProcessExporterPodName = "process-exporter"
	PromNamespace          = "openshift-monitoring"
	PromPodName            = "prometheus-k8s-0"
	PromLocalURL           = "http://localhost:9090/api/v1/"
)

const (
	// SnoMgmtCoreLimit Number of cores used for management. Rest should be reserved for customer work load.
	// If system is hyperthreaded, then mgmt cpus would be core count * 2.
	SnoMgmtCoreLimit       = 2
	ThreadSiblingsListPath = "/sys/devices/system/cpu/cpu%v/topology/thread_siblings_list"
)
