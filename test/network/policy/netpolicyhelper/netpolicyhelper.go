package netpolicyhelper

import (
	"fmt"

	. "github.com/onsi/gomega"
	"golang.org/x/net/context"

	multinetpolicyapiv1 "github.com/k8snetworkplumbingwg/multi-networkpolicy/pkg/apis/k8s.cni.cncf.io/v1beta1"
	operv1 "github.com/openshift/api/operator/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/policy/netpolicyparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type MultiNetworkPolicyOpt func(*multinetpolicyapiv1.MultiNetworkPolicy)

// MakeMultiNetworkPolicy defines MultiNetworkPolicy CR with provided namespace.
func MakeMultiNetworkPolicy(nameSpace, targetNetwork string,
	opts ...MultiNetworkPolicyOpt) *multinetpolicyapiv1.MultiNetworkPolicy {
	ret := multinetpolicyapiv1.MultiNetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-multinetworkpolicy-",
			Namespace:    nameSpace,
			Annotations: map[string]string{
				"k8s.v1.cni.cncf.io/policy-for": targetNetwork,
			},
		},
	}

	for _, opt := range opts {
		opt(&ret)
	}

	return &ret
}

// WithPodSelector adds podSelector option to the MultiNetworkPolicy.
func WithPodSelector(podSelector metav1.LabelSelector) MultiNetworkPolicyOpt {
	return func(policy *multinetpolicyapiv1.MultiNetworkPolicy) {
		policy.Spec.PodSelector = podSelector
	}
}

// WithPolicyTypes adds PolicyTypes option if not exists to the MultiNetworkPolicy.
func WithPolicyTypes(policyTypes []multinetpolicyapiv1.MultiPolicyType) MultiNetworkPolicyOpt {
	return func(policy *multinetpolicyapiv1.MultiNetworkPolicy) {
		for _, policyType := range policyTypes {
			policy.Spec.PolicyTypes = appendIfNotPresent(policy.Spec.PolicyTypes, policyType)
		}
	}
}

// WithEmptyIngressRules adds empty ingress rule to the MultiNetworkPolicy.
func WithEmptyIngressRules() MultiNetworkPolicyOpt {
	return func(policy *multinetpolicyapiv1.MultiNetworkPolicy) {
		policy.Spec.Ingress = []multinetpolicyapiv1.MultiNetworkPolicyIngressRule{}
	}
}

// WithIngressRule adds provided ingress rule to the MultiNetworkPolicy.
func WithIngressRule(rule multinetpolicyapiv1.MultiNetworkPolicyIngressRule) MultiNetworkPolicyOpt {
	return func(policy *multinetpolicyapiv1.MultiNetworkPolicy) {
		policy.Spec.Ingress = append(policy.Spec.Ingress, rule)
	}
}

// WithEgressRule adds provided egress rule to the MultiNetworkPolicy.
func WithEgressRule(rule multinetpolicyapiv1.MultiNetworkPolicyEgressRule) MultiNetworkPolicyOpt {
	return func(policy *multinetpolicyapiv1.MultiNetworkPolicy) {
		policy.Spec.Egress = append(policy.Spec.Egress, rule)
	}
}

// CreatePolicy creates the MultiNetworkPolicy.
func CreatePolicy() MultiNetworkPolicyOpt {
	return func(policy *multinetpolicyapiv1.MultiNetworkPolicy) {
		err := helper.Apiclient.Create(context.Background(), policy)
		Expect(err).ToNot(HaveOccurred())
	}
}

// FailCreatePolicy should fail to create the MultiNetworkPolicy.
func FailCreatePolicy() MultiNetworkPolicyOpt {
	return func(policy *multinetpolicyapiv1.MultiNetworkPolicy) {
		err := helper.Apiclient.Create(context.Background(), policy)
		Expect(err).To(HaveOccurred())
	}
}

// EnableMultiNetworkPolicy enable the NetworkOperator's MultiNetworkPolicy if it's not enabled.
func EnableMultiNetworkPolicy() {
	useMultiNetworkPolicy := getMultiNetworkPolicy()
	if !useMultiNetworkPolicy {
		setMultiNetworkPolicy(true)
		validateMultiNetworkPolicyRunning()
	}
}

// DisableMultiNetworkPolicy disable the NetworkOperator's MultiNetworkPolicy if it's not enabled.
func DisableMultiNetworkPolicy() {
	useMultiNetworkPolicy := getMultiNetworkPolicy()
	if useMultiNetworkPolicy {
		setMultiNetworkPolicy(false)
		validateMultiNetworkPolicyDeleted()
	}
}

// CleanMultiNetworkPoliciesFromNamespace deletes all MultiNetworkPolicies from the namespace.
func CleanMultiNetworkPoliciesFromNamespace(namespaceName string) error {
	multiNetworkPolicyList := multinetpolicyapiv1.MultiNetworkPolicyList{}
	err := helper.Apiclient.List(context.Background(), &multiNetworkPolicyList,
		runtimeclient.InNamespace(namespaceName))

	if err != nil {
		return err
	}

	for _, multiNetworkPolicy := range multiNetworkPolicyList.Items {
		err = helper.Apiclient.Delete(context.Background(), &multiNetworkPolicy)
		if err != nil {
			return err
		}
	}

	return nil
}

// CreateClientPod defines and creates client pod for MultiNetworkPolicy tests.
func CreateClientPod(nodeName, labelName string, annotation map[string]string) *v1.Pod {
	podDefinition := defineTestPod(annotation, labelName, nodeName)
	helper.WaitUntilPodCreatedAndRunning(podDefinition, netpolicyparameters.PodWaitingTime)

	return podDefinition
}

// DefinePodAnnotation defines pod annotation.
func DefinePodAnnotation(sriovNetworkName, ipAddress string) (map[string]string, error) {
	var podNetworks []multus.NetworkSelectionElement

	_, subnet, _ := nethelper.DefineIPFamily(ipAddress)
	podNetAnnotation := pod.NewPodNetBuilder()

	podNetworks = append(podNetworks, *pod.DefinePodNetStaticMacIP(sriovNetworkName, "",
		fmt.Sprintf("%s/%s", ipAddress, subnet)))

	return podNetAnnotation.WithNetworks(podNetworks).Annotation.ConvertNetworksAnnotationToMap()
}

// CreateServerPod defines and creates server pod for MultiNetworkPolicy tests.
func CreateServerPod(nodeName, labelName string, annotation map[string]string, initCommand []string) *v1.Pod {
	container2Command := []string{"bash", "-c",
		fmt.Sprintf("testcmd --listen -interface=net1 --protocol=tcp --mtu=1500 -port=%s",
			netpolicyparameters.Port5003)}

	additionalContainers := make(map[string][]string)
	additionalContainers[netpolicyparameters.Container5003Name] = container2Command

	podDefinition := pod.RedefineWithPrivilegedInitContainer(
		pod.RedefineWithMultiplePrivilegedContainers(defineTestPod(annotation, labelName, nodeName),
			additionalContainers), initCommand)

	helper.WaitUntilPodCreatedAndRunning(podDefinition, netpolicyparameters.PodWaitingTime)

	return podDefinition
}

// RunTCPTraffic sends TCP traffic from clientPod to serverIP.
func RunTCPTraffic(clientPod *v1.Pod, serverIP, port string) error {
	command := []string{"testcmd", fmt.Sprintf("-port=%s", port), "-interface=net1",
		fmt.Sprintf("-server=%s", serverIP), "-protocol=tcp", "-mtu=1200"}

	buffer, err := pod.ExecCommand(helper.Apiclient, *clientPod, command)
	if err != nil {
		return fmt.Errorf("%w: %s", err, buffer.String())
	}

	return nil
}

// PingIPViaInterface sends ICMP traffic from clientPod to serverIP over VRF.
func PingIPViaInterface(clientPod *v1.Pod, serverIP, vrfName string) error {
	buffer, err := pod.ExecCommand(helper.Apiclient, *clientPod, []string{"ping", "-I", vrfName, serverIP, "-c3"})
	if err != nil {
		return fmt.Errorf("%w: %s", err, buffer.String())
	}

	return nil
}

func validateMultiNetworkPolicyRunning() {
	Eventually(func() error {
		return helper.IsDaemonsetReady(helper.Apiclient,
			netparameters.MultusNamespace, netpolicyparameters.NetworkPolicyDaemonsetName)
	}, 2*netpolicyparameters.PodWaitingTime, netpolicyparameters.RetryInterval).ShouldNot(HaveOccurred(),
		"NetworkPolicy daemonset is not ready")
}

func validateMultiNetworkPolicyDeleted() {
	Eventually(func() bool {
		isNetworkPolicyDaemonsetInstalled, _ := helper.IsDaemonsetInstalled(helper.Apiclient,
			netparameters.MultusNamespace, netpolicyparameters.NetworkPolicyDaemonsetName)

		return isNetworkPolicyDaemonsetInstalled
	}, netpolicyparameters.PodWaitingTime, netpolicyparameters.RetryInterval).Should(BeFalse(),
		"NetworkPolicy daemonset is not deleted")
}

// getMultiNetworkPolicy returns the NetworkOperator's MultiNetworkPolicy configuration.
func getMultiNetworkPolicy() bool {
	networkOperatorConfg := &operv1.Network{}
	err := helper.Apiclient.Get(
		context.TODO(), runtimeclient.ObjectKey{Name: netparameters.NetworkOperatorConfigName}, networkOperatorConfg)
	Expect(err).ToNot(HaveOccurred(), "failed to get NetworkOperator")

	return *networkOperatorConfg.Spec.UseMultiNetworkPolicy
}

// setMultiNetworkPolicy set NetworkOperator's MultiNetworkPolicy configuration.
func setMultiNetworkPolicy(state bool) {
	networkOperatorConfg := &operv1.Network{}
	err := helper.Apiclient.Get(
		context.TODO(), runtimeclient.ObjectKey{Name: netparameters.NetworkOperatorConfigName}, networkOperatorConfg)
	Expect(err).ToNot(HaveOccurred())

	networkOperatorConfg.Spec.UseMultiNetworkPolicy = &state

	err = helper.Apiclient.Update(context.Background(), networkOperatorConfg)
	Expect(err).ToNot(HaveOccurred())
}

func defineTestPod(annotation map[string]string, labelName, nodeName string) *v1.Pod {
	command := []string{"bash", "-c",
		fmt.Sprintf("testcmd --listen -interface=net1 --protocol=tcp --mtu=1500 -port=%s",
			netpolicyparameters.Port5001)}

	return pod.RedefineAsPrivileged(
		pod.RedefineWithCommand(
			pod.RedefineWithLabel(
				pod.RedefinePodWithAnnotation(
					pod.DefinePodOnNode(netpolicyparameters.TestNamespace, helper.Config.Network.TestContainerImage, nodeName),
					annotation), netpolicyparameters.LabelType, labelName), command, []string{}))
}

func appendIfNotPresent(input []multinetpolicyapiv1.MultiPolicyType,
	newElement multinetpolicyapiv1.MultiPolicyType) []multinetpolicyapiv1.MultiPolicyType {
	for _, element := range input {
		if element == newElement {
			return input
		}
	}

	return append(input, newElement)
}
