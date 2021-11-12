package rancpuparameters

import (
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

var (
	// ReporterNamespacesToDump tells to reporter from where to collect logs
	ReporterNamespacesToDump = map[string]string{
		parameters.PerformanceAddonOperatorNamespace: "performance",
		ran.NamespaceTesting: "other",
	}
	// ReporterCrds tells to reporter what resources to collect
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
	}
)

// RAN CPU metric names/prefixes
const (
	RanCpuMetricOsDaemon  = "ranmetrics_cpu_os_daemon"
	RanCpuMetricInfraPods = "ranmetrics_cpu_infra_pods"
	RanCpuMetricTotal     = "ranmetrics_cpu_total"
)
