package netcnihelper

import (
	"context"
	"encoding/json"
	"fmt"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	globalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"strings"
	"time"
)

// TestVRFWAIPScenario verifies that VRF feature works as expected.
func TestVRFWAIPScenario(node string, ipStack string, config *config.Config, nodes []string,
	vrfNetworkBlue []string, vrfNetworkRed []string) {
	var (
		podClientNodeLabel string
		podServerNodeLabel string

		podClientIpamConfig string
		podServerIpamConfig string
	)

	VRFParameters, err := netcniparameters.NewVRFTestParameters(node, ipStack)
	Expect(err).ToNot(HaveOccurred())

	if VRFParameters.Node == netcniparameters.DiffNode && len(nodes) < 2 {
		Skip(fmt.Sprintf("There is not enough nodes to run test with following parameter %s", node))
	}

	By("Validating test parameters")

	if VRFParameters.Node == netcniparameters.SameNode {
		podClientNodeLabel = nodes[0]
		podServerNodeLabel = nodes[0]
	} else if VRFParameters.Node == netcniparameters.DiffNode {
		podClientNodeLabel = nodes[0]
		podServerNodeLabel = nodes[1]
	}

	By("Define client/server pods")

	if VRFParameters.IPStack == netcniparameters.IPStackIPv4 {
		podClientIpamConfig = fmt.Sprintf(
			`[{"name": "%s", "mac": "%s"}, {"name": "%s", "mac": "%s"}]`,
			vrfNetworkBlue[0], "20:04:0f:f1:88:A1", vrfNetworkRed[0], "20:04:0f:f1:88:B2")

		podServerIpamConfig = fmt.Sprintf(
			`[{"name": "%s", "mac": "%s"}, {"name": "%s", "mac": "%s"}]`,
			vrfNetworkBlue[0], "20:04:0f:f1:88:A3", vrfNetworkRed[0], "20:04:0f:f1:88:B4")
	} else if VRFParameters.IPStack == netcniparameters.IPStackIPv6 {
		podClientIpamConfig = fmt.Sprintf(
			`[{"name": "%s", "mac": "%s"}, {"name": "%s", "mac": "%s"}]`,
			vrfNetworkBlue[1], "20:04:0f:f1:88:A1", vrfNetworkRed[1], "20:04:0f:f1:88:B2")

		podServerIpamConfig = fmt.Sprintf(
			`[{"name": "%s", "mac": "%s"}, {"name": "%s", "mac": "%s"}]`,
			vrfNetworkBlue[1], "20:04:0f:f1:88:A3", vrfNetworkRed[1], "20:04:0f:f1:88:B4")
	}

	podClient := pod.RedefineAsNetRaw(
		pod.RedefinePodWithNetwork(
			pod.DefinePodOnNode(netcniparameters.TestNamespace, config.Network.TestContainerImage, podClientNodeLabel),
			podClientIpamConfig,
		),
	)

	podServer := defineServerPodMultiHTTPContainers(config, podServerNodeLabel, podServerIpamConfig)

	By("Running client/server pods")

	runningClientPod := globalHelper.WaitUntilPodCreatedAndRunning(podClient, netcniparameters.PodWaitingTime)
	runningServerPod := globalHelper.WaitUntilPodCreatedAndRunning(podServer, netcniparameters.PodWaitingTime)

	By("Validating client/server VRFs configuration")

	vrfBlueClientIP := getPodIPs(runningClientPod, netcniparameters.VRFBlueName)[0]
	vrfRedClientIP := getPodIPs(runningClientPod, netcniparameters.VRFRedName)[0]

	podHasCorrectVrfConfig(podClient.Name,
		[]map[string]string{
			{"vrfName": netcniparameters.VRFBlueName, "vrfClientIP": vrfBlueClientIP, "vrfInterface": "net1"},
			{"vrfName": netcniparameters.VRFRedName, "vrfClientIP": vrfRedClientIP, "vrfInterface": "net2"}})
	podHasCorrectVrfConfig(podServer.Name,
		[]map[string]string{
			{"vrfName": netcniparameters.VRFBlueName, "vrfClientIP": vrfBlueClientIP, "vrfInterface": "net1"},
			{"vrfName": netcniparameters.VRFRedName, "vrfClientIP": vrfRedClientIP, "vrfInterface": "net2"}})

	By("Validating client/server ICMP VRF connectivity")

	vrfRedServerIP := getPodIPs(runningServerPod, netcniparameters.VRFRedName)[0]
	vrfBlueServerIP := getPodIPs(runningServerPod, netcniparameters.VRFBlueName)[0]

	err = pingIPViaVRF(*runningClientPod, netcniparameters.VRFRedName, vrfRedServerIP, false)
	Expect(err).ToNot(HaveOccurred())
	err = pingIPViaVRF(*runningClientPod, netcniparameters.VRFBlueName, vrfBlueServerIP, false)
	Expect(err).ToNot(HaveOccurred())

	By("Validating client/server TCP VRF connectivity")

	err = httpViaVRF(*runningClientPod, vrfRedServerIP, netcniparameters.VRFRedName, false)
	Expect(err).ToNot(HaveOccurred())
	err = httpViaVRF(*runningClientPod, vrfBlueServerIP, netcniparameters.VRFBlueName, false)
	Expect(err).ToNot(HaveOccurred())
	err = pod.DeletePodAndWait(globalHelper.Apiclient, podServer)
	Expect(err).ToNot(HaveOccurred())

	By("Validating client/server ICMP negative test")
	Eventually(func() error {
		_, err := globalHelper.Apiclient.Pods(netcniparameters.TestNamespace).Get(
			context.Background(),
			podServer.Name,
			metav1.GetOptions{})

		return err
	}, netcniparameters.PodWaitingTime, 5*time.Second).Should(HaveOccurred())

	err = pingIPViaVRF(*runningClientPod, netcniparameters.VRFBlueName, vrfBlueServerIP, true)
	Expect(err).ToNot(HaveOccurred())
	err = pingIPViaVRF(*runningClientPod, netcniparameters.VRFRedName, vrfRedServerIP, true)
	Expect(err).ToNot(HaveOccurred())
	By("Validating client/server TCP negative test")

	err = httpViaVRF(*runningClientPod, vrfRedServerIP, netcniparameters.VRFRedName, true)
	Expect(err).ToNot(HaveOccurred())
	err = httpViaVRF(*runningClientPod, vrfBlueServerIP, netcniparameters.VRFBlueName, true)
	Expect(err).ToNot(HaveOccurred())
}

// return an array of IPs in a pod related to a given lowercase string network name.
func getPodIPs(runningPod *k8sv1.Pod, networkName string) []string {
	networks, err := podNetworksParser(runningPod.Annotations[netcniparameters.AnnotationNetStat])
	Expect(err).ToNot(HaveOccurred())

	network, err := findNetworkName(networks, networkName)
	Expect(err).ToNot(HaveOccurred())

	return network.IPs
}

// Parsers a string of annotation to PodNetwork array
// creates an error in parsing failed.
func podNetworksParser(strFormat string) ([]netattdefv1.NetworkStatus, error) {
	var vrfNetworkStatus []netattdefv1.NetworkStatus

	if err := json.Unmarshal([]byte(strFormat), &vrfNetworkStatus); err != nil {
		return nil, fmt.Errorf("network not found")
	}

	return vrfNetworkStatus, nil
}

// Finds and return a PodNetwork related to a given lowercase string network name
// creates an error if PodNetwork with a name.
func findNetworkName(vrfNetworksStatus []netattdefv1.NetworkStatus,
	networkName string) (netattdefv1.NetworkStatus, error) {
	for _, podNetwork := range vrfNetworksStatus {
		if strings.Contains(strings.ToLower(podNetwork.Name), networkName) {
			return podNetwork, nil
		}
	}

	return netattdefv1.NetworkStatus{}, fmt.Errorf("network not found")
}
