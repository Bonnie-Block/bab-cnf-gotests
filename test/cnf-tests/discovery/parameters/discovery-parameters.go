package parameters

import "time"

type EnvironmentConfig struct {
	DpdkTestImage string `envconfig:"DPDK_IMAGE_VERSION"`
	CnfTestImage  string `envconfig:"CNF_IMAGE_VERSION"`
	TestImageRegistry string `envconfig:"CONTAINER_REPO"`
}
var (
	DiscoveryPerformanceProfileName = "discovery-mode-profile"
	TestNamespace = "discovery-mode-validation"
	DiscoveryPtpSlaveNodeLabel       = "ptp/discovery-slave"
	DiscoveryPtpGrandmasterNodeLabel = "ptp/discovery-grandmaster"
	DiscoveryPtpGrandmasterProfile = "discovery-master-profile"
	DiscoveryPtpWorkerProfile = "discovery-worker-profile"
	SriovWaitingTime                time.Duration = 35 * time.Minute
)


