package helper

import (
	"fmt"
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
)

const (
	ipv4Subnet int = 24
	ipv6Subnet int = 64
)

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
	subnet := ipv4Subnet
	if strings.Contains(ipAddress, ":") {
		subnet = ipv6Subnet
	}
	pod.Annotations = map[string]string{"k8s.v1.cni.cncf.io/networks": fmt.Sprintf(`[
		{
			"name": "%s", 
			"mac": "%s",
			"ips": ["%s/%d"]
		}
	]`, networkName, macAddress, ipAddress, subnet)}

	return pod
}

// DefinePodWithStaticMacAndDualIpam sets pod network with static IPAM config with Static Mac address
func DefinePodWithStaticMacAndDualIpam(pod *corev1.Pod, networkName string, ip4address string, ip6address string, macAddress string) *corev1.Pod {
	pod.Annotations = map[string]string{"k8s.v1.cni.cncf.io/networks": fmt.Sprintf(`[
		{
			"name": "%s", 
			"mac": "%s",
			"ips": ["%s/%d","%s/%d"]
		}
	]`, networkName, macAddress, ip4address, ipv4Subnet, ip6address, ipv6Subnet)}

	return pod
}

// DefinePodWithStaticDualIpamAndDynamicMac sets pod network with static dual IPAM config with Dynamic Mac address
func DefinePodWithStaticDualIpamAndDynamicMac(pod *corev1.Pod, networkName string, ip4address string, ip6address string) *corev1.Pod {
	pod.Annotations = map[string]string{"k8s.v1.cni.cncf.io/networks": fmt.Sprintf(`[
		{
			"name": "%s",
			"ips": ["%s/%d","%s/%d"]
		}
	]`, networkName, ip4address, ipv4Subnet, ip6address, ipv6Subnet)}

	return pod
}

// DefinePodWithStaticIpamAndDynamicMac sets pod network with static IPAM config with Dynamic Mac address
func DefinePodWithStaticIpamAndDynamicMac(pod *corev1.Pod, networkName string, ipAddress string) *corev1.Pod {
	subnet := ipv4Subnet
	if strings.Contains(ipAddress, ":") {
		subnet = ipv6Subnet
	}
	pod.Annotations = map[string]string{"k8s.v1.cni.cncf.io/networks": fmt.Sprintf(`[
		{
			"name": "%s",
			"ips": ["%s/%d"]
		}
	]`, networkName, ipAddress, subnet)}

	return pod
}

// DefinePodCommand sets pod command
func DefinePodCommand(pod *corev1.Pod, command []string) *corev1.Pod {
	pod.Spec.Containers[0].Command = command
	return pod
}

// DefinePodCommands sets a container for every command
func DefinePodCommands(pod *corev1.Pod, commands ...[]string) *corev1.Pod {
	for index := 0; index < len(commands); index++ {
		if index == len(pod.Spec.Containers) {
			pod.Spec.Containers = append(pod.Spec.Containers, *pod.Spec.Containers[0].DeepCopy())
		}
		pod.Spec.Containers[index].Command = commands[index]
		pod.Spec.Containers[index].Name = fmt.Sprintf("test-%d", index)
	}

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

// DefinePodCommandsWithDualIpamAndMac returns a pod with 2 ip addresses and 2 containers
func DefinePodCommandsWithDualIpamAndMac(podDefinition *corev1.Pod, networkName string, ip4address string, ip6address string, macAddress string, podCommands ...[]string) *corev1.Pod {
	if macAddress == "" {
		podDefinition = DefinePodCommands(
			DefinePodWithStaticDualIpamAndDynamicMac(podDefinition, networkName, ip4address, ip6address),
			podCommands...)
	} else {
		podDefinition = DefinePodCommands(
			DefinePodWithStaticMacAndDualIpam(podDefinition, networkName, ip4address, ip6address, macAddress),
			podCommands...)
	}

	return podDefinition
}
