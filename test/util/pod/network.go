package pod

import (
	"encoding/json"

	multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
)

type NetworkAnnotation struct {
	Networks *[]multus.NetworkSelectionElement
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
