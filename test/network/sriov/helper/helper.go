package helper

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/gomega"
	sriovv1 "github.com/openshift/sriov-network-operator/pkg/apis/sriovnetwork/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"

	testclient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
)

// WaitForSRIOVStable waits until sriov stable
func WaitForSRIOVStable(clients *testclient.ClientSet, operatorNamespace string, waitingTime time.Duration) {
	// This used to be to check for sriov not to be stable first,
	// then stable. The issue is that if no configuration is applied, then
	// the status won't never go to not stable and the test will fail.
	// TODO: find a better way to handle this scenario
	Eventually(func() bool {
		res, err := cluster.SriovStable(operatorNamespace, clients)
		Expect(err).ToNot(HaveOccurred())
		return res
	}, waitingTime, 1*time.Second).Should(BeTrue())

	Eventually(func() bool {
		isClusterReady, err := cluster.IsClusterStable(clients)
		Expect(err).ToNot(HaveOccurred())
		return isClusterReady
	}, waitingTime, 1*time.Second).Should(BeTrue())
}

// ValidateSriovVFsAvailableOnNodes validates that VFs are avaliable on Nodes
func ValidateSriovVFsAvailableOnNodes(clients *testclient.ClientSet, nodes []string, NetworkPolicies []*sriovv1.SriovNetworkNodePolicy, VfNumber int) {
	for _, node := range nodes {
		validateSriovVFsNodeAllocatedResources(clients, node, NetworkPolicies, VfNumber)
	}
}

func validateSriovVFsNodeAllocatedResources(clients *testclient.ClientSet, node string, SriovNetworkPolicies []*sriovv1.SriovNetworkNodePolicy, VfNumber int) {
	for _, networkPolicy := range SriovNetworkPolicies {
		Eventually(func() int64 {
			testedNode, err := clients.Nodes().Get(context.Background(), node, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			resNum, _ := testedNode.Status.Allocatable[corev1.ResourceName("openshift.io/"+networkPolicy.Spec.ResourceName)]
			allocatable, _ := resNum.AsInt64()
			return allocatable
		}, 10*time.Minute, time.Second).Should(Equal(int64(VfNumber)))
	}
}

// DefineSriovNetwork builds SriovNetwork resource
func DefineSriovNetwork(name string, resourceName string, ipamStatic bool) *sriovv1.SriovNetwork {
	ipam := `{ "type": "dhcp" }`
	if ipamStatic {
		ipam = `{ "type": "static" }`
	}
	return &sriovv1.SriovNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: parameters.OperatorNamespace,
		},
		Spec: sriovv1.SriovNetworkSpec{
			ResourceName:     resourceName,
			IPAM:             ipam,
			Capabilities:     `{ "mac": true, "ips": true }`,
			NetworkNamespace: parameters.OperatorTestNamespace,
		}}
}

// DefineSriovPolicy build SriovPolicy resource
func DefineSriovPolicy(name string, sriovInt *sriovv1.InterfaceExt, VfsNumber int, pfRange string, mtu int, resourceName string, devType string) *sriovv1.SriovNetworkNodePolicy {
	conf, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())
	return &sriovv1.SriovNetworkNodePolicy{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name,
			Namespace:    parameters.OperatorNamespace,
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

// CompareNodeSriovInterfaces validates if nodes have the same interface spec
func CompareNodeSriovInterfaces(sriovInfos *cluster.EnabledNodes) error {
	baseInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
	if err != nil {
		return fmt.Errorf("can not get sriov device")
	}
	for _, node := range sriovInfos.Nodes {
		sriovInterfaces, err := sriovInfos.FindSriovDevices(node)
		if err != nil {
			return fmt.Errorf("can not get sriov device")
		}
		for index := range sriovInterfaces {
			if baseInterfaces[index].Name != sriovInterfaces[index].Name &&
				baseInterfaces[index].Vendor != sriovInterfaces[index].Vendor &&
				baseInterfaces[index].TotalVfs != sriovInterfaces[index].TotalVfs {
				return fmt.Errorf("sriov network interfaces on Nodes are not identical")
			}
		}
	}
	return nil
}

// DefinePodWithStaticMacAndIpam sets pod network with static IPAM config with Static Mac address
func DefinePodWithStaticMacAndIpam(pod *corev1.Pod, networkName string, ipAddress string, macAddress string) *corev1.Pod {

	pod.Annotations = map[string]string{"k8s.v1.cni.cncf.io/networks": fmt.Sprintf(`[
		{
			"name": "%s", 
			"mac": "%s",
			"ips": ["%s/24"]
		}
	]`, networkName, macAddress, ipAddress)}

	return pod
}

// DefinePodWithStaticIpamAndDynamicMac sets pod network with static IPAM config with Dynamic Mac address
func DefinePodWithStaticIpamAndDynamicMac(pod *corev1.Pod, networkName string, ipAddress string) *corev1.Pod {

	pod.Annotations = map[string]string{"k8s.v1.cni.cncf.io/networks": fmt.Sprintf(`[
		{
			"name": "%s",
			"ips": ["%s/24"]
		}
	]`, networkName, ipAddress)}

	return pod
}

// DefinePodCommand sets pod command
func DefinePodCommand(pod *corev1.Pod, command []string) *corev1.Pod {
	pod.Spec.Containers[0].Command = command
	return pod
}

// DefinePodCommandWithIpamAndMac checks this  static Mac or dynamic Mac
func DefinePodCommandWithIpamAndMac(podDefinition *corev1.Pod, networkName string, ipaddress string, macAddress string, podCommand []string) *corev1.Pod {
	if macAddress == "" {
		podDefinition = DefinePodCommand(
			DefinePodWithStaticIpamAndDynamicMac(podDefinition, networkName, ipaddress),
			podCommand)
	} else {
		podDefinition = DefinePodCommand(
			DefinePodWithStaticMacAndIpam(podDefinition, networkName, ipaddress, macAddress),
			podCommand)
	}

	return podDefinition
}
