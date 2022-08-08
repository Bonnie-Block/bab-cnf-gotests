package vrf

import (
	"fmt"
	"strings"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nad"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcnihelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
)

var _ = Describe("CNF VRF", func() {

	describe := netcnihelper.DescribeParameters

	var (
		nodeListString []string
		vrfBlue        netattdefv1.NetworkAttachmentDefinition
		vrfRed         netattdefv1.NetworkAttachmentDefinition
		testSetupFail  = true
	)

	execute.BeforeAll(func() {
		nodeListString = generalHelper.GetNodeListStringByLabel(
			strings.Split(generalHelper.Config.General.CnfNodeLabel, "/")[1],
		)
		By(fmt.Sprintf("Create %s namespace", netcniparameters.TestNamespace))
		err := namespaces.Create(netcniparameters.TestNamespace, generalHelper.Apiclient)
		Expect(err).ToNot(HaveOccurred(), "error creating namespace")
		validMacVlanInterfaces := netcnihelper.GetNodeValidMacVlanInterface(nodeListString[0], generalHelper.Config, 1)

		removeSRIOVNetworksAndNADsFromNamespace()

		By("Adding NADs")
		vrfBlue = netcnihelper.AddVRFNad(
			"test-vrf-blue",
			validMacVlanInterfaces[0].Name,
			netcniparameters.VRFBlueName,
			nad.DefineIpam("dhcp"))
		vrfRed = netcnihelper.AddVRFNad(
			"test-vrf-red",
			validMacVlanInterfaces[0].Name,
			netcniparameters.VRFRedName,
			nad.DefineIpam("dhcp"))
		testSetupFail = false
	})

	BeforeEach(func() {
		if testSetupFail {
			Fail("Test failed due to error in BeforeAll")
		}

		By("Cleaning up resources before test")
		err := namespaces.CleanPods(netcniparameters.TestNamespace, generalHelper.Apiclient)
		Expect(err).ToNot(HaveOccurred())
	})

	// 36325
	DescribeTable("Integration: NAD, IPAM: dynamic, Interfaces: 1, Scheme: 2 Pods 2 VRFs ip network overlap",
		func(node string, ipStack string) {
			vrfClientNetConfig, vrfServerNetConfig := netcnihelper.DefineVrfTestParamStaticMac(vrfRed.Name, vrfBlue.Name)
			testVRFScenario(
				node,
				ipStack,
				netcniparameters.VRFIpamDHCP,
				generalHelper.Config,
				nodeListString,
				vrfClientNetConfig,
				vrfServerNetConfig)
		},
		Entry(describe, netcniparameters.SameNode, netcniparameters.IPStackIPv4),
		Entry(describe, netcniparameters.DiffNode, netcniparameters.IPStackIPv4),
	)
})
