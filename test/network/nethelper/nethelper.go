package nethelper

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
func DefineFRRPod(masterNodeName string, namespace string) *k8sv1.Pod {
	pod := &k8sv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "frr-pod",
			Namespace: namespace,
		},
		Spec: k8sv1.PodSpec{
			HostNetwork: true,
			Volumes: []k8sv1.Volume{
				{
					Name: netparameters.MasterConfigMapName,
					VolumeSource: k8sv1.VolumeSource{
						ConfigMap: &k8sv1.ConfigMapVolumeSource{
							LocalObjectReference: k8sv1.LocalObjectReference{
								Name: netparameters.MasterConfigMapName,
							},
						},
					},
				},
			},
			NodeSelector: map[string]string{
				parameters.LabelHostname: masterNodeName,
			},
			Tolerations: []k8sv1.Toleration{
				{
					Key:    fmt.Sprintf("%s/%s", nodes.LabelRole, parameters.RoleMaster),
					Effect: "NoSchedule",
				},
			},
			Containers: []k8sv1.Container{
				{
					Name:  "frr",
					Image: helper.Config.Network.FrrImage,
					VolumeMounts: []k8sv1.VolumeMount{
						{
							Name:      netparameters.MasterConfigMapName,
							MountPath: "/etc/frr",
						},
					},
					SecurityContext: &k8sv1.SecurityContext{
						Capabilities: &k8sv1.Capabilities{
							Add: []k8sv1.Capability{
								"NET_ADMIN",
								"NET_RAW",
								"SYS_ADMIN",
							},
						},
					},
				},
			},
		},
	}

	return pod
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
