package helper

import (
	"context"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WaitForSRIOVStable waits until sriov stable
func WaitForSRIOVStable(operatorNamespace string, waitingTime time.Duration) {
	// This used to be to check for sriov not to be stable first,
	// then stable. The issue is that if no configuration is applied, then
	// the status won't never go to not stable and the test will fail.
	// TODO: find a better way to handle this scenario
	time.Sleep(10 * time.Second)
	Eventually(func() bool {
		res, err := cluster.SriovStable(operatorNamespace, Apiclient)
		Expect(err).ToNot(HaveOccurred())
		return res
	}, waitingTime, 1*time.Second).Should(BeTrue())

	Eventually(func() bool {
		isClusterReady, err := cluster.IsClusterStable(Apiclient)
		Expect(err).ToNot(HaveOccurred())
		return isClusterReady
	}, waitingTime, 1*time.Second).Should(BeTrue())
}

// DefineSriovPolicy build SriovPolicy resource
func DefineSriovPolicy(
	name string,
	namepace string,
	sriovInt *sriovv1.InterfaceExt,
	VfsNumber int,
	pfRange string,
	mtu int,
	resourceName string,
	devType string) *sriovv1.SriovNetworkNodePolicy {

	conf, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())
	return &sriovv1.SriovNetworkNodePolicy{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name,
			Namespace:    namepace,
		},

		Spec: sriovv1.SriovNetworkNodePolicySpec{
			NodeSelector: map[string]string{
				conf.General.CnfNodeLabel: "",
			},
			NumVfs:       VfsNumber,
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

// ValidateSriovVFsAvailableOnNodes validates that VFs are avaliable on Nodes
func ValidateSriovVFsAvailableOnNodes(
	nodes []string, NetworkPolicies []*sriovv1.SriovNetworkNodePolicy, VfNumber int) {
	for _, node := range nodes {
		validateSriovVFsNodeAllocatedResources(node, NetworkPolicies, VfNumber)
	}
}

func validateSriovVFsNodeAllocatedResources(node string, SriovNetworkPolicies []*sriovv1.SriovNetworkNodePolicy, VfNumber int) {
	for _, networkPolicy := range SriovNetworkPolicies {
		Eventually(func() int64 {
			testedNode, err := Apiclient.Nodes().Get(context.Background(), node, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			resNum, _ := testedNode.Status.Allocatable[corev1.ResourceName("openshift.io/"+networkPolicy.Spec.ResourceName)]
			allocatable, _ := resNum.AsInt64()
			return allocatable
		}, 20*time.Minute, time.Second).Should(Equal(int64(VfNumber)))
	}
}
