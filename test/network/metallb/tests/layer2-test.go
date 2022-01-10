package tests

import (
	"context"
	"fmt"
	"log"
	"time"

	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	k8sv1 "k8s.io/api/core/v1"
)

var _ = Describe("CNF MetalLB", func() {

	var (
		nodeListString []string
		metallbIPList  []string
	)

	metallbIP, err := helper.Config.GetMetallbVirtIP()
	Expect(err).ToNot(HaveOccurred())

	execute.BeforeAll(func() {
		By("Checking MetalLB operator is installed and running")
		Eventually(netmetallbhelper.IsMetalLBAvailable, deployTimeout, interval).ShouldNot(HaveOccurred())
	})

	BeforeEach(func() {
		metallbIPList, err = helper.Config.GetMetallbVirtIP()
		Expect(err).ToNot(HaveOccurred())

		if len(metallbIPList) < 2 {
			Skip("The environment IP variable is not set or less than 2")
		}
		Expect(netmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
			helper.Config.General.CnfNodeLabel, "/")[1],
			metallbIPList[0])).Should(BeTrue())

		By(fmt.Sprintf("should select nodes by role %s ", parameters.RoleWorker), func() {
			workerNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
			Expect(err).ToNot(HaveOccurred())
			for _, node := range workerNodeList {
				nodeListString = append(nodeListString, node.Name)
			}
			if len(nodeListString) < 2 {
				Skip("Need at least 2 nodes to run MetalLB test")
			}
		})
	})

	AfterEach(func() {

		By("should delete Address Pool and test Pod after test", func() {
			addresspool := netmetallbhelper.DefineMetallbAddressPool(metallbIP)
			err = namespaces.CleanPods(netmlbparameters.DefaultNameSpace, helper.Apiclient)
			Expect(err).ToNot(HaveOccurred())

			err = namespaces.CleanPods(netmlbparameters.TestNamespace, helper.Apiclient)
			Expect(err).ToNot(HaveOccurred())

			err = netmetallbhelper.DeleteAddressPool(addresspool)
			Expect(err).ToNot(HaveOccurred())

			err = helper.Apiclient.Services(netmlbparameters.TestNamespace).Delete(context.Background(),
				netmlbparameters.MetalLBService, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
		})
	})

	// 42936
	It("Validate MetalLB Layer 2 functionality", func() {
		addresspool := netmetallbhelper.DefineMetallbAddressPool(metallbIP)

		By("should have valid environment IP variable for MetalLB address pool", func() {
			if len(metallbIPList) < 2 {
				Skip("The environment IP variable is not set or less than 2")
			}
			Expect(netmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
				helper.Config.General.CnfNodeLabel, "/")[1],
				metallbIPList[0])).Should(BeTrue())
		})

		By("should create an Address Pool", func() {
			err = netmetallbhelper.CreateAddressPool(addresspool)
			Expect(err).ToNot(HaveOccurred())
		})

		By("should create a MetalLB service", func() {
			netmetallbhelper.CreateLBService(helper.Apiclient, netmlbparameters.TestNamespace)
		})

		By("should create nginx test pods", func() {
			netmetallbhelper.MLBClientPod(nodeListString[0], helper.Config.Network.TestContainerImage)
			netmetallbhelper.MLBClientPod(nodeListString[1], helper.Config.Network.TestContainerImage)
		})

		By("should validate arping", func() {
			node, err := netmetallbhelper.GetLBServiceAnnouncingNodeName()
			Expect(err).ToNot(HaveOccurred())
			err = netmetallbhelper.Arping(metallbIPList[0], helper.Config.Network.TestContainerImage,
				nodeListString, node, false)
			Expect(err).ToNot(HaveOccurred())
		})

		By("should validate curl", func() {
			node, err := netmetallbhelper.GetLBServiceAnnouncingNodeName()
			Expect(err).ToNot(HaveOccurred())
			err = netmetallbhelper.CurlMlbPod(metallbIPList[0], helper.Config.Network.TestContainerImage,
				nodeListString, node, false)
			Expect(err).ToNot(HaveOccurred())
		})

	})
	// OCP-42751
	It("Failure of MetalLB announcing speaker node", func() {

		_ = helper.CreatePrivilegedPods(helper.Config.Network.TestContainerImage)

		By("should have valid environment IP variable for MetalLB address pool", func() {
			if len(metallbIPList) < 2 {
				Skip("The environment IP variable is not set or less than 2")
			}
			Expect(netmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
				helper.Config.General.CnfNodeLabel, "/")[1],
				metallbIPList[0])).Should(BeTrue())
		})

		By("should create an Address Pool", func() {
			err := netmetallbhelper.CreateAddressPool(netmetallbhelper.DefineMetallbAddressPool(metallbIP))
			Expect(err).ToNot(HaveOccurred())
		})

		By("should create a MetalLB service", func() {
			netmetallbhelper.CreateLBService(helper.Apiclient, netmlbparameters.TestNamespace)
		})

		By("should create nginx test pods", func() {
			deploy := netmetallbhelper.MLBClientPod(nodeListString[0], helper.Config.Network.TestContainerImage)
			Expect(deploy.Status.Phase).To(Equal(k8sv1.PodRunning))
			deploy = netmetallbhelper.MLBClientPod(nodeListString[1], helper.Config.Network.TestContainerImage)
			Expect(deploy.Status.Phase).To(Equal(k8sv1.PodRunning))
		})

		announcingNodeName, err := netmetallbhelper.GetLBServiceAnnouncingNodeName()
		log.Printf("Node %s is the MetalLB service announcer node", announcingNodeName)
		Expect(err).ToNot(HaveOccurred())

		serviceNodeAPI, err := helper.Apiclient.Nodes().Get(context.Background(), announcingNodeName,
			metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())

		By("should have new MetalLB announcing node during reboot of announcing node", func() {
			helper.SoftRebootNodeAndWaitForDisconnect(serviceNodeAPI)
			announcingNodeDuringReboot, err := netmetallbhelper.GetLBServiceAnnouncingNodeName()
			log.Printf("Node %s is the new announcer node", announcingNodeDuringReboot)
			Expect(err).ToNot(HaveOccurred())

			Expect(nodeListString).To(ContainElement(announcingNodeDuringReboot),
				"No announcing node found")
			Expect(announcingNodeDuringReboot).NotTo(Equal(announcingNodeName),
				"No new node was found")
		})

		By("should validate arping", func() {
			announcingNodeDuringReboot, err := netmetallbhelper.GetLBServiceAnnouncingNodeName()
			Expect(err).ToNot(HaveOccurred())
			err = netmetallbhelper.Arping(metallbIPList[0], helper.Config.Network.TestContainerImage,
				nodeListString, announcingNodeDuringReboot, true)
			Expect(err).ToNot(HaveOccurred())
		})

		By("should validate curl", func() {
			announcingNodeDuringReboot, err := netmetallbhelper.GetLBServiceAnnouncingNodeName()
			Expect(err).ToNot(HaveOccurred())
			err = netmetallbhelper.CurlMlbPod(metallbIPList[0], helper.Config.Network.TestContainerImage,
				nodeListString, announcingNodeDuringReboot, true)
			Expect(err).ToNot(HaveOccurred())
		})

		By("should return node to Ready state after reboot", func() {
			helper.WaitForNodeReachable(serviceNodeAPI)
			err := nodes.WaitForNodesReady(helper.Apiclient, 3*time.Minute, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())
		})

		By(fmt.Sprintf("should have node %s return to announcing node after reboot", announcingNodeName),
			func() {
				Eventually(func() string {
					announcingNodeAfterReboot, _ := netmetallbhelper.GetLBServiceAnnouncingNodeName()

					return announcingNodeAfterReboot
				}, 4*time.Minute, 10*time.Second).Should(Equal(announcingNodeName))
			})
		By("should validate arping", func() {
			node, err := netmetallbhelper.GetLBServiceAnnouncingNodeName()
			Expect(err).ToNot(HaveOccurred())
			err = netmetallbhelper.Arping(metallbIPList[0], helper.Config.Network.TestContainerImage,
				nodeListString, node, false)
			Expect(err).ToNot(HaveOccurred())
		})

		By("should validate curl", func() {
			node, err := netmetallbhelper.GetLBServiceAnnouncingNodeName()
			Expect(err).ToNot(HaveOccurred())
			err = netmetallbhelper.CurlMlbPod(metallbIPList[0], helper.Config.Network.TestContainerImage,
				nodeListString, node, false)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
