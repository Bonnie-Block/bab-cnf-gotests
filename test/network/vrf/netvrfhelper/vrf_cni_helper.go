package netvrfhelper

import (
	"context"
	"fmt"
	"strings"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/netvrfparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

// GetNodeValidMacVlanInterface returns list of node interfaces that can be used for macvlan
func GetNodeValidMacVlanInterface(nodeName string, config *config.Config, requestNumber int) []nodes.NodeInterface {
	By("Select host interface for mac-vlan")
	var macVlanInterfaces []nodes.NodeInterface
	nodeInterfaceList, err := nodes.GetPhysicalNodeInterfaces(generalHelper.Apiclient, nodeName)
	Expect(err).ToNot(HaveOccurred())

	for _, oneInterface := range nodeInterfaceList {
		if !oneInterface.Bridge && !oneInterface.DefRoute && oneInterface.Physical && oneInterface.UP {
			macVlanInterfaces = append(macVlanInterfaces, oneInterface)
		}
	}
	validMacVlanInterfaces, err := getNodeInterfaces(config, macVlanInterfaces, requestNumber)
	Expect(err).ToNot(HaveOccurred())

	return validMacVlanInterfaces
}

// AddVRFNad creates a Network Attachment Definition
func AddVRFNad(nadName string, ifName string, vrfName string) netattdefv1.NetworkAttachmentDefinition {
	vrfDefinition := netattdefv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: nadName,
			Namespace:    netvrfparameters.TestNamespace,
		},
		Spec: netattdefv1.NetworkAttachmentDefinitionSpec{
			Config: fmt.Sprintf(
				`{"cniVersion": "0.4.0", "name": "macvlan-vrf", "plugins": [{"type": "macvlan","master": "%s","ipam": {"type": "static"}},{"type": "vrf","vrfname": "%s"}]}`,
				ifName,
				vrfName),
		},
	}
	err := generalHelper.Apiclient.Create(context.Background(), &vrfDefinition)
	Expect(err).ToNot(HaveOccurred())
	return vrfDefinition
}

// GetNodeInterfaces returns list of requested interfaces
func getNodeInterfaces(
	c *config.Config,
	nodeInterfaceList []nodes.NodeInterface,
	requestedNumber int) ([]nodes.NodeInterface, error) {

	var validNodeIntefaceList []nodes.NodeInterface
	if c.Network.SriovInterfaces == "" {
		return nil, fmt.Errorf("Environment variable CNF_INTERFACES_LIST is not set")
	}
	requestedNodeInterfaceList := strings.Split(c.Network.SriovInterfaces, ",")
	if len(requestedNodeInterfaceList) < requestedNumber {
		return nil, fmt.Errorf("CNF_INTERFACES_LIST has less interfaces than requested by test suite")
	}

	for _, availableNodeInterface := range nodeInterfaceList {
		for _, requestedNodeInterface := range requestedNodeInterfaceList {
			if availableNodeInterface.Name == requestedNodeInterface {
				validNodeIntefaceList = append(validNodeIntefaceList, availableNodeInterface)
			}
		}
	}
	if len(validNodeIntefaceList) < requestedNumber {
		return nil, fmt.Errorf(
			"Requested interfaces %v are not present on cluster node",
			requestedNodeInterfaceList)
	}
	return validNodeIntefaceList, nil
}
