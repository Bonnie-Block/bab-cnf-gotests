package netmetallbhelper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	metallbv1alpha1 "github.com/metallb/metallb-operator/api/v1alpha1"
	metallbv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	"github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	k8sv1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type queryOutput struct {
	Data data
}
type data struct {
	Result []result
}

type result struct {
	Metric metric
}

type metric struct {
	Pod string
}

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

// IsMetalLBAvailable verifies that metallb installed and running.
func IsMetalLBAvailable() error {
	err := helper.IsDaemonsetReady(helper.Apiclient,
		netmlbparameters.MetalLBOperatorNameSpace,
		netmlbparameters.MetalLBDaemonsetName)
	if err != nil {
		return errors.Errorf("MetalLB speaker daemonset not ready")
	}

	isMLBDeploymentReady, err := helper.IsDeploymentReady(helper.Apiclient,
		netmlbparameters.MetalLBOperatorNameSpace,
		netmlbparameters.MetalLBDeploymentName)
	if err != nil {
		return err
	}

	if !isMLBDeploymentReady {
		return errors.Errorf("MetalLB controller deployment not ready")
	}

	return nil
}

// DefineBFDProfile returns BFDprofile definition.
func DefineBFDProfile(name string) *metallbv1beta1.BFDProfile {
	return &metallbv1beta1.BFDProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: netmlbparameters.MetalLBOperatorNameSpace,
		},
		Spec: metallbv1beta1.BFDProfileSpec{
			ReceiveInterval:  uint32Ptr(100),
			TransmitInterval: uint32Ptr(100),
			DetectMultiplier: uint32Ptr(3),
			EchoInterval:     uint32Ptr(100),
			EchoMode:         pointer.BoolPtr(true),
			PassiveMode:      pointer.BoolPtr(false),
			MinimumTTL:       uint32Ptr(5),
		},
	}
}

// DefineBGPPeerWithBFD returns BGPPeer definition with BFD configuration.
func DefineBGPPeerWithBFD(peerAdress string, asn uint32, bfdProfile string) *metallbv1beta1.BGPPeer {
	return &metallbv1beta1.BGPPeer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      netmlbparameters.BGPPeerName,
			Namespace: netmlbparameters.MetalLBOperatorNameSpace,
		},
		Spec: metallbv1beta1.BGPPeerSpec{
			MyASN:      64500,
			ASN:        asn,
			Address:    peerAdress,
			RouterID:   "10.10.10.10",
			BFDProfile: bfdProfile,
		},
	}
}

// DefineBFDMLBConfigMap returns configmap definition with FRR BFD configuration.
func DefineBFDMLBConfigMap(ipAddresses []string, configMapName string, asn int) *k8sv1.ConfigMap {
	configMapData := make(map[string]string)
	configMapData["daemons"] = netmlbparameters.DaemonsFile

	bfdConfig, err := defineBFDConfig(ipAddresses, asn)
	Expect(err).ToNot(HaveOccurred())

	configMapData["frr.conf"] = bfdConfig
	configMap := nethelper.DefineFRRConfigMap(configMapName, netmlbparameters.TestNamespace, configMapData)

	return configMap
}

// defineBFDConfig returns string which represents BFD config file peering to all given IP addresses.
func defineBFDConfig(neighborsIPAddresses []string, asn int) (string, error) {
	if len(neighborsIPAddresses) < 1 {
		return "", fmt.Errorf("list of neigbors ip addresses is empty")
	}

	asnStr := strconv.Itoa(asn)
	bfdConfig := "bfd\n router bgp 64501\n"

	for _, ipAddress := range neighborsIPAddresses {
		bfdConfig += fmt.Sprintf(" neighbor %s remote-as %s\n neighbor %s bfd\n", ipAddress, asnStr, ipAddress)
	}

	bfdConfig += "!"

	return bfdConfig, nil
}

type BGPDescription struct {
	BGPState string `json:"bgpState"`
}

// IsBGPNeighborshipHasState verifies that BGP session on a pod has given state.
func IsBGPNeighborshipHasState(frrPod *k8sv1.Pod, neighborIPAddress string, state string) bool {
	bgpStateOut, err := pod.ExecCommand(helper.Apiclient, *frrPod,
		[]string{"vtysh", "-u", "-c", "sh bgp neighbors json"})
	Expect(err).ToNot(HaveOccurred())

	result := map[string]BGPDescription{}
	err = json.Unmarshal(bgpStateOut.Bytes(), &result)
	Expect(err).ToNot(HaveOccurred(), bgpStateOut.String())

	return result[neighborIPAddress].BGPState == state
}

// UpdateSpeakerNodeSelector updates SpeakerNodeSelector in Metallb CR.
func UpdateSpeakerNodeSelector(namespace string, nodeSelector map[string]string) error {
	metallb := &metallbv1beta1.MetalLB{}

	err := helper.Apiclient.Get(context.Background(), types.NamespacedName{Name: "metallb", Namespace: namespace}, metallb)
	if err != nil {
		return err
	}

	metallb.Spec.SpeakerNodeSelector = nodeSelector

	err = helper.Apiclient.Update(context.Background(), metallb)
	if err != nil {
		return err
	}

	return nil
}

// DeleteAllBFDProfiles removes all BFDProfile CRs.
func DeleteAllBFDProfiles() error {
	bfdProfileList := metallbv1beta1.BFDProfileList{}

	err := helper.Apiclient.List(context.Background(), &bfdProfileList,
		runtimeclient.InNamespace(netmlbparameters.MetalLBOperatorNameSpace))
	if err != nil {
		return err
	}

	for _, bfdProfile := range bfdProfileList.Items {
		err = helper.Apiclient.Delete(context.Background(), &bfdProfile)
		if err != nil {
			return err
		}
	}

	// Failed due to BZ 2050824. The BFD should be uncommented once the BZ is fixed
	// Eventually(func() bool {
	//	return IsProtocolConfigured(netmlbparameters.BFDConfigPrefix)
	// }, 1*time.Minute, 2*time.Second).Should(BeFalse(), "BFD configuration is not removed")

	return nil
}

// UpdateToDefaultSpeakerNodeSelector updates a Metallb CR to the default SpeakerNodeSelector.
func UpdateToDefaultSpeakerNodeSelector() error {
	metallb := &metallbv1beta1.MetalLB{}

	err := helper.Apiclient.Get(context.Background(),
		types.NamespacedName{Name: netmlbparameters.MetalLBCRName,
			Namespace: netmlbparameters.MetalLBOperatorNameSpace}, metallb)
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(metallb.Spec.SpeakerNodeSelector, netmlbparameters.SpeakerNodeSelectorWorker) {
		metallb.Spec.SpeakerNodeSelector = netmlbparameters.SpeakerNodeSelectorWorker

		err = helper.Apiclient.Update(context.Background(), metallb)
		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteAllBGPPeers removes all BGPPeer CRs.
func DeleteAllBGPPeers() error {
	bgpPeerList := metallbv1beta1.BGPPeerList{}

	err := helper.Apiclient.List(context.Background(), &bgpPeerList,
		runtimeclient.InNamespace(netmlbparameters.MetalLBOperatorNameSpace))
	if err != nil {
		return err
	}

	for _, bgpPeer := range bgpPeerList.Items {
		err = helper.Apiclient.Delete(context.Background(), &bgpPeer)
		if err != nil {
			return err
		}
	}

	Eventually(func() bool {
		return IsProtocolConfigured(netmlbparameters.BGPConfigPrefix)
	}, 1*time.Minute, 2*time.Second).Should(BeFalse(), "BGP configuration is not removed")

	return nil
}

// AreSpeakersReady verifies that Speakers are up and running.
func AreSpeakersReady() bool {
	daemonSet := &appsv1.DaemonSet{}

	err := helper.Apiclient.Get(context.Background(),
		types.NamespacedName{Name: netmlbparameters.MetalLBDaemonsetName,
			Namespace: netmlbparameters.MetalLBOperatorNameSpace}, daemonSet)
	Expect(err).ToNot(HaveOccurred())

	if daemonSet.Status.DesiredNumberScheduled == 0 ||
		daemonSet.Status.DesiredNumberScheduled != daemonSet.Status.NumberAvailable {
		return false
	}

	return true
}

// CreateMetallb creates Metallb CR.
func CreateMetallb() *metallbv1beta1.MetalLB {
	metallb := defineMetallb()

	err := helper.Apiclient.Get(context.Background(), runtimeclient.ObjectKey{Namespace: metallb.Namespace,
		Name: metallb.Name}, metallb)
	if apiErrors.IsNotFound(err) {
		Expect(helper.Apiclient.Create(context.Background(), metallb)).Should(Succeed())
	} else {
		Expect(err).ToNot(HaveOccurred())
	}

	return metallb
}

// defineMetallb returns Metallb definition.
func defineMetallb() *metallbv1beta1.MetalLB {
	return &metallbv1beta1.MetalLB{
		ObjectMeta: metav1.ObjectMeta{
			Name:      netmlbparameters.MetalLBCRName,
			Namespace: netmlbparameters.MetalLBOperatorNameSpace,
		},
		Spec: metallbv1beta1.MetalLBSpec{
			SpeakerNodeSelector: netmlbparameters.SpeakerNodeSelectorWorker,
		},
	}
}

// DeleteLabelFromWorkers removes a label from all workers.
func DeleteLabelFromWorkers(label string) error {
	workerNodes, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
	if err != nil {
		return err
	}

	if len(workerNodes) == 0 {
		return fmt.Errorf("worker node list is empty")
	}

	for _, node := range workerNodes {
		delete(node.Labels, label)

		_, err = helper.Apiclient.Nodes().Update(context.Background(), &node, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to remove label from %s %w", node.Name, err)
		}
	}

	return nil
}

// CollectMetalLBMetricsByPod returns MetalLB metrics from speaker pods by prefix.
func CollectMetalLBMetricsByPod(speakerPods []k8sv1.Pod, prefix string) (map[string][]string, []string) {
	uniqueMetricKeys := []string{}
	monitoredEntriesByPod := map[string][]string{}

	Expect(speakerPods).ShouldNot(BeEmpty(), "List of Speakers is empty")

	for _, speakerPod := range speakerPods {
		podEntries := []string{}

		var (
			stdout bytes.Buffer
			err    error
		)

		Eventually(func() error {
			stdout, err = pod.ExecCommand(helper.Apiclient, speakerPod, []string{"curl", "localhost:7473/metrics"})
			if len(strings.Split(stdout.String(), "\n")) == 0 {
				return fmt.Errorf("empty response")
			}

			return err
		}, 1*time.Minute, 2*time.Second).ShouldNot(HaveOccurred())

		for _, line := range strings.Split(stdout.String(), "\n") {
			if strings.HasPrefix(line, prefix) {
				metricsKey := line[0:strings.Index(line, "{")]
				podEntries = append(podEntries, metricsKey)
				uniqueMetricKeys = appendIfMissing(uniqueMetricKeys, metricsKey)
			}
		}

		monitoredEntriesByPod[speakerPod.Name] = podEntries
	}

	Expect(uniqueMetricKeys).ShouldNot(BeEmpty(), "There is no metrics on a pod")
	Expect(monitoredEntriesByPod).ShouldNot(BeEmpty(), "There is no metrics on a pod")

	return monitoredEntriesByPod, uniqueMetricKeys
}

// CollectPrometheusMetrics returns  metrics from prometheus pod by uniqueMetricKeys.
func CollectPrometheusMetrics(uniqueMetricKeys []string) map[string][]string {
	prometheusPods, err := helper.Apiclient.Pods(parameters.PromNamespace).List(context.Background(),
		metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=prometheus",
		})
	Expect(err).ToNot(HaveOccurred())
	Expect(prometheusPods.Items).NotTo(BeEmpty())

	podsPerPrometheusMetricKey := map[string][]string{}

	Expect(uniqueMetricKeys).NotTo(BeEmpty())

	for _, metricsKey := range uniqueMetricKeys {
		podsPerKey := []string{}

		command := []string{
			"curl",
			fmt.Sprintf("%squery?query=%s", parameters.PromLocalURL, metricsKey),
		}
		stdout, err := pod.ExecCommand(helper.Apiclient, prometheusPods.Items[0], command)
		Expect(err).ToNot(HaveOccurred())

		var queryOutput queryOutput
		err = json.Unmarshal(stdout.Bytes(), &queryOutput)
		Expect(err).ToNot(HaveOccurred(), stdout.String())

		for _, result := range queryOutput.Data.Result {
			podsPerKey = append(podsPerKey, result.Metric.Pod)
		}

		podsPerPrometheusMetricKey[metricsKey] = podsPerKey
	}

	Expect(podsPerPrometheusMetricKey).NotTo(BeEmpty(), "There is no metrics on a Prometheus pod")

	return podsPerPrometheusMetricKey
}

// ContainSameMetrics verifies that metricsByPod have prometheusMetrics.
func ContainSameMetrics(metricsByPod map[string][]string, prometheusMetrics map[string][]string) error {
	for podName, monitoringKeys := range metricsByPod {
		for _, key := range monitoringKeys {
			if podsWithMetric, ok := prometheusMetrics[key]; ok {
				// We only check if the element is present, but do not compare the values
				// New values are reported periodically, and there is a risk of discrepancies
				// in the values read from metalLB Speaker pods and the ones read from prometheus
				if hasElement(podsWithMetric, podName) {
					continue
				}
			}

			return fmt.Errorf("metric %s on pod %s was not reported", key, podName)
		}
	}

	return nil
}

// IsProtocolConfigured checks for the presence of a protocol prefix in running-config on Speakers.
func IsProtocolConfigured(protocolPrefix string) bool {
	speakerPodList, err := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).List(
		context.Background(),
		metav1.ListOptions{LabelSelector: netmlbparameters.SpeakersLabelSelector},
	)
	Expect(err).ToNot(HaveOccurred())

	for _, speakerPod := range speakerPodList.Items {
		configStateOut, err := pod.ExecCommand(helper.Apiclient, speakerPod,
			[]string{"vtysh", "-c", "sh run"})
		Expect(err).ToNot(HaveOccurred())

		configs := strings.Split(configStateOut.String(), "!")
		for _, config := range configs {
			if strings.HasPrefix(strings.TrimSpace(config), protocolPrefix) {
				return true
			}
		}
	}

	return false
}

func appendIfMissing(slice []string, newItem string) []string {
	if hasElement(slice, newItem) {
		return slice
	}

	return append(slice, newItem)
}

func hasElement(slice []string, item string) bool {
	for _, sliceItem := range slice {
		if item == sliceItem {
			return true
		}
	}

	return false
}

func uint32Ptr(n uint32) *uint32 {
	return &n
}
