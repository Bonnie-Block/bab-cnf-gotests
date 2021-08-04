package helper

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
)

var podWaitingTime time.Duration = 5 * time.Minute

// SriovNetworkOptions additional options for SriovNetwork
type SriovNetworkOptions func(*sriovv1.SriovNetwork)

// CreateSriovNetwork adds sriov network
func CreateSriovNetwork(clientSet *client.ClientSet, intf *sriovv1.InterfaceExt, name string, namespace string, operatorNamespace string, resourceName string, ipam string, options ...SriovNetworkOptions) error {
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

// StrParamInListOfParams validates if specific sting parameter is valid
func StrParamInListOfParams(param string, paramRange []string) error {
	for _, parameter := range paramRange {
		if param == parameter {
			return nil
		}
	}
	return fmt.Errorf("error: wrong parameter %v", param)
}

// GetPodIPStacks returns pod ip stack.
func GetPodIPStacks(config *config.Config, apiclient *client.ClientSet) int {
	var ipStack []string
	nodesList, err := nodes.GetByLabel(apiclient, config.General.CnfNodeLabel)
	Expect(err).ToNot(HaveOccurred())

	podDefenition := pod.RedefineWithRestartPolicy(
		pod.RedefineWithCommand(
			pod.DefinePodOnNode("default", config.Network.TestContainerImage, nodesList.Items[0].Name),
			[]string{"sleep", "INF"}, []string{}),
		k8sv1.RestartPolicyNever)
	ipVersionPod, err := apiclient.Pods("default").Create(context.Background(), podDefenition, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() k8sv1.PodPhase {
		ipVersionPod, _ = apiclient.Pods("default").Get(context.Background(), ipVersionPod.Name, metav1.GetOptions{})
		return ipVersionPod.Status.Phase
	}, podWaitingTime, time.Second).Should(Equal(k8sv1.PodRunning), fmt.Sprint("Pod start fail"))
	for _, ip := range ipVersionPod.Status.PodIPs {
		if strings.Contains(ip.IP, ":") {
			ipStack = appendIfMissing(ipStack, "6")
		} else if net.ParseIP(ip.IP) != nil {
			ipStack = appendIfMissing(ipStack, "4")
		}
	}
	Expect(len(ipStack) > 0).To(BeTrue(), fmt.Sprint("Can not detect IP stack of test pod"))

	err = namespaces.CleanPods("default", apiclient)
	Expect(err).ToNot(HaveOccurred())

	ipVersion, err := strconv.Atoi(strings.Join(ipStack, ""))
	Expect(err).ToNot(HaveOccurred())

	if ipVersion == 64 {
		ipVersion = 46
	}
	return ipVersion
}

func appendIfMissing(slice []string, element string) []string {
	for _, value := range slice {
		if value == element {
			return slice
		}
	}
	return append(slice, element)
}
