package tests

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	k8sv1 "k8s.io/api/core/v1"
)

var _ = Describe("MetalLB BGP", func() {

	var (
		workerNodeList []k8sv1.Node
		masterNodeList []k8sv1.Node
	)

	describe := netmetallbhelper.CreateMetallbParametersInJSON

	execute.BeforeAll(func() {
		var ipV6Address string
		clusterIPStack := netmetallbhelper.ValidateClusterIPStack()

		By(fmt.Sprintf("Running test on %s cluster", clusterIPStack))

		ipv4metalLBIPList, ipv6metalLBIPList, err := netmetallbhelper.GetMetalLBIPByFamily()
		Expect(err).ToNot(HaveOccurred())

		switch clusterIPStack {
		case netparameters.IPV4Family:
			if len(ipv4metalLBIPList) < 2 {
				Skip("The environment IP variable is not set or less than 2")
			}
		case netparameters.DualIPFamily:
			if len(ipv4metalLBIPList) < 2 || len(ipv6metalLBIPList) < 2 {
				Skip("The environment IP variable is not set or less than 2")
			}
		case netparameters.IPV6Family:
			if len(ipv6metalLBIPList) < 2 {
				Skip("The environment IPV6 variable is not set or less than 2")
			}
		}

		netmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
			helper.Config.General.CnfNodeLabel, "/")[1],
			clusterIPStack,
			ipv4metalLBIPList[0],
			ipV6Address)

		workerNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
		Expect(len(workerNodeList)).To(BeNumerically(">", 1))
		Expect(err).ToNot(HaveOccurred())

		masterNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
		Expect(len(masterNodeList)).Should(Equal(3))
		Expect(err).ToNot(HaveOccurred())

		By("should activate SCTP module")
		masterNodeList = []k8sv1.Node{masterNodeList[0], masterNodeList[1]}
		netmetallbhelper.ActivateSCTPModuleOnMaster(masterNodeList)
	})

	BeforeEach(func() {
		By("Setup Metallb")
		netmetallbhelper.SetupMetalLB()
	})

	AfterEach(func() {
		By("should delete AddressPool, Service, BGP Peers and test Pod after test")
		externalNadList := []string{netmlbparameters.ExternalNADName, netmlbparameters.External2NADName}
		masterConfigMapList := []string{netparameters.MasterConfigMapName, netparameters.Master2ConfigMapName}
		netmetallbhelper.RemoveMetallbBGPTestSetup(externalNadList, masterConfigMapList)
	})

	// 49837
	DescribeTable("Allow two specific pools to BGP Peers",
		func(ipStack string, bgpASN int, trafficPolicy string) {
			netmetallbhelper.TestBGPPeerSpecificIPAddressPools(
				ipStack,
				workerNodeList,
				masterNodeList,
				bgpASN,
				trafficPolicy)
		},
		Entry(describe, netparameters.IPV4Family, netmlbparameters.IBGPASN, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netparameters.IPV4Family, netmlbparameters.IBGPASN, netmlbparameters.ExtTrafPolCluster),
		Entry(describe, netparameters.IPV6Family, netmlbparameters.IBGPASN, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netparameters.IPV6Family, netmlbparameters.IBGPASN, netmlbparameters.ExtTrafPolCluster),
		Entry(describe, netparameters.DualIPFamily, netmlbparameters.IBGPASN, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netparameters.DualIPFamily, netmlbparameters.IBGPASN, netmlbparameters.ExtTrafPolCluster),
		Entry(describe, netparameters.IPV4Family, netmlbparameters.EBGPASN, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netparameters.IPV4Family, netmlbparameters.EBGPASN, netmlbparameters.ExtTrafPolCluster),
		Entry(describe, netparameters.IPV6Family, netmlbparameters.EBGPASN, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netparameters.IPV6Family, netmlbparameters.EBGPASN, netmlbparameters.ExtTrafPolCluster),
		Entry(describe, netparameters.DualIPFamily, netmlbparameters.EBGPASN, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netparameters.DualIPFamily, netmlbparameters.EBGPASN, netmlbparameters.ExtTrafPolCluster),
	)
})
