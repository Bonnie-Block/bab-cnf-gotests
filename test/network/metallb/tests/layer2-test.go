package tests

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/networkmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/networkmlbparameters"
	generalParamters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metallbv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"

	metallbutils "github.com/metallb/metallb-operator/test/e2e/metallb"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	generalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CNF MetalLB", func() {

	var metallb *metallbv1beta1.MetalLB

	var nodeListString []string
	var metallbIPList []string

	execute.BeforeAll(func() {
		isMetalLBOperatorInstalled, err := generalHelper.IsDeploymentReady(generalHelper.Apiclient,
			networkmlbparameters.MetalLBOperatorNameSpace, networkmlbparameters.MetalLBOperatorDeploymentName)
		if isMetalLBOperatorInstalled == false {
			Skip("MetalLB Operator is not installed")
		} else {
			Expect(err).ToNot(HaveOccurred())
		}
	})

	BeforeEach(func() {
		var err error

		metallbIPList, err = generalHelper.Config.GetMetallbVirtIP()
		Expect(err).ToNot(HaveOccurred())

		if len(metallbIPList) < 2 {
			Skip("The environment IP variable is not set or less than 2")
		}
		Expect(networkmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
			generalHelper.Config.General.CnfNodeLabel, "/")[1],
			metallbIPList[0])).Should(BeTrue())

		By(fmt.Sprintf("should select nodes by role %s ", generalParamters.RoleWorker), func() {
			nodeList, err := nodes.GetByRole(generalHelper.Apiclient, generalParamters.RoleWorker)
			Expect(err).ToNot(HaveOccurred())
			for _, node := range nodeList {
				nodeListString = append(nodeListString, node.Name)
			}
			if len(nodeListString) < 2 {
				Skip("Need at least 2 nodes to run MetalLB test")
			}
		})

		By("should deploy MetalLB", func() {
			metallb, err = metallbutils.Get(networkmlbparameters.MetalLBOperatorNameSpace, networkmlbparameters.UseMetallbResourcesFromFile)
			Expect(err).ToNot(HaveOccurred())
			err = generalHelper.Apiclient.Get(context.Background(), goclient.ObjectKey{Namespace: metallb.Namespace, Name: metallb.Name}, metallb)
			if errors.IsNotFound(err) {
				Expect(generalHelper.Apiclient.Create(context.Background(), metallb)).Should(Succeed())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})

		By("should have MetalLB controller in running state", func() {
			Eventually(func() bool {
				isMetalLBControllerRunning, err := generalHelper.IsDeploymentReady(generalHelper.Apiclient,
					networkmlbparameters.MetalLBOperatorNameSpace, networkmlbparameters.MetalLBDeploymentName)
				if err != nil {
					return false
				}
				if isMetalLBControllerRunning == true {
					return true
				}
				return false
			}, metallbutils.Timeout, metallbutils.Interval).Should(BeTrue())
		})

		By("checking MetalLB daemonset is in running state", func() {

			Eventually(func() bool {
				daemonSetRunning, daemonSetDesired := generalHelper.CountDaemonsets(generalHelper.Apiclient, networkmlbparameters.MetalLBOperatorNameSpace, networkmlbparameters.MetalLBDaemonsetName)
				return daemonSetRunning == daemonSetDesired
			}, metallbutils.DeployTimeout, metallbutils.Interval).Should(BeTrue())
		})
	})

	AfterEach(func() {

		By("should remove MetalLB Custom Resource", func() {
			deployment, err := generalHelper.Apiclient.Deployments(metallb.Namespace).Get(context.Background(), networkmlbparameters.MetalLBDeploymentName, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(deployment.OwnerReferences).ToNot(BeNil())
			Expect(deployment.OwnerReferences[0].Kind).To(Equal("MetalLB"))

			daemonset, err := generalHelper.Apiclient.DaemonSets(metallb.Namespace).Get(context.Background(), networkmlbparameters.MetalLBDaemonsetName, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(daemonset.OwnerReferences).ToNot(BeNil())
			Expect(daemonset.OwnerReferences[0].Kind).To(Equal("MetalLB"))

			metallbutils.Delete(metallb)
		})

		By("should delete Address Pool and test Pod after test", func() {
			addresspool := networkmetallbhelper.DefineMetallbAddressPool()
			err := namespaces.CleanPods(networkmlbparameters.DefaultNameSpace, generalHelper.Apiclient)
			Expect(err).ToNot(HaveOccurred())

			err = namespaces.CleanPods(networkmlbparameters.TestNamespace, generalHelper.Apiclient)
			Expect(err).ToNot(HaveOccurred())

			err = networkmetallbhelper.DeleteAddressPool(addresspool)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	//OCP-42936
	It("Validate MetalLB Layer 2 functionality", func() {
		addresspool := networkmetallbhelper.DefineMetallbAddressPool()

		By("should have valid environment IP variable for MetalLB address pool", func() {
			if len(metallbIPList) < 2 {
				Skip("The environment IP variable is not set or less than 2")
			}
			Expect(networkmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
				generalHelper.Config.General.CnfNodeLabel, "/")[1],
				metallbIPList[0])).Should(BeTrue())
		})

		By("should create an Address Pool", func() {
			err := networkmetallbhelper.CreateAddressPool(addresspool)
			Expect(err).ToNot(HaveOccurred())
		})

		By("should create a MetalLB service", func() {
			networkmetallbhelper.CreateLBService(generalHelper.Apiclient, networkmlbparameters.TestNamespace)
		})

		By("should create nginx test pods", func() {
			networkmetallbhelper.MLBClientPod(nodeListString[0], generalHelper.Config.Network.TestContainerImage)
			networkmetallbhelper.MLBClientPod(nodeListString[1], generalHelper.Config.Network.TestContainerImage)
		})

		By("should validate arping", func() {
			testPod := networkmetallbhelper.MLBTestPod(nodeListString[0],
				networkmlbparameters.DefaultNameSpace, generalHelper.Config.Network.TestContainerImage)
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
				networkmlbparameters.DefaultNameSpace, generalHelper.Config.Network.TestContainerImage)
			Expect(networkmetallbhelper.CurlMlbPod(*testPod, metallbIPList[0])).Should(ContainSubstring("html"),
				"Curl was unable to connect to nginx")
		})
	})

})
