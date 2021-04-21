package parameters

import (
	fecv1 "github.com/open-ness/openshift-operator/sriov-fec/api/v1"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

const (
	OperatorNamespace = "vran-acceleration-operators"
	Acc100DeviceID    = "0d5c"

	TotalNumberBbdevTests          = 31
	ExpectedNumberBbdevTestsPassed = 27
	Acc100ResourceName             = "intel.com/intel_fec_acc100"
)

var (
	TestLabel = map[string]string{"testKey": "testValue"}
	// ReporterNamespacesToDump tells to reporter from where to collect logs
	ReporterNamespacesToDump = map[string]string{
		"openshift-performance-addon-operator": "performance",
		OperatorNamespace:                      "other",
		helper.TestNamespace:                   "other",
	}
	// ReporterCrds tells to reporter what resources to collect
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &fecv1.SriovFecNodeConfigList{}},
		{Cr: &fecv1.SriovFecClusterConfigList{}},
	}
)
