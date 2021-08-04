package helper

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	globalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
)

// TestVRFScenario verifies that VRF feature works as expected
func TestVRFScenario(apiclient *client.ClientSet, node string, ipStack string, ipOverLap string, config *config.Config, nodes []string,
	VRFNetworkBlue string, VRFNetworkRed string) {
	var (
		podClientNodeLabel        string
		podServerNodeLabel        string
		redVRFNetworkPrefix       string
		blueVRFNetworkPrefix      string
		podClientVRFRedIPAddress  string
		podServerVRFRedIPAddress  string
		podClientVRFBlueIPAddress string
		podServerVRFBlueIPAddress string
	)

	VRFParameters, err := parameters.NewVRFTestParameters(node, ipStack)
	Expect(err).ToNot(HaveOccurred())

	if VRFParameters.Node == parameters.DiffNode && len(nodes) < 2 {
		Skip(fmt.Sprintf("There is not enough nodes to run test with following parameter %s", node))
	}

	By("Validating test parameters")
	if VRFParameters.Node == parameters.SameNode {
		podClientNodeLabel = nodes[0]
		podServerNodeLabel = nodes[0]
	} else if VRFParameters.Node == parameters.DiffNode {
		podClientNodeLabel = nodes[0]
		podServerNodeLabel = nodes[1]
	}

	switch ipOverLap {

	case "overLapToSDN":

		if ipStack == parameters.IPStackIPv6 {
			Skip("Skipping SDN IPv6 is not currently tested")
		}
		if VRFParameters.Node == parameters.SameNode {
			redVRFNetworkPrefix = "24"
		} else if VRFParameters.Node == parameters.DiffNode {
			redVRFNetworkPrefix = "8"
		}

		By("Getting overlapping SDN IP Addresses for VRF Red")
		podClientVRFRedIPAddress = getOverlapIP(apiclient, podClientNodeLabel, config.Network.TestContainerImage)
		podServerVRFRedIPAddress = getOverlapIP(apiclient, podServerNodeLabel, config.Network.TestContainerImage)

		if ipStack == parameters.IPStackIPv4 && net.ParseIP(podClientVRFRedIPAddress).To4() == nil {
			Skip("Skipping IPv4 test. Cluster only supports IPv6 protocol")
		}

		By("Setting overlapping IP Address for VRF Blue")
		podClientVRFBlueIPAddress = "10.255.255.1"
		podServerVRFBlueIPAddress = "10.255.255.2"
		blueVRFNetworkPrefix = "24"

	case "overLapToVRF":

		if ipStack == parameters.IPStackIPv4 {
			By("Setting overlapping non-SDN IP Addresses for VRF Red")
			podClientVRFBlueIPAddress = "10.255.255.1"
			podServerVRFBlueIPAddress = "10.255.255.2"
			podClientVRFRedIPAddress = "10.255.255.3"
			podServerVRFRedIPAddress = "10.255.255.4"
			blueVRFNetworkPrefix = "24"
			redVRFNetworkPrefix = "24"
		} else {
			podClientVRFBlueIPAddress = "2001:100::1"
			podServerVRFBlueIPAddress = "2001:100::2"
			podClientVRFRedIPAddress = "2001:100::3"
			podServerVRFRedIPAddress = "2001:100::4"
			redVRFNetworkPrefix = "64"
			blueVRFNetworkPrefix = "64"
		}

	case "nonOverLap":

		if ipStack == parameters.IPStackIPv4 {
			By("Setting overlapping non-SDN IP Addresses for VRF Red")
			podClientVRFBlueIPAddress = "10.255.255.1"
			podServerVRFBlueIPAddress = "10.255.255.2"
			podClientVRFRedIPAddress = "192.168.255.3"
			podServerVRFRedIPAddress = "192.168.255.4"
			blueVRFNetworkPrefix = "24"
			redVRFNetworkPrefix = "24"
		} else {
			podClientVRFBlueIPAddress = "2201:100::1"
			podServerVRFBlueIPAddress = "2201:100::2"
			podClientVRFRedIPAddress = "2201:200::3"
			podServerVRFRedIPAddress = "2201:200::4"
			redVRFNetworkPrefix = "64"
			blueVRFNetworkPrefix = "64"
		}
	default:
		{
			Fail(fmt.Sprintf("%v scenario doesn't exsit", ipOverLap))
		}
	}

	By("Define client/server pods")
	podClientIpamConfig := fmt.Sprintf(`[{"name": "%s", "mac": "%s", "ips": ["%s/%s"]}, {"name": "%s", "mac": "%s", "ips": ["%s/%s"]}]`,
		VRFNetworkBlue, "20:04:0f:f1:88:A1", podClientVRFBlueIPAddress, blueVRFNetworkPrefix, VRFNetworkRed,
		"20:04:0f:f1:88:B2", podClientVRFRedIPAddress, redVRFNetworkPrefix)
	podClient := pod.RedefineAsPrivileged(
		pod.RedefinePodWithNetwork(pod.DefinePodOnNode(parameters.TestNamespace, config.Network.TestContainerImage, podClientNodeLabel), podClientIpamConfig))
	podServerIpamConfig := fmt.Sprintf(`[{"name": "%s", "mac": "%s", "ips": ["%s/%s"]}, {"name": "%s", "mac": "%s", "ips": ["%s/%s"]}]`,
		VRFNetworkBlue, "20:04:0f:f1:88:A3", podServerVRFBlueIPAddress, blueVRFNetworkPrefix, VRFNetworkRed,
		"20:04:0f:f1:88:B4", podServerVRFRedIPAddress, redVRFNetworkPrefix)
	podServer := pod.RedefineAsPrivileged(
		pod.RedefinePodWithNetwork(pod.DefinePodOnNode(parameters.TestNamespace, config.Network.TestContainerImage, podServerNodeLabel), podServerIpamConfig))
	By("Running client/server pods")
	runningClientPod := globalHelper.WaitUntilPodCreatedAndRunning(podClient, parameters.PodWaitingTime)
	globalHelper.WaitUntilPodCreatedAndRunning(podServer, parameters.PodWaitingTime)

	By("Validating client/server VRFs configuration")
	podHasCorrectVrfConfig(apiclient, podClient.Name,
		[]map[string]string{
			{"vrfName": parameters.VRFBlueName, "vrfClientIP": podClientVRFBlueIPAddress, "vrfInterface": "net1"},
			{"vrfName": parameters.VRFRedName, "vrfClientIP": podClientVRFRedIPAddress, "vrfInterface": "net2"}})
	podHasCorrectVrfConfig(apiclient, podServer.Name,
		[]map[string]string{
			{"vrfName": parameters.VRFBlueName, "vrfClientIP": podServerVRFBlueIPAddress, "vrfInterface": "net1"},
			{"vrfName": parameters.VRFRedName, "vrfClientIP": podServerVRFRedIPAddress, "vrfInterface": "net2"}})

	By("Validating client/server IP VRF connectivity")
	err = pingIPViaVRF(apiclient, *runningClientPod, parameters.VRFRedName, podServerVRFRedIPAddress)
	Expect(err).ToNot(HaveOccurred())
	err = pingIPViaVRF(apiclient, *runningClientPod, parameters.VRFBlueName, podServerVRFBlueIPAddress)
	Expect(err).ToNot(HaveOccurred())
	err = apiclient.Pods(parameters.TestNamespace).Delete(context.Background(), podServer.Name, metav1.DeleteOptions{GracePeriodSeconds: pointer.Int64Ptr(0)})
	Expect(err).ToNot(HaveOccurred())

	By("Validating client/server IP negative test")
	Eventually(func() error {
		_, err := apiclient.Pods(parameters.TestNamespace).Get(context.Background(), podServer.Name, metav1.GetOptions{})
		return err
	}, parameters.PodWaitingTime, 5*time.Second).Should(HaveOccurred())
	err = pingIPViaVRF(apiclient, *runningClientPod, parameters.VRFBlueName, podServerVRFBlueIPAddress)
	Expect(err).To(HaveOccurred())
	err = pingIPViaVRF(apiclient, *runningClientPod, parameters.VRFRedName, podServerVRFRedIPAddress)
	Expect(err).To(HaveOccurred())
	if ipOverLap == "overLapToSDN" {
		err = pingIPViaVRF(apiclient, *runningClientPod, "eth0", podServerVRFRedIPAddress)
		Expect(err).ToNot(HaveOccurred())
	}
}

func podHasCorrectVrfConfig(cs *client.ClientSet, podName string, vrfMapsConfig []map[string]string) {
	runningPod, err := cs.Pods(parameters.TestNamespace).Get(context.Background(), podName, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())
	var ipStack string
	if ipStack == parameters.IPStackIPv4 {
		for _, vrfMapConfig := range vrfMapsConfig {
			validateVrfIPAddrCommand := []string{"ip", "addr", "show", fmt.Sprintf("%s", vrfMapConfig["vrfInterface"])}
			Eventually(func() bool {
				vrfIface, _ := pod.ExecCommand(cs, *runningPod, validateVrfIPAddrCommand)
				return strings.Contains(vrfIface.String(), vrfMapConfig["vrfClientIP"])
			}, parameters.PodWaitingTime, 5*time.Second).Should(BeTrue(), fmt.Errorf("VRF interface is not present"))

			validateVRFRouteTableCommand := []string{"ip", "route", "show", "vrf", fmt.Sprintf("%s", vrfMapConfig["vrfName"])}
			Eventually(func() bool {
				vrfRouteTable, _ := pod.ExecCommand(cs, *runningPod, validateVRFRouteTableCommand)
				return strings.Contains(vrfRouteTable.String(), vrfMapConfig["vrfClientIP"])
			}, parameters.PodWaitingTime, 5*time.Second).Should(BeTrue(), fmt.Errorf(fmt.Sprintf("VRF %s route table is not present", vrfMapConfig["vrfName"])))
		}
	} else if ipStack == parameters.IPStackIPv6 {
		for _, vrfMapConfig := range vrfMapsConfig {
			validateVrfIPAddrCommand := []string{"ip", "-6", "addr", "show", fmt.Sprintf("%s", vrfMapConfig["vrfInterface"])}
			Eventually(func() bool {
				vrfIface, _ := pod.ExecCommand(cs, *runningPod, validateVrfIPAddrCommand)
				return strings.Contains(vrfIface.String(), vrfMapConfig["vrfClientIP"])
			}, parameters.PodWaitingTime, 5*time.Second).Should(BeTrue(), fmt.Errorf("VRF interface is not present"))

			validateVRFRouteTableCommand := []string{"ip", "-6", "route", "show", "vrf", fmt.Sprintf("%s", vrfMapConfig["vrfName"])}
			Eventually(func() bool {
				vrfRouteTable, _ := pod.ExecCommand(cs, *runningPod, validateVRFRouteTableCommand)
				_, ipnet, _ := net.ParseCIDR(vrfMapConfig["vrfClientIP"] + "/64")
				return strings.Contains(vrfRouteTable.String(), ipnet.String())
			}, parameters.PodWaitingTime, 5*time.Second).Should(BeTrue(), fmt.Errorf(fmt.Sprintf("VRF %s route table is not present", vrfMapConfig["vrfName"])))
		}
	}
}

func pingIPViaVRF(cs *client.ClientSet, client k8sv1.Pod, vrfName string, DestIPAddr string) error {
	var pingCommand []string
	if net.ParseIP(DestIPAddr).To4() != nil {
		pingCommand = []string{"ping", "-I", vrfName, "-c5", DestIPAddr}
	} else if net.ParseIP(DestIPAddr).To4() == nil {
		pingCommand = []string{"ping6", "-I", vrfName, "-c5", DestIPAddr}
	}
	pingStatus, err := pod.ExecCommand(cs, client, pingCommand)
	if err != nil {
		return err
	}
	if strings.Contains(pingStatus.String(), " 0% packet loss") {
		return nil
	}
	return fmt.Errorf("Connectivity test error")
}

func getOverlapIP(cs *client.ClientSet, nodeName string, podImage string) string {
	tempPodDefinition := pod.RedefineAsPrivileged(pod.DefinePodOnNode(parameters.TestNamespace, podImage, nodeName))
	err := cs.Create(context.Background(), tempPodDefinition)
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() k8sv1.PodPhase {
		tempPod, _ := cs.Pods(parameters.TestNamespace).Get(context.Background(), tempPodDefinition.Name, metav1.GetOptions{})
		return tempPod.Status.Phase
	}, parameters.PodWaitingTime, time.Second).Should(Equal(k8sv1.PodRunning))

	pod, err := cs.Pods(parameters.TestNamespace).Get(context.Background(), tempPodDefinition.Name, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())
	return pod.Status.PodIP
}

// AddVRFNad creates a Network Attachment Definition
func AddVRFNad(cs *client.ClientSet, NadName string, ifName string, vrfName string) netattdefv1.NetworkAttachmentDefinition {
	vrfDefinition := netattdefv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: NadName,
			Namespace:    parameters.TestNamespace,
		},
		Spec: netattdefv1.NetworkAttachmentDefinitionSpec{
			Config: fmt.Sprintf(`{"cniVersion": "0.4.0", "name": "macvlan-vrf", "plugins": [{"type": "macvlan","master": "%s","ipam": {"type": "static"}},{"type": "vrf","vrfname": "%s"}]}`, ifName, vrfName),
		},
	}
	err := cs.Create(context.Background(), &vrfDefinition)
	Expect(err).ToNot(HaveOccurred())
	return vrfDefinition
}

//DefineSriovNetworkMetaPluginsVRFConfig  plugin for discovering and advertising SR-IOV VFs
func DefineSriovNetworkMetaPluginsVRFConfig(VRFName string) func(network *sriovv1.SriovNetwork) {
	return func(network *sriovv1.SriovNetwork) {
		network.Spec.MetaPluginsConfig = fmt.Sprintf(`{"type": "vrf", "vrfname": "%s"}`, VRFName)
	}
}

// GetNodeInterfaces returns list of requested interfaces
func GetNodeInterfaces(c *config.Config, nodeInterfaceList []nodes.NodeInterface, requestedNumber int) ([]nodes.NodeInterface, error) {
	var validNodeIntefaceList []nodes.NodeInterface
	if c.Network.SriovInterfaces == "" {
		return nil, fmt.Errorf("Environment variable CNF_INTERFACES_LIST is not set")
	}
	requestedNodeInterfaceList := strings.Split(c.Network.SriovInterfaces, ",")
	if len(requestedNodeInterfaceList) < requestedNumber {
		return nil, fmt.Errorf("CNF_INTERFACES_LIST has less interfaces than requested by test suite")
	}

	for _, availableNodeInterface := range nodeInterfaceList {
		for _, requestedNodeInterface := range requestedNodeInterfaceList {
			if availableNodeInterface.Name == requestedNodeInterface {
				validNodeIntefaceList = append(validNodeIntefaceList, availableNodeInterface)
			}
		}
	}
	if len(validNodeIntefaceList) < requestedNumber {
		return nil, fmt.Errorf("Requested interfaces %v are not present on cluster node", requestedNodeInterfaceList)
	}
	return validNodeIntefaceList, nil
}
