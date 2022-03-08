package tests

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"

	metallbutils "github.com/metallb/metallb-operator/test/e2e/metallb"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
)

var _ = Describe("MetalLB BGP", func() {

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
		if clusterIPStack == netmlbparameters.DualIPStack {
			ipV6Address = metalLBIPList[2]
		}

		netmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
			helper.Config.General.CnfNodeLabel, "/")[1],
			clusterIPStack,
			ipV4Address,
			ipV6Address)
	})

	BeforeEach(func() {
		By("Setup Metallb")
		netmetallbhelper.SetupMetalLB()
	})

	AfterEach(func() {

		By("should delete AddressPool, Service, BGP Peers and test Pod after test")
		netmetallbhelper.DeleteAllAddressPools()
		err := netmetallbhelper.DeleteAllLBServices(netmlbparameters.TestNamespace)
		Expect(err).ToNot(HaveOccurred())
		err = netmetallbhelper.DeleteAllBGPPeers()
		Expect(err).ToNot(HaveOccurred())
		err = namespaces.CleanPods(netmlbparameters.TestNamespace, helper.Apiclient)
		Expect(err).ToNot(HaveOccurred())
		err = nethelper.DeleteNADs([]string{netmlbparameters.ExternalNADName}, netmlbparameters.TestNamespace)
		Expect(err).ToNot(HaveOccurred())
		err = netmetallbhelper.DeleteConfigMap(netparameters.MasterConfigMapName, netmlbparameters.TestNamespace)
		Expect(err).ToNot(HaveOccurred())

		By("Should remove Metallb Configuration")
		metallb, err := metallbutils.Get(
			netmlbparameters.MetalLBOperatorNameSpace,
			netmlbparameters.UseMetallbResourcesFromFile,
		)
		Expect(err).ToNot(HaveOccurred())

		metallbutils.Delete(metallb)
	})

	// 49447
	DescribeTable("Verify data plane traffic over IBGP routes",
		func(ipStack string, trafficPolicy string) {
			netmetallbhelper.TestBGPTable(
				ipStack,
				trafficPolicy,
				netmlbparameters.IBGPASN)
		},
		Entry(describe, netmlbparameters.SingleIPv4Stack, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netmlbparameters.SingleIPv4Stack, netmlbparameters.ExtTrafPolCluster),
		Entry(describe, netmlbparameters.SingleIPv6Stack, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netmlbparameters.SingleIPv6Stack, netmlbparameters.ExtTrafPolCluster),
		Entry(describe, netmlbparameters.DualIPStack, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netmlbparameters.DualIPStack, netmlbparameters.ExtTrafPolCluster),
	)

	// 49449
	DescribeTable("Verify data plane traffic over EBGP routes",
		func(ipStack string, trafficPolicy string) {
			netmetallbhelper.TestBGPTable(
				ipStack,
				trafficPolicy,
				netmlbparameters.EBGPASN)
		},
		Entry(describe, netmlbparameters.SingleIPv4Stack, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netmlbparameters.SingleIPv4Stack, netmlbparameters.ExtTrafPolCluster),
		Entry(describe, netmlbparameters.SingleIPv6Stack, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netmlbparameters.SingleIPv6Stack, netmlbparameters.ExtTrafPolCluster),
		Entry(describe, netmlbparameters.DualIPStack, netmlbparameters.ExtTrafPolLocal),
		Entry(describe, netmlbparameters.DualIPStack, netmlbparameters.ExtTrafPolCluster),
	)
})
