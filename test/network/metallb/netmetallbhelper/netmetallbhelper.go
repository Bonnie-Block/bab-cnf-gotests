package netmetallbhelper

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"

	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
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
	//Checks that the METALLB_ADDR_LIST is in the range of the cluster br-ex interface
	node := GetNodeListStringByLabel(cnfNodeLabel)
	event, _ := Apiclient.Nodes().Get(context.Background(), node[0], metav1.GetOptions{})
	v, _ := event.Annotations[netmlbparameters.AnnotationPrimaryIfaddr]
	// Output example {"ipv4":"10.46.56.13/24"} len = 5
	nodeOutput := strings.Split(v, "\"")
	Expect(len(nodeOutput)).Should(Equal(5))
	_, nodeNet, err := net.ParseCIDR(nodeOutput[3])
	Expect(err).ToNot(HaveOccurred())
	if !nodeNet.Contains(net.ParseIP(metallbEnvIP)) {
		Skip("The environment IP variable is out of cluster br-ex IP range")
	}
	return true
}

//DefineMetallbAddressPool defines a MetalLB L2 Address Pool using env IP var METALLB_ADDR_LIST for the IP address range
func DefineMetallbAddressPool() *metallbv1alpha1.AddressPool {
	metallbIP, err := Config.GetMetallbVirtIP()
	Expect(err).ToNot(HaveOccurred())
	ap := &metallbv1alpha1.AddressPool{
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
	return ap
}

//CreateAddressPool creates the MetalLB L2 Address Pool using func defineMetallbAddressPool
func CreateAddressPool(addresspool *metallbv1alpha1.AddressPool) error {
	return Apiclient.Create(context.Background(), addresspool)
}

//DeleteAddressPool from namespace metallb-system using func defineMetallbAddressPool
func DeleteAddressPool(addresspool *metallbv1alpha1.AddressPool) error {
	return Apiclient.Delete(context.Background(), addresspool)
}

//CreateLBService create an external service using the MetalLB Address Pool allowing connectivity from network host
//interface br-ex to the nginx pod on port 30101
func CreateLBService(cs *client.ClientSet, namespace string) *k8sv1.Service {
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
	activeService, err := cs.Services(namespace).Create(context.Background(), &service, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	return activeService
}

//GetLBServiceEvents searches for node name in following string example:
// "announcing from node "helix13.lab.eng.tlv2.redhat.com"
func GetLBServiceEvents() (string, error) {
	serviceEvents, err := Apiclient.Events(netmlbparameters.TestNamespace).List(context.Background(), metav1.ListOptions{FieldSelector: "reason=nodeAssigned"})
	re := regexp.MustCompile(`"([^\"]+)"`)
	node := re.FindAllString(serviceEvents.Items[0].Message, -1)
	nodeName := strings.Trim(node[0], "\"")
	return nodeName, err
}

//SpeakerNodeMac locates the MAC address of the node interface br-ex found in func GetLBServiceEvents()
// {"mode":"shared","interface-id":"br-ex_helix13.lab.eng.tlv2.redhat.com","mac-address":"34:48:ed:f3:88:c4",
// "ip-addresses":["10.46.56.13/24"],"ip-address":"10.46.56.13/24","next-hops":["10.46.56.254"],"next-hop":
// "10.46.56.254","node-port-enable":"true","vlan-id":"0"}
func SpeakerNodeMac(metallbNode string) (string, error) {
	event, err := Apiclient.Nodes().Get(context.Background(), metallbNode, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())
	v, _ := event.Annotations[netmlbparameters.AnnotationL3GW]
	for _, i := range strings.Split(v, ",") {
		if strings.Contains(string(i), "mac-address") {
			re := regexp.MustCompile("([0-9a-fA-F]{2}[:]){5}([0-9a-fA-F]{2})")
			m := re.FindAllString(i, -1)
			return fmt.Sprintf(strings.Join(m, "")), err
		}
	}
	return "", fmt.Errorf("Failed to find service node mac")
}

//MLBTestPod creates a pod connected to the host network br-ex interface
func MLBTestPod(node string, ns string, image string) *k8sv1.Pod {
	podDefPrivHostNet := pod.RedefineAsPrivileged(pod.DefineWithHostNetwork(node, ns, image))
	runningPod := WaitUntilPodCreatedAndRunning(podDefPrivHostNet, netmlbparameters.PodWaitingTime)
	return runningPod
}

//MLBClientPod with nginx listening on port 80
func MLBClientPod(node string, image string) *k8sv1.Pod {
	podDefNodeLabel := redefineWithLabel(pod.DefinePodOnNode(netmlbparameters.TestNamespace, image, node))
	podDefPrivCommand := pod.RedefineAsPrivileged(pod.RedefineWithCommand(podDefNodeLabel,
		[]string{"/bin/bash", "-c"},
		[]string{"nginx && sleep INF"}))
	runningPod := WaitUntilPodCreatedAndRunning(podDefPrivCommand, netmlbparameters.PodWaitingTime)
	return runningPod
}

//redefineWithLabel updates DefinePodOnNode() with label
func redefineWithLabel(pod *k8sv1.Pod) *k8sv1.Pod {
	pod.ObjectMeta.Labels = map[string]string{"app": "nginx"}
	return pod
}

//Arping verifies only one node replies to arping and that the service node br-ex mac matches the output
func Arping(client k8sv1.Pod, DestIPAddr string) []string {
	command := fmt.Sprint("arping -c1 -I br-ex ", DestIPAddr)
	arpStatus, err := pod.ExecCommand(Apiclient, client, []string{"bash", "-c", command})
	Expect(err).ToNot(HaveOccurred())
	macs := arpStatus.String()
	return strings.Split(macs, "\n")
}

//CurlMlbPod verifies that nginx web service is available via the external service IP
func CurlMlbPod(client k8sv1.Pod, DestIPAddr string) string {
	command := fmt.Sprint("curl ", DestIPAddr)
	curlStatus, err := pod.ExecCommand(Apiclient, client, []string{"bash", "-c", command})
	Expect(err).ToNot(HaveOccurred())
	return curlStatus.String()
}
