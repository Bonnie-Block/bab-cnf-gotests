package netmlbparameters

import (
	"fmt"
	"time"

	metallbv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

const (
	TestNamespace                             = "cni-test"
	AddressPoolName                           = "address-pool"
	AddressPoolL2                             = "layer2-pool"
	Layer2                                    = "layer2"
	BGP                                       = "bgp"
	SingleIPv4Stack                           = "singleIPv4Stack"
	SingleIPv6Stack                           = "singleIPv6Stack"
	DualIPStack                               = "dualIPStack"
	EBGPProtocol                              = "eBGP"
	IBPGPProtocol                             = "ibgp"
	ClientIpv4IP                              = "172.16.0.1"
	InternalRouter1IPv4                       = "172.16.0.253"
	InternalRouter2IPv4                       = "172.16.0.254"
	IPV6Family                                = "ipv6"
	ScenarioMultihop                          = "multi-hop"
	ScenarioSingleHop                         = "single-hop"
	PodWaitingTime              time.Duration = 2 * time.Minute
	Interval                                  = 1 * time.Second
	Timeout                                   = 3 * time.Minute
	TimeoutBFDBGP                             = 5 * time.Second
	AnnotationPrimaryIfaddr                   = "k8s.ovn.org/node-primary-ifaddr"
	AnnotationL3GW                            = "k8s.ovn.org/l3-gateway-config"
	UseMetallbResourcesFromFile               = false
	MetalLBDeploymentName                     = "controller"
	MetalLBDaemonsetName                      = "speaker"
	MetalLBCRName                             = "metallb"
	MetalLBOperatorNameSpace                  = "metallb-system"
	MetalLBAddressPool                        = "metallb.universe.tf/address-pool"
	ExtTrafPolCluster                         = "Cluster"
	SpeakersLabelSelector                     = "component=speaker"
	BFDProfileName                            = "bfdprofile"
	BGPPassword                               = "bgp-test"
	AppLabel1                                 = "nginx1"
	AppLabel2                                 = "nginx2"
	IBGPASN                                   = 64500
	EBGPASN                                   = 64501
	SpeakerNodeTestLabel                      = "metallbtest"
	BGPStateEstablished                       = "Established"
	MonitoringLabel                           = "openshift.io/cluster-monitoring"
	BFDStatusUp                               = "up"
	BFDStatusDown                             = "down"
	BFDConfigPrefix                           = "bfd"
	BGPConfigPrefix                           = "router bgp"
	Wget                                      = "wget"
	Curl                                      = "curl"
	InternalNADName                           = "internal"
	AddressPoolS1Name                         = "address-pools1"
	AddressPoolS2Name                         = "address-pools2"
	ExtTrafPolLocal                           = "Local"
	ExternalNADName                           = "external"
	TestContainerName                         = "testcontainer"
)

var (
	// ReporterNamespacesToDump tells to reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		"openshift-performance-addon-operator": "performance",
		MetalLBOperatorNameSpace:               MetalLBOperatorNameSpace,
		TestNamespace:                          "other",
	}

	// ReporterCrds tells to reporter what resources to collect.
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &metallbv1beta1.AddressPool{}},
		{Cr: &metallbv1beta1.AddressPoolList{}},
		{Cr: &metallbv1beta1.BFDProfile{}},
		{Cr: &metallbv1beta1.BGPPeer{}},
		{Cr: &metallbv1beta1.MetalLB{}},
	}

	SpeakerNodeSelectorWorker = map[string]string{
		fmt.Sprintf("%s/%s", nodes.LabelRole, parameters.RoleWorker): ""}
	MetalLBMultihopIPv4List = []string{"3.3.3.1", "3.3.3.5"}

	TrafficPolicies   = []string{ExtTrafPolCluster, ExtTrafPolLocal}
	IPStackParameters = []string{SingleIPv4Stack, SingleIPv6Stack,
		DualIPStack}
)

// MlbTestParameters contains test parameters for MetalLB tests.
type MlbTestParameters struct {
	Node string
}
