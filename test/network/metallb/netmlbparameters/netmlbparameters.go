package netmlbparameters

import (
	"fmt"
	"time"

	metallbv1beta1 "go.universe.tf/metallb/api/v1beta1"

	metallboperatorv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	k8sv1 "k8s.io/api/core/v1"
)

const (
	TestNamespace                             = "metallb-test"
	AddressPoolName                           = "address-pool"
	AddressPoolL2                             = "layer2-pool"
	Layer2                                    = "layer2"
	BGP                                       = "bgp"
	EBGPProtocol                              = "eBGP"
	IBPGPProtocol                             = "iBGP"
	ClientIpv4IP                              = "172.16.0.1"
	InternalRouter1IPv4                       = "172.16.0.253"
	InternalRouter2IPv4                       = "172.16.0.254"
	IPSecondaryInterface1                     = "3.3.3.10"
	IPSecondaryInterface2                     = "3.3.3.20"
	ScenarioMultihop                          = "multi-hop"
	ScenarioSingleHop                         = "single-hop"
	PodWaitingTime              time.Duration = 2 * time.Minute
	Interval                                  = 1 * time.Second
	Timeout                                   = 3 * time.Minute
	TimeoutBFDBGP                             = 5 * time.Second
	UpdateIntervalMetallb                     = 60 * time.Second
	WorkloadStableDuration                    = 20 * time.Second
	AnnotationPrimaryIfaddr                   = "k8s.ovn.org/node-primary-ifaddr"
	AnnotationL3GW                            = "k8s.ovn.org/l3-gateway-config"
	UseMetallbResourcesFromFile               = false
	MetalLBDeploymentName                     = "controller"
	MetalLBDaemonsetName                      = "speaker"
	MetalLBCRName                             = "metallb"
	MetalLBOperatorNameSpace                  = "metallb-system"
	MetalLBAddressPool                        = "metallb.universe.tf/address-pool"
	SpeakersLabelSelector                     = "component=speaker"
	BFDProfileName                            = "bfdprofile"
	BGPPassword                               = "bgp-test"
	AppLabel1                                 = "nginx1"
	AppLabel2                                 = "nginx2"
	IBGPASN                                   = 64500
	EBGPASN                                   = 64501
	SpeakerNodeTestLabel                      = "metallbtest"
	BGPStateEstablished                       = "Established"
	BGPDefaultHoldTimer                       = 90000
	BGPDefaultKeepAliveTimer                  = 30000
	BGPUpdatedHoldTimer                       = 30000
	BGPUpdatedKeepAliveTimer                  = 10000
	MonitoringLabel                           = "openshift.io/cluster-monitoring"
	BFDStatusUp                               = "up"
	BFDStatusDown                             = "down"
	BFDConfigPrefix                           = "bfd"
	BGPConfigPrefix                           = "router bgp"
	AddressPoolS1Name                         = "address-pools1"
	AddressPoolS2Name                         = "address-pools2"
	ExtTrafPolLocal                           = "Local"
	ExtTrafPolCluster                         = "Cluster"
	ExternalNADName                           = "external"
	External2NADName                          = "external2"
	InternalNADName                           = "internal"
	TestContainerName                         = "testcontainer"
	ProtocolSCTP                              = "sctp"
	ProtocolTCP                               = "tcp"
	BGPAdvertisementName                      = "bgpadvertisement"
	BGPAdvertisement2Name                     = "bgpadvertisement2"
	L2AdvertisementName                       = "l2advertisement"
	BREXInterface                             = "br-ex"
	PrefixLen32                               = int32(32)
	PrefixLen28                               = int32(28)
	PrefixLen128                              = int32(128)
	PrefixLen126                              = int32(126)
	PrefixLen64                               = int32(64)
	CommunityNoAdv                            = "65535:65282" // 0xFFFFFF02: NO_ADVERTISE
	CustomCommunity                           = "500:500"
	LocalPref100                              = uint32(100)
	LocalPref400                              = uint32(400)
	LocalPref500                              = uint32(500)
	AcceptedPrefixCounter                     = "AcceptedPrefixCounter"
	SentPrefixCounter                         = "SentPrefixCounter"
	PropagateFalse                            = "propagateFalse"
	PropagateTrue                             = "propagateTrue"
	BGPPeerName1v4                            = "bgp-peer1v4"
	BGPPeerName1v6                            = "bgp-peer1v6"
	BGPPeerName2v4                            = "bgp-peer2v4"
	BGPPeerName2v6                            = "bgp-peer2v6"
	LogLevelDebug                             = "debugging"
	LogLevelInfo                              = "informational"
	FRRContainerName                          = "frr"
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
		{Cr: &metallbv1beta1.IPAddressPoolList{}},
		{Cr: &metallbv1beta1.BGPAdvertisementList{}},
		{Cr: &metallbv1beta1.L2AdvertisementList{}},
		{Cr: &metallbv1beta1.AddressPoolList{}},
		{Cr: &metallbv1beta1.BFDProfileList{}},
		{Cr: &metallbv1beta1.BGPPeerList{}},
		{Cr: &metallboperatorv1beta1.MetalLBList{}},
	}

	SpeakerNodeSelectorWorker = map[string]string{
		fmt.Sprintf("%s/%s", nodes.LabelRole, parameters.RoleWorker): ""}
	IPv4AddressesLBList = []string{"3.3.3.1", "3.3.3.5"}

	TrafficPolicies = []string{string(k8sv1.ServiceExternalTrafficPolicyTypeLocal),
		string(k8sv1.ServiceExternalTrafficPolicyTypeCluster)}
	IPStackParameters = []string{netparameters.IPV4Family, netparameters.IPV6Family,
		netparameters.DualIPFamily}
	BGPPeers         = []string{EBGPProtocol, IBPGPProtocol}
	BGPASNParameters = []int{IBGPASN, EBGPASN}
)

// MlbTestParameters contains test parameters for MetalLB tests.
type MlbTestParameters struct {
	Node string
}

type MetalLBLogLevel string
