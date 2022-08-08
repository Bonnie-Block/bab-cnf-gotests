package netcnihelper

import (
	"fmt"
	"strings"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nad"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

// GetNodeValidMacVlanInterface returns list of node interfaces that can be used for macvlan.
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

// AddVRFNad creates a Network Attachment Definition for static and dynamic IP addresses.
func AddVRFNad(nadName, ifName, vrfName string, ipam *nad.IPAM) netattdefv1.NetworkAttachmentDefinition {
	nadPlugins := []*nad.Plugin{
		nad.DefineMacVlanPlugin(ifName, ipam),
		nad.DefineVrfPlugin(vrfName),
	}

	nadBuilder, err := nad.NewNadBuilder(nadName, netcniparameters.TestNamespace).WithPlugins(nadPlugins).Build()
	Expect(err).ToNot(HaveOccurred())
	err = nadBuilder.Create(generalHelper.Apiclient)
	Expect(err).ToNot(HaveOccurred(), "error to build nad config with duplicated flag")

	return nadBuilder.Definition
}

// GetNodeInterfaces returns list of requested interfaces.
func getNodeInterfaces(
	conf *config.Config,
	nodeInterfaceList []nodes.NodeInterface,
	requestedNumber int) ([]nodes.NodeInterface, error) {
	var validNodeInterfaceList []nodes.NodeInterface

	if conf.Network.SriovInterfaces == "" {
		return nil, fmt.Errorf("environment variable CNF_INTERFACES_LIST is not set")
	}

	requestedNodeInterfaceList := strings.Split(conf.Network.SriovInterfaces, ",")

	if len(requestedNodeInterfaceList) < requestedNumber {
		return nil, fmt.Errorf("CNF_INTERFACES_LIST has less interfaces than requested by test suite")
	}

	for _, availableNodeInterface := range nodeInterfaceList {
		for _, requestedNodeInterface := range requestedNodeInterfaceList {
			if availableNodeInterface.Name == requestedNodeInterface {
				validNodeInterfaceList = append(validNodeInterfaceList, availableNodeInterface)
			}
		}
	}

	if len(validNodeInterfaceList) < requestedNumber {
		return nil, fmt.Errorf(
			"requested interfaces %v are not present on cluster node",
			requestedNodeInterfaceList)
	}

	return validNodeInterfaceList, nil
}
