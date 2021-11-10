package ran

const (
	EnvWorkloadDuration = "RAN_WORKLOAD_DURATION"
)

const (
	// NamespaceTesting contains the name of the testing namespace
	NamespaceTesting = "ran-test"
)

const (
	ProcessExporterPodName = "process-exporter"
	PromNamespace          = "openshift-monitoring"
	PromPodName            = "prometheus-k8s-0"
	PromContainer          = "prometheus"
	PromLocalUrl           = "http://localhost:9090/api/v1/"
)

const (
	// FilePathKubeletConfig contains the kubelet.conf file path
	FilePathKubeletConfig = "/etc/kubernetes/kubelet.conf"
)

const (
	// ContainerMachineConfigDaemon contains the name of the machine-config-daemon container
	ContainerMachineConfigDaemon = "machine-config-daemon"
)

const (
	// SnoMgmtCoreLimit Number of cores used for management. Rest should be reserved for customer work load.
	// If system is hyperthreaded, then mgmt cpus would be core count * 2.
	SnoMgmtCoreLimit       = 2
	ThreadSiblingsListPath = "/sys/devices/system/cpu/cpu%v/topology/thread_siblings_list"
)
