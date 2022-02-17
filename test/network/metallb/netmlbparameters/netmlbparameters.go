package netmlbparameters

import (
	"fmt"
	"time"

	metallbv1alpha1 "github.com/metallb/metallb-operator/api/v1alpha1"
	metallbv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

const (
	TestNamespace                               = "cni-test"
	DefaultNameSpace                            = "default"
	AddressPoolL2                               = "layer2-pool"
	Layer2                                      = "layer2"
	BGP                                         = "bgp"
	AddressPoolS1v4Name                         = "address-pools1v4"
	AddressPoolS2v4Name                         = "address-pools2v4"
	SingleIPv4Stack                             = "singleIPv4Stack"
	SingleIPv6Stack                             = "singleIPv6Stack"
	DualIPStack                                 = "dualIPStack"
	PodWaitingTime                time.Duration = 2 * time.Minute
	Interval                                    = 2 * time.Second
	Timeout                                     = 1800 * time.Second
	AnnotationPrimaryIfaddr                     = "k8s.ovn.org/node-primary-ifaddr"
	AnnotationL3GW                              = "k8s.ovn.org/l3-gateway-config"
	UseMetallbResourcesFromFile                 = false
	MetalLBOperatorDeploymentName               = "metallb-operator-controller-manager"
	MetalLBDeploymentName                       = "controller"
	MetalLBDaemonsetName                        = "speaker"
	MetalLBCRName                               = "metallb"
	MetalLBOperatorNameSpace                    = "metallb-system"
	MetalLBAddressPool                          = "metallb.universe.tf/address-pool"
	MetalLBService                              = "metallb-service"
	ExtTrafPolCluster                           = "Cluster"
	ComponentSpeaker                            = "component=speaker"
	BFDProfileName                              = "bfdprofile"
	BGPPeerName                                 = "peer-sample"
	BGPPassword                                 = "bgp-test"
	MasterConfigMapName                         = "frr-master-node-config"
	AppLabel1                                   = "nginx1"
	AppLabel2                                   = "nginx2"
	IBGPASN                                     = 64500
	EBGPASN                                     = 64501
	SpeakerNodeTestLabel                        = "metallbtest"
	BGPStateEstablished                         = "Established"
	SpeakersLabelSelector                       = "component=speaker"
	MonitoringLabel                             = "openshift.io/cluster-monitoring"
	BFDStatusUp                                 = "up"
	BFDStatusDown                               = "down"
	BFDConfigPrefix                             = "bfd"
	BGPConfigPrefix                             = "router bgp"
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
		{Cr: &metallbv1alpha1.AddressPool{}},
		{Cr: &metallbv1alpha1.AddressPoolList{}},
		{Cr: &metallbv1beta1.BFDProfile{}},
		{Cr: &metallbv1beta1.BGPPeer{}},
		{Cr: &metallbv1beta1.MetalLB{}},
	}

	SpeakerNodeSelectorWorker = map[string]string{
		fmt.Sprintf("%s/%s", nodes.LabelRole, parameters.RoleWorker): ""}
)

// MlbTestParameters contains test parameters for MetalLB tests.
type MlbTestParameters struct {
	Node string
}
