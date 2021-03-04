package tests

import (
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

var _ = Describe("CNF VRF", func() {

	describe := func(node string, ipStack string) string {

		VRFParameters, err := parameters.NewVRFTestParameters(node, ipStack)
		if err != nil {
			return fmt.Sprintf("error in parameters: node=%s, ipStack=%s", node, ipStack)
		}
		params, err := json.Marshal(VRFParameters)
		if err != nil {
			return fmt.Sprintf("error in parameters: node=%s, ipStack=%s", node, ipStack)
		}

		return fmt.Sprintf("%s", string(params))
	}

	var nodeListString []string
	var masterMacVlanInterfaceName string
	var secondMacVlanInterfaceName string
	var vrfBlue netattdefv1.NetworkAttachmentDefinition
	var vrfRed netattdefv1.NetworkAttachmentDefinition
	config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())

	execute.BeforeAll(func() {
		By(fmt.Sprintf("Select nodes by label %s ", parameters.LabelNodeRole))
		nodesList, err := nodes.GetByRole(apiclient, parameters.LabelNodeRole)
		Expect(err).ToNot(HaveOccurred())
		for _, node := range nodesList {
			nodeListString = append(nodeListString, node.Name)
		}

		By(fmt.Sprintf("Create %s namespace", parameters.TestNamespace))
		err = namespaces.Create(parameters.TestNamespace, apiclient)
		Expect(err).ToNot(HaveOccurred())

		By("Select host interface for mac-vlan")
		nodeInterfaceList, err := nodes.GetPhysicalNodeInterfaces(apiclient, nodesList[0].Name)
		Expect(err).ToNot(HaveOccurred())
		for _, oneInterface := range nodeInterfaceList {
			if !oneInterface.Bridge && !oneInterface.DefRoute && oneInterface.Physical && oneInterface.UP {
				masterMacVlanInterfaceName = oneInterface.Name
			}
		}
		if masterMacVlanInterfaceName == "" {
			Skip("There is not valid interface on Node for VRF tests")
		}
		Expect(err).ToNot(HaveOccurred())

		By("Select host dual interface for mac-vlan")
		nodeInterfaceList, err = nodes.GetPhysicalNodeInterfaces(apiclient, nodesList[0].Name)
		Expect(err).ToNot(HaveOccurred())
		for _, secondInterface := range nodeInterfaceList {
			if !secondInterface.Bridge && !secondInterface.DefRoute && secondInterface.Name != masterMacVlanInterfaceName && secondInterface.Physical && secondInterface.UP {
				secondMacVlanInterfaceName = secondInterface.Name
			}
		}
		if secondMacVlanInterfaceName == "" {
			Skip("There is not two valid interfaces on Node for VRF tests")
		}

		By("Adding NADs")
		vrfBlue = helper.AddVRFNad(apiclient, "vrf-blue", masterMacVlanInterfaceName, parameters.VRFBlueName)
		vrfRed = helper.AddVRFNad(apiclient, "vrf-red", secondMacVlanInterfaceName, parameters.VRFRedName)
	})

	BeforeEach(func() {
		By("Cleaning up resources before test")
		err := namespaces.CleanPods(parameters.TestNamespace, apiclient)
		Expect(err).ToNot(HaveOccurred())
	})

	DescribeTable("Integration: NAD, IPAM: static, Interfaces: 2, Scheme: 2 Pods 2 VRFs network overlap",
		func(node string, ipStack string) {
			helper.TestVRFScenario(apiclient, node, ipStack, false, config, nodeListString, vrfBlue.Name, vrfRed.Name)
		},
		Entry(describe, parameters.SameNode, parameters.IPStackIPv4),
		Entry(describe, parameters.DiffNode, parameters.IPStackIPv4),
	)
})
