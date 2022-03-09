package nethelper

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"time"

	v1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// SriovNetworkOptions additional options for SriovNetwork.
type SriovNetworkOptions func(*sriovv1.SriovNetwork)

// CreateSriovNetwork adds sriov network.
func CreateSriovNetwork(
	clientSet *client.ClientSet,
	intf *sriovv1.InterfaceExt,
	name string,
	namespace string,
	operatorNamespace string,
	resourceName string,
	ipam string,
	options ...SriovNetworkOptions) error {
	sriovNetwork := &sriovv1.SriovNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: operatorNamespace,
		},
		Spec: sriovv1.SriovNetworkSpec{
			ResourceName:     resourceName,
			IPAM:             ipam,
			NetworkNamespace: namespace,
			// Enable the linkState instead of auto so even if the PF is down we can still use the VF
			// for pod to pod connectivity tests in the same host
			LinkState: "enable",
		}}

	for _, o := range options {
		o(sriovNetwork)
	}

	// We need this to be able to run the connectivity checks on Mellanox cards
	if intf.DeviceID == "1015" {
		sriovNetwork.Spec.SpoofChk = "off"
	}

	err := clientSet.Create(context.Background(), sriovNetwork)

	return err
}

// CompareNodeSriovInterfaces validates if nodes have the same interface spec.
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

// StrParamInListOfParams validates if specific sting parameter is valid.
func StrParamInListOfParams(param string, paramRange []string) error {
	for _, parameter := range paramRange {
		if param == parameter {
			return nil
		}
	}

	return fmt.Errorf("error: wrong parameter %v", param)
}

// NodeIPsForFamily returns nodes' IP addresses matching the ip family.
func NodeIPsForFamily(nodes []k8sv1.Node, family string) []string {
	res := []string{}

	for _, n := range nodes {
		for _, a := range n.Status.Addresses {
			if a.Type == k8sv1.NodeInternalIP {
				if family != "dual" && IPFamilyForAddress(a.Address) != family {
					continue
				}
				res = append(res, a.Address)
			}
		}
	}

	return res
}

// IPFamilyForAddress get a ip address and returns it's IP family type.
func IPFamilyForAddress(ip string) string {
	ipNet := net.ParseIP(ip)
	if ipNet.To4() == nil {
		return "ipv6"
	}

	return "ipv4"
}

// DefineFRRConfigMap returns configmap definition with FRR configuration.
func DefineFRRConfigMap(configMapName string, testNamespace string, configMapData map[string]string) *k8sv1.ConfigMap {
	configMapData["vtysh.conf"] = ""

	configMap := &k8sv1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: testNamespace,
		},
		Data: configMapData,
	}

	return configMap
}

// DefineFRRPod returns Pod required for the FRR test setup.
func DefineFRRPod(masterNodeName string, namespace string, hostNetwork bool) *k8sv1.Pod {
	frrPod := pod.RedefineAsPrivileged(
		pod.RedefineOnMaster(
			pod.DefinePodOnNode(namespace, helper.Config.Network.FrrImage, masterNodeName)))
	if hostNetwork {
		frrPod = pod.RedefineWithHostNetwork(frrPod)
	}

	return pod.RedefineWithVolume(pod.RedefineWithCommand(frrPod, []string{}, []string{}),
		netparameters.MasterConfigMapName,
		"/etc/frr",
		k8sv1.VolumeSource{
			ConfigMap: &k8sv1.ConfigMapVolumeSource{
				LocalObjectReference: k8sv1.LocalObjectReference{
					Name: netparameters.MasterConfigMapName,
				},
			},
		}, false)
}

type BFDDescription struct {
	BFDStatus string `json:"status"`
	BFDPeer   string `json:"peer"`
}

// IsBFDHasStatus verifies that BFD session on a pod has given status.
func IsBFDHasStatus(frrPod *k8sv1.Pod, bfdPeer string, status string) error {
	bfdStatusOut, err := pod.ExecCommand(helper.Apiclient, *frrPod,
		[]string{"vtysh", "-c", "sh bfd peers brief json"})
	if err != nil {
		return err
	}

	result := []BFDDescription{}

	err = json.Unmarshal(bfdStatusOut.Bytes(), &result)
	if err != nil {
		return err
	}

	for _, peer := range result {
		if peer.BFDPeer == bfdPeer && peer.BFDStatus != status {
			return fmt.Errorf("%s bfd status is %s (expected %s)", peer.BFDPeer, peer.BFDStatus, status)
		}
	}

	return nil
}

// DefineDhcpServerOnNad creates nad with static ipam and dhcp server on top of it.
func DefineDhcpServerOnNad(
	namespace string, intName string, nodeName string, serverIP string, addressMap map[string]string) error {
	vrfDefinitionDhcp := v1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "dhcp",
			Namespace:    namespace,
		},
		Spec: v1.NetworkAttachmentDefinitionSpec{
			Config: fmt.Sprintf(
				`{"cniVersion": "0.4.0", "name": "macvlan-vrf", "plugins":`+
					`[{"type": "macvlan","master": "%s","ipam": {"type": "static"}}]}`,
				intName),
		},
	}

	err := helper.Apiclient.Create(context.Background(), &vrfDefinitionDhcp)
	if err != nil {
		return err
	}

	ipAddr, subnet, err := net.ParseCIDR(fmt.Sprintf("%s/%s", serverIP, "24"))
	if err != nil {
		return err
	}

	bAddress, err := lastAddr(subnet)
	if err != nil {
		return err
	}

	helper.WaitUntilPodCreatedAndRunning(
		DefineDhcpServerPod(namespace, vrfDefinitionDhcp.Name, nodeName,
			ipAddr, subnet, bAddress, addressMap), 300*time.Second)

	return nil
}

// DefineDhcpServerPod will create dhcp server pod and return it.
func DefineDhcpServerPod(
	namespace string, networkName string, nodeName string, serverIP net.IP,
	subnet *net.IPNet, bAddress net.IP, addressMap map[string]string) *k8sv1.Pod {
	var (
		hosts string
		index int
	)

	for key, value := range addressMap {
		hosts += fmt.Sprintf(" host test%d \"{hardware ethernet %s; fixed-address %s; max-lease-time 7200;}\"",
			index, key, value)
		index++
	}

	return pod.RedefineAsPrivileged(
		pod.RedefineWithVolume(
			pod.RedefineWithInitContainer(
				pod.RedefineWithCommand(
					pod.RedefineAsNetRaw(
						pod.RedefinePodWithNetwork(
							pod.DefinePodOnNode(namespace, helper.Config.Network.TestContainerImage, nodeName),
							fmt.Sprintf(`[{"name": "%s", "ips": ["%s/%s"]}]`,
								networkName, serverIP, "8"),
						),
					),
					[]string{"/bin/bash", "-c"}, []string{"/usr/sbin/dhcpd -cf /etc/dhcp/dhcpd.conf && sleep INF"}),
				[]string{"bash", "-c", fmt.Sprintf("echo subnet %s netmask %s \"{option broadcast-address ",
					subnet.IP, net.IP(subnet.Mask).String()) +
					fmt.Sprintf("%s; default-lease-time 3600; allow duplicates; max-lease-time 7200; interface net1;}\"", bAddress) +
					fmt.Sprintf("%s > /etc/dhcp/dhcpd.conf", hosts)}),
			"dhcp", "/etc/dhcp/", k8sv1.VolumeSource{EmptyDir: &k8sv1.EmptyDirVolumeSource{}},
			false),
	)
}

// DeleteNADs removes all given Network Attachment Definition in given namespace.
func DeleteNADs(nadNames []string, namespace string) error {
	nad := &v1.NetworkAttachmentDefinition{}
	for _, nadName := range nadNames {
		err := helper.Apiclient.Get(context.Background(), goclient.ObjectKey{Namespace: namespace,
			Name: nadName}, nad)
		if err != nil {
			return err
		}

		err = helper.Apiclient.Delete(context.Background(), nad)
		if err != nil {
			return err
		}
	}

	return nil
}

func lastAddr(network *net.IPNet) (net.IP, error) {
	if network.IP.To4() == nil {
		return net.IP{}, fmt.Errorf("%s", "does not support IPv6 addresses.")
	}

	ip := make(net.IP, len(network.IP.To4()))
	binary.BigEndian.PutUint32(
		ip, binary.BigEndian.Uint32(network.IP.To4())|^binary.BigEndian.Uint32(net.IP(network.Mask).To4()))

	return ip, nil
}
