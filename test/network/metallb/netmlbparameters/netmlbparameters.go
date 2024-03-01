package netmlbparameters

import (
	"fmt"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"

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
	TestNamespace                               = "metallb-test"
	AddressPoolName                             = "address-pool"
	AddressPoolL2                               = "layer2-pool"
	Layer2                                      = "layer2"
	BGP                                         = "bgp"
	EBGPProtocol                                = "eBGP"
	IBPGPProtocol                               = "iBGP"
	ClientIpv4IP                                = "172.16.0.1"
	InternalRouter1IPv4                         = "172.16.0.253"
	InternalRouter2IPv4                         = "172.16.0.254"
	InternalRouterSecondNetIPv4                 = "172.16.1.254"
	InternalClient1IPv4                         = "172.16.0.1"
	InternalClient2IPv4                         = "172.16.1.1"
	IPSecondaryInterface1                       = "3.3.3.10"
	IPSecondaryInterface2                       = "3.3.3.20"
	ScenarioMultihop                            = "multi-hop"
	ScenarioSingleHop                           = "single-hop"
	PodWaitingTime                time.Duration = 3 * time.Minute
	Interval                                    = 1 * time.Second
	Timeout                                     = 3 * time.Minute
	TimeoutBFDBGP                               = 5 * time.Second
	UpdateIntervalMetallb                       = 60 * time.Second
	WorkloadStableDuration                      = 20 * time.Second
	AnnotationPrimaryIfaddr                     = "k8s.ovn.org/node-primary-ifaddr"
	AnnotationL3GW                              = "k8s.ovn.org/l3-gateway-config"
	UseMetallbResourcesFromFile                 = false
	MetalLBDeploymentName                       = "controller"
	MetalLBDaemonsetName                        = "speaker"
	MetalLBCRName                               = "metallb"
	MetalLBOperatorNameSpace                    = "metallb-system"
	MetalLBAddressPool                          = "metallb.universe.tf/address-pool"
	HelmChartKeyExample                         = "example"
	HelmChartKeyMyClass                         = "myclass"
	HelmChartKeyOperator                        = "Exists"
	HelmChartKeyEffect                          = "NoExecute"
	HelmChartDemoController                     = "demo-controller"
	HelmChartControllerTest                     = "controller-test"
	HelmChartDemoSpeaker                        = "demo-speaker"
	HelmChartSpeakerTest                        = "speaker-test"
	HelmChartHighPriority                       = "high-priority"
	SpeakersLabelSelector                       = "component=speaker"
	BFDProfileName                              = "bfdprofile"
	BGPPassword                                 = "bgp-test"
	AppLabel1                                   = "nginx1"
	AppLabel2                                   = "nginx2"
	IBGPASN                                     = 64500
	EBGPASN                                     = 64501
	EBGPASN2                                    = 65502
	SpeakerNodeTestLabel                        = "metallbtest"
	BGPStateEstablished                         = "Established"
	BGPDefaultHoldTimer                         = 90000
	BGPDefaultKeepAliveTimer                    = 30000
	BGPUpdatedHoldTimer                         = 30000
	BGPUpdatedKeepAliveTimer                    = 10000
	MonitoringLabel                             = "openshift.io/cluster-monitoring"
	BFDStatusUp                                 = "up"
	BFDStatusDown                               = "down"
	BFDConfigPrefix                             = "bfd"
	BGPConfigPrefix                             = "router bgp"
	AddressPoolS1Name                           = "address-pools1"
	AddressPoolS2Name                           = "address-pools2"
	ExtTrafPolLocal                             = "Local"
	ExtTrafPolCluster                           = "Cluster"
	ExternalNADName                             = "external"
	External2NADName                            = "external2"
	InternalNADName                             = "internal"
	TestContainerName                           = "testcontainer"
	ProtocolSCTP                                = "sctp"
	ProtocolTCP                                 = "tcp"
	BGPAdvertisementName                        = "bgpadvertisement"
	BGPAdvertisement2Name                       = "bgpadvertisement2"
	L2AdvertisementName                         = "l2advertisement"
	BREXInterface                               = "br-ex"
	PrefixLen32                                 = int32(32)
	PrefixLen28                                 = int32(28)
	PrefixLen128                                = int32(128)
	PrefixLen126                                = int32(126)
	PrefixLen64                                 = int32(64)
	CommunityNoAdv                              = "65535:65282" // 0xFFFFFF02: NO_ADVERTISE
	CustomCommunity                             = "500:500"
	LocalPref100                                = uint32(100)
	LocalPref400                                = uint32(400)
	LocalPref500                                = uint32(500)
	AcceptedPrefixCounter                       = "AcceptedPrefixCounter"
	SentPrefixCounter                           = "SentPrefixCounter"
	PropagateFalse                              = "propagateFalse"
	PropagateTrue                               = "propagateTrue"
	BGPPeerName1v4                              = "bgp-peer1v4"
	BGPPeerName1v6                              = "bgp-peer1v6"
	BGPPeerName2v4                              = "bgp-peer2v4"
	BGPPeerName2v6                              = "bgp-peer2v6"
	LogLevelDebug                               = "debugging"
	LogLevelInfo                                = "informational"
	MetalLBOperatorDeploymentName               = "metallb-operator-controller-manager"
	FRRContainerName                            = "frr"
	NodeIntFacePrimarySubnet                    = "192.168.1.0/24"
	NodeIntFacePrimaryIPAddr                    = "192.168.1.1"
	NodeIntFacePrimaryNextHop                   = "192.168.1.2"
	NodeIntFaceSecondarySubnet                  = "192.168.2.0/24"
	NodeIntFaceSecondaryIPAddr                  = "192.168.2.1"
	NodeIntFaceSecondaryNextHop                 = "192.168.2.2"
	BGPDstPrimaryNetwork                        = "172.16.0.0/24"
	BGPDstSecondaryNetwork                      = "172.16.1.0/24"
	NMStatePolicyName                           = "simple-policy"
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
		{Cr: &metallbv1beta1.BFDProfileList{}},
		{Cr: &metallbv1beta1.BGPPeerList{}},
		{Cr: &metallboperatorv1beta1.MetalLBList{}},
	}

	SpeakerNodeSelectorWorker = map[string]string{
		fmt.Sprintf("%s/%s", nodes.LabelRole, parameters.RoleWorker): ""}
	IPv4AddressesLBList  = []string{"3.3.3.1", "3.3.3.5"}
	IPv4AddressesLB2List = []string{"4.4.4.1", "4.4.4.5"}

	TrafficPolicies = []string{string(k8sv1.ServiceExternalTrafficPolicyTypeLocal),
		string(k8sv1.ServiceExternalTrafficPolicyTypeCluster)}
	IPStackParameters = []string{netparameters.IPV4Family, netparameters.IPV6Family,
		netparameters.DualIPFamily}
	BGPPeers                 = []string{EBGPProtocol, IBPGPProtocol}
	BGPASNParameters         = []int{IBGPASN, EBGPASN}
	DumpFileName             = "/traffic 2>&1"
	BashCMD                  = []string{"/bin/bash", "-c"}
	TCPDumpCMDNew            = "tcpdump -nnn --immediate-mode -l port 80 or proto 1"
	TCPDumpCMDNet1           = []string{fmt.Sprintf("%s %s > %s", TCPDumpCMDNew, "-i net1", DumpFileName)}
	TCPDumpCMD               = []string{fmt.Sprintf("%s %s > %s", TCPDumpCMDNew, "-i eth0", DumpFileName)}
	NetAdminNetRawSysAdminSC = nethelper.DefineSecurityContext(
		[]k8sv1.Capability{"NET_ADMIN", "NET_RAW", "SYS_ADMIN"}, true,
	)
	FrrConfigMapS1Name = "frr-config-1"
	FrrConfigMapS2Name = "frr-config-2"
)

// MlbTestParameters contains test parameters for MetalLB tests.
type (
	MlbTestParameters struct {
		Node string
	}

	// NodeResourcePatch updates node based on Path and Value vars.
	NodeResourcePatch struct {
		Operation string            `json:"op"`
		Path      string            `json:"path"`
		Value     map[string]string `json:"value"`
	}

	MetalLBLogLevel string

	NMStateVlan struct {
		BeseIface string `yaml:"base-iface"`
		ID        uint16 `yaml:"id"`
	}

	NMStateIPv4Address struct {
		Address []map[string]interface{} `yaml:"address"`
		Dhcp    bool                     `yaml:"dhcp"`
		Enabled bool                     `yaml:"enabled"`
	}

	NMStateInterface struct {
		Name  string             `yaml:"name"`
		Type  string             `yaml:"type,omitempty"`
		State string             `yaml:"state"`
		IPv4  NMStateIPv4Address `yaml:"ipv4,omitempty"`
		Vlan  NMStateVlan        `yaml:"vlan,omitempty"`
	}

	NMStateRoute struct {
		Destination      string `yaml:"destination"`
		Metric           int    `yaml:"metric"`
		NextHopAddress   string `yaml:"next-hop-address"`
		NextHopInterface string `yaml:"next-hop-interface"`
		TableID          int    `yaml:"table-id"`
		State            string `yaml:"state,omitempty"`
	}
	NMStateRoutesConfig struct {
		Config []NMStateRoute `yaml:"config,omitempty"`
	}
	NMStateConfiguration struct {
		Interfaces []NMStateInterface  `yaml:"interfaces,omitempty"`
		Routes     NMStateRoutesConfig `yaml:"routes,omitempty"`
	}
)
