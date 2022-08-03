package tests

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	k8sv1 "k8s.io/api/core/v1"
)

var _ = Describe("MetalLB BGP", func() {

	var (
		workerNodeList []k8sv1.Node
		masterNodeList []k8sv1.Node
		testSetupFail  = true
	)

	describe := netmetallbhelper.CreateParametersInJSON

	execute.BeforeAll(func() {
		var ipV6Address string

		clusterIPStack := netmetallbhelper.ValidateClusterIPStack()

		By(fmt.Sprintf("Running test on %s cluster", clusterIPStack))

		ipv4metalLBIPList, ipv6metalLBIPList, err := netmetallbhelper.GetMetalLBIPByFamily()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("An unexpected error occurred while"+
			" determining the IP addresses from the METALLB_ADDR_LIST environment variable.: %s", err))

		if clusterIPStack == netparameters.DualIPFamily {
			Expect(len(ipv6metalLBIPList)).To(BeNumerically(">", 0))
			ipV6Address = ipv6metalLBIPList[0]
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
		netmetallbhelper.ActivateSCTPModuleOnMaster([]k8sv1.Node{masterNodeList[0]})
		testSetupFail = false
	})

	BeforeEach(func() {
		if testSetupFail {
			Fail("Test failed due to error in BeforeAll")
		}
		By("Setup Metallb")
		netmetallbhelper.SetupMetalLB()
	})

	AfterEach(func() {

		By("should delete AddressPool, Service, BGP Peers and test Pod after test")
		externalNadList := []string{netmlbparameters.ExternalNADName}
		masterConfigMapList := []string{netparameters.MasterConfigMapName}
		netmetallbhelper.RemoveMetallbBGPTestSetup(externalNadList, masterConfigMapList)
	})

	// 49447
	DescribeTable("Verify data plane traffic over IBGP routes",
		func(ipStack string, trafficPolicy string) {
			netmetallbhelper.TestBGPTable(
				ipStack,
				workerNodeList,
				masterNodeList,
				trafficPolicy,
				netmlbparameters.IBGPASN)
		},
		Entry(describe, netparameters.IPV4Family, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netparameters.IPV4Family, netmlbparameters.ExtTrafPolCluster),
		Entry(describe, netparameters.IPV6Family, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netparameters.IPV6Family, netmlbparameters.ExtTrafPolCluster),
		Entry(describe, netparameters.DualIPFamily, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netparameters.DualIPFamily, netmlbparameters.ExtTrafPolCluster),
	)

	// 49449
	DescribeTable("Verify data plane traffic over EBGP routes",
		func(ipStack string, trafficPolicy string) {
			netmetallbhelper.TestBGPTable(
				ipStack,
				workerNodeList,
				masterNodeList,
				trafficPolicy,
				netmlbparameters.EBGPASN)
		},
		Entry(describe, netparameters.IPV4Family, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netparameters.IPV4Family, netmlbparameters.ExtTrafPolCluster),
		Entry(describe, netparameters.IPV6Family, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netparameters.IPV6Family, netmlbparameters.ExtTrafPolCluster),
		Entry(describe, netparameters.DualIPFamily, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netparameters.DualIPFamily, netmlbparameters.ExtTrafPolCluster),
	)
})
