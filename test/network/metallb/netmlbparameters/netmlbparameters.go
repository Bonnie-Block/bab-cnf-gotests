package netmlbparameters

import (
	"time"

	metallbv1 "github.com/metallb/metallb-operator/api/v1alpha1"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

const (
	TestNamespace                               = "cni-test"
	DefaultNameSpace                            = "default"
	AddressPool                                 = "brexpool"
	PodWaitingTime                time.Duration = 2 * time.Minute
	Timeout                                     = 1800 * time.Second
	AnnotationPrimaryIfaddr                     = "k8s.ovn.org/node-primary-ifaddr"
	AnnotationL3GW                              = "k8s.ovn.org/l3-gateway-config"
	UseMetallbResourcesFromFile                 = false
	MetalLBOperatorDeploymentName               = "metallb-operator-controller-manager"
	MetalLBDeploymentName                       = "controller"
	MetalLBDaemonsetName                        = "speaker"
	MetalLBOperatorNameSpace                    = "metallb-system"
	MetalLBAddressPool                          = "metallb.universe.tf/address-pool"
	MetalLBService                              = "metallb-service"
)

var (
	// ReporterNamespacesToDump tells to reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		"openshift-performance-addon-operator": "performance",
		MetalLBOperatorNameSpace:               "metallb-system",
		TestNamespace:                          "other",
	}

	// ReporterCrds tells to reporter what resources to collect.
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &metallbv1.AddressPool{}},
		{Cr: &metallbv1.AddressPoolList{}},
	}
)

// MlbTestParameters contains test parameters for MetalLB tests.
type MlbTestParameters struct {
	Node string
}
