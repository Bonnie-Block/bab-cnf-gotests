package netmetallbhelper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metallbv1beta1 "go.universe.tf/metallb/api/v1beta1"

	metallboperatorv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

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

		ChangedGWMode = false
	}
}

// GetGWMode returns the NetworkOperator's  GW mode: false - share GW mode, true - local GW mode.
func GetGWMode() bool {
	networkOperatorConfg := &operv1.Network{}
	err := helper.Apiclient.Get(
		context.TODO(), runtimeclient.ObjectKey{Name: netparameters.NetworkOperatorConfigName}, networkOperatorConfg)
	Expect(err).ToNot(HaveOccurred())

	return networkOperatorConfg.Spec.DefaultNetwork.OVNKubernetesConfig.GatewayConfig.RoutingViaHost
}

// SetLocalGWMode set NetworkOperator's local GW mode if true, if false - share GW mode.
func SetLocalGWMode(state bool) {
	networkOperatorConfg := &operv1.Network{}
	err := helper.Apiclient.Get(
		context.TODO(), runtimeclient.ObjectKey{Name: netparameters.NetworkOperatorConfigName}, networkOperatorConfg)
	Expect(err).ToNot(HaveOccurred())

	networkOperatorConfg.Spec.DefaultNetwork.OVNKubernetesConfig.GatewayConfig.RoutingViaHost = state

	err = helper.Apiclient.Update(context.Background(), networkOperatorConfg)
	Expect(err).ToNot(HaveOccurred())
}

// isNetworkOperatorInCondition parses NetworkOperator conditions.
// Returns true if  NetworkOperator is in given condition, otherwise false.
func isNetworkOperatorInCondition(condition string, status operv1.ConditionStatus) bool {
	networkOperatorConfg := &operv1.Network{}
	err := helper.Apiclient.Get(
		context.TODO(), runtimeclient.ObjectKey{Name: netparameters.NetworkOperatorConfigName}, networkOperatorConfg)
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
	}, 30*time.Second, netmlbparameters.Interval).Should(BeTrue())
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
func IsEnvVarMetallbIPinNodeExtNetRange(cnfNodeLabel string, ipStack string,
	metallbEnvIPv4 string, metallbEnvIPv6 string) {
	// Checks that the METALLB_ADDR_LIST is in the range of the cluster br-ex interface.
	node := helper.GetNodeListStringByLabel(cnfNodeLabel)
	Expect(len(node)).To(BeNumerically(">", 0), "No Node found in list")

	event, err := helper.Apiclient.Nodes().Get(context.Background(), node[0], metav1.GetOptions{})

	Expect(err).ToNot(HaveOccurred())

	val := event.Annotations[netmlbparameters.AnnotationPrimaryIfaddr]

	// Output example [{ ipv4 : 10.46.56.13/24 }] len = 5
	ipListOutput := strings.Split(val, "\"")

	switch ipStack {
	case netparameters.IPV4Family:
		loadBalancerIPValid(ipListOutput[3], metallbEnvIPv4)
	case netparameters.IPV6Family:
		loadBalancerIPValid(ipListOutput[7], metallbEnvIPv6)
	case netparameters.DualIPFamily:
		loadBalancerIPValid(ipListOutput[3], metallbEnvIPv4)
		loadBalancerIPValid(ipListOutput[7], metallbEnvIPv6)
	default:
		Fail("Incorrect IPStack output")
	}
}

// DefineAndCreateLBService create an external service using the MetalLB Address Pool allowing
// connectivity from network host interface br-ex to the nginx pod on port 30101.
func DefineAndCreateLBService(namespace string, iPStack string, addresspool string, appLabel string, protocolL4 string,
	trafficPolicy k8sv1.ServiceExternalTrafficPolicyType) error {
	portNum := int32(80)
	protocol := k8sv1.ProtocolTCP
	ipFamilyPolicy := k8sv1.IPFamilyPolicySingleStack
	ipFamily := []k8sv1.IPFamily{"IPv4"}

	if protocolL4 == netmlbparameters.ProtocolSCTP {
		portNum = 50000
		protocol = k8sv1.ProtocolSCTP
	}

	switch iPStack {
	case netparameters.IPV6Family:
		ipFamily = []k8sv1.IPFamily{"IPv6"}

	case netparameters.DualIPFamily:
		ipFamily = []k8sv1.IPFamily{"IPv4", "IPv6"}
		ipFamilyPolicy = k8sv1.IPFamilyPolicyRequireDualStack
	}

	service := k8sv1.Service{

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
					Protocol: protocol,
					Port:     portNum,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: portNum,
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

	if err != nil {
		return fmt.Errorf("error defining LB service for %s - %w", addresspool, err)
	}

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
		netmlbparameters.TestNamespace).List(context.Background(),
		metav1.ListOptions{FieldSelector: "reason=nodeAssigned"})
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

// DefineAndRunMlbClientPod with nginx listening on port 80 and sctp on port 50000.
func DefineAndRunMlbClientPod(node string, image string, appLabel string, argCommand []string) *k8sv1.Pod {
	podDefNodeLabel := pod.RedefineWithLabel(
		pod.DefinePodOnNode(netmlbparameters.TestNamespace, image, node), "app", appLabel)
	podDefPrivCommand := pod.RedefineAsPrivileged(pod.RedefineWithCommand(podDefNodeLabel,
		[]string{"/bin/bash", "-c"},
		argCommand))
	runningPod := helper.WaitUntilPodCreatedAndRunning(podDefPrivCommand, netmlbparameters.PodWaitingTime)

	return runningPod
}

func DefineMlbServerPod(node, image, appLabel string, argCommand []string) *k8sv1.Pod {
	podDefNodeLabel := pod.RedefineWithLabel(
		pod.DefinePodOnNode(netmlbparameters.TestNamespace, image, node), "app", appLabel)

	return pod.RedefineAsPrivileged(pod.RedefineWithCommand(podDefNodeLabel,
		[]string{"/bin/bash", "-c"},
		argCommand))
}

// DefineAndRunMlbServerPod runs server pod based on given image and cmd arguments.
func DefineAndRunMlbServerPod(node, image, appLabel string, argCommand []string) *k8sv1.Pod {
	podDefPrivCommand := DefineMlbServerPod(node, image, appLabel, argCommand)

	return helper.WaitUntilPodCreatedAndRunning(podDefPrivCommand, netmlbparameters.PodWaitingTime)
}

// DefineAndRunMlbServerPodWithSecondContainer runs server pod with two containers based on given image
// and cmd arguments.
func DefineAndRunMlbServerPodWithSecondContainer(
	node, image, appLabel string, argCommand []string, container *k8sv1.Container) *k8sv1.Pod {
	mlbServerPod := DefineMlbServerPod(node, image, appLabel, argCommand)
	mlbServerPod.Spec.Containers = append([]k8sv1.Container{*container}, mlbServerPod.Spec.Containers...)

	return helper.WaitUntilPodCreatedAndRunning(mlbServerPod, netmlbparameters.PodWaitingTime)
}

// DefineMlbPodMaster creates a pod on a Master node.
func DefineMlbPodMaster(node string, ns string, image string) *k8sv1.Pod {
	podDefPrivHostNet := pod.RedefineAsPrivileged(pod.DefineWithHostNetwork(node, ns, image))
	podMaster := pod.RedefineOnMaster(podDefPrivHostNet)

	return podMaster
}

func DefineMlbPodWithNetwork(node string,
	ns string,
	image string,
	nadName string,
	ipAddress string) (*k8sv1.Pod, error) {
	podMaster := DefineMlbPodMaster(node, ns, image)
	podMaster.Spec.HostNetwork = false

	_, subnet, err := nethelper.DefineIPFamily(ipAddress)
	if err != nil {
		return nil, err
	}

	return pod.RedefinePodWithNetwork(podMaster, fmt.Sprintf(`[{"name": "%s","ips": ["%s/%s"]}]`,
		nadName, ipAddress, subnet)), nil
}

// Arping verifies only one node replies to arping and that the service node br-ex mac matches the output.
func Arping(client *k8sv1.Pod, destIPAddr string, node string) error {
	arpStatus, err := pod.ExecCommand(helper.Apiclient, *client, []string{"bash", "-c", fmt.Sprint("arping -I net1 ",
		destIPAddr, " -c3")})
	Expect(err).ToNot(HaveOccurred())

	macs := arpStatus.String()
	output := strings.Split(macs, "\n")
	lineCount := 0

	for _, reply := range output {
		if strings.Contains(reply, "Unicast") {
			lineCount++
		}
	}
	// When using the NAD interface the mac address of eth0 is included in the arp replies adding an extra line count.
	Expect(lineCount).To(Equal(4), "An incorrect number of arp replies were received")
	// Verifies the output mac addresses matches the annoucing node mac address
	nodeMac, err := SpeakerNodeMac(node)
	Expect(strings.Join(output, "\n")).Should(ContainSubstring(strings.ToUpper(nodeMac)),
		"ARP request was not received from the announcing node")
	Expect(err).ToNot(HaveOccurred())

	return err
}

// HTTPMlbPod verifies that nginx web service is available via the external service IP.
func HTTPMlbPod(
	client *k8sv1.Pod,
	sourceIPAddr string,
	destIPAddr string,
	ipFamily string,
	containerName string, protocolLayer string) (string, error) {
	var (
		command    string
		httpStatus bytes.Buffer
	)

	if protocolLayer == netmlbparameters.Layer2 {
		// This is a workaround a NAD issue in which the macvlan mac address is not being populated on the infra switch.
		arpOutput, err := pod.ExecCommand(helper.Apiclient, *client, []string{"bash", "-c", fmt.Sprint("arping -I net1 ",
			destIPAddr, " -c2")})
		if err != nil {
			return arpOutput.String(), fmt.Errorf("arpping command failed - %w", err)
		}
	}

	command = fmt.Sprintf("curl --interface %s %s --max-time 10", sourceIPAddr, destIPAddr)

	if ipFamily == netparameters.IPV6Family {
		command = fmt.Sprint("curl --interface ", sourceIPAddr, "[", destIPAddr, "]", "--max-time 5")
	}

	httpStatus, err := pod.ExecCommand(helper.Apiclient, *client, []string{"bash", "-c", command},
		containerName)

	if err != nil {
		return httpStatus.String(), fmt.Errorf("curl command failed - %w", err)
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
			append(netmlbparameters.VtyshFRRCmdPrefix, "sh bgp neighbors json"))
		Expect(err).ToNot(HaveOccurred())

		return json.Unmarshal(bgpStateOut.Bytes(), &result)
	}, 5*time.Second, netmlbparameters.Interval).ShouldNot(HaveOccurred())

	return result[neighborIPAddress].BGPState == state
}

// updateSpeakerNodeSelector updates SpeakerNodeSelector in Metallb CR.
func updateSpeakerNodeSelector(namespace string, nodeSelector map[string]string) error {
	metallb := &metallboperatorv1beta1.MetalLB{}

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
	metallb := &metallboperatorv1beta1.MetalLB{}

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
			stdout, err = pod.ExecCommand(helper.Apiclient, speakerPod, []string{"curl", "localhost:29151/metrics"})
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
			[]string{"vtysh", "-c", "sh run"}, netmlbparameters.FRRContainerName)
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

// DeleteAllIPAddressPools removes all IPaddresspools in metallb-system.
func DeleteAllIPAddressPools() {
	apList := metallbv1beta1.IPAddressPoolList{}
	err := helper.Apiclient.List(context.Background(), &apList,
		runtimeclient.InNamespace(netmlbparameters.MetalLBOperatorNameSpace))
	Expect(err).ToNot(HaveOccurred())

	for _, ap := range apList.Items {
		err = helper.Apiclient.Delete(context.Background(), &ap)
		Expect(err).ToNot(HaveOccurred())
	}
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

// DeleteAllL2Advertisements removes all L2Advertisements in metallb-system.
func DeleteAllL2Advertisements() error {
	l2AdvertisementList := metallbv1beta1.L2AdvertisementList{}

	err := helper.Apiclient.List(context.Background(), &l2AdvertisementList,
		runtimeclient.InNamespace(netmlbparameters.MetalLBOperatorNameSpace))
	if err != nil {
		return err
	}

	for _, l2Advertisement := range l2AdvertisementList.Items {
		err = helper.Apiclient.Delete(context.Background(), &l2Advertisement)
		if err != nil {
			return err
		}
	}

	return nil
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

		return len(speakerPodList.Items) == 0
	}, 1*time.Minute, 1*time.Second).Should(BeTrue())

	for _, worker := range workerNodeList {
		_, err = nodes.LabelNode(helper.Apiclient, worker.Name, netmlbparameters.SpeakerNodeTestLabel, "")
		Expect(err).ToNot(HaveOccurred())
	}

	Eventually(AreSpeakersReady, netmlbparameters.PodWaitingTime, netmlbparameters.Interval).
		Should(BeTrue(), "Speaker pods are not ready")
}

// AddOrDeleteSpeakerStaticRoute removes or creates static routs on all Speaker pods.
func AddOrDeleteSpeakerStaticRoute(action string, nextHopMap map[string]string, destIP string) (string, error) {
	var buffer bytes.Buffer

	speakerPodList, err := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).List(
		context.Background(),
		metav1.ListOptions{LabelSelector: netmlbparameters.SpeakersLabelSelector},
	)
	if err != nil {
		return "", err
	}

	for _, speakerPod := range speakerPodList.Items {
		buffer, err = pod.ExecCommand(helper.Apiclient,
			speakerPod,
			[]string{"ip", "route", action, destIP, "via", nextHopMap[speakerPod.Spec.NodeName]},
			netmlbparameters.FRRContainerName)
		if err != nil {
			return buffer.String(), err
		}
	}

	return buffer.String(), nil
}

// ValidateIPs checks given IP addresses if they belong to IPFamily.
func ValidateIPs(ipAddressList []string, ipFamily string) error {
	var ipAddresses []string

	switch ipFamily {
	case netparameters.IPV4Family:
		ipAddresses = []string{ipAddressList[0], ipAddressList[1]}

	case netparameters.IPV6Family:
		ipAddresses = []string{ipAddressList[2], ipAddressList[3]}
	}

	for _, ipAddress := range ipAddresses {
		ipStack, _, err := nethelper.DefineIPFamily(ipAddress)
		if err != nil {
			return err
		}

		if ipStack != ipFamily {
			return fmt.Errorf("%s is not from %s", ipAddress, ipFamily)
		}
	}

	return nil
}

// DeleteConfigMaps deletes all configmaps from list in namespace.
func DeleteConfigMaps(configMapNameList []string, namespace string) error {
	configMap := &k8sv1.ConfigMap{}

	for _, configMapName := range configMapNameList {
		err := helper.Apiclient.Get(context.Background(), runtimeclient.ObjectKey{Namespace: namespace,
			Name: configMapName}, configMap)
		if err != nil {
			return err
		}

		err = helper.Apiclient.Delete(context.Background(), configMap)
		if err != nil {
			return err
		}
	}

	return nil
}

// ValidateClusterIPStack verifies if the cluster is a SingleStack or DualStack.
func ValidateClusterIPStack() string {
	var clusterIPStack string

	workerNodes, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
	Expect(err).ToNot(HaveOccurred())

	clusterIPv4Address := nethelper.NodeIPsForFamily(workerNodes, netparameters.IPV4Family)
	clusterIPv6Address := nethelper.NodeIPsForFamily(workerNodes, netparameters.IPV6Family)

	// "TODO: after fix of Bz https://bugzilla.redhat.com/show_bug.cgi?id=2073754
	// replace if statement with following commented line".
	//	if len(clusterIPv6Address) == 0 {
	if len(clusterIPv6Address) != 2 {
		clusterIPStack = netparameters.IPV4Family
	}

	if len(clusterIPv4Address) == 0 && len(clusterIPv6Address) == 2 {
		clusterIPStack = netparameters.IPV6Family
	}

	if len(clusterIPv6Address) == 2 {
		clusterIPStack = netparameters.DualIPFamily
	}

	return clusterIPStack
}

func loadBalancerIPValid(ipAddress string, lbIpaddress string) {
	_, nodeNet, err := net.ParseCIDR(ipAddress)
	Expect(err).ToNot(HaveOccurred())

	if !nodeNet.Contains(net.ParseIP(lbIpaddress)) {
		ipVersion, _, _ := nethelper.DefineIPFamily(lbIpaddress)

		Skip(fmt.Sprintf("The environment IP variable is out of cluster br-ex %s range", ipVersion))
	}
}

// ActivateSCTPModuleOnMaster creates privPods on list of nodes from masterNodeList. After activation the pod and
// namespace are removed.
func ActivateSCTPModuleOnMaster(masterNodeList []k8sv1.Node) {
	if namespaces.Exists(parameters.PrivPodNamespace, helper.Apiclient) {
		By("Remove cnfgotestpriv namespace")

		err := namespaces.DeleteAndWait(helper.Apiclient, parameters.PrivPodNamespace,
			netmlbparameters.Timeout)
		Expect(err).ToNot(HaveOccurred(), "failed to delete cnfgotestpriv namespace")
	}

	By(fmt.Sprintf("Creating %s namespace", parameters.PrivPodNamespace))
	err := namespaces.Create(parameters.PrivPodNamespace, helper.Apiclient)
	Expect(err).ShouldNot(HaveOccurred(), "error creating cnfgotestpriv namespace")

	for _, masterNode := range masterNodeList {
		masterPrivPod := createPrivilegedPodMaster(helper.Config.Network.TestContainerImage, masterNode.Name)
		_, err = helper.ExecCommandOnNodeWithHostBinaries(&masterNode, []string{"modprobe", "sctp"})
		Expect(err).ToNot(HaveOccurred(), "Failed to load SCTP module")

		output, err := pod.ExecCommand(helper.Apiclient, *masterPrivPod, []string{"/bin/bash", "-c", "lsmod | grep sctp"})
		Expect(err).ToNot(HaveOccurred(), "error running command with pod.ExecCommand")
		Expect(output.String()).To(ContainSubstring("libcrc32c"))
	}

	By("Remove cnfgotestpriv namespace")

	err = namespaces.DeleteAndWait(helper.Apiclient, parameters.PrivPodNamespace,
		netmlbparameters.Timeout)
	Expect(err).ToNot(HaveOccurred(), "failed to delete cnfgotestpriv namespace")
}

// AddOrDeleteNodeSecIPAddViaSpeaker removes or adds IP address to the secondary Node interface via speaker pod.
func AddOrDeleteNodeSecIPAddViaSpeaker(action string,
	workerNodeName string,
	ipaddress string,
	secInterface string) (string, error) {
	fieldSelector := fmt.Sprintf("spec.nodeName=%s", workerNodeName)

	speakerPodList, err := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).List(
		context.Background(),
		metav1.ListOptions{
			LabelSelector: netmlbparameters.SpeakersLabelSelector, FieldSelector: fieldSelector},
	)
	if err != nil {
		return "", fmt.Errorf("failed to get MetalLB speaker pods: %w", err)
	}

	if len(speakerPodList.Items) != 1 {
		return "", fmt.Errorf("wrong number of speakers(%d) on the worker node %s",
			len(speakerPodList.Items), workerNodeName)
	}

	_, subnet, err := nethelper.DefineIPFamily(ipaddress)
	if err != nil {
		return "", err
	}

	buffer, err := pod.ExecCommand(helper.Apiclient, speakerPodList.Items[0], []string{"ip", "add", action,
		netmlbparameters.IPSecondaryInterface1 + "/" + subnet, "dev", secInterface}, netmlbparameters.FRRContainerName)
	if err != nil {
		return buffer.String(), err
	}

	return buffer.String(), err
}

// GetMetalLBIPByFamily returns mettalLB IP addresses  from env var METALLB_ADDR_LIST sorted by IPFamily.
func GetMetalLBIPByFamily() ([]string, []string, error) {
	var (
		ipv4IPList []string
		ipv6IPList []string
	)

	metalLBIPList, err := helper.Config.GetMetallbVirtIP()
	if err != nil {
		return nil, nil, err
	}

	for _, ipAddress := range metalLBIPList {
		ipFamily, _, err := nethelper.DefineIPFamily(ipAddress)
		if err != nil {
			return nil, nil, err
		}

		switch ipFamily {
		case netparameters.IPV4Family:
			ipv4IPList = append(ipv4IPList, ipAddress)
		case netparameters.IPV6Family:
			ipv6IPList = append(ipv6IPList, ipAddress)
		}
	}

	return ipv4IPList, ipv6IPList, nil
}

func DescribeMetalLBCRDParameters(externalTrafficPolicy k8sv1.ServiceExternalTrafficPolicyType) string {
	metallbCRDTestParameters, err := netmlbparameters.NewMetallbCRDTestParameters(externalTrafficPolicy)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error in parameters: TrafficPolicy=%s", externalTrafficPolicy))

	myPrams, err := json.Marshal(metallbCRDTestParameters)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to Marshal: TrafficPolicy=%s", externalTrafficPolicy))

	return string(myPrams)
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

// SetLogLevel updates MetalLB with the loglevel debug or informational.
func SetLogLevel(logLevel metallboperatorv1beta1.MetalLBLogLevel) error {
	metallb, err := metallbutils.Get(
		netmlbparameters.MetalLBOperatorNameSpace,
		netmlbparameters.UseMetallbResourcesFromFile,
	)
	if err != nil {
		return fmt.Errorf("error unable to locate metallb: %w", err)
	}

	err = helper.Apiclient.Get(context.Background(), runtimeclient.ObjectKey{Namespace: metallb.Namespace,
		Name: metallb.Name}, metallb)
	if err != nil {
		return fmt.Errorf("error unable retieve metallb object: %w", err)
	}

	metallb.Spec.LogLevel = logLevel
	err = helper.Apiclient.Update(context.TODO(), metallb)

	if err != nil {
		return fmt.Errorf("error unable to update log level, %s: %w", logLevel, err)
	}

	Eventually(AreSpeakersReady, netmlbparameters.PodWaitingTime, netmlbparameters.Interval).
		Should(BeTrue(), "Speaker pods are not ready")

	Eventually(func() error {
		return helper.IsDaemonsetReady(helper.Apiclient,
			netmlbparameters.MetalLBOperatorNameSpace, netmlbparameters.MetalLBDaemonsetName)
	}, 2*time.Minute, netmlbparameters.UpdateIntervalMetallb).ShouldNot(HaveOccurred())

	return nil
}

// ValidateLogLevel verifies the loglevel on the FRR speaker with show logging.
func ValidateLogLevel(logLevel string) error {
	speakerPods, err := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).
		List(context.Background(), metav1.ListOptions{
			LabelSelector: netmlbparameters.SpeakersLabelSelector,
		})
	if err != nil {
		return fmt.Errorf("error unable to retrieve speaker pods: %w", err)
	}

	var removedLogLevel string

	switch logLevel {
	case netmlbparameters.LogLevelDebug:
		removedLogLevel = netmlbparameters.LogLevelInfo
	case netmlbparameters.LogLevelInfo:
		removedLogLevel = netmlbparameters.LogLevelDebug
	}

	for _, speakerFRRPod := range speakerPods.Items {
		outPut, err := pod.ExecCommand(helper.Apiclient, speakerFRRPod,
			[]string{"vtysh", "-c", "show logging"}, netmlbparameters.FRRContainerName)
		if err != nil {
			return fmt.Errorf("error on executing command: %s: %w", outPut.String(), err)
		}

		logStings := strings.Contains(outPut.String(), logLevel) && !strings.Contains(outPut.String(), removedLogLevel)
		if !logStings {
			return fmt.Errorf("error log level is not configured with %s", logLevel)
		}
	}

	return nil
}
