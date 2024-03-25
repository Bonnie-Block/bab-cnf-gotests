package netmetallbhelper

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateRoutesMap verifies number of speaker pods is equal to the next hop list.
func CreateRoutesMap(podList k8sv1.PodList, nextHopList []string) (map[string]string, error) {
	if len(podList.Items) == 0 {
		return nil, fmt.Errorf("pod list is empty")
	}

	if len(nextHopList) == 0 {
		return nil, fmt.Errorf("nexthop IP addresses list is empty")
	}

	if len(nextHopList) < len(podList.Items) {
		return nil, fmt.Errorf("number of speaker IP addresses[%d] is less then number of pods[%d]",
			len(nextHopList), len(podList.Items))
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
	clientIP string,
	masterNodeName string,
	nadName string) *k8sv1.Pod {
	masterConfigMap := defineBFDMLBConfigMap(bgpPeerAddresses,
		netparameters.MasterConfigMapName,
		netmlbparameters.IBGPASN, bgpProtocol)
	_, err := helper.Apiclient.ConfigMaps(netmlbparameters.TestNamespace).Create(
		context.Background(),
		masterConfigMap,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())

	clientPodDefinition := defineFRRPodWithNetworkAndIP(masterNodeName,
		clientIP,
		nadName)

	return helper.WaitUntilPodCreatedAndRunning(clientPodDefinition, netmlbparameters.Timeout)
}
