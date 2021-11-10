package netvrfparameters

import (
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/helper"
	generalParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

const (
	ResourceNameVRF                    = "sriovnicvrf"
	ResourceNameVRFVf1                 = "sriovnicvrfvf1"
	ResourceNameVRFVf2                 = "sriovnicvrfvf2"
	TestSriovNetworkRed                = "test-vrf-sriov-network-red"
	TestSriovNetworkBlue               = "test-vrf-sriov-network-blue"
	SriovPolicyName                    = "test-sriov-policy-vrf"
	SameNode                           = "Same Node"
	DiffNode                           = "Different Node"
	IPStackIPv4                        = "ipv4"
	IPStackIPv6                        = "ipv6"
	TestNamespace                      = "vrf-cni-test"
	PodWaitingTime       time.Duration = 2 * time.Minute
	VRFBlueName                        = "blue"
	VRFRedName                         = "red"
	WaitingTime          time.Duration = 20 * time.Minute
	TCPPort                            = 8080
)

var (
	podClientVRFBlueIPAddress string
	podServerVRFBlueIPAddress string
	NodeParameters            = []string{SameNode, DiffNode}
	ipStackParameters         = []string{IPStackIPv4, IPStackIPv6}
	// ReporterNamespacesToDump tells to reporter from where to collect logs
	ReporterNamespacesToDump = map[string]string{
		"openshift-performance-addon-operator":   "performance",
		generalParameters.SriovOperatorNamespace: "sriov",
		TestNamespace:                            "other",
	}

	// ReporterCrds tells to reporter what resources to collect
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &sriovv1.SriovNetworkNodePolicyList{}},
		{Cr: &sriovv1.SriovNetworkList{}},
		{Cr: &sriovv1.SriovNetworkNodeStateList{}},
		{Cr: &sriovv1.SriovOperatorConfigList{}},
	}
)

// VrfTestParameters contains test parameters for vrf cni tests
type VrfTestParameters struct {
	Node    string
	IPStack string
}

// NewVRFTestParameters constructor for VRFTestParameters
func NewVRFTestParameters(Node string, IPStack string) (*VrfTestParameters, error) {
	VRFTestParameters := new(VrfTestParameters)
	err := helper.StrParamInListOfParams(Node, NodeParameters)
	if err != nil {
		return nil, err
	}
	VRFTestParameters.Node = Node

	err = helper.StrParamInListOfParams(IPStack, ipStackParameters)
	if err != nil {
		return nil, err
	}
	VRFTestParameters.IPStack = IPStack

	return VRFTestParameters, nil
}
