package pod

import (
	"encoding/json"
	"net"

	multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
)

type (
	NetworkAnnotation struct {
		Networks *[]multus.NetworkSelectionElement
	}

	NetworkPodDefinitionBuilder struct {
		Annotation NetworkAnnotation
		Networks   []multus.NetworkSelectionElement
	}
)

// ConvertNetworksAnnotationToJSONString converts array of network struct in to map object.
func (netAnnotation *NetworkAnnotation) ConvertNetworksAnnotationToJSONString() (string, error) {
	mapAnnotation, err := netAnnotation.ConvertNetworksAnnotationToMap()

	if err != nil {
		return "", err
	}

	mapJSONString, err := json.Marshal(mapAnnotation)

	if err != nil {
		return "", err
	}

	return string(mapJSONString), nil
}

// ConvertNetworksAnnotationToMap converts array of network struct in to map object.
func (netAnnotation *NetworkAnnotation) ConvertNetworksAnnotationToMap() (map[string]string, error) {
	mapAnnotation := map[string]string{}
	podNetworks, err := json.Marshal(netAnnotation.Networks)

	if err != nil {
		return nil, err
	}

	mapAnnotation["k8s.v1.cni.cncf.io/networks"] = string(podNetworks)

	return mapAnnotation, nil
}

// NewPodNetBuilder creates new instance of NetworkPodDefinitionBuilder.
func NewPodNetBuilder() *NetworkPodDefinitionBuilder {
	return &NetworkPodDefinitionBuilder{
		Networks:   []multus.NetworkSelectionElement{},
		Annotation: NetworkAnnotation{}}
}

func (netPodDefBuilder *NetworkPodDefinitionBuilder) WithNetworks(
	networks []multus.NetworkSelectionElement) *NetworkPodDefinitionBuilder {
	netPodDefBuilder.Annotation.Networks = &networks

	return netPodDefBuilder
}

func defineNetwork(name string) *multus.NetworkSelectionElement {
	return &multus.NetworkSelectionElement{
		Name: name,
	}
}
func DefinePodNetStaticIP(name, ipAddr string, gateway ...string) *multus.NetworkSelectionElement {
	netConfig := defineNetwork(name)
	netConfig.IPRequest = []string{ipAddr}

	if len(gateway) > 0 {
		netConfig.GatewayRequest = []net.IP{net.ParseIP(gateway[0])}
	}

	return netConfig
}
func DefinePodNetStaticMac(name, mac string) *multus.NetworkSelectionElement {
	netCOnfig := defineNetwork(name)
	netCOnfig.MacRequest = mac

	return netCOnfig
}

// DefinePodNetStaticMacIP defines pod network.
func DefinePodNetStaticMacIP(name, macAddress, ipAddr string) *multus.NetworkSelectionElement {
	netConfig := DefinePodNetStaticIP(name, ipAddr)
	netConfig.MacRequest = macAddress

	return netConfig
}
