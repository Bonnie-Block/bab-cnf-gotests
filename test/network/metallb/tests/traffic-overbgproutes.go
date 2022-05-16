package tests

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
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
	)

	describe := netmetallbhelper.CreateParametersInJSON

	execute.BeforeAll(func() {
		var ipV6Address string

		clusterIPStack := netmetallbhelper.ValidateClusterIPStack()

		By(fmt.Sprintf("Running test on %s cluster", clusterIPStack))

		metalLBIPList, err := helper.Config.GetMetallbVirtIP()
		Expect(err).ToNot(HaveOccurred())

		if len(metalLBIPList) < 2 {
			Skip("The environment IP variable is not set or less than 2")
		}

		ipV4Address := metalLBIPList[0]
		if clusterIPStack == netparameters.DualIPFamily {
			ipV6Address = metalLBIPList[2]
		}

		netmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
			helper.Config.General.CnfNodeLabel, "/")[1],
			clusterIPStack,
			ipV4Address,
			ipV6Address)

		workerNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
		Expect(len(workerNodeList)).To(BeNumerically(">", 1))
		Expect(err).ToNot(HaveOccurred())

		masterNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
		Expect(len(masterNodeList)).Should(Equal(3))
		Expect(err).ToNot(HaveOccurred())

		By("should activate SCTP module")
		netmetallbhelper.ActivateSCTPModuleOnMaster(masterNodeList[0])

	})

	BeforeEach(func() {
		By("Setup Metallb")
		netmetallbhelper.SetupMetalLB()
	})

	AfterEach(func() {

		By("should delete AddressPool, Service, BGP Peers and test Pod after test")
		netmetallbhelper.RemoveMetallbBGPTestSetup()
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
