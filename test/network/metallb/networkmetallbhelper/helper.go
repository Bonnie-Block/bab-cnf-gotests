package networkmetallbhelper

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"

	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	globalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	metallbParameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/networkvrfhelper"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	metallbv1alpha1 "github.com/metallb/metallb-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/intstr"

	k8sv1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// CheckEnvVar validates that the enviromnental IP variable is a valid IP and in the
// same IP range as the br-ex interface of the cluster under-test.
// MetallB Down-stream tests will only run on clusters Helix 2,3 and 7.
func CheckEnvVar() bool {
	Config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())
	metallbEnvIPVar := Config.GetMetallbEnvVar()
	//Checks that the METALLB_ADDR_LIST is set and has valid IP addresses
	for _, v := range metallbEnvIPVar {
		if net.ParseIP(v) == nil {
			Skip("The environment IP variable is not set or is not a valid IP")
		}
	}
	//Checks that the METALLB_ADDR_LIST is in the range of the cluster br-ex interface
	node := networkvrfhelper.GetNodeListStringByLabel(strings.Split(Config.General.CnfNodeLabel, "/")[1])
	event, _ := globalHelper.Apiclient.Nodes().Get(context.Background(), node[0], metav1.GetOptions{})
	v, _ := event.Annotations["k8s.ovn.org/node-primary-ifaddr"]
	// Output example {"ipv4":"10.46.56.13/24"} len = 5
	nodeOutput := strings.Split(v, "\"")
	Expect(len(nodeOutput)).Should(Equal(5))
	_, nodeNet, err := net.ParseCIDR(nodeOutput[3])
	Expect(err).ToNot(HaveOccurred())
	if !nodeNet.Contains(net.ParseIP(metallbEnvIPVar[0])) {
		Skip("The environment IP variable is out of cluster br-ex IP range")
	}
	return true
}

//DefineMetallbAddressPool defines a MetalLB L2 Address Pool using env IP var METALLB_ADDR_LIST for the IP address range
func DefineMetallbAddressPool() *metallbv1alpha1.AddressPool {
	Config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())
	ap := &metallbv1alpha1.AddressPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      metallbParameters.AddressPool,
			Namespace: metallbParameters.MetalLBOperatorNameSpace,
			Annotations: map[string]string{
				metallbParameters.MetalLBAddressPool: metallbParameters.AddressPool,
			},
		},
		Spec: metallbv1alpha1.AddressPoolSpec{
			Name:     metallbParameters.AddressPool,
			Protocol: "layer2",
			Addresses: []string{
				fmt.Sprintln(Config.GetMetallbEnvVar()[0], "-", Config.GetMetallbEnvVar()[1]),
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
				metallbParameters.MetalLBAddressPool: metallbParameters.AddressPool,
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
	serviceEvents, err := globalHelper.Apiclient.Events(metallbParameters.TestNamespace).List(context.Background(), metav1.ListOptions{FieldSelector: "reason=nodeAssigned"})
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
	event, err := globalHelper.Apiclient.Nodes().Get(context.Background(), metallbNode, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())
	v, _ := event.Annotations["k8s.ovn.org/l3-gateway-config"]
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
	runningPod := globalHelper.WaitUntilPodCreatedAndRunning(podDefPrivHostNet, metallbParameters.PodWaitingTime)
	return runningPod
}

//MLBClientPod with nginx listening on port 80
func MLBClientPod(node string, image string) *k8sv1.Pod {
	podDefNodeLabel := redefineWithLabel(pod.DefinePodOnNode(metallbParameters.TestNamespace, image, node))
	podDefPrivCommand := pod.RedefineAsPrivileged(pod.RedefineWithCommand(podDefNodeLabel,
		[]string{"/bin/bash", "-c"},
		[]string{"nginx && sleep INF"}))
	runningPod := globalHelper.WaitUntilPodCreatedAndRunning(podDefPrivCommand, metallbParameters.PodWaitingTime)
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
	arpStatus, err := pod.ExecCommand(globalHelper.Apiclient, client, []string{"bash", "-c", command})
	Expect(err).ToNot(HaveOccurred())
	macs := arpStatus.String()
	return strings.Split(macs, "\n")
}

//CurlMlbPod verifies that nginx web service is available via the external service IP
func CurlMlbPod(client k8sv1.Pod, DestIPAddr string) string {
	command := fmt.Sprint("curl ", DestIPAddr)
	curlStatus, err := pod.ExecCommand(globalHelper.Apiclient, client, []string{"bash", "-c", command})
	Expect(err).ToNot(HaveOccurred())
	return curlStatus.String()
}
