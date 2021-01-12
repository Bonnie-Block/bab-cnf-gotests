package parameters

const (
	TestNamespace                   = "ptp-operator-test"
	OperatorNamespace               = "openshift-ptp"
	PtpSlaveNodeLabel               = "ptp/test-slave"
	PtpGrandmasterNodeLabel         = "ptp/test-grandmaster"
	PtpDaemonsetName                = "linuxptp-daemon"
	PtpContainerName                = "linuxptp-daemon-container"
)

var (
	PtpGrandMasterPolicyNameArr = []string{"test-grandmaster", "test-grandmaster1"}
	PtpSlavePolicyNameArr       = []string{"test-slave", "test-slave1"}
	PtpGrandMasterPolicyName    = "test-grandmaster"
	PtpSlavePolicyName          = "test-slave"
)
