package parameters

import (
	"fmt"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

const (
	MTUJumbo                          = 9000
	MTUCustom                         = 1450
	MTUStandart                       = 1500
	DefaultVFSNumber                  = 5
	ConnectivityDiffNode              = "2 pods on different node"
	ConnectivitySameNodeDiffPF        = "2 pods on the same node 2 different PF"
	ConnectivitySameNodeSamePF        = "2 pods on same node same PF"
	CommunicationProtocolUnicastICMP  = "unicast-icmp"
	CommunicationProtocolUnicastTCP   = "unicast-tcp"
	CommunicationProtocolUnicastUDP   = "unicast-udp"
	CommunicationProtocolMulticastUDP = "multicast-udp"
	CommunicationProtocolBroadcastUDP = "broadcast-udp"
	CommunicationProtocolUnicastSCTP  = "unicast-sctp"
	OperatorTestNamespace             = "sriov-operator-tests"
	OperatorNamespace                 = "openshift-sriov-network-operator"
	SriovErrorProtocolMessage         = "Unsupported test parameter"
)

var (
	mtuParameters          = []int{MTUCustom, MTUJumbo, MTUStandart}
	connectivityParameters = []string{
		ConnectivityDiffNode, ConnectivitySameNodeDiffPF, ConnectivitySameNodeSamePF}

	protocolParameters = []string{CommunicationProtocolUnicastICMP, CommunicationProtocolUnicastTCP,
		CommunicationProtocolUnicastUDP, CommunicationProtocolMulticastUDP,
		CommunicationProtocolBroadcastUDP, CommunicationProtocolUnicastSCTP}
	// ReporterNamespacesToDump tells to reporter from where to collect logs
	ReporterNamespacesToDump = map[string]string{
		"openshift-performance-addon-operator": "performance",
		OperatorNamespace:                      "sriov",
		OperatorTestNamespace:                  "other",
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
