package ranwpparameters

import (
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

var (
	// ReporterNamespacesToDump tells to reporter from where to collect logs
	ReporterNamespacesToDump = map[string]string{
		parameters.PerformanceAddonOperatorNamespace: "performance",
		ran.NamespaceTesting:                         "other",
	}
	// ReporterCrds tells to reporter what resources to collect
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
	}
)

const (
	AnnotationPrefixCpuShare    = "resources.workload.openshift.io"
	AnnotationWpNamespaceKey    = "workload.openshift.io/allowed"
	AnnotationWpNamespaceValue  = "management"
	AnnotationWpPodKey          = "target.workload.openshift.io/management"
	AnnotationWpPodValue        = "{\"effect\": \"PreferredDuringScheduling\"}"
	AnnotationWpResource        = "management.workload.openshift.io/cores"
	AnnotationWpMutationWarning = "workload.openshift.io/warning"
	WarningQoSChange            = "skip pod CPUs requests modifications because it will change the pod QoS class from Burstable to BestEffort"
	WarningQoSGuaranteed        = "skip pod CPUs requests modifications because it has guaranteed QoS class"
	WarningCpuReqAndLimit       = "skip pod CPUs requests modifications because pod container has both CPU limit and request"
)
