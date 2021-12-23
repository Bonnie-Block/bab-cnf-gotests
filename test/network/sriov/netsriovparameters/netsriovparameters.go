package netsriovparameters

import (
	"fmt"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
)

const (
	MTUJumbo                          = 9000
	MTUCustom                         = 1450
	MTUStandart                       = 1500
	ConnectivityDiffNode              = "2 pods on different node"
	ConnectivitySameNodeDiffPF        = "2 pods on the same node 2 different PF"
	ConnectivitySameNodeSamePF        = "2 pods on same node same PF"
	ConnectivityDiffNodeDiffPF        = "2 pods on different node 2 different PFs"
	ConnectivityDiffNodeSamePF        = "2 pods on different node same PF"
	CommunicationProtocolUnicastICMP  = "unicast-icmp"
	CommunicationProtocolUnicastTCP   = "unicast-tcp"
	CommunicationProtocolUnicastUDP   = "unicast-udp"
	CommunicationProtocolMulticastUDP = "multicast-udp"
	CommunicationProtocolBroadcastUDP = "broadcast-udp"
	CommunicationProtocolUnicastSCTP  = "unicast-sctp"
	OperatorTestNamespace             = "sriov-operator-tests"
	OperatorNamespace                 = "openshift-sriov-network-operator"
	SriovErrorProtocolMessage         = "Unsupported test parameter"
	SriovNetworkUsualMTUName          = "test-sriov-static-usual"
	SriovNetworkCustomMTUName         = "test-sriov-static-custom"
	SriovNetworkJumboFrameName        = "test-sriov-static-jumbo"
	SriovNetworkUsualMTUNameDiff      = "test-sriov-static-usual-diff"
	SriovNetworkCustomMTUNameDiff     = "test-sriov-static-custom-diff"
	SriovNetworkJumboFrameNameDiff    = "test-sriov-static-jumbo-diff"
	SriovNetworkBondName              = "test-sriov-static-bond"
	SriovNetworkBondNameDiff          = "test-sriov-static-bond-diff"
	ClientPodIP                       = "192.168.100.1"
	ClientPodIPv6                     = "2001:1db8:85a3::1"
	ClientMacAddress                  = "20:04:0f:f1:88:01"
	ServerPodIP                       = "192.168.100.2"
	ServerPodIpv6                     = "2001:1db8:85a3::2"
	ServerMacAddress                  = "20:04:0f:f1:88:03"
	TestPort                          = 50000
	TestInterfaceName                 = "net1"
	TestBondInterfaceName             = "bond0"
	NADBondName                       = "bond-net"
	MulticastIPv6Address              = "FF05:0:0:0:0:0:0:18C"
	MulticastIPAddress                = "224.255.0.10"
)

var (
	WaitingTime        = 35 * time.Minute
	PodWaitingTime     = 1 * time.Minute
	DualPodWaitingTime = 3 * time.Minute

	mtuParameters          = []int{MTUCustom, MTUJumbo, MTUStandart}
	connectivityParameters = []string{
		ConnectivityDiffNode,
		ConnectivitySameNodeDiffPF,
		ConnectivitySameNodeSamePF}

	connectivityBondParameters = []string{
		ConnectivityDiffNodeDiffPF,
		ConnectivityDiffNodeSamePF}

	protocolParameters = []string{CommunicationProtocolUnicastICMP, CommunicationProtocolUnicastTCP,
		CommunicationProtocolUnicastUDP, CommunicationProtocolMulticastUDP,
		CommunicationProtocolBroadcastUDP, CommunicationProtocolUnicastSCTP}
	// ReporterNamespacesToDump tells to reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		"openshift-performance-addon-operator": "performance",
		OperatorNamespace:                      "sriov",
		OperatorTestNamespace:                  "other",
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

// ConnectivityTestParameters contains test parameters for connectivity.
type ConnectivityTestParameters struct {
	Protocol     string
	MTU          int
	Connectivity string
}

// NewConnectivityTestParameters creates new instance of ConnectivityTestParameters.
func NewConnectivityTestParameters(mtu int,
	connectivity string,
	protocol string, bond bool) (*ConnectivityTestParameters, error) {
	connectivityTestParameters := new(ConnectivityTestParameters)
	err := validateIntParam(mtu, mtuParameters)

	if err != nil {
		return nil, err
	}

	connectivityTestParameters.MTU = mtu

	if bond {
		err = validateSrtParam(connectivity, connectivityBondParameters)
	} else {
		err = validateSrtParam(connectivity, connectivityParameters)
	}

	if err != nil {
		return nil, err
	}

	connectivityTestParameters.Connectivity = connectivity
	err = validateSrtParam(protocol, protocolParameters)

	if err != nil {
		return nil, err
	}

	connectivityTestParameters.Protocol = protocol

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
