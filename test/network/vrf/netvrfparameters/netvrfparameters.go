package netvrfparameters

import (
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

const (
	ResourceNameVRF         = "sriovnicvrf"
	ResourceNameVRFVf1      = "sriovnicvrfvf1"
	ResourceNameVRFVf2      = "sriovnicvrfvf2"
	TestSriovNetworkRed     = "test-vrf-sriov-network-red"
	TestSriovNetworkBlue    = "test-vrf-sriov-network-blue"
	SriovPolicyName         = "test-sriov-policy-vrf"
	SameNode                = "Same Node"
	DiffNode                = "Different Node"
	IPStackIPv4             = "ipv4"
	IPStackIPv6             = "ipv6"
	TestNamespace           = "vrf-cni-test"
	PodWaitingTime          = 2 * time.Minute
	VRFBlueName             = "blue"
	VRFRedName              = "red"
	WaitingTime             = 20 * time.Minute
	TCPPort                 = 8080
	VRFClientIPAddress      = "10.255.255.1"
	VRFServerIPAddress      = "10.255.255.2"
	VRFClientMacAddressBlue = "20:04:0f:f1:88:A1"
	VRFClientMacAddressRed  = "20:04:0f:f1:88:B2"
	VRFServerMacAddressBlue = "20:04:0f:f1:88:A3"
	VRFServerMacAddressRed  = "20:04:0f:f1:88:B4"
	VRFIpamStatic           = "static"
	VRFIpamDHCP             = "dhcp"
	IpamWhereabouts         = "whereabouts"
	WhereaboutsV4Range1     = "192.168.100.0/24"
	WhereaboutsV6Range1     = "2001:1db8:85a3::0/96"
	AnnotationNetStat       = "k8s.v1.cni.cncf.io/network-status"
)

var (
	NodeParameters    = []string{SameNode, DiffNode}
	ipStackParameters = []string{IPStackIPv4, IPStackIPv6}
	// ReporterNamespacesToDump tells to reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		"openshift-performance-addon-operator":   "performance",
		generalParameters.SriovOperatorNamespace: "sriov",
		TestNamespace:                            "other",
	}

	// ReporterCrds tells to reporter what resources to collect.
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &sriovv1.SriovNetworkNodePolicyList{}},
		{Cr: &sriovv1.SriovNetworkList{}},
		{Cr: &sriovv1.SriovNetworkNodeStateList{}},
		{Cr: &sriovv1.SriovOperatorConfigList{}},
	}
)

// VrfTestParameters contains test parameters for vrf cni tests.
type VrfTestParameters struct {
	Node    string
	IPStack string
}

// NewVRFTestParameters constructor for VRFTestParameters.
func NewVRFTestParameters(node string, ipStack string) (*VrfTestParameters, error) {
	VRFTestParameters := new(VrfTestParameters)
	err := nethelper.StrParamInListOfParams(node, NodeParameters)

	if err != nil {
		return nil, err
	}

	VRFTestParameters.Node = node
	err = nethelper.StrParamInListOfParams(ipStack, ipStackParameters)

	if err != nil {
		return nil, err
	}

	VRFTestParameters.IPStack = ipStack

	return VRFTestParameters, nil
}
