package netmetallbhelper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os/exec"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	metallbv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	metallbutils "github.com/metallb/metallb-operator/test/e2e/metallb"
	operv1 "github.com/openshift/api/operator/v1"
	"github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	k8sv1 "k8s.io/api/core/v1"
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

type BGPDescription struct {
	BGPState string `json:"bgpState"`
}

var ChangedGWMode bool

// RestoreNodeGWMode restores the NetworkOperator's share GW mode if it has been changed.
func RestoreNodeGWMode() {
	if ChangedGWMode {
		SetLocalGWMode(false)
		WaitNetworkOperator()
	}
}

// GetGWMode returns the NetworkOperator's  GW mode: false - share GW mode, true - local GW mode.
// This func should be changed (removed cmd) when
// sriov-fec operator(https://github.com/smart-edge-open/openshift-operator/tree/main/sriov-fec)
// bumps github.com/go-logr/logr to version more than 1.0.0.
func GetGWMode() bool {
	cmd := exec.Command("oc", "get",
		"network.operator", "cluster",
		"-o=jsonpath='{.spec.defaultNetwork.ovnKubernetesConfig.gatewayConfig.routingViaHost}'")

	commandOutput, err := cmd.Output()
	Expect(err).ToNot(HaveOccurred())

	strState := strings.Trim(string(commandOutput), "'")

	state, err := strconv.ParseBool(strState)
	Expect(err).ToNot(HaveOccurred())

	return state
}

// SetLocalGWMode set NetworkOperator's local GW mode if true, if false - share GW mode.
// This func should be changed (removed cmd) when
// sriov-fec operator(https://github.com/smart-edge-open/openshift-operator/tree/main/sriov-fec)
// bumps github.com/go-logr/logr to version more than 1.0.0.
func SetLocalGWMode(state bool) {
	command := fmt.Sprintf("oc patch network.operator cluster -p '{\"spec\": {\"defaultNetwork\":"+
		" {\"ovnKubernetesConfig\": {\"gatewayConfig\": {\"routingViaHost\": %t}}}}}' --type=merge", state)

	commandOutput, err := exec.Command("/bin/sh", "-c", command).Output()
	Expect(err).ToNot(HaveOccurred(), string(commandOutput))
}

// isNetworkOperatorInCondition parses NetworkOperator conditions.
// Returns true if  NetworkOperator is in given condition, otherwise false.
func isNetworkOperatorInCondition(condition string, status operv1.ConditionStatus) bool {
	networkOperatorConfg := &operv1.Network{}
	err := helper.Apiclient.Get(
		context.TODO(), runtimeclient.ObjectKey{Name: "cluster"}, networkOperatorConfg)
	Expect(err).ToNot(HaveOccurred())

	for _, c := range networkOperatorConfg.Status.OperatorStatus.Conditions {
		if c.Type == condition && c.Status == status {
			return true
		}
	}

	return false
}

// WaitNetworkOperator waits for NetworkOperator to become available.
func WaitNetworkOperator() {
	// Update started
	Eventually(func() bool {
		return isNetworkOperatorInCondition(operv1.OperatorStatusTypeProgressing, operv1.ConditionTrue)
	}, 5*time.Second, netmlbparameters.Interval).Should(BeTrue())
	// Update finished
	Eventually(func() bool {
		return isNetworkOperatorInCondition(operv1.OperatorStatusTypeProgressing, operv1.ConditionFalse)
	}, 10*time.Minute, netmlbparameters.Interval).Should(BeTrue())
	// Update finished successfully
	Eventually(func() bool {
		return isNetworkOperatorInCondition(operv1.OperatorStatusTypeAvailable, operv1.ConditionTrue)
	}, 5*time.Second, 3*netmlbparameters.Interval).Should(BeTrue())
}

// IsEnvVarMetallbIPinNodeExtNetRange validates that the enviromnental IP variable
// is in the same IP range as the br-ex interface of the cluster under-test.
// MetallB Down-stream tests will only run on clusters Helix 2,3 and 7.
func IsEnvVarMetallbIPinNodeExtNetRange(cnfNodeLabel string, metallbEnvIP string) {
	// Checks that the METALLB_ADDR_LIST is in the range of the cluster br-ex interface.
	node := helper.GetNodeListStringByLabel(cnfNodeLabel)
	Expect(len(node)).To(BeNumerically(">", 0), "No Node found in list")

	event, err := helper.Apiclient.Nodes().Get(context.Background(), node[0], metav1.GetOptions{})

	Expect(err).ToNot(HaveOccurred())

	val := event.Annotations[netmlbparameters.AnnotationPrimaryIfaddr]
	// Output example [{ ipv4 : 10.46.56.13/24 }] len = 5
	ipListOutput := strings.Split(val, "\"")

	switch len(ipListOutput) {
	case 5:
		log.Println("Cluster is a Single Stack")
	case 9:
		log.Println("Cluster is a Dual Stack")
	default:
		Fail("Incorrect IPStack output")
	}

	_, nodeNet, err := net.ParseCIDR(ipListOutput[3])
	Expect(err).ToNot(HaveOccurred())

	if !nodeNet.Contains(net.ParseIP(metallbEnvIP)) {
		Skip("The environment IP variable is out of cluster br-ex IP range")
	}
}

// DefineAndCreateLBService create an external service using the MetalLB Address Pool allowing
// connectivity from network host interface br-ex to the nginx pod on port 30101.
func DefineAndCreateLBService(namespace string, iPStack string, addresspool string, appLabel string,
	trafficPolicy k8sv1.ServiceExternalTrafficPolicyType) error {
	var (
		service  k8sv1.Service
		ipFamily []k8sv1.IPFamily
	)

	ipFamilyPolicy := k8sv1.IPFamilyPolicySingleStack

	switch iPStack {
	case netmlbparameters.SingleIPv4Stack:
		ipFamily = []k8sv1.IPFamily{"IPv4"}

	case netmlbparameters.SingleIPv6Stack:
		ipFamily = []k8sv1.IPFamily{"IPv6"}

	case netmlbparameters.DualIPStack:
		ipFamily = []k8sv1.IPFamily{"IPv4", "IPv6"}
		ipFamilyPolicy = k8sv1.IPFamilyPolicyRequireDualStack
	}

	service = k8sv1.Service{

		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				netmlbparameters.MetalLBAddressPool: addresspool,
			},
			GenerateName: "service-",
			Namespace:    namespace,
		},
		Spec: k8sv1.ServiceSpec{
			Selector: map[string]string{
				"app": appLabel,
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
			ExternalTrafficPolicy: trafficPolicy,
			Type:                  "LoadBalancer",
			IPFamilies:            ipFamily,
			IPFamilyPolicy:        &ipFamilyPolicy,
		},
	}

	_, err := helper.Apiclient.Services(namespace).Create(context.Background(),
		&service, metav1.CreateOptions{})

	return err
}

// DeleteAllLBServices deletes all the service in a specific namespace.
func DeleteAllLBServices(namespace string) error {
	allServices, err := helper.Apiclient.Services(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, service := range allServices.Items {
		err = helper.Apiclient.Services(namespace).Delete(context.Background(),
			service.Name,
			metav1.DeleteOptions{GracePeriodSeconds: pointer.Int64Ptr(0)})
		if err != nil {
			return err
		}
	}

	return nil
}

// GetLBServiceAnnouncingNodeName searches for node name in following string example:
// "announcing from node "helix13.lab.eng.tlv2.redhat.com".
func GetLBServiceAnnouncingNodeName() string {
	var allEvents []string

	serviceEvents, err := helper.Apiclient.Events(
		netmlbparameters.TestNamespace).List(context.Background(), metav1.ListOptions{FieldSelector: "reason=nodeAssigned"})
	Expect(err).ToNot(HaveOccurred())

	seriveSortedEvents := sortServiceTimeStamp(serviceEvents)
	Expect(len(seriveSortedEvents)).To(BeNumerically(">", 0), "No events were found")

	lastSortedEvent := seriveSortedEvents[len(seriveSortedEvents)-1]

	for _, index := range strings.Split(lastSortedEvent.String(), "}") {
		if strings.Contains(index, "announcing from node") {
			re := regexp.MustCompile(`"([^\"]+)"`)
			event := re.FindString(index)
			allEvents = append(allEvents, event)
		}
	}

	return strings.Trim(allEvents[len(allEvents)-1], "\"")
}

// sortServiceTimeStamp api output returns logs out of order this sorts the events by their timestamps.
func sortServiceTimeStamp(serviceEvents *k8sv1.EventList) []k8sv1.Event {
	var res []k8sv1.Event

	res = append(res, serviceEvents.Items...)

	sort.Slice(res, func(i int, j int) bool {
		return res[i].LastTimestamp.Before(&res[j].LastTimestamp)
	})

	return res
}

// GetNodeIndex retrieves a list of annoucing and non-annoucing node indexes.
func GetNodeIndex() map[string]int {
	workerNodeList := helper.GetNodeListStringByLabel(parameters.RoleWorker)
	announcingNodeName := GetLBServiceAnnouncingNodeName()

	res := map[string]int{"announcerNodeIndex": 0, "nonannouncerNodeIndex": 0}

	for nodeIndex, workerName := range workerNodeList {
		if workerName == announcingNodeName && nodeIndex == 0 {
			res["nonannouncerNodeIndex"] = 1
		}

		if workerName == announcingNodeName && nodeIndex == 1 {
			res["announcerNodeIndex"] = 1
		}
	}

	return res
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

// DefineAndRunMlbClientPod with nginx listening on port 80.
func DefineAndRunMlbClientPod(node string, image string, appLabel string) *k8sv1.Pod {
	podDefNodeLabel := pod.RedefineWithLabel(
		pod.DefinePodOnNode(netmlbparameters.TestNamespace, image, node), "app", appLabel)
	podDefPrivCommand := pod.RedefineAsPrivileged(pod.RedefineWithCommand(podDefNodeLabel,
		[]string{"/bin/bash", "-c"},
		[]string{"nginx && sleep INF"}))
	runningPod := helper.WaitUntilPodCreatedAndRunning(podDefPrivCommand, netmlbparameters.PodWaitingTime)

	return runningPod
}

// DefineAndRunMlbPodMaster creates a pod on a Master node.
func DefineAndRunMlbPodMaster(node string, ns string, image string) *k8sv1.Pod {
	podDefPrivHostNet := pod.RedefineAsPrivileged(pod.DefineWithHostNetwork(node, ns, image))
	podMaster := pod.RedefineOnMaster(podDefPrivHostNet)
	runningPod := helper.WaitUntilPodCreatedAndRunning(podMaster, netmlbparameters.PodWaitingTime)

	return runningPod
}

// Arping verifies only one node replies to arping and that the service node br-ex mac matches the output.
func Arping(client *k8sv1.Pod, destIPAddr string, node string) error {
	arpStatus, err := pod.ExecCommand(helper.Apiclient, *client, []string{"bash", "-c", fmt.Sprint("arping -I br-ex ",
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

// HTTPMlbPod verifies that nginx web service is available via the external service IP.
func HTTPMlbPod(client *k8sv1.Pod, destIPAddr string, method string) (string, error) {
	var command string

	switch method {
	case netmlbparameters.Curl:
		command = fmt.Sprint("curl ", destIPAddr)
	case netmlbparameters.Wget:
		command = fmt.Sprint("wget -qO- ", destIPAddr)
	}

	httpStatus, err := pod.ExecCommand(helper.Apiclient, *client,
		[]string{"bash", "-c", command})
	if err != nil {
		return httpStatus.String(), err
	}

	if !strings.Contains(httpStatus.String(), "html") {
		return httpStatus.String(), fmt.Errorf("unable to connect to nginx")
	}

	return httpStatus.String(), nil
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

// IsBGPNeighborshipHasState verifies that BGP session on a pod has given state.
func IsBGPNeighborshipHasState(frrPod *k8sv1.Pod, neighborIPAddress string, state string) bool {
	result := map[string]BGPDescription{}

	Eventually(func() error {
		bgpStateOut, err := pod.ExecCommand(helper.Apiclient, *frrPod,
			[]string{"vtysh", "-u", "-c", "sh bgp neighbors json"})
		Expect(err).ToNot(HaveOccurred())

		return json.Unmarshal(bgpStateOut.Bytes(), &result)
	}, 5*time.Second, netmlbparameters.Interval).ShouldNot(HaveOccurred())

	return result[neighborIPAddress].BGPState == state
}

// updateSpeakerNodeSelector updates SpeakerNodeSelector in Metallb CR.
func updateSpeakerNodeSelector(namespace string, nodeSelector map[string]string) error {
	metallb := &metallbv1beta1.MetalLB{}

	err := helper.Apiclient.Get(context.Background(),
		types.NamespacedName{Name: netmlbparameters.MetalLBCRName, Namespace: namespace}, metallb)
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
				if nethelper.StrParamInListOfParams(podName, podsWithMetric) == nil {
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

// SetupMetalLB deploys metallb.
func SetupMetalLB() {
	By("should deploy MetalLB")

	metallb, err := metallbutils.Get(
		netmlbparameters.MetalLBOperatorNameSpace,
		netmlbparameters.UseMetallbResourcesFromFile,
	)
	Expect(err).ToNot(HaveOccurred())
	err = helper.Apiclient.Get(context.Background(), runtimeclient.ObjectKey{Namespace: metallb.Namespace,
		Name: metallb.Name}, metallb)

	if err != nil {
		metallb.Spec.SpeakerNodeSelector = netmlbparameters.SpeakerNodeSelectorWorker
		Expect(helper.Apiclient.Create(context.Background(), metallb)).Should(Succeed())
	}

	By("should have MetalLB controller in running state")
	Eventually(func() bool {
		isMetalLBControllerRunning, err := helper.IsDeploymentReady(helper.Apiclient,
			netmlbparameters.MetalLBOperatorNameSpace, netmlbparameters.MetalLBDeploymentName)
		if err != nil {
			return false
		}

		return isMetalLBControllerRunning
	}, netmlbparameters.PodWaitingTime, netmlbparameters.Interval).Should(BeTrue())

	By("Checking MetalLB operator is installed and running")
	Eventually(IsMetalLBAvailable,
		netmlbparameters.PodWaitingTime,
		netmlbparameters.Interval).ShouldNot(HaveOccurred())
}

// DeleteAllAddressPools removes all addresspools in metallb-system.
func DeleteAllAddressPools() {
	apList := metallbv1beta1.AddressPoolList{}
	err := helper.Apiclient.List(context.Background(), &apList,
		runtimeclient.InNamespace(netmlbparameters.MetalLBOperatorNameSpace))
	Expect(err).ToNot(HaveOccurred())

	for _, ap := range apList.Items {
		err = helper.Apiclient.Delete(context.Background(), &ap)
		Expect(err).ToNot(HaveOccurred())
	}
}

// UpdateSpeakerNodeLabel adds label metallbtest to the speaker nodes.  This label will be used in Metallb in order to
// simulate a node failure.
func UpdateSpeakerNodeLabel() {
	workerNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
	Expect(err).ToNot(HaveOccurred())

	err = updateSpeakerNodeSelector(netmlbparameters.MetalLBOperatorNameSpace,
		map[string]string{netmlbparameters.SpeakerNodeTestLabel: ""})
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() bool {
		speakerPodList, _ := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).List(
			context.Background(),
			metav1.ListOptions{LabelSelector: netmlbparameters.SpeakersLabelSelector},
		)

		return len(speakerPodList.Items) == len(workerNodeList)
	}, 1*time.Minute, 1*time.Second).Should(BeTrue())

	for _, worker := range workerNodeList {
		_, err = nodes.LabelNode(helper.Apiclient, worker.Name, netmlbparameters.SpeakerNodeTestLabel, "")
		Expect(err).ToNot(HaveOccurred())
	}

	Eventually(AreSpeakersReady, netmlbparameters.PodWaitingTime, netmlbparameters.Interval).
		Should(BeTrue(), "Speaker pods are not ready")
}

// AddOrDeleteSpeakerStaticRoute removes or creates static routs on all Speaker pods.
func AddOrDeleteSpeakerStaticRoute(action string, speakerRoutesMap map[string]string) (string, error) {
	var buffer bytes.Buffer

	speakerPodList, err := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).List(
		context.Background(),
		metav1.ListOptions{LabelSelector: netmlbparameters.SpeakersLabelSelector},
	)
	if err != nil {
		return "", err
	}

	for _, speakerPod := range speakerPodList.Items {
		buffer, err = pod.ExecCommand(helper.Apiclient, speakerPod, []string{"ip", "route", action,
			netmlbparameters.ClientIpv4IP + "/32", "via", speakerRoutesMap[speakerPod.Spec.NodeName]})
		if err != nil {
			return buffer.String(), err
		}
	}

	return buffer.String(), nil
}

// ValidateIPs checks given IP addresses if they belong to IPFamily.
func ValidateIPs(ipAddresses []string, ipFamily string) error {
	for _, ipAddress := range ipAddresses {
		IP := net.ParseIP(ipAddress)
		if IP == nil {
			return fmt.Errorf("%s is not valid IP", ipAddress)
		}

		switch ipFamily {
		case netmlbparameters.SingleIPv4Stack:
			if !strings.Contains(ipAddress, ".") {
				return fmt.Errorf("%s is not from %s", ipAddress, ipFamily)
			}
		case netmlbparameters.SingleIPv6Stack:
			if !strings.Contains(ipAddress, ":") {
				return fmt.Errorf("%s is not from %s", ipAddress, ipFamily)
			}
		}
	}

	return nil
}

func DeleteConfigMap(configMapName string, namespace string) error {
	configMap := &k8sv1.ConfigMap{}

	err := helper.Apiclient.Get(context.Background(), runtimeclient.ObjectKey{Namespace: namespace,
		Name: configMapName}, configMap)
	if err != nil {
		return err
	}

	err = helper.Apiclient.Delete(context.Background(), configMap)
	if err != nil {
		return err
	}

	return nil
}

func appendIfMissing(slice []string, newItem string) []string {
	if nethelper.StrParamInListOfParams(newItem, slice) == nil {
		return slice
	}

	return append(slice, newItem)
}

func uint32Ptr(n uint32) *uint32 {
	return &n
}
