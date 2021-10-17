package tests

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	networkmetallbhelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/parameters"
	generalParamters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
)

var _ = Describe("CNF MetalLB", func() {

	var nodeListString []string
	var metallbIPList []string

	Config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())

	execute.BeforeAll(func() {

		metallbIPList, err = Config.GetMetallbVirtIP()
		Expect(err).ToNot(HaveOccurred())

		By("should have valid environment IP variable", func() {
			if len(metallbIPList) < 2 {
				Skip("The environment IP variable is not set or less than 2")
			}
			Expect(networkmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
				Config.General.CnfNodeLabel, "/")[1],
				metallbIPList[0])).Should(BeTrue())
		})

		By(fmt.Sprintf("should select nodes by label %s ", generalParamters.RoleWorker))
		nodeList, err := nodes.GetByRole(generalHelper.Apiclient, generalParamters.RoleWorker)
		Expect(err).ToNot(HaveOccurred())
		for _, node := range nodeList {
			nodeListString = append(nodeListString, node.Name)
		}
		if len(nodeListString) < 2 {
			Skip("Need at least 2 nodes to run MetalLB test")
		}

		By("should have the MetalLB Operator deployment is installed", func() {
			isDeploymentInstalled, err := generalHelper.IsDeploymentInstalled(generalHelper.Apiclient,
				parameters.MetalLBOperatorNameSpace, parameters.MetalLBOperatorDeploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(isDeploymentInstalled).To(Equal(true), "MetalLB operator is not installed")
		})

		By("should have the MetalLB Operator deployment in running state", func() {
			isDeploymentReady, err := generalHelper.IsDeploymentReady(generalHelper.Apiclient,
				parameters.MetalLBOperatorNameSpace, parameters.MetalLBOperatorDeploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(isDeploymentReady).To(Equal(true), "MetalLB operator is not running")
		})

		By("should have MetalLB daemonset is in running state", func() {
			daemonsetRun, daemonsetDesired := generalHelper.CountDaemonsets(generalHelper.Apiclient,
				parameters.MetalLBOperatorNameSpace, parameters.MetalLBDaemonsetName)
			Expect(daemonsetRun).To(Equal(daemonsetDesired),
				"Not all MetalLB Speaker daemonsets are ready")
		})

		By("should create an Address Pool", func() {
			err := networkmetallbhelper.CreateAddressPool(networkmetallbhelper.DefineMetallbAddressPool())
			Expect(err).ToNot(HaveOccurred())
		})

		By("should create a MetalLB service", func() {
			networkmetallbhelper.CreateLBService(generalHelper.Apiclient, parameters.TestNamespace)
		})

		By("should create nginx test pods", func() {
			networkmetallbhelper.MLBClientPod(nodeListString[0], Config.Network.TestContainerImage)
			networkmetallbhelper.MLBClientPod(nodeListString[1], Config.Network.TestContainerImage)
		})
	})

	execute.AfterAll(func() {
		By("should delete Address Pool and test Pod after test", func() {
			err := networkmetallbhelper.DeleteAddressPool(networkmetallbhelper.DefineMetallbAddressPool())
			Expect(err).ToNot(HaveOccurred())

			err = namespaces.CleanPods(parameters.DefaultNameSpace, generalHelper.Apiclient)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	//OCP-42936
	It("Validate MetalLB Layer 2 functionality", func() {
		By("should validate arping", func() {
			testPod := networkmetallbhelper.MLBTestPod(nodeListString[0],
				parameters.DefaultNameSpace, Config.Network.TestContainerImage)
			// https://bugzilla.redhat.com/show_bug.cgi?id=1987445 - fix in 4.10
			// Verifies that only one node answers the arp request
			output := networkmetallbhelper.Arping(*testPod, metallbIPList[0])
			lineCount := 0
			for _, reply := range output {
				if strings.Contains(reply, "Unicast") {
					lineCount++
				}
			}
			Expect(lineCount).To(Equal(2), "An incorrect number of arp replies were received")
			// Verifies the output mac addresses matches the annoucing node mac address
			node, err := networkmetallbhelper.GetLBServiceEvents()
			nodeMac, err := networkmetallbhelper.SpeakerNodeMac(node)
			Expect(string(strings.Join(output, "\n"))).Should(ContainSubstring(strings.ToUpper(nodeMac)),
				"ARP request was not recieved from the announcing node")
			Expect(err).ToNot(HaveOccurred())

		})

		By("should validate curl", func() {
			testPod := networkmetallbhelper.MLBTestPod(nodeListString[0],
				parameters.DefaultNameSpace, Config.Network.TestContainerImage)
			Expect(networkmetallbhelper.CurlMlbPod(*testPod, metallbIPList[0])).Should(ContainSubstring("html"),
				"Curl was unable to connect to nginx")
		})
	})
})
