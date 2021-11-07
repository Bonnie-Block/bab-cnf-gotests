package networkmlbparameters

import (
	"time"

	metallbv1 "github.com/metallb/metallb-operator/api/v1alpha1"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

const (
	TestNamespace                                = "cni-test"
	DefaultNameSpace                             = "default"
	AddressPool                                  = "brexpool"
	PodWaitingTime                 time.Duration = 2 * time.Minute
	WaitingTime                    time.Duration = 20 * time.Minute
	Timeout                                      = 1800 * time.Second
	TimeoutIntervals                             = time.Minute * 3
	Interval                                     = time.Second * 2
	AnnotationPrimaryIfaddr                      = "k8s.ovn.org/node-primary-ifaddr"
	AnnotationL3GW                               = "k8s.ovn.org/l3-gateway-config"
	UseMetallbResourcesFromFile                  = false
	MetalLBOperatorDeploymentName                = "metallb-operator-controller-manager"
	MetalLBOperatorDeploymentLabel               = "controller-manager"
	MetalLBDeploymentName                        = "controller"
	MetalLBDaemonsetName                         = "speaker"
	MetalLBOperatorNameSpace                     = "metallb-system"
	MetalLBAddressPool                           = "metallb.universe.tf/address-pool"
)

var (
	//ReporterNamespacesToDump tells to reporter from where to collect logs
	ReporterNamespacesToDump = map[string]string{
		"openshift-performance-addon-operator": "performance",
		MetalLBOperatorNameSpace:               "metallb-system",
		TestNamespace:                          "other",
	}
	NodeParameters = []string{"SameNode", "DiffNode"}

	// ReporterCrds tells to reporter what resources to collect
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &metallbv1.AddressPool{}},
		{Cr: &metallbv1.AddressPoolList{}},
	}
)

// MlbTestParameters contains test parameters for MetalLB tests
type MlbTestParameters struct {
	Node string
}

// NewMLBTestParameters constructor for MetalLBTestParameters
func NewMLBTestParameters(Node string) (*MlbTestParameters, error) {
	MLBTestParameters := new(MlbTestParameters)
	err := helper.StrParamInListOfParams(Node, NodeParameters)
	if err != nil {
		return nil, err
	}
	MLBTestParameters.Node = Node
	return MLBTestParameters, nil
}
