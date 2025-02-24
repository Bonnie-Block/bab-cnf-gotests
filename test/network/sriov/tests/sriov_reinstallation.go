package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	admregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CNF SRIOV", Ordered, func() {

	// Don't run these test cases if CNF_GOTESTS_SRIOV_SMOKE is true
	if !sriovSmokeTestMode {

		var (
			sriovInterfaces       []*sriovv1.InterfaceExt
			sriovCrdsList         []*v1.CustomResourceDefinition
			clientPod             *corev1.Pod
			connectivityScenarion string
		)
		execute.BeforeAll(func() {
			netsriovhelper.VerifySriovOperatorInstalledAndPreconfigured(namespace, operatorGroup, sriovSubscription)

			By("Collect info about installed SR-IOV operator")
			sriovInfos, err := cluster.DiscoverSriov(
				helper.Apiclient,
				parameters.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(sriovInfos.Nodes)).To(BeNumerically(">=", 1))
			connectivityScenarion = netsriovparameters.ConnectivitySameNodeSamePF
			if len(sriovInfos.Nodes) > 1 {
				connectivityScenarion = netsriovparameters.ConnectivityDiffNodeDiffPF
			}
			sriovInterfaces, err = sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
			Expect(err).ToNot(HaveOccurred())

			By("Validate that test namespace exist")
			Expect(namespaces.Exists(netsriovparameters.OperatorTestNamespace, helper.Apiclient)).To(BeTrue())
			Expect(namespaces.CleanPods(netsriovparameters.OperatorTestNamespace, helper.Apiclient)).ToNot(HaveOccurred())

			By("Run Client and Server pods")
			netsriovhelper.RunServerPod(
				netsriovparameters.CommunicationProtocolUnicastICMP, netsriovparameters.MTUStandard,
				connectivityScenarion, sriovInfos, helper.Config,
				netsriovparameters.SriovStaticNetworkUsualMTUName, nil, false,
				netsriovparameters.ServerMacAddress, netsriovparameters.ServerPodIP,
				netsriovparameters.TestInterfaceName, netsriovparameters.IpamStatic)

			clientPodDefinition := netsriovhelper.DefineClientPod(netsriovparameters.CommunicationProtocolUnicastICMP,
				sriovInfos.Nodes, netsriovparameters.SriovStaticNetworkUsualMTUName, nil,
				netsriovparameters.ClientPodIP, netsriovparameters.ClientMacAddress,
				helper.Config.Network.TestContainerImage, parameters.SleepCommand, netsriovparameters.IpamStatic,
				netsriovparameters.TestInterfaceName)

			clientPod, err = helper.Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
				context.Background(),
				clientPodDefinition,
				metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			netsriovhelper.WaitUntilPodInStatus(
				clientPod,
				"Client",
				parameters.SleepCommand,
				corev1.PodRunning,
				netsriovparameters.PodWaitingTime)
		})

		// 46528
		It("Operator re-installation. Verify SR-IOV operator control plane is operational before removal",
			polarion.ID("46528"), func() {
				sriovPolicies, err := helper.Apiclient.SriovNetworkNodePolicies(parameters.SriovOperatorNamespace).List(
					context.Background(), metav1.ListOptions{})
				Expect(err).ToNot(HaveOccurred())
				Expect(len(sriovPolicies.Items)).Should(BeNumerically(">", 1))
				sriovPolicyInstalled := false
				for _, sriovPolicy := range sriovPolicies.Items {
					if strings.Contains(sriovPolicy.Name, netsriovparameters.SriovNetworkPolicyMTUUsual) {
						sriovPolicyInstalled = true
					}
				}
				Expect(sriovPolicyInstalled).To(BeTrue())

				sriovNetworks, err := helper.Apiclient.SriovNetworks(
					parameters.SriovOperatorNamespace).List(context.Background(), metav1.ListOptions{})
				Expect(err).ToNot(HaveOccurred())
				sriovNetworkInstalled := false
				for _, sriovNetwork := range sriovNetworks.Items {
					if strings.Contains(sriovNetwork.Name, netsriovparameters.SriovStaticNetworkUsualMTUNameDiff) {
						sriovNetworkInstalled = true
					}
				}
				Expect(sriovNetworkInstalled).To(BeTrue())
			})

		// 46529
		It("Operator re-installation. Verify SR-IOV operator data plane is operational before removal",
			polarion.ID("46529"), func() {
				clientPodCommand, err := netsriovhelper.DefineTestCommandParameters(
					false, netsriovparameters.CommunicationProtocolUnicastICMP, netsriovparameters.MTUStandard,
					netsriovparameters.ServerPodIP, 0, netsriovparameters.TestInterfaceName)
				Expect(err).ToNot(HaveOccurred())

				By("Test connectivity")
				_, err = pod.ExecCommand(helper.Apiclient, *clientPod, clientPodCommand)
				Expect(err).ToNot(HaveOccurred())
			})

		// 46530
		It("Operator re-installation. Verify all SR-IOV components are deleted when operator is removed",
			polarion.ID("46530"), func() {

				By("Clean all SR-IOV policies and networks")
				err := namespaces.Clean(parameters.SriovOperatorNamespace, netsriovparameters.OperatorTestNamespace,
					helper.Apiclient, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(netsriovhelper.IsSriovPreConfigured()).To(BeFalse())

				By("Check if cluster is SNO")
				isSingleNode, err := nodes.IsSingleNodeCluster(helper.Apiclient)
				Expect(err).ToNot(HaveOccurred())

				var snoTimeoutMultiplier time.Duration = 1
				if isSingleNode {
					snoTimeoutMultiplier = 2
				}

				By("Wait until SR-IOV cluster is stable ")
				helper.WaitForSRIOVStable(parameters.SriovOperatorNamespace, netsriovparameters.WaitingTime, snoTimeoutMultiplier)

				By("Remove SR-IOV operator config")
				err = helper.Apiclient.SriovOperatorConfigs(parameters.SriovOperatorNamespace).
					Delete(context.Background(), "default", metav1.DeleteOptions{})
				Expect(err).ToNot(HaveOccurred(), "Failed to remove default SriovOperatorConfig")

				By(fmt.Sprintf("Waiting for MutatingWebhooks removal: %v",
					netsriovparameters.SriovMutationWebhooks))
				for _, sriovMutationWebhookName := range netsriovparameters.SriovMutationWebhooks {
					Eventually(func() error {
						mutatingWebhookConfiguration := &admregv1.MutatingWebhookConfiguration{}
						err := helper.Apiclient.Get(context.TODO(), goclient.ObjectKey{
							Name: sriovMutationWebhookName,
						}, mutatingWebhookConfiguration)

						return err
					}, 2*time.Minute, netsriovparameters.SriovOperatorDeploymentRetry).Should(HaveOccurred(),
						fmt.Sprintf("MutationWebhook %s was not removed", sriovMutationWebhookName))
				}

				By(fmt.Sprintf("Waiting for ValidatingWebhook removal: %s",
					netsriovparameters.SriovValidationWebhook))
				Eventually(func() error {
					validatingWebhookConfiguration := &admregv1.ValidatingWebhookConfiguration{}
					err := helper.Apiclient.Get(context.TODO(), goclient.ObjectKey{
						Name: netsriovparameters.SriovValidationWebhook,
					}, validatingWebhookConfiguration)

					return err
				}, 2*time.Minute, netsriovparameters.SriovOperatorDeploymentRetry).Should(HaveOccurred(),
					fmt.Sprintf("ValidatingWebhook %s was not removed", netsriovparameters.SriovValidationWebhook))

				By("Remove sriov subscription")
				err = helper.Apiclient.Subscriptions(
					parameters.SriovOperatorNamespace).Delete(context.Background(), sriovSubscription.Name, metav1.DeleteOptions{})
				Expect(err).ToNot(HaveOccurred())
				By("Remove SR-IOV CSV")
				csvs, err := helper.Apiclient.ClusterServiceVersions(
					parameters.SriovOperatorNamespace).List(context.Background(), metav1.ListOptions{})
				Expect(err).ToNot(HaveOccurred())
				var sriovCsv string
				for _, csv := range csvs.Items {
					if strings.Contains(csv.Name, "sriov-network-operator") {
						sriovCsv = csv.Name
					}
				}
				Expect(sriovCsv).ToNot(BeEmpty())

				err = helper.Apiclient.ClusterServiceVersions(
					parameters.SriovOperatorNamespace).Delete(context.Background(), sriovCsv, metav1.DeleteOptions{})
				Expect(err).ToNot(HaveOccurred())

				By("Remove SR-IOV CRDs")
				for _, crdName := range netsriovparameters.SriovCrds {
					crd := &v1.CustomResourceDefinition{}
					err = helper.Apiclient.Get(context.Background(), goclient.ObjectKey{Name: crdName}, crd)
					Expect(err).ToNot(HaveOccurred())
					sriovCrdsList = append(sriovCrdsList, crd)
				}

				for _, crds := range sriovCrdsList {
					err := helper.Apiclient.Delete(context.Background(), crds)
					Expect(err).ToNot(HaveOccurred())
				}

				By("Remove SR-IOV namespace")
				err = namespaces.DeleteAndWait(helper.Apiclient, parameters.SriovOperatorNamespace, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred())
			})

		// 46531
		It("Operator re-installation. Validate that SR-IOV resources can not be deployed without SR-IOV operator",
			polarion.ID("46531"),
			func() {

				By("Validate that SR-IOV operator namespace was removed")
				_, err := helper.Apiclient.Namespaces().Get(
					context.Background(), parameters.SriovOperatorNamespace, metav1.GetOptions{})
				Expect(err).To(HaveOccurred())

				By("Validate that SR-IOV api doesn't work")
				tmpSriovPolicy := helper.DefineSriovPolicy(
					"test-policy", parameters.SriovOperatorNamespace, sriovInterfaces[0], 5,
					"0-1", 1500, "testresourceusual", "netdevice")
				_, err = helper.Apiclient.SriovNetworkNodePolicies(parameters.SriovOperatorNamespace).Create(
					context.Background(),
					tmpSriovPolicy,
					metav1.CreateOptions{})
				Expect(err).To(HaveOccurred())
				_, err = helper.Apiclient.SriovNetworks(netsriovparameters.OperatorTestNamespace).Create(
					context.Background(),
					netsriovhelper.DefineSriovNetwork("test-network", "testresourceusual"),
					metav1.CreateOptions{},
				)
				Expect(err).To(HaveOccurred())
			})

		// 46532
		It("Operator re-installation. Validate that re-installed SR-IOV operator’s control plane is up and running.",
			polarion.ID("46532"),
			func() {

				By("Deploy SR-IOV operator namespace")
				Expect(namespaces.Exists(parameters.SriovOperatorNamespace, helper.Apiclient)).To(BeFalse())

				By("Deploy SR-IOV operator")
				err := netsriovhelper.DeploySriovOperator(namespace, &operatorGroup, sriovSubscription)
				Expect(err).ToNot(HaveOccurred())

				By("Deploy SR-IOV operator config")
				Eventually(func() error {
					_, err := helper.IsDeploymentInstalled(
						helper.Apiclient, parameters.SriovOperatorNamespace, netsriovparameters.SriovOperatorDeploymentName)

					return err
				}, netsriovparameters.SriovOperatorDeploymentTime, netsriovparameters.SriovOperatorDeploymentRetry).
					ShouldNot(HaveOccurred(), fmt.Sprintf("sriov deployment %s is not ready",
						netsriovparameters.SriovOperatorDeploymentName))

				err = helper.Apiclient.Create(context.Background(), netsriovhelper.DefineOperatorConfig())
				Expect(err).ToNot(HaveOccurred(), "Failed to apply sriov operator config")

				By("Validate that SR-IOV operator installed with all the components")
				Eventually(
					netsriovhelper.IsSriovOperatorInstalled,
					netsriovparameters.SriovOperatorDeploymentTime,
					netsriovparameters.SriovOperatorDeploymentRetry).ShouldNot(HaveOccurred())
				Eventually(func() error {
					_, err = cluster.DiscoverSriov(helper.Apiclient, parameters.SriovOperatorNamespace)

					return err
				}, netsriovparameters.SriovOperatorDeploymentTime,
					netsriovparameters.SriovOperatorDeploymentRetry).ShouldNot(HaveOccurred())

				mutationWebHook := admregv1.MutatingWebhookConfigurationList{}
				err = helper.Apiclient.List(context.Background(), &mutationWebHook)
				Expect(err).ToNot(HaveOccurred())
				isWebhookResourceInjectorPolicyIgnore := false
				isSriovMutationWebhooksPolicyFail := false
				for _, each := range mutationWebHook.Items {
					if elementInList(netsriovparameters.SriovMutationWebhooks, each.Name) {
						if each.Name == netsriovparameters.SriovWebhookResourceInjector {
							Expect(*each.Webhooks[0].FailurePolicy).To(BeIdenticalTo(admregv1.FailurePolicyType("Ignore")))
							isWebhookResourceInjectorPolicyIgnore = true
						}
						if each.Name == netsriovparameters.SriovWebhookOperator {
							Expect(*each.Webhooks[0].FailurePolicy).To(BeIdenticalTo(admregv1.FailurePolicyType("Fail")))
							isSriovMutationWebhooksPolicyFail = true
						}
					}
				}
				Expect(isWebhookResourceInjectorPolicyIgnore).To(BeTrue())
				Expect(isSriovMutationWebhooksPolicyFail).To(BeTrue())

				isValidationWebhookConfigFail := false
				validationWebhookConfig := admregv1.ValidatingWebhookConfigurationList{}
				err = helper.Apiclient.List(context.Background(), &validationWebhookConfig)
				for _, each := range validationWebhookConfig.Items {
					if each.Name == netsriovparameters.SriovValidationWebhook {
						Expect(*each.Webhooks[0].FailurePolicy).To(BeIdenticalTo(admregv1.FailurePolicyType("Fail")))
						isValidationWebhookConfigFail = true
					}
				}
				Expect(isValidationWebhookConfigFail).To(BeTrue())

				By("Configure SR-IOV operator")
				netsriovhelper.SriovPreConfiguration()
			})

		// 46533
		It("Operator re-installation. Validate that re-installed SR-IOV operator’s data plane is up and running",
			polarion.ID("46533"),
			func() {
				By("Run Server pod")
				sriovInfos, err := cluster.DiscoverSriov(helper.Apiclient, parameters.SriovOperatorNamespace)
				Expect(err).ToNot(HaveOccurred())
				netsriovhelper.RunServerPod(
					netsriovparameters.CommunicationProtocolUnicastICMP, netsriovparameters.MTUStandard,
					connectivityScenarion, sriovInfos, helper.Config,
					netsriovparameters.SriovStaticNetworkUsualMTUName, nil, false,
					netsriovparameters.ServerMacAddress, netsriovparameters.ServerPodIP,
					netsriovparameters.TestInterfaceName, netsriovparameters.IpamStatic)

				By("Run Client pod")
				clientPodDefinition := netsriovhelper.DefineClientPod(
					netsriovparameters.CommunicationProtocolUnicastICMP, sriovInfos.Nodes,
					netsriovparameters.SriovStaticNetworkUsualMTUName, nil, netsriovparameters.ClientPodIP,
					netsriovparameters.ClientMacAddress, helper.Config.Network.TestContainerImage,
					parameters.SleepCommand, netsriovparameters.IpamStatic, netsriovparameters.TestInterfaceName)

				clientPod, err := helper.Apiclient.Pods(netsriovparameters.OperatorTestNamespace).Create(
					context.Background(),
					clientPodDefinition,
					metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
				netsriovhelper.WaitUntilPodInStatus(
					clientPod,
					"Client",
					parameters.SleepCommand,
					corev1.PodRunning,
					netsriovparameters.PodWaitingTime)
				clientPodCommand, err := netsriovhelper.DefineTestCommandParameters(
					false, netsriovparameters.CommunicationProtocolUnicastICMP, netsriovparameters.MTUStandard,
					netsriovparameters.ServerPodIP, 0, netsriovparameters.TestInterfaceName)
				Expect(err).ToNot(HaveOccurred())

				By("Test connectivity")
				_, err = pod.ExecCommand(helper.Apiclient, *clientPod, clientPodCommand)
				Expect(err).ToNot(HaveOccurred())
			})
	}

})

func elementInList(elements []string, element string) bool {
	for _, oneElement := range elements {
		if oneElement == element {
			return true
		}
	}

	return false
}
