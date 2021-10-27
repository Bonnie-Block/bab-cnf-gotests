package tests

import (
	"context"
	"fmt"
	"log"
	"time"

	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metallbv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"

	metallbutils "github.com/metallb/metallb-operator/test/e2e/metallb"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	k8sv1 "k8s.io/api/core/v1"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CNF MetalLB", func() {

	var (
		metallb        *metallbv1beta1.MetalLB
		nodeListString []string
		metallbIPList  []string
	)

	metallbIP, err := helper.Config.GetMetallbVirtIP()
	Expect(err).ToNot(HaveOccurred())

	execute.BeforeAll(func() {
		isMetalLBOperatorInstalled, err := helper.IsDeploymentReady(helper.Apiclient,
			netmlbparameters.MetalLBOperatorNameSpace, netmlbparameters.MetalLBOperatorDeploymentName)
		if !isMetalLBOperatorInstalled {
			Skip("MetalLB Operator is not installed")
		} else {
			Expect(err).ToNot(HaveOccurred())
		}
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

		By("should deploy MetalLB", func() {
			metallb, err = metallbutils.Get(
				netmlbparameters.MetalLBOperatorNameSpace,
				netmlbparameters.UseMetallbResourcesFromFile,
			)
			Expect(err).ToNot(HaveOccurred())
			err = helper.Apiclient.Get(context.Background(), goclient.ObjectKey{Namespace: metallb.Namespace,
				Name: metallb.Name}, metallb)
			if errors.IsNotFound(err) {
				Expect(helper.Apiclient.Create(context.Background(), metallb)).Should(Succeed())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})

		By("should have MetalLB controller in running state", func() {
			Eventually(func() bool {
				isMetalLBControllerRunning, err := helper.IsDeploymentReady(helper.Apiclient,
					netmlbparameters.MetalLBOperatorNameSpace, netmlbparameters.MetalLBDeploymentName)
				if err != nil {
					return false
				}
				if isMetalLBControllerRunning {
					return true
				}

				return false
			}, netmlbparameters.DeployTimeout, netmlbparameters.Interval).Should(BeTrue())
		})

		By("checking MetalLB daemonset is in running state", func() {
			Eventually(func() error {
				return helper.IsDaemonsetReady(helper.Apiclient,
					netmlbparameters.MetalLBOperatorNameSpace, netmlbparameters.MetalLBDaemonsetName)
			}, netmlbparameters.DeployTimeout, netmlbparameters.Interval).ShouldNot(HaveOccurred())
		})
	})

	AfterEach(func() {

		By("should remove MetalLB Custom Resource", func() {
			deployment, err := helper.Apiclient.Deployments(metallb.Namespace).Get(context.Background(),
				netmlbparameters.MetalLBDeploymentName, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(deployment.OwnerReferences).ToNot(BeNil())
			Expect(deployment.OwnerReferences[0].Kind).To(Equal("MetalLB"))

			daemonset, err := helper.Apiclient.DaemonSets(metallb.Namespace).Get(context.Background(),
				netmlbparameters.MetalLBDaemonsetName, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(daemonset.OwnerReferences).ToNot(BeNil())
			Expect(daemonset.OwnerReferences[0].Kind).To(Equal("MetalLB"))

			metallbutils.Delete(metallb)

		})

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
