package netmetallbhelper

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	metallbv1alpha1 "github.com/metallb/metallb-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/intstr"

	k8sv1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// IsEnvVarMetallbIPinNodeExtNetRange validates that the enviromnental IP variable
// is in the same IP range as the br-ex interface of the cluster under-test.
// MetallB Down-stream tests will only run on clusters Helix 2,3 and 7.
func IsEnvVarMetallbIPinNodeExtNetRange(cnfNodeLabel string, metallbEnvIP string) bool {
	// Checks that the METALLB_ADDR_LIST is in the range of the cluster br-ex interface.
	node := helper.GetNodeListStringByLabel(cnfNodeLabel)
	event, _ := helper.Apiclient.Nodes().Get(context.Background(), node[0], metav1.GetOptions{})
	val := event.Annotations[netmlbparameters.AnnotationPrimaryIfaddr]
	// Output example {"ipv4":"10.46.56.13/24"} len = 5
	nodeOutput := strings.Split(val, "\"")
	Expect(len(nodeOutput)).Should(Equal(5))
	_, nodeNet, err := net.ParseCIDR(nodeOutput[3])
	Expect(err).ToNot(HaveOccurred())

	if !nodeNet.Contains(net.ParseIP(metallbEnvIP)) {
		Skip("The environment IP variable is out of cluster br-ex IP range")
	}

	return true
}

// DefineMetallbAddressPool defines a MetalLB L2 Address Pool using env IP var METALLB_ADDR_LIST
// for the IP address range.
func DefineMetallbAddressPool(metallbIP []string) *metallbv1alpha1.AddressPool {
	return &metallbv1alpha1.AddressPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      netmlbparameters.AddressPool,
			Namespace: netmlbparameters.MetalLBOperatorNameSpace,
			Annotations: map[string]string{
				netmlbparameters.MetalLBAddressPool: netmlbparameters.AddressPool,
			},
		},
		Spec: metallbv1alpha1.AddressPoolSpec{
			Protocol: "layer2",
			Addresses: []string{
				fmt.Sprintln(metallbIP[0], "-", metallbIP[1]),
			},
		},
	}
}

// CreateAddressPool creates the MetalLB L2 Address Pool using func defineMetallbAddressPool.
func CreateAddressPool(addresspool *metallbv1alpha1.AddressPool) error {
	return helper.Apiclient.Create(context.Background(), addresspool)
}

// DeleteAddressPool from namespace metallb-system using func defineMetallbAddressPool.
func DeleteAddressPool(addresspool *metallbv1alpha1.AddressPool) error {
	return helper.Apiclient.Delete(context.Background(), addresspool)
}

// CreateLBService create an external service using the MetalLB Address Pool allowing connectivity from network host
// interface br-ex to the nginx pod on port 30101.
func CreateLBService(clientSet *client.ClientSet, namespace string) *k8sv1.Service {
	service := k8sv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				netmlbparameters.MetalLBAddressPool: netmlbparameters.AddressPool,
			},
			Name:      "metallb-service",
			Namespace: namespace,
		},
		Spec: k8sv1.ServiceSpec{
			Selector: map[string]string{
				"app": "nginx",
			},
			Ports: []k8sv1.ServicePort{
				{
					Protocol: k8sv1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 80,
					},
				},
			},
			Type: "LoadBalancer",
		},
	}
	activeService, err := clientSet.Services(namespace).Create(context.Background(),
		&service, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())

	return activeService
}

// GetLBServiceAnnouncingNodeName searches for node name in following string example:
// "announcing from node "helix13.lab.eng.tlv2.redhat.com".
func GetLBServiceAnnouncingNodeName() (string, error) {
	var allEvents []string

	serviceEvents, err := helper.Apiclient.Events(netmlbparameters.TestNamespace).List(context.Background(),
		metav1.ListOptions{FieldSelector: "reason=nodeAssigned"})

	for _, index := range strings.Split(serviceEvents.String(), "}") {
		if strings.Contains(index, "announcing from node") {
			re := regexp.MustCompile(`"([^\"]+)"`)
			event := re.FindString(index)
			allEvents = append(allEvents, event)
		}
	}

	numOfEvents := len(allEvents)
	lastEvent := strings.Trim(allEvents[numOfEvents-1], "\"")

	return lastEvent, err
}

// SpeakerNodeMac locates the MAC address of the node interface br-ex found in func GetLBServiceNodeName()
// {"mode":"shared","interface-id":"br-ex_helix13.lab.eng.tlv2.redhat.com","mac-address":"34:48:ed:f3:88:c4",
// "ip-addresses":["10.46.56.13/24"],"ip-address":"10.46.56.13/24","next-hops":["10.46.56.254"],"next-hop":
// "10.46.56.254","node-port-enable":"true","vlan-id":"0"}.
func SpeakerNodeMac(metallbNode string) (string, error) {
	event, err := helper.Apiclient.Nodes().Get(context.Background(), metallbNode, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())

	val := event.Annotations[netmlbparameters.AnnotationL3GW]

	for _, index := range strings.Split(val, ",") {
		if strings.Contains(index, "mac-address") {
			re := regexp.MustCompile("([0-9a-fA-F]{2}[:]){5}([0-9a-fA-F]{2})")
			mFind := re.FindAllString(index, -1)

			return strings.Join(mFind, ""), err
		}
	}

	return "", fmt.Errorf("failed to find service node mac")
}

// MLBTestPod creates a pod connected to the host network br-ex interface.
func MLBTestPod(node string, ns string, image string) *k8sv1.Pod {
	podDefPrivHostNet := pod.RedefineAsPrivileged(pod.DefineWithHostNetwork(node, ns, image))
	runningPod := helper.WaitUntilPodCreatedAndRunning(podDefPrivHostNet, netmlbparameters.PodWaitingTime)

	return runningPod
}

// MLBClientPod with nginx listening on port 80.
func MLBClientPod(node string, image string) *k8sv1.Pod {
	podDefNodeLabel := redefineWithLabel(pod.DefinePodOnNode(netmlbparameters.TestNamespace, image, node))
	podDefPrivCommand := pod.RedefineAsPrivileged(pod.RedefineWithCommand(podDefNodeLabel,
		[]string{"/bin/bash", "-c"},
		[]string{"nginx && sleep INF"}))
	runningPod := helper.WaitUntilPodCreatedAndRunning(podDefPrivCommand, netmlbparameters.PodWaitingTime)

	return runningPod
}

// redefineWithLabel updates DefinePodOnNode() with label.
func redefineWithLabel(pod *k8sv1.Pod) *k8sv1.Pod {
	pod.ObjectMeta.Labels = map[string]string{"app": "nginx"}

	return pod
}

// Arping verifies only one node replies to arping and that the service node br-ex mac matches the output.
func Arping(destIPAddr string, image string, nodeListString []string, node string, reboot bool) error {
	var indexInt int

	for index, value := range nodeListString {
		if value == node && reboot {
			indexInt = index
		}

		if value == node && index == 0 && !reboot {
			indexInt = 1
		}
	}

	testPod := MLBTestPod(nodeListString[indexInt],
		netmlbparameters.DefaultNameSpace, image)

	arpStatus, err := pod.ExecCommand(helper.Apiclient, *testPod, []string{"bash", "-c", fmt.Sprint("arping -I br-ex ",
		destIPAddr, " -c2")})
	Expect(err).ToNot(HaveOccurred())

	macs := arpStatus.String()
	output := strings.Split(macs, "\n")
	lineCount := 0

	for _, reply := range output {
		if strings.Contains(reply, "Unicast") {
			lineCount++
		}
	}

	Expect(lineCount).To(Equal(2), "An incorrect number of arp replies were received")
	// Verifies the output mac addresses matches the annoucing node mac address
	nodeMac, err := SpeakerNodeMac(node)
	Expect(strings.Join(output, "\n")).Should(ContainSubstring(strings.ToUpper(nodeMac)),
		"ARP request was not received from the announcing node")
	Expect(err).ToNot(HaveOccurred())

	return err
}

// Arping verifies only one node replies to arping and that the service node br-ex mac matches the output.
func IPAddBrEx(client k8sv1.Pod) ([]string, error) {
	ipAddr, err := pod.ExecCommand(helper.Apiclient, client, []string{"bash", "-c", "ip a show br-ex"})
	Expect(err).ToNot(HaveOccurred())

	return strings.Split(ipAddr.String(), ","), err
}

// CurlMlbPod verifies that nginx web service is available via the external service IP.
func CurlMlbPod(destIPAddr string, image string, nodeListString []string, node string, reboot bool) error {
	var indexInt int

	for index, v := range nodeListString {
		if v == node && reboot {
			indexInt = index
		}

		if v == node && index == 0 && !reboot {
			indexInt = 1
		}
	}

	testPod := MLBTestPod(nodeListString[indexInt],
		netmlbparameters.DefaultNameSpace, image)
	curlStatus, err := pod.ExecCommand(helper.Apiclient, *testPod, []string{"bash", "-c", fmt.Sprint("curl ", destIPAddr)})
	Expect(err).ToNot(HaveOccurred())

	Expect(curlStatus.String()).Should(ContainSubstring("html"),
		"Curl was unable to connect to nginx")

	return err
}
