package netmetallbhelper

import (
	"context"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMetalLBBFD(scenario string, clientPod *k8sv1.Pod,
	firstWorkerNodeAddress string, secondWorkerNodeAddress string) {
	By("Changing the label selector for Metallb and adding a label for Workers")

	workerNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
	Expect(err).ToNot(HaveOccurred())
	Expect(len(workerNodeList)).To(BeNumerically(">", 1))

	err = updateSpeakerNodeSelector(netmlbparameters.MetalLBOperatorNameSpace,
		map[string]string{netmlbparameters.SpeakerNodeTestLabel: ""})
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() int {
		speakerPodList, _ := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).List(
			context.Background(),
			metav1.ListOptions{LabelSelector: netmlbparameters.SpeakersLabelSelector},
		)

		return len(speakerPodList.Items)
	}, netmlbparameters.PodWaitingTime, netmlbparameters.Interval).Should(BeNumerically("==", 0))

	for _, worker := range workerNodeList {
		_, err = nodes.LabelNode(helper.Apiclient, worker.Name, netmlbparameters.SpeakerNodeTestLabel, "")
		Expect(err).ToNot(HaveOccurred())
	}

	Eventually(AreSpeakersReady, netmlbparameters.Timeout, netmlbparameters.Interval).
		Should(BeTrue(), "Speaker pods are not ready")
	By("Checking that BGP and BFD sessions are established and up")
	Eventually(func() bool {
		return IsBGPNeighborshipHasState(clientPod, firstWorkerNodeAddress,
			netmlbparameters.BGPStateEstablished)
	}, netmlbparameters.Timeout, netmlbparameters.Interval).Should(BeTrue())
	Eventually(func() error {
		return nethelper.IsBFDHasStatus(clientPod, firstWorkerNodeAddress,
			netmlbparameters.BFDStatusUp)
	}, netmlbparameters.TimeoutBFDBGP, netmlbparameters.Interval).ShouldNot(HaveOccurred())

	Eventually(func() bool {
		return IsBGPNeighborshipHasState(clientPod, secondWorkerNodeAddress,
			netmlbparameters.BGPStateEstablished)
	}, netmlbparameters.Timeout, netmlbparameters.Interval).Should(BeTrue())
	Eventually(func() error {
		return nethelper.IsBFDHasStatus(clientPod, secondWorkerNodeAddress,
			netmlbparameters.BFDStatusUp)
	}, netmlbparameters.TimeoutBFDBGP, netmlbparameters.Interval).ShouldNot(HaveOccurred())

	By("Removing Speaker pod and checking that speaker pod is down")

	workerNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
	Expect(len(workerNodeList)).To(BeNumerically(">", 1))
	Expect(err).ToNot(HaveOccurred())

	delete(workerNodeList[0].Labels, netmlbparameters.SpeakerNodeTestLabel)
	_, err = helper.Apiclient.Nodes().Update(context.Background(), &workerNodeList[0], metav1.UpdateOptions{})
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() int {
		speakerPodList, _ := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).List(
			context.Background(),
			metav1.ListOptions{LabelSelector: netmlbparameters.SpeakersLabelSelector},
		)

		return len(speakerPodList.Items)
	}, netmlbparameters.PodWaitingTime, netmlbparameters.Interval).Should(BeNumerically("==", len(workerNodeList)-1))

	By("Checking that BGP and BFD sessions are down with one BGPpeer and continue to work with another")
	Expect(nethelper.IsBFDHasStatus(clientPod, firstWorkerNodeAddress,
		netmlbparameters.BFDStatusDown)).ShouldNot(HaveOccurred())
	Expect(IsBGPNeighborshipHasState(clientPod, firstWorkerNodeAddress,
		netmlbparameters.BGPStateEstablished)).ToNot(BeTrue())

	Expect(nethelper.IsBFDHasStatus(clientPod, secondWorkerNodeAddress,
		netmlbparameters.BFDStatusUp)).ShouldNot(HaveOccurred())
	Expect(IsBGPNeighborshipHasState(clientPod, secondWorkerNodeAddress,
		netmlbparameters.BGPStateEstablished)).To(BeTrue())

	if scenario == netmlbparameters.ScenarioMultihop {
		httpOutput, err := HTTPMlbPod(clientPod, netmlbparameters.MetalLBMultihopIPv4List[0],
			netmlbparameters.Wget, netparameters.IPV4Family, parameters.MainContainerName)
		Expect(err).ToNot(HaveOccurred(), httpOutput)
	}

	By("Bringing Speaker pod back and checking that speaker pods are up and running")

	_, err = nodes.LabelNode(helper.Apiclient,
		workerNodeList[0].Name,
		netmlbparameters.SpeakerNodeTestLabel, "")
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() int {
		speakerPodList, _ := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).List(
			context.Background(),
			metav1.ListOptions{LabelSelector: netmlbparameters.SpeakersLabelSelector},
		)

		return len(speakerPodList.Items)
	}, netmlbparameters.PodWaitingTime, netmlbparameters.Interval).Should(BeNumerically("==", len(workerNodeList)))

	By("Checking that BGP and BFD sessions are established and up")
	Eventually(func() bool {
		return IsBGPNeighborshipHasState(clientPod, firstWorkerNodeAddress,
			netmlbparameters.BGPStateEstablished)
	}, netmlbparameters.Timeout, netmlbparameters.Interval).Should(BeTrue())
	Eventually(func() error {
		return nethelper.IsBFDHasStatus(clientPod, firstWorkerNodeAddress,
			netmlbparameters.BFDStatusUp)
	}, netmlbparameters.TimeoutBFDBGP, netmlbparameters.Interval).ShouldNot(HaveOccurred())

	Eventually(func() bool {
		return IsBGPNeighborshipHasState(clientPod, secondWorkerNodeAddress,
			netmlbparameters.BGPStateEstablished)
	}, netmlbparameters.Timeout, netmlbparameters.Interval).Should(BeTrue())
	Eventually(func() error {
		return nethelper.IsBFDHasStatus(clientPod, secondWorkerNodeAddress,
			netmlbparameters.BFDStatusUp)
	}, netmlbparameters.TimeoutBFDBGP, netmlbparameters.Interval).ShouldNot(HaveOccurred())

	if scenario == netmlbparameters.ScenarioMultihop {
		httpOutput, err := HTTPMlbPod(clientPod, netmlbparameters.MetalLBMultihopIPv4List[0],
			netmlbparameters.Wget, netparameters.IPV4Family, parameters.MainContainerName)
		Expect(err).ToNot(HaveOccurred(), httpOutput)
	}
}

// CreateRoutesMap verifies number of speaker pods is equal to the next hop list.
func CreateRoutesMap(podList k8sv1.PodList, nextHopList []string) (map[string]string, error) {
	if len(podList.Items) == 0 {
		return nil, fmt.Errorf("pod list is empty")
	}

	if len(nextHopList) == 0 {
		return nil, fmt.Errorf("nexthop IP addresses list is empty")
	}

	if len(nextHopList) == 4 {
		if len(podList.Items) != len(nextHopList)-2 {
			return nil, fmt.Errorf("number of speaker IP addresses[%d] is not equal to number of pods[%d]",
				len(podList.Items), len(podList.Items))
		}
	} else {
		if len(podList.Items) != len(nextHopList) {
			return nil, fmt.Errorf("number of speaker IP addresses[%d] is not equal to number of pods[%d]",
				len(podList.Items), len(podList.Items))
		}
	}

	routesMap := make(map[string]string)

	for num, speakerPod := range podList.Items {
		routesMap[speakerPod.Spec.NodeName] = nextHopList[num]
	}

	return routesMap, nil
}

func CreateBGPWithBFD(bgpProtocol string, bgpPeerAddress string) {
	By("Creating BFD profile")

	bfdProfileDefinition := defineBFDProfile(netmlbparameters.BFDProfileName)
	err := helper.Apiclient.Create(context.Background(), bfdProfileDefinition)
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() bool {
		return IsProtocolConfigured(netmlbparameters.BFDConfigPrefix)
	}, netmlbparameters.Timeout, netmlbparameters.Interval).
		Should(BeTrue(), "BFD is not configured on the Speakers")

	By("Creating BGP Peers")

	bgpPeerDefinition := defineSpeakerBGPPeer(bgpPeerAddress, uint32(0),
		bgpProtocol, netmlbparameters.BFDProfileName)

	err = helper.Apiclient.Create(context.Background(), bgpPeerDefinition)
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() bool {
		return IsProtocolConfigured(netmlbparameters.BGPConfigPrefix)
	}, netmlbparameters.Timeout, netmlbparameters.Interval).Should(BeTrue(), "BGP is not configured on the Speakers")
}

func CreateClientOnMaster(bgpProtocol string,
	bgpPeerAddresses []string,
	scenario string,
	masterNodeName string,
	internalNADName string) *k8sv1.Pod {
	By("Creating FRR client pod on a Master node")

	masterConfigMap := defineBFDMLBConfigMap(bgpPeerAddresses,
		netparameters.MasterConfigMapName,
		netmlbparameters.IBGPASN, bgpProtocol)
	_, err := helper.Apiclient.ConfigMaps(netmlbparameters.TestNamespace).Create(
		context.TODO(),
		masterConfigMap,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())

	var clientPodDefinition *k8sv1.Pod

	switch scenario {
	case netmlbparameters.ScenarioMultihop:
		clientPodDefinition = defineFRRPodWithNetworkAndIP(masterNodeName,
			netmlbparameters.ClientIpv4IP,
			internalNADName)
	case netmlbparameters.ScenarioSingleHop:
		clientPodDefinition = nethelper.DefineFRRPod(masterNodeName, netmlbparameters.TestNamespace, true)
	default:
		Fail("wrong scenario name")
	}

	return helper.WaitUntilPodCreatedAndRunning(clientPodDefinition, netmlbparameters.Timeout)
}

func DescribeBFDParameters(bgpPeer string, ipStack string) string {
	metallbBFDTestParameters, err := netmlbparameters.NewMetallbBFDTestParameters(bgpPeer, ipStack)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error in parameters: BGPPeer=%s, "+
		"IPStack=%s", bgpPeer, ipStack))

	myPrams, err := json.Marshal(metallbBFDTestParameters)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error in parameters: BGPPeer=%s, "+
		"IPStack=%s", bgpPeer, ipStack))

	return string(myPrams)
}
