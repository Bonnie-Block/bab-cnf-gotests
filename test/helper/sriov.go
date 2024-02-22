package helper

import (
	"context"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var ChangedNodeDrainState bool

func RestoreNodeDrainState(operatorNamespace string) {
	if ChangedNodeDrainState {
		SetDisableNodeDrainState(false, operatorNamespace)
	}
}

// WaitForSRIOVStable waits until sriov stable
// "TODO: Convert waiting time in to int32".
func WaitForSRIOVStable(operatorNamespace string, waitingTime time.Duration, snoTimeoutMultiplier time.Duration) {
	// This used to be to check for sriov not to be stable first,
	// then stable. The issue is that if no configuration is applied, then
	// the status won't never go to not stable and the test will fail.
	// "TODO: find a better way to handle this scenario"
	time.Sleep(time.Duration(10+int32(snoTimeoutMultiplier)*10) * time.Second)
	Eventually(func() bool {
		// ignoring the error. This can eventually be executed against a single node cluster,
		// and if a reconfiguration triggers a reboot then the api calls will return an error
		res, _ := cluster.SriovStable(operatorNamespace, Apiclient)

		return res
	}, waitingTime, 1*time.Second).Should(BeTrue())

	Eventually(func() bool {
		isClusterReady, err := cluster.IsClusterStable(Apiclient)
		Expect(err).ToNot(HaveOccurred())

		return isClusterReady
	}, waitingTime, 1*time.Second).Should(BeTrue())
}

// DefineSriovPolicy build SriovPolicy resource.
func DefineSriovPolicy(
	name string,
	namepace string,
	sriovInt *sriovv1.InterfaceExt,
	vfsNumber int,
	pfRange string,
	mtu int,
	resourceName string,
	devType string) *sriovv1.SriovNetworkNodePolicy {
	return &sriovv1.SriovNetworkNodePolicy{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name,
			Namespace:    namepace,
		},

		Spec: sriovv1.SriovNetworkNodePolicySpec{
			NodeSelector: map[string]string{
				Config.General.CnfNodeLabel: "",
			},
			NumVfs:       vfsNumber,
			Mtu:          mtu,
			ResourceName: resourceName,
			Priority:     99,
			NicSelector: sriovv1.SriovNetworkNicSelector{
				PfNames: []string{sriovInt.Name + pfRange},
			},
			DeviceType: devType,
		},
	}
}

// ValidateSriovVFsAvailableOnNodes validates that VFs are available on Nodes.
func ValidateSriovVFsAvailableOnNodes(
	nodes []string, networkPolicies []*sriovv1.SriovNetworkNodePolicy, vfNumber int) {
	for _, node := range nodes {
		validateSriovVFsNodeAllocatedResources(node, networkPolicies, vfNumber)
	}
}

func validateSriovVFsNodeAllocatedResources(
	node string, sriovNetworkPolicies []*sriovv1.SriovNetworkNodePolicy, vfNumber int) {
	for _, networkPolicy := range sriovNetworkPolicies {
		Eventually(func() int64 {
			testedNode, err := Apiclient.Nodes().Get(context.Background(), node, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			resNum := testedNode.Status.Allocatable[corev1.ResourceName("openshift.io/"+networkPolicy.Spec.ResourceName)]
			allocatable, _ := resNum.AsInt64()

			return allocatable
		}, 20*time.Minute, time.Second).Should(Equal(int64(vfNumber)))
	}
}

func GetNodeDrainState(operatorNamespace string) bool {
	sriovOperatorConfg := &sriovv1.SriovOperatorConfig{}
	err := Apiclient.Get(
		context.Background(), runtimeclient.ObjectKey{Name: "default", Namespace: operatorNamespace}, sriovOperatorConfg)
	Expect(err).ToNot(HaveOccurred())

	return sriovOperatorConfg.Spec.DisableDrain
}

func SetDisableNodeDrainState(state bool, operatorNamespace string) {
	sriovOperatorConfg := &sriovv1.SriovOperatorConfig{}
	err := Apiclient.Get(
		context.Background(), runtimeclient.ObjectKey{Name: "default", Namespace: operatorNamespace}, sriovOperatorConfg)
	Expect(err).ToNot(HaveOccurred())

	sriovOperatorConfg.Spec.DisableDrain = state
	err = Apiclient.Update(context.Background(), sriovOperatorConfg)
	Expect(err).ToNot(HaveOccurred())
}
