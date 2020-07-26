package parameters

import (
	"fmt"
)

const (
	MTUJumbo                          = 9000
	MTUCustom                         = 1450
	MTUStandart                       = 1500
	DefaultVFSNumber                  = 5
	ConnectivityDiffNode              = "2 pods on different node"
	ConnectivitySameNodeDiffPF        = "2 pods on the same node 2 different PF"
	ConnectivitySameNodeSamePF        = "2 pods on same node same PF"
	ConnectivityPodExtPodInt          = "pod to external/External to pod"
	CommunicationProtocolUnicastICMP  = "unicast-icmp"
	CommunicationProtocolUnicastTCP   = "unicast-tcp"
	CommunicationProtocolUnicastUDP   = "unicast-udp"
	CommunicationProtocolMulticastUDP = "multicast-udp"
	CommunicationProtocolBroadcastUDP = "broadcast-udp"
	CommunicationProtocolSctpUDP      = "unicast-sctp"
	OperatorTestNamespace             = "sriov-operator-tests"
	OperatorNamespace                 = "openshift-sriov-network-operator"
)

var (
	mtuParameters          = []int{MTUCustom, MTUJumbo, MTUStandart}
	connectivityParameters = []string{
		ConnectivityDiffNode, ConnectivitySameNodeDiffPF, ConnectivitySameNodeSamePF, ConnectivityPodExtPodInt}

	protocolParameters = []string{CommunicationProtocolUnicastICMP, CommunicationProtocolUnicastTCP,
		CommunicationProtocolUnicastUDP, CommunicationProtocolMulticastUDP,
		CommunicationProtocolBroadcastUDP, CommunicationProtocolSctpUDP}
)

// ConnectivityTestParameters contains test parameters for connectivity
type ConnectivityTestParameters struct {
	Protocol     string
	MTU          int
	Connectivity string
}

// NewConnectivityTestParameters creates new instance of ConnectivityTestParameters
func NewConnectivityTestParameters(MTU int, Connectivity string, Protocol string) (*ConnectivityTestParameters, error) {
	connectivityTestParameters := new(ConnectivityTestParameters)
	err := validateIntParam(MTU, mtuParameters)
	if err != nil {
		return nil, err
	}
	connectivityTestParameters.MTU = MTU
	err = validateSrtParam(Connectivity, connectivityParameters)
	if err != nil {
		return nil, err
	}
	connectivityTestParameters.Connectivity = Connectivity
	err = validateSrtParam(Protocol, protocolParameters)
	if err != nil {
		return nil, err
	}
	connectivityTestParameters.Protocol = Protocol
	return connectivityTestParameters, nil
}

func validateIntParam(intParam int, intParamRange []int) error {
	for _, parameter := range intParamRange {
		if intParam == parameter {
			return nil
		}
	}
	return fmt.Errorf("error: wrong parameter %v", intParam)
}

func validateSrtParam(param string, paramRange []string) error {
	for _, parameter := range paramRange {
		if param == parameter {
			return nil
		}
	}
	return fmt.Errorf("error: wrong parameter %v", param)
}
