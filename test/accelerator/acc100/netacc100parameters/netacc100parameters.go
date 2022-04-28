package netacc100parameters

import (
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	fecv2 "github.com/smart-edge-open/sriov-fec-operator/sriov-fec/api/v2"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

const (
	OperatorNamespace              = "vran-acceleration-operators"
	Acc100DeviceID                 = "0d5c"
	DeploymentSriovFecName         = "sriov-fec-controller-manager"
	TotalNumberBbdevTests          = 31
	ExpectedNumberBbdevTestsPassed = 27
	Acc100ResourceName             = "intel.com/intel_fec_acc100"
	TestNamespace                  = "vran-acceleration-operators-test"
)

var (
	TestLabel = map[string]string{"testKey": "testValue"}
	// ReporterNamespacesToDump tells to reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		"openshift-performance-addon-operator": "performance",
		OperatorNamespace:                      "other",
		TestNamespace:                          "other",
	}
	// ReporterCrds tells to reporter what resources to collect.
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &fecv2.SriovFecNodeConfigList{}},
		{Cr: &fecv2.SriovFecClusterConfigList{}},
	}
)
