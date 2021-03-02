package parameters

import (
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	ptpv1 "github.com/openshift/ptp-operator/pkg/apis/ptp/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

const (
	TestNamespace           = "ptp-operator-test"
	OperatorNamespace       = "openshift-ptp"
	PtpSlaveNodeLabel       = "ptp/test-slave"
	PtpGrandmasterNodeLabel = "ptp/test-grandmaster"
	PtpDaemonsetName        = "linuxptp-daemon"
	PtpContainerName        = "linuxptp-daemon-container"
)

var (
	PtpGrandMasterPolicyNameArr = []string{"test-grandmaster", "test-grandmaster1"}
	PtpSlavePolicyNameArr       = []string{"test-slave", "test-slave1"}
	PtpGrandMasterPolicyName    = "test-grandmaster"
	PtpSlavePolicyName          = "test-slave"
	// ReporterNamespacesToDump tells to reporter from where to collect logs
	ReporterNamespacesToDump = map[string]string{
		"openshift-performance-addon-operator": "performance",
		OperatorNamespace:                      "sriov",
		TestNamespace:                          "other",
	}
	// ReporterCrds tells to reporter what resources to collect
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &ptpv1.PtpConfigList{}},
		{Cr: &ptpv1.NodePtpDeviceList{}},
		{Cr: &ptpv1.PtpOperatorConfigList{}},
		{Cr: &sriovv1.SriovNetworkNodePolicyList{}},
		{Cr: &sriovv1.SriovNetworkList{}},
		{Cr: &sriovv1.SriovNetworkNodeStateList{}},
		{Cr: &sriovv1.SriovOperatorConfigList{}},
	}
)
