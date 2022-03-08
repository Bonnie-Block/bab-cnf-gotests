package parameters

const (
	// LabelHostname contains the key for the hostname label.
	LabelHostname = "kubernetes.io/hostname"
	// RoleMaster contains the master role.
	RoleMaster = "master"
	// RoleWorker contains the worker role.
	RoleWorker = "worker"
	// PrivPodNamespace is the namespace for privileged pods in cnf gotests.
	PrivPodNamespace = "cnfgotestpriv"
	// PromNamespace is the Prometheus namespace.
	PromNamespace = "openshift-monitoring"
	// PromLocalURL is the Prometheus http API to get metrics.
	PromLocalURL = "http://localhost:9090/api/v1/"
	// MainContainerName is the container name of CNF test pod.
	MainContainerName = "test"
)
