package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

var _ = Describe("CNF VRF", func() {

	describe := func(node string, ipStack string) string {

		VRFParameters, err := parameters.NewVRFTestParameters(node, ipStack)
		if err != nil {
			return fmt.Sprintf("error in parameters: node=%s, ipStack=%s", node, ipStack)
		}
		params, err := json.Marshal(VRFParameters)
		if err != nil {
			return fmt.Sprintf("error in parameters: node=%s, ipStack=%s", node, ipStack)
		}

		return fmt.Sprintf("%s", string(params))
	}
	config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())

	var nodesList []k8sv1.Node
	var masterMacVlanInterfaceName string
	var vrfBlue netattdefv1.NetworkAttachmentDefinition
	var vrfRed netattdefv1.NetworkAttachmentDefinition

	execute.BeforeAll(func() {
		By(fmt.Sprintf("Create %s namespace", parameters.TestNamespace))
		err := namespaces.Create(parameters.TestNamespace, apiclient)
		Expect(err).ToNot(HaveOccurred())

		By(fmt.Sprintf("Select nodes by label %s ", parameters.LabelNodeRole))
		nodesList, err = nodes.GetByRole(apiclient, parameters.LabelNodeRole)
		Expect(err).ToNot(HaveOccurred())

		By("Select host interface for mac-vlan")
		nodeInterfaceList, err := nodes.GetPhysicalNodeInterfaces(apiclient, nodesList[0].Name)
		Expect(err).ToNot(HaveOccurred())
		for _, oneInterface := range nodeInterfaceList {
			if !oneInterface.Bridge && !oneInterface.DefRoute && oneInterface.Physical && oneInterface.UP {
				masterMacVlanInterfaceName = oneInterface.Name
			}
		}
		if masterMacVlanInterfaceName == "" {
			Skip("There is not valid interface on Node for VRF tests")
		}
		Expect(err).ToNot(HaveOccurred())

		By("Adding NADs")
		vrfBlue = addVRFNad(apiclient, "test-vrf-blue", masterMacVlanInterfaceName, parameters.VRFBlueName)
		vrfRed = addVRFNad(apiclient, "test-vrf-red", masterMacVlanInterfaceName, parameters.VRFRedName)
	})

	AfterEach(func() {
		By("Cleaning up resources after test")
		err := namespaces.CleanPods(parameters.TestNamespace, apiclient)
		Expect(err).ToNot(HaveOccurred())
	})

	DescribeTable("Integration: NAD, IPAM: static, Interfaces: 2, Scheme: 2 Pods 2 VRFs OCP Primary network overlap",
		func(node string, ipStack string) {
			var podClientNodeLabel string
			var podServerNodeLabel string
			var redVRFNetworkPrefix string
			By("Validating test parameters")
			VRFParameters, err := parameters.NewVRFTestParameters(node, ipStack)
			Expect(err).ToNot(HaveOccurred())

			if VRFParameters.Node == parameters.SameNode {
				podClientNodeLabel = nodesList[0].Name
				podServerNodeLabel = nodesList[0].Name
				redVRFNetworkPrefix = "24"
			} else {
				if len(nodesList) < 2 {
					Skip(fmt.Sprintf("There is not enough nodes to run test with following parameter %s", node))
				}
				podClientNodeLabel = nodesList[0].Name
				podServerNodeLabel = nodesList[1].Name
				redVRFNetworkPrefix = "8"
			}

			By("Getting overlapping IP addresses")
			podClientVRFRedOverlappingIP := getOverlapIP(apiclient, podClientNodeLabel, config.Network.TestContainerImage)
			podServerVRFRedOverlappingIP := getOverlapIP(apiclient, podServerNodeLabel, config.Network.TestContainerImage)
			var podClientVRFBlueIPAddress string
			var podServerVRFBlueIPAddress string

			if ipStack == parameters.IPStackIPv4 && net.ParseIP(podClientVRFRedOverlappingIP).To4() == nil {
				Skip("Skipping IPv4 test. Cluster supports IPv6 protocol")
			} else if ipStack == parameters.IPStackIPv4 && net.ParseIP(podClientVRFRedOverlappingIP).To4() != nil {
				podClientVRFBlueIPAddress = "10.255.255.1"
				podServerVRFBlueIPAddress = "10.255.255.2"
			} else {
				Skip("Unpsupported protocol parameter")
			}
			By("Define client/server pods")
			podClientIpamConfig := fmt.Sprintf(`[{"name": "%s", "mac": "%s", "ips": ["%s/24"]}, {"name": "%s", "mac": "%s", "ips": ["%s/%s"]}]`,
				vrfBlue.Name, "20:04:0f:f1:88:A1", podClientVRFBlueIPAddress, vrfRed.Name, "20:04:0f:f1:88:B2", podClientVRFRedOverlappingIP, redVRFNetworkPrefix)
			podClient := pod.RedefineAsPrivileged(
				pod.RedefinePodWithNetwork(pod.DefinePodOnNode(parameters.TestNamespace, config.Network.TestContainerImage, podClientNodeLabel), podClientIpamConfig))
			podServerIpamConfig := fmt.Sprintf(`[{"name": "%s", "mac": "%s", "ips": ["%s/24"]}, {"name": "%s", "mac": "%s", "ips": ["%s/%s"]}]`,
				vrfBlue.Name, "20:04:0f:f1:88:A3", podServerVRFBlueIPAddress, vrfRed.Name, "20:04:0f:f1:88:B4", podServerVRFRedOverlappingIP, redVRFNetworkPrefix)
			podServer := pod.RedefineAsPrivileged(
				pod.RedefinePodWithNetwork(pod.DefinePodOnNode(parameters.TestNamespace, config.Network.TestContainerImage, podServerNodeLabel), podServerIpamConfig))

			By("Running client/server pods")
			runningClientPod := helper.WaitUntilPodCreatedAndRunning(apiclient, podClient, parameters.TestNamespace, parameters.PodWaitingTime)
			helper.WaitUntilPodCreatedAndRunning(apiclient, podServer, parameters.TestNamespace, parameters.PodWaitingTime)
			podHasCorrectVrfConfig(apiclient, podClient.Name,
				[]map[string]string{
					{"vrfName": parameters.VRFBlueName, "vrfClientIP": podClientVRFBlueIPAddress, "vrfInterface": "net1"},
					{"vrfName": parameters.VRFRedName, "vrfClientIP": podClientVRFRedOverlappingIP, "vrfInterface": "net2"}})
			podHasCorrectVrfConfig(apiclient, podServer.Name,
				[]map[string]string{
					{"vrfName": parameters.VRFBlueName, "vrfClientIP": podServerVRFBlueIPAddress, "vrfInterface": "net1"},
					{"vrfName": parameters.VRFRedName, "vrfClientIP": podServerVRFRedOverlappingIP, "vrfInterface": "net2"}})

			err = pingIPViaVRF(apiclient, *runningClientPod, parameters.VRFRedName, podServerVRFRedOverlappingIP)
			Expect(err).ToNot(HaveOccurred())
			err = pingIPViaVRF(apiclient, *runningClientPod, parameters.VRFBlueName, podServerVRFBlueIPAddress)
			Expect(err).ToNot(HaveOccurred())
			err = apiclient.Pods(parameters.TestNamespace).Delete(context.Background(), podServer.Name, metav1.DeleteOptions{GracePeriodSeconds: pointer.Int64Ptr(0)})
			Expect(err).ToNot(HaveOccurred())

			By("Positive connectivity tests - success. Running negative tests")
			Eventually(func() error {
				_, err := apiclient.Pods(parameters.TestNamespace).Get(context.Background(), podServer.Name, metav1.GetOptions{})
				return err
			}, parameters.PodWaitingTime, 5*time.Second).Should(HaveOccurred())

			err = pingIPViaVRF(apiclient, *runningClientPod, parameters.VRFBlueName, podServerVRFBlueIPAddress)
			Expect(err).To(HaveOccurred())
			err = pingIPViaVRF(apiclient, *runningClientPod, parameters.VRFRedName, podServerVRFRedOverlappingIP)
			Expect(err).To(HaveOccurred())
			err = pingIPViaVRF(apiclient, *runningClientPod, "eth0", podServerVRFRedOverlappingIP)
			Expect(err).ToNot(HaveOccurred())
		},
		Entry(describe, parameters.SameNode, parameters.IPStackIPv4),
		Entry(describe, parameters.DiffNode, parameters.IPStackIPv4),
	)
})

func pingIPViaVRF(cs *client.ClientSet, client k8sv1.Pod, vrfName string, DestIPAddr string) error {
	pingCommand := []string{"ping", "-I", vrfName, "-c5", DestIPAddr}
	pingStatus, err := pod.ExecCommand(cs, client, pingCommand)
	if err != nil {
		return err
	}
	if strings.Contains(pingStatus.String(), " 0% packet loss") {
		return nil
	}
	return fmt.Errorf("Connectivity test error")
}

func addVRFNad(cs *client.ClientSet, NadName string, ifName string, vrfName string) netattdefv1.NetworkAttachmentDefinition {
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

func podHasCorrectVrfConfig(cs *client.ClientSet, podName string, vrfMapsConfig []map[string]string) {
	runningPod, err := cs.Pods(parameters.TestNamespace).Get(context.Background(), podName, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())
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
}
