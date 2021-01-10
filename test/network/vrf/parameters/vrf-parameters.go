package parameters

import (
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/helper"
)

const (
	ResourceNameVRF                    = "sriovnicvrf"
	TestSriovNetworkRed                = "test-vrf-sriov-network-red"
	TestSriovNetworkBlue               = "test-vrf-sriov-network-blue"
	SriovPolicyName                    = "test-sriov-policy-vrf"
	SameNode                           = "Same Node"
	DiffNode                           = "Different Node"
	IPStackIPv4                        = "ipv4"
	IPStackIPv6                        = "ipv6"
	HostnameLabel                      = "kubernetes.io/hostname"
	TestNamespace                      = "vrf-cni-test"
	PodWaitingTime       time.Duration = 2 * time.Minute
	VRFBlueName                        = "blue"
	VRFRedName                         = "red"
	LabelNodeRole                      = "worker"
	WaitingTime          time.Duration = 20 * time.Minute
)

var (
	podClientVRFBlueIPAddress string
	podServerVRFBlueIPAddress string
	nodeParameters            = []string{SameNode, DiffNode}
	ipStackParameters         = []string{IPStackIPv4, IPStackIPv6}
)

// VrfTestParameters contains test parameters for vrf cni tests
type VrfTestParameters struct {
	Node    string
	IPStack string
}

// NewVRFTestParameters constructor for VRFTestParameters
func NewVRFTestParameters(Node string, IPStack string) (*VrfTestParameters, error) {
	VRFTestParameters := new(VrfTestParameters)
	err := helper.StrParamInListOfParams(Node, nodeParameters)
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
