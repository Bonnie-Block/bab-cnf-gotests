package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/policy/netpolicyhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/policy/netpolicyparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nad"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	cniTypes "github.com/containernetworking/cni/pkg/types"
	multinetpolicyapiv1 "github.com/k8snetworkplumbingwg/multi-networkpolicy/pkg/apis/k8s.cni.cncf.io/v1beta1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("MultiNetworkPolicy sriov", func() {
	var (
		testSetupFail                              = true
		workerNodeList                             []corev1.Node
		serverPod, firstClientPod, secondClientPod *corev1.Pod
	)

	execute.BeforeAll(func() {
		By("Setup SRIOV configuration")
		var err error
		workerNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(workerNodeList)).To(BeNumerically(">", 1))

		sriovInfos, err := cluster.DiscoverSriov(helper.Apiclient, parameters.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "error to discover SR-IOV interfaces")
		setupSriovBeforeAll(sriovInfos, "static")

		testSetupFail = false
	})

	BeforeEach(func() {
		if testSetupFail {
			Fail("Test failed due to error in BeforeAll")
		}
		By("Enable MultiNetworkPolicy feature if needed")
		netpolicyhelper.EnableMultiNetworkPolicy()

		By("Remove configuration before the test")
		removeConfigurationBeforeTest()
	})

	Context("NonVRF", func() {
		BeforeEach(func() {
			By("Create test pods")
			firstClientPod = createClientPod(netpolicyparameters.SriovNetworkName, workerNodeList[1].Name,
				netpolicyparameters.Pod2IPAddress, netpolicyparameters.LabelPod2Name)
			secondClientPod = createClientPod(netpolicyparameters.SriovNetworkName, workerNodeList[1].Name,
				netpolicyparameters.Pod3IPAddress, netpolicyparameters.LabelPod3Name)
			serverPod = createServerPod(netpolicyparameters.SriovNetworkName, workerNodeList[0].Name)
		})

		AfterEach(func() {
			By("Remove MultiNetworkPolicies")
			err := netpolicyhelper.CleanMultiNetworkPoliciesFromNamespace(netpolicyparameters.TestNamespace)
			Expect(err).ToNot(HaveOccurred(), "failed to delete MultiNetworkPolicy")

			By("Traffic verification after MultiNetworkPolicy removal")
			err = netpolicyhelper.RunTCPTraffic(firstClientPod, netpolicyparameters.Pod1IPAddress, netpolicyparameters.Port5001)
			Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("pod %s can NOT reach %s with port %s",
				firstClientPod.Name, serverPod.Name, netpolicyparameters.Port5001))
		})

		// 53901
		It("Ingress Default rule without PolicyType deny all", func() {
			By("Apply MultiNetworkPolicy with ingress rule deny all without PolicyType field")
			netpolicyhelper.MakeMultiNetworkPolicy(netpolicyparameters.TestNamespace, netpolicyparameters.SriovNetworkName,
				netpolicyhelper.WithPodSelector(netpolicyparameters.LabelSelectorPod1),
				netpolicyhelper.WithEmptyIngressRules(),
				netpolicyhelper.CreatePolicy(),
			)

			By("Traffic verification")
			// All traffic should be blocked to the serverPod
			err := netpolicyhelper.RunTCPTraffic(firstClientPod, netpolicyparameters.Pod1IPAddress, netpolicyparameters.Port5001)
			Expect(err).Should(HaveOccurred(), fmt.Sprintf("unexpectedly pod %s can reach %s with port %s",
				firstClientPod.Name, serverPod.Name, netpolicyparameters.Port5001))

			err = netpolicyhelper.RunTCPTraffic(secondClientPod, netpolicyparameters.Pod1IPAddress, netpolicyparameters.Port5003)
			Expect(err).Should(HaveOccurred(), fmt.Sprintf("unexpectedly pod %s can reach %s with port %s",
				secondClientPod.Name, serverPod.Name, netpolicyparameters.Port5003))

			// Traffic between firstClientPod and secondClientPod should not be affected (not blocked)
			err = netpolicyhelper.RunTCPTraffic(secondClientPod, netpolicyparameters.Pod2IPAddress, netpolicyparameters.Port5001)
			Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("pod %s can NOT reach %s with port %s",
				secondClientPod.Name, firstClientPod.Name, netpolicyparameters.Port5001))
		})

		// 53899
		// The test fails due to OCPBUGS-974
		It("Ingress Default rule without PolicyType allow all", func() {
			By("Apply MultiNetworkPolicy with ingress rule allow all without PolicyType field")
			netpolicyhelper.MakeMultiNetworkPolicy(netpolicyparameters.TestNamespace, netpolicyparameters.SriovNetworkName,
				netpolicyhelper.WithPodSelector(metav1.LabelSelector{}),
				netpolicyhelper.WithIngressRule(multinetpolicyapiv1.MultiNetworkPolicyIngressRule{}),
				netpolicyhelper.CreatePolicy(),
			)

			By("Traffic verification")
			// All traffic is accepted
			err := netpolicyhelper.RunTCPTraffic(firstClientPod, netpolicyparameters.Pod1IPAddress, netpolicyparameters.Port5001)
			Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("pod %s can NOT reach %s with port %s",
				firstClientPod.Name, serverPod.Name, netpolicyparameters.Port5001))

			err = netpolicyhelper.RunTCPTraffic(secondClientPod, netpolicyparameters.Pod1IPAddress, netpolicyparameters.Port5001)
			Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("pod %s can NOT reach %s with port %s",
				secondClientPod.Name, serverPod.Name, netpolicyparameters.Port5001))

			err = netpolicyhelper.RunTCPTraffic(firstClientPod, netpolicyparameters.Pod3IPAddress, netpolicyparameters.Port5001)
			Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("pod %s can NOT reach %s with port %s",
				firstClientPod.Name, secondClientPod.Name, netpolicyparameters.Port5001))
		})

		// 53900
		It("Egress TCP endPort allow specific pod", func() {
			By("Apply MultiNetworkPolicy with egress rule allow ports in range 5000-5002")
			var (
				policyPort5001 = intstr.FromInt(5001)
				protoTCP       = corev1.ProtocolTCP
			)
			netpolicyhelper.MakeMultiNetworkPolicy(netpolicyparameters.TestNamespace, netpolicyparameters.SriovNetworkName,
				netpolicyhelper.WithPodSelector(netpolicyparameters.LabelSelectorPod2),
				netpolicyhelper.WithPolicyTypes([]multinetpolicyapiv1.MultiPolicyType{multinetpolicyapiv1.PolicyTypeEgress}),
				netpolicyhelper.WithEgressRule(multinetpolicyapiv1.MultiNetworkPolicyEgressRule{
					To: []multinetpolicyapiv1.MultiNetworkPolicyPeer{{
						PodSelector: &netpolicyparameters.LabelSelectorPod1,
					}},
					Ports: []multinetpolicyapiv1.MultiNetworkPolicyPort{{
						// Remove comment and delete Port5001 when the bug OCPBUGS-975 is fixed
						//Port:     &policyPort5000,
						//EndPort:  &policyPort5002,
						Port:     &policyPort5001,
						Protocol: &protoTCP,
					}},
				}),
				netpolicyhelper.CreatePolicy(),
			)

			By("Traffic verification")
			// Traffic from firstClientPod to serverPod with port range 5000-5002 should pass.
			err := netpolicyhelper.RunTCPTraffic(firstClientPod, netpolicyparameters.Pod1IPAddress, netpolicyparameters.Port5001)
			Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("pod %s can NOT reach %s with port %s",
				firstClientPod.Name, serverPod.Name, netpolicyparameters.Port5001))

			// Port 5003 is out of the accepted port range. Traffic should be dropped.
			err = netpolicyhelper.RunTCPTraffic(firstClientPod, netpolicyparameters.Pod1IPAddress, netpolicyparameters.Port5003)
			Expect(err).Should(HaveOccurred(), fmt.Sprintf("unexpectedly pod %s can reach %s with port %s",
				firstClientPod.Name, serverPod.Name, netpolicyparameters.Port5003))

			// Traffic between firstClientPod and secondClientPod is not allowed
			err = netpolicyhelper.RunTCPTraffic(firstClientPod, netpolicyparameters.Pod3IPAddress, netpolicyparameters.Port5001)
			Expect(err).Should(HaveOccurred(), fmt.Sprintf("unexpectedly pod %s can reach %s with port %s",
				firstClientPod.Name, secondClientPod.Name, netpolicyparameters.Port5001))

			// Traffic between secondClientPod and serverPod is not affected by rule.
			err = netpolicyhelper.RunTCPTraffic(secondClientPod, netpolicyparameters.Pod1IPAddress, netpolicyparameters.Port5001)
			Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("pod %s can NOT reach %s with port %s",
				secondClientPod.Name, serverPod.Name, netpolicyparameters.Port5001))

			err = netpolicyhelper.RunTCPTraffic(secondClientPod, netpolicyparameters.Pod1IPAddress, netpolicyparameters.Port5003)
			Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("pod %s can NOT reach %s with port %s",
				secondClientPod.Name, serverPod.Name, netpolicyparameters.Port5003))
		})

		// 53898
		It("Ingress and Egress allow IPv4 address", func() {
			By("Apply MultiNetworkPolicy with ingress and egress rules allow specific IPv4 addresses")
			netpolicyhelper.MakeMultiNetworkPolicy(netpolicyparameters.TestNamespace, netpolicyparameters.SriovNetworkName,
				netpolicyhelper.WithPodSelector(netpolicyparameters.LabelSelectorPod1),
				netpolicyhelper.WithPolicyTypes([]multinetpolicyapiv1.MultiPolicyType{multinetpolicyapiv1.PolicyTypeEgress,
					multinetpolicyapiv1.PolicyTypeIngress}),
				netpolicyhelper.WithIngressRule(multinetpolicyapiv1.MultiNetworkPolicyIngressRule{
					From: []multinetpolicyapiv1.MultiNetworkPolicyPeer{{
						IPBlock: &multinetpolicyapiv1.IPBlock{
							CIDR: netpolicyparameters.Pod2IPAddress + "/" + netparameters.IPSubnet32,
						},
					}},
				}),
				netpolicyhelper.WithEgressRule(multinetpolicyapiv1.MultiNetworkPolicyEgressRule{
					To: []multinetpolicyapiv1.MultiNetworkPolicyPeer{{
						IPBlock: &multinetpolicyapiv1.IPBlock{
							CIDR: netpolicyparameters.Pod3IPAddress + "/" + netparameters.IPSubnet32,
						},
						PodSelector: &netpolicyparameters.LabelSelectorPod3,
					}},
				}),
				netpolicyhelper.CreatePolicy(),
			)

			By("Traffic verification")
			// Traffic from firstClientPod to serverPod with source IP netpolicyparameters.Pod2IPAddress should pass
			err := netpolicyhelper.RunTCPTraffic(firstClientPod, netpolicyparameters.Pod1IPAddress, netpolicyparameters.Port5001)
			Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("pod %s can NOT reach %s with port %s",
				firstClientPod.Name, serverPod.Name, netpolicyparameters.Port5001))

			// Traffic from serverPod to secondClientPod with destination IP netpolicyparameters.Pod3IPAddress should pass
			err = netpolicyhelper.RunTCPTraffic(serverPod, netpolicyparameters.Pod3IPAddress, netpolicyparameters.Port5001)
			Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("pod %s can NOT reach %s with port %s",
				serverPod.Name, secondClientPod.Name, netpolicyparameters.Port5001))

			// All other traffic should be dropped
			err = netpolicyhelper.RunTCPTraffic(secondClientPod, netpolicyparameters.Pod1IPAddress, netpolicyparameters.Port5001)
			Expect(err).Should(HaveOccurred(), fmt.Sprintf("unexpectedly pod %s can reach %s with port %s",
				secondClientPod.Name, serverPod.Name, netpolicyparameters.Port5001))

			err = netpolicyhelper.RunTCPTraffic(serverPod, netpolicyparameters.Pod2IPAddress, netpolicyparameters.Port5001)
			Expect(err).Should(HaveOccurred(), fmt.Sprintf("unexpectedly pod %s can reach %s with port %s",
				serverPod.Name, firstClientPod.Name, netpolicyparameters.Port5001))
		})

		It("Disable multi-network policy", func() {
			By("Apply MultiNetworkPolicy with ingress rule deny all")
			netpolicyhelper.MakeMultiNetworkPolicy(netpolicyparameters.TestNamespace, netpolicyparameters.SriovNetworkName,
				netpolicyhelper.WithPodSelector(netpolicyparameters.LabelSelectorPod1),
				netpolicyhelper.WithPolicyTypes([]multinetpolicyapiv1.MultiPolicyType{multinetpolicyapiv1.PolicyTypeIngress}),
				netpolicyhelper.WithEmptyIngressRules(),
				netpolicyhelper.CreatePolicy(),
			)

			By("Traffic verification")
			// All traffic should be blocked to the serverPod
			err := netpolicyhelper.RunTCPTraffic(firstClientPod, netpolicyparameters.Pod1IPAddress, netpolicyparameters.Port5001)
			Expect(err).Should(HaveOccurred(), fmt.Sprintf("unexpectedly pod %s can reach %s with port %s",
				firstClientPod.Name, serverPod.Name, netpolicyparameters.Port5001))

			err = netpolicyhelper.RunTCPTraffic(secondClientPod, netpolicyparameters.Pod1IPAddress, netpolicyparameters.Port5003)
			Expect(err).Should(HaveOccurred(), fmt.Sprintf("unexpectedly pod %s can reach %s with port %s",
				secondClientPod.Name, serverPod.Name, netpolicyparameters.Port5003))

			// Traffic between firstClientPod and secondClientPod should not be affected (not blocked)
			err = netpolicyhelper.RunTCPTraffic(secondClientPod, netpolicyparameters.Pod2IPAddress, netpolicyparameters.Port5001)
			Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("pod %s can NOT reach %s with port %s",
				secondClientPod.Name, firstClientPod.Name, netpolicyparameters.Port5001))

			By("Disable MultiNetworkPolicy feature")
			netpolicyhelper.DisableMultiNetworkPolicy()

			By("Traffic verification with MultiNetworkPolicy disabled")
			// All traffic is accepted and there is no any policy because feature is off
			err = netpolicyhelper.RunTCPTraffic(firstClientPod, netpolicyparameters.Pod1IPAddress,
				netpolicyparameters.Port5001)
			Expect(err).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %s", secondClientPod.Name, serverPod.Name,
					netpolicyparameters.Port5001))

			err = netpolicyhelper.RunTCPTraffic(secondClientPod, netpolicyparameters.Pod1IPAddress, netpolicyparameters.Port5003)
			Expect(err).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %s", secondClientPod.Name, serverPod.Name,
					netpolicyparameters.Port5003))

			By("Applying MultiNetworkPolicy should fail")
			netpolicyhelper.MakeMultiNetworkPolicy(netpolicyparameters.TestNamespace, netpolicyparameters.SriovNetworkName,
				netpolicyhelper.WithPolicyTypes([]multinetpolicyapiv1.MultiPolicyType{multinetpolicyapiv1.PolicyTypeIngress}),
				netpolicyhelper.FailCreatePolicy())

			By("Enable MultiNetworkPolicy feature")
			netpolicyhelper.EnableMultiNetworkPolicy()
		})
	})

	// 53897
	// Remove comment when the bug OCPBUGS-976 is fixed

	// Context("VRF", func() {
	//	BeforeEach(func() {
	//		firstClientPod = createClientPod(netpolicyparameters.SriovNetworkNameWithVRF, workerNodeList[1].Name,
	//			netpolicyparameters.Pod2IPAddress, netpolicyparameters.LabelPod2Name)
	//		secondClientPod = createClientPod(netpolicyparameters.SriovNetworkNameWithVRF, workerNodeList[1].Name,
	//			netpolicyparameters.Pod3IPAddress, netpolicyparameters.LabelPod3Name)
	//		serverPod = createServerPod(netpolicyparameters.SriovNetworkNameWithVRF, workerNodeList[0].Name)
	//	})
	//
	//	It("Ingress Default rule deny all", func() {
	//		By("Apply MultiNetworkPolicy with ingress rule deny all traffic")
	//		netpolicyhelper.MakeMultiNetworkPolicy(netpolicyparameters.TestNamespace, netpolicyparameters.SriovNetworkName,
	//			netpolicyhelper.WithPodSelector(netpolicyparameters.LabelSelectorPod1),
	//			netpolicyhelper.WithPolicyTypes([]multinetpolicyapiv1.MultiPolicyType{multinetpolicyapiv1.PolicyTypeIngress}),
	//			netpolicyhelper.WithEmptyIngressRules(),
	//			netpolicyhelper.CreatePolicy(),
	//		)
	//
	//		By("Traffic verification")
	//		// All traffic should be blocked to the serverPod
	//		err := netpolicyhelper.PingIPViaInterface(firstClientPod, netpolicyparameters.Pod1IPAddress,
	//			netpolicyparameters.VRFName)
	//		Expect(err).Should(HaveOccurred(),
	//			fmt.Sprintf("unexpectedly pod %s can reach %s via ping", firstClientPod.Name, serverPod.Name))
	//
	//		err = netpolicyhelper.PingIPViaInterface(secondClientPod, netpolicyparameters.Pod1IPAddress,
	//			netpolicyparameters.VRFName)
	//		Expect(err).Should(HaveOccurred(),
	//			fmt.Sprintf("unexpectedly pod %s can reach %s via ping", secondClientPod.Name, serverPod.Name))
	//
	//		// Traffic between firstClientPod and secondClientPod should not be affected (not blocked)
	//		err = netpolicyhelper.PingIPViaInterface(secondClientPod, netpolicyparameters.Pod2IPAddress,
	//			netpolicyparameters.VRFName)
	//		Expect(err).ShouldNot(HaveOccurred(),
	//			fmt.Sprintf("pod %s can NOT reach %s via ping", secondClientPod.Name, firstClientPod.Name))
	// })
	// })
})

// setupSriovBeforeAll prepares sriov before tests.
func setupSriovBeforeAll(sriovInfos *cluster.EnabledNodes, ipamType string) {
	validSriovInterfaces := nethelper.GatherSriovInterfaces(sriovInfos, helper.Config, 1)

	By(fmt.Sprintf("Clean test namespace %s", netpolicyparameters.TestNamespace))
	err := namespaces.Clean(
		parameters.SriovOperatorNamespace,
		netpolicyparameters.TestNamespace,
		helper.Apiclient,
		false)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error to clean namespace %s", netpolicyparameters.TestNamespace))

	By("Waiting until SRIOV become stable")
	helper.WaitForSRIOVStable(parameters.SriovOperatorNamespace, netpolicyparameters.WaitingTime, 1)

	By("Define sr-iov Policies")
	nethelper.DefineAndCreateSriovPoliciesListOnSriovInterfaceList([]string{netpolicyparameters.ResourceNamePolicy},
		validSriovInterfaces, 5)

	By("Define sr-iov network ipam config")

	ipam, err := nethelper.MarshalTypeToString(cniTypes.IPAM{Type: ipamType})
	Expect(err).ToNot(HaveOccurred(), "error to marshal ipam to string")

	By("Define and create sr-iov network")

	err = nethelper.CreateSriovNetwork(
		helper.Apiclient,
		validSriovInterfaces[0],
		netpolicyparameters.SriovNetworkName,
		netpolicyparameters.TestNamespace,
		parameters.SriovOperatorNamespace,
		netpolicyparameters.ResourceNamePolicy,
		ipam)
	Expect(err).ToNot(HaveOccurred(), "error creating sr-iov network")

	By("Define sr-iov network cni vrf plugin config")

	vrfPlugin, err := nethelper.MarshalTypeToString(nad.DefineVrfPlugin(netpolicyparameters.VRFName))
	Expect(err).ToNot(HaveOccurred(), "error to marshal vrf to string")

	By("Define and create sr-iov network with vrf")
	nethelper.DefineAndCreateSriovNetwork(validSriovInterfaces[0],
		netpolicyparameters.SriovNetworkNameWithVRF, netpolicyparameters.ResourceNamePolicy, ipam, vrfPlugin,
		netpolicyparameters.TestNamespace)

	By("Waiting until SRIOV become stable")
	helper.WaitForSRIOVStable(parameters.SriovOperatorNamespace, netpolicyparameters.WaitingTime, 1)
}

func removeConfigurationBeforeTest() {
	err := namespaces.CleanPodAndWaitUntilItsEmpty(helper.Apiclient, netpolicyparameters.TestNamespace)
	Expect(err).ToNot(HaveOccurred(), "failed to clean test namespace")
	err = netpolicyhelper.CleanMultiNetworkPoliciesFromNamespace(netpolicyparameters.TestNamespace)
	Expect(err).ToNot(HaveOccurred(), "failed to delete MultiNetworkPolicy")
}

func createServerPod(sriovNetwork, nodeName string) *corev1.Pod {
	serverPodAnnotation, err := netpolicyhelper.DefinePodAnnotation(sriovNetwork,
		netpolicyparameters.Pod1IPAddress)
	Expect(err).ToNot(HaveOccurred(), "failed to define pod annotation for serverPod")

	return netpolicyhelper.CreateServerPod(nodeName, netpolicyparameters.LabelPod1Name,
		serverPodAnnotation, defineInitContainerCommand(sriovNetwork))
}

func createClientPod(sriovNetwork, nodeName, ipaddress, label string) *corev1.Pod {
	clientPodAnnotation, err := netpolicyhelper.DefinePodAnnotation(sriovNetwork,
		ipaddress)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("failed to define pod annotation for clientPod with IPAddress: %s", ipaddress))

	return netpolicyhelper.CreateClientPod(nodeName, label,
		clientPodAnnotation)
}

func defineInitContainerCommand(sriovNetwork string) []string {
	if sriovNetwork == netpolicyparameters.SriovNetworkNameWithVRF {
		return []string{"bash", "-c",
			fmt.Sprintf("ping -I %s %s -c 3 -w 90 && ping -I %s %s -c 3 -w 90",
				netpolicyparameters.VRFName, netpolicyparameters.Pod2IPAddress,
				netpolicyparameters.VRFName, netpolicyparameters.Pod3IPAddress)}
	}

	return []string{"bash", "-c",
		fmt.Sprintf("ping %s -c 3 -w 90 && ping %s -c 3 -w 90",
			netpolicyparameters.Pod2IPAddress, netpolicyparameters.Pod3IPAddress)}
}
