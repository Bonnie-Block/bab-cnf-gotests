package vrf

import (
	"fmt"

	cniTypes "github.com/containernetworking/cni/pkg/types"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/tests"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nad"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
)

// SetupSriovBeforeAll prepare env before test.
func SetupSriovBeforeAll(config *config.Config, sriovInfos *cluster.EnabledNodes, ipamType string, dual bool) {
	var (
		resourceNameRange              = []string{netcniparameters.ResourceNameVRF}
		requestedInterface             = 1
		resourceNameVrfRed             = netcniparameters.ResourceNameVRF
		resourceNameVrfBlue            = netcniparameters.ResourceNameVRF
		sriovNetworkInterfaceIndexRed  = 0
		sriovNetworkInterfaceIndexBlue = 0
	)

	if dual {
		resourceNameRange = []string{netcniparameters.ResourceNameVRFVf1, netcniparameters.ResourceNameVRFVf2}
		requestedInterface = 2
		resourceNameVrfRed = netcniparameters.ResourceNameVRFVf1
		resourceNameVrfBlue = netcniparameters.ResourceNameVRFVf2
		sriovNetworkInterfaceIndexRed = 0
		sriovNetworkInterfaceIndexBlue = 1
	}

	validSriovInterfaces := nethelper.GatherSriovInterfaces(sriovInfos, config, requestedInterface)

	By(fmt.Sprintf("Clean test namespace %s", netcniparameters.TestNamespace))
	tests.CleanSriovNetworkSriovPolicyNadsAndPods()

	By("Waiting until SRIOV become stable")
	tests.WaitUntilSriovBecomesStable()

	By("Define sr-iov Policies")
	nethelper.DefineAndCreateSriovPoliciesListOnSriovInterfaceList(resourceNameRange, validSriovInterfaces, 5)

	By("Define sr-iov network ipam config")

	ipam, err := nethelper.MarshalTypeToString(cniTypes.IPAM{Type: ipamType})
	Expect(err).ToNot(HaveOccurred(), "error to marshal ipam to string")

	By("Define sr-iov network cni vrf red plugin config")

	vrfRedPlugin, err := nethelper.MarshalTypeToString(nad.DefineVrfPlugin(netcniparameters.VRFRedName))
	Expect(err).ToNot(HaveOccurred(), "error to marshal vrf red to string")

	By("Define sr-iov network cni vrf blue plugin config")

	vrfBluePlugin, err := nethelper.MarshalTypeToString(nad.DefineVrfPlugin(netcniparameters.VRFBlueName))
	Expect(err).ToNot(HaveOccurred(), "error to marshal vrf blue to string")

	By("Define and create sr-iov networks red")
	nethelper.DefineAndCreateSriovNetwork(validSriovInterfaces[sriovNetworkInterfaceIndexRed],
		netcniparameters.TestSriovNetworkRed, resourceNameVrfRed, ipam, vrfRedPlugin,
		netcniparameters.TestNamespace)

	By("Define and create sr-iov networks blue")
	nethelper.DefineAndCreateSriovNetwork(validSriovInterfaces[sriovNetworkInterfaceIndexBlue],
		netcniparameters.TestSriovNetworkBlue, resourceNameVrfBlue, ipam, vrfBluePlugin,
		netcniparameters.TestNamespace)

	By("Waiting until SRIOV become stable")
	tests.WaitUntilSriovBecomesStable()

	By("Waiting until SRIOV-NAD become available")
	tests.WaitUntilSriovNadListBecomeAvailable(
		[]string{netcniparameters.TestSriovNetworkRed, netcniparameters.TestSriovNetworkBlue},
	)
}
