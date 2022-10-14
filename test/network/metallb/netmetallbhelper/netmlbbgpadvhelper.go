package netmetallbhelper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	metallbv1beta1 "go.universe.tf/metallb/api/v1beta1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestBGPAdvertismentTable(ipStack string,
	ipv4metalLBIPList []string, ipv6metalLBIPList []string,
	workerNodeList []k8sv1.Node,
	masterNodeList []k8sv1.Node, prefixLenght int32) {
	By("should create external FRR container")

	clusterIPStack := ValidateClusterIPStack()

	var ipv6Address string

	if clusterIPStack != netparameters.IPV4Family {
		ipv6Address = ipv6metalLBIPList[0]
	}

	masterNodeFRRPod := CreateFRRContainerOnMaster(workerNodeList,
		masterNodeList[0],
		ipv4metalLBIPList[0], ipv6Address,
		ipStack,
		netmlbparameters.IBGPASN,
		netmlbparameters.ExternalNADName,
		netparameters.MasterConfigMapName,
		netmlbparameters.PropagateFalse)

	By("should create a BGP addresspool")

	addresspoolIPList := netmlbparameters.AddressPoolV4Prefix32

	if ipStack == netparameters.IPV6Family {
		addresspoolIPList = netmlbparameters.AddressPoolV6Prefix128
	}

	err := helper.Apiclient.Create(
		context.Background(),
		DefineMetalLBIPAddressPool(
			addresspoolIPList,
			ipStack,
			netmlbparameters.AddressPoolS1Name),
	)
	Expect(err).ToNot(HaveOccurred())

	err = helper.Apiclient.Create(
		context.Background(),
		DefineBGPAdvertisement(netmlbparameters.BGPAdvertisementName, []string{netmlbparameters.AddressPoolS1Name},
			ipStack, prefixLenght, netmlbparameters.LocalPref100),
	)
	Expect(err).ToNot(HaveOccurred())

	By("should create service with 1 backend pods")

	err = DefineAndCreateLBService(
		netmlbparameters.TestNamespace,
		ipStack,
		netmlbparameters.AddressPoolS1Name,
		netmlbparameters.AppLabel1,
		netmlbparameters.ProtocolTCP,
		netmlbparameters.ExtTrafPolCluster)
	Expect(err).ToNot(HaveOccurred())

	DefineAndRunMlbServerPod(workerNodeList[0].Name,
		helper.Config.Network.TestContainerImage,
		netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandNGINX})

	By("should create a BGP Peer on Speakers")

	ipFamily := netparameters.IPV4Family

	bgpPeerName := netmlbparameters.BGPPeerName1v4
	workerNodesAdresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)

	if ipStack != netparameters.IPV4Family {
		workerNodesV6Adresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV6Family)
		workerNodesAdresses = append(workerNodesAdresses, workerNodesV6Adresses...)
		ipFamily = netparameters.IPV6Family
		bgpPeerName = netmlbparameters.BGPPeerName1v6
		ipv6Address = ipv6metalLBIPList[0]
	}

	err = CreateSpeakerBGPPeerIPStack(ipStack, ipv4metalLBIPList[0], ipv6Address,
		netmlbparameters.IBGPASN, bgpPeerName)
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() bool {
		return CheckNeighborsStatus(masterNodeFRRPod, ipStack,
			workerNodesAdresses)
	}, 1*time.Minute, netmlbparameters.Interval).Should(BeTrue())

	By("should validate BGP route prefix")
	validatePrefix(masterNodeFRRPod, workerNodesAdresses, ipStack, prefixLenght)

	By("should validate BGP route community")

	err = ValidateRouteCommunity(masterNodeFRRPod, netmlbparameters.CommunityNoAdv, ipFamily)
	Expect(err).ToNot(HaveOccurred())
}

func TestBGPAdvertismentTableUpdates(masterNodeList []k8sv1.Node, workerNodeList []k8sv1.Node,
	ipv4metalLBIPList []string, ipv6metalLBIPList []string, ipStack string, prefixLenght int32) {
	By("should create external FRR container")

	clusterIPStack := ValidateClusterIPStack()

	var ipv6Address string

	if clusterIPStack != netparameters.IPV4Family {
		ipv6Address = ipv6metalLBIPList[0]
	}

	masterNodeFRRPod := CreateFRRContainerOnMaster(workerNodeList, masterNodeList[0],
		ipv4metalLBIPList[0], ipv6Address,
		ipStack,
		netmlbparameters.IBGPASN,
		netmlbparameters.ExternalNADName,
		netparameters.MasterConfigMapName,
		netmlbparameters.PropagateFalse)

	By("should create a IPAddressPool and BGPAdvertisement")

	addresspoolIPList := netmlbparameters.AddressPoolV4Prefix32

	if ipStack == netparameters.IPV6Family {
		addresspoolIPList = netmlbparameters.AddressPoolV6Prefix128
	}

	ipAddressPool := DefineMetalLBIPAddressPool(
		addresspoolIPList,
		ipStack,
		netmlbparameters.AddressPoolS1Name)

	err := helper.Apiclient.Create(context.Background(), ipAddressPool)
	Expect(err).ToNot(HaveOccurred())

	bgpAdvertisementDefinition := DefineBGPAdvertisement(
		netmlbparameters.BGPAdvertisementName,
		[]string{ipAddressPool.Name},
		ipStack,
		prefixLenght,
		netmlbparameters.LocalPref100)
	err = helper.Apiclient.Create(context.Background(), bgpAdvertisementDefinition)
	Expect(err).ToNot(HaveOccurred())

	By("should create service with 1 backend pods")

	err = DefineAndCreateLBService(
		netmlbparameters.TestNamespace,
		ipStack,
		netmlbparameters.AddressPoolS1Name,
		netmlbparameters.AppLabel1,
		netmlbparameters.ProtocolTCP,
		netmlbparameters.ExtTrafPolCluster)
	Expect(err).ToNot(HaveOccurred())

	DefineAndRunMlbServerPod(workerNodeList[0].Name,
		helper.Config.Network.TestContainerImage,
		netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandNGINX})

	By("should create a BGP Peer on Speakers")

	ipFamily := netparameters.IPV4Family

	bgpPeerName := netmlbparameters.BGPPeerName1v4

	workerNodesAdresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)

	if ipStack != netparameters.IPV4Family {
		workerNodesV6Adresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV6Family)
		workerNodesAdresses = append(workerNodesAdresses, workerNodesV6Adresses...)
		ipFamily = netparameters.IPV6Family
		bgpPeerName = netmlbparameters.BGPPeerName1v6
		ipv6Address = ipv6metalLBIPList[0]
	}

	err = CreateSpeakerBGPPeerIPStack(ipStack, ipv4metalLBIPList[0], ipv6Address,
		netmlbparameters.IBGPASN, bgpPeerName)
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() bool {
		return CheckNeighborsStatus(masterNodeFRRPod, ipStack,
			workerNodesAdresses)
	}, 1*time.Minute, netmlbparameters.Interval).Should(BeTrue())

	By("should validate BGP route prefix")
	validatePrefix(masterNodeFRRPod, workerNodesAdresses, ipStack, prefixLenght)

	By("should validate BGP route community")

	err = ValidateRouteCommunity(masterNodeFRRPod, netmlbparameters.CommunityNoAdv, ipFamily)
	Expect(err).ToNot(HaveOccurred())

	By("should validate BGP route Local Preference")

	err = ValidateLocalPref(masterNodeFRRPod, netmlbparameters.LocalPref400, ipFamily)
	Expect(err).ToNot(HaveOccurred())

	By("Update BGP Advertisements")

	switch prefixLenght {
	case netmlbparameters.PrefixLen32:
		updateBGPAdvertisement(bgpAdvertisementDefinition, netmlbparameters.PrefixLen28)
		prefixLenght = netmlbparameters.PrefixLen28

	case netmlbparameters.PrefixLen128:
		updateBGPAdvertisement(bgpAdvertisementDefinition, netmlbparameters.PrefixLen126)
		prefixLenght = netmlbparameters.PrefixLen126
	}

	By("should validate updated BGP route prefix")

	validatePrefix(masterNodeFRRPod, workerNodesAdresses, ipStack, prefixLenght)

	By("should validate updated BGP route community")

	err = ValidateRouteCommunity(masterNodeFRRPod, netmlbparameters.CustomCommunity, ipFamily)
	Expect(err).ToNot(HaveOccurred())

	By("should validate updated BGP route Local Preference")

	err = ValidateLocalPref(masterNodeFRRPod, netmlbparameters.LocalPref500, ipFamily)
	Expect(err).ToNot(HaveOccurred())
}

func validatePrefix(masterNodeFRRPod *k8sv1.Pod, workerNodesAdresses []string, ipStack string, prefixLenght int32) {
	var (
		routes   []string
		ipFamily string
	)

	switch ipStack {
	case netparameters.IPV4Family:
		routes = []string{netmlbparameters.AddressPoolV4Prefix28[0]}
		if prefixLenght == netmlbparameters.PrefixLen32 {
			routes = []string{netmlbparameters.AddressPoolV4Prefix32[0]}
		}

		ipFamily = netparameters.IPV4Family

	case netparameters.IPV6Subnet:
		routes = []string{netmlbparameters.AddressPoolV6Prefix126[0]}
		if prefixLenght != netmlbparameters.PrefixLen128 {
			routes = []string{netmlbparameters.AddressPoolV6Prefix128[0]}
		}

		ipFamily = netparameters.IPV6Family
	}

	Eventually(func() error {
		return CheckBGPRoutes(
			masterNodeFRRPod,
			workerNodesAdresses,
			routes,
			ipFamily,
			prefixLenght)
	}, 2*time.Minute, netmlbparameters.TimeoutBFDBGP).ShouldNot(HaveOccurred())
}

// UpdateBGPPeerTimers updates the timer setting on the speakers, which changes the time setting for all bgp peers.
func UpdateBGPPeerTimers() error {
	bgpPeerList := metallbv1beta1.BGPPeerList{}

	err := helper.Apiclient.List(context.Background(), &bgpPeerList,
		runtimeclient.InNamespace(netmlbparameters.MetalLBOperatorNameSpace))
	if err != nil {
		return err
	}

	for _, bgpPeer := range bgpPeerList.Items {
		bgpPeer.Spec.HoldTime = metav1.Duration{Duration: netmlbparameters.BGPUpdatedHoldTimer * time.Millisecond}
		bgpPeer.Spec.KeepaliveTime = metav1.Duration{Duration: netmlbparameters.BGPUpdatedKeepAliveTimer * time.Millisecond}
		err = helper.Apiclient.Update(context.Background(), &bgpPeer)

		return err
	}

	return nil
}

func ResetBGPPeer(frrPod *k8sv1.Pod) error {
	_, err := pod.ExecCommand(helper.Apiclient, *frrPod,
		[]string{"vtysh", "-c", "clear ip bgp *"})
	if err != nil {
		return err
	}

	return nil
}

func updateBGPAdvertisement(bgpAdvertisement *metallbv1beta1.BGPAdvertisement, prefixLen int32) {
	bgpAdvertisement.Spec = metallbv1beta1.BGPAdvertisementSpec{
		Communities:       []string{netmlbparameters.CustomCommunity},
		AggregationLength: &prefixLen,
		LocalPref:         netmlbparameters.LocalPref500,
	}

	err := helper.Apiclient.Update(context.Background(), bgpAdvertisement)
	Expect(err).ToNot(HaveOccurred())
}

func TestBGPBlockRouteAdvertisment(ipStack string,
	ipv4metalLBIPList []string, ipv6metalLBIPList []string,
	masterNodeList []k8sv1.Node,
	workerNodeList []k8sv1.Node) {
	By("should create external FRR container")

	clusterIPStack := ValidateClusterIPStack()

	var ipv6Address string

	if clusterIPStack != netparameters.IPV4Family {
		ipv6Address = ipv6metalLBIPList[0]
	}

	masterNodeFRRPod := CreateFRRContainerOnMaster(workerNodeList, masterNodeList[0],
		ipv4metalLBIPList[0], ipv6Address,
		ipStack,
		netmlbparameters.IBGPASN,
		netmlbparameters.ExternalNADName,
		netparameters.MasterConfigMapName,
		netmlbparameters.PropagateTrue)

	var workerNodeListString []string

	for _, node := range workerNodeList {
		workerNodeListString = append(workerNodeListString, node.Name)
	}

	By("should create a IPAddresspool and BGPAdvertisement")

	addresspoolIPList := netmlbparameters.AddressPoolV4Prefix32

	if ipStack == netparameters.IPV6Family {
		addresspoolIPList = netmlbparameters.AddressPoolV6Prefix128
	}

	err := helper.Apiclient.Create(
		context.Background(),
		DefineMetalLBIPAddressPool(
			addresspoolIPList,
			ipStack,
			netmlbparameters.AddressPoolS1Name),
	)
	Expect(err).ToNot(HaveOccurred())

	bgpAdvertisementDefinition := DefineBGPAdvertisement(
		netmlbparameters.BGPAdvertisementName,
		[]string{netmlbparameters.AddressPoolS1Name},
		ipStack,
		netmlbparameters.PrefixLen32,
		netmlbparameters.LocalPref100)
	err = helper.Apiclient.Create(context.Background(), bgpAdvertisementDefinition)
	Expect(err).ToNot(HaveOccurred())

	By("should create service")

	err = DefineAndCreateLBService(
		netmlbparameters.TestNamespace,
		ipStack,
		netmlbparameters.AddressPoolS1Name,
		netmlbparameters.AppLabel1,
		netmlbparameters.ProtocolTCP,
		netmlbparameters.ExtTrafPolCluster)
	Expect(err).ToNot(HaveOccurred())

	By("should create 2 backend pods")

	DefineAndRunMlbServerPod(workerNodeListString[0],
		helper.Config.Network.TestContainerImage,
		netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandNGINX})

	DefineAndRunMlbServerPod(workerNodeListString[1],
		helper.Config.Network.TestContainerImage,
		netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandNGINX})

	By("should create a BGP Peer on Speakers")

	workerNodesAdresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)
	workerNodesV6Adresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV6Family)

	if ipStack != netparameters.IPV4Family {
		workerNodesAdresses = append(workerNodesAdresses, workerNodesV6Adresses...)
		ipv6Address = ipv6metalLBIPList[0]
	}

	err = CreateSpeakerBGPPeerIPStack(ipStack, ipv4metalLBIPList[0], ipv6Address,
		netmlbparameters.IBGPASN, netmlbparameters.BGPPeerName1v6)
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() bool {
		return CheckNeighborsStatus(masterNodeFRRPod, ipStack,
			workerNodesAdresses)
	}, 1*time.Minute, netmlbparameters.Interval).Should(BeTrue())

	Eventually(func() bool {
		return CheckNeighborsStatus(masterNodeFRRPod, ipStack,
			workerNodesAdresses)
	}, 1*time.Minute, netmlbparameters.Interval).Should(BeTrue())

	By("should validate BGP route is advertised from external FRR")

	masterFRRSlice := []k8sv1.Pod{*masterNodeFRRPod}

	Eventually(func() error {
		_, err := parseAddressFamilyInfo(masterFRRSlice, netmlbparameters.SentPrefixCounter)

		return err
	}, 2*time.Minute, netmlbparameters.TimeoutBFDBGP).ShouldNot(HaveOccurred())

	By("should validate BGP route is not received on Speakers")

	speakerPods, err := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).
		List(context.Background(), metav1.ListOptions{
			LabelSelector: netmlbparameters.SpeakersLabelSelector,
		})
	Expect(err).ToNot(HaveOccurred())

	acceptedPrefixes, err := parseAddressFamilyInfo(speakerPods.Items, netmlbparameters.AcceptedPrefixCounter)
	Expect(err).ToNot(HaveOccurred())
	Expect(acceptedPrefixes).To(Equal(0))
}

func parseAddressFamilyInfo(frrPods []k8sv1.Pod, addressFamilyInfo string) (int, error) {
	var (
		vtyshRes bytes.Buffer
		err      error
	)

	if strings.Contains(frrPods[0].Name, "speaker") {
		vtyshRes, err = pod.ExecCommand(helper.Apiclient, frrPods[0],
			append(netmlbparameters.VtyshFRRCmdPrefix, "sh bgp neighbors json"),
			netmlbparameters.FRRContainerName)
	} else {
		vtyshRes, err = pod.ExecCommand(helper.Apiclient, frrPods[0],
			append(netmlbparameters.VtyshFRRCmdPrefix, "sh bgp neighbors json"))
	}

	if err != nil {
		return 0, err
	}

	neighborList := map[string]netmlbparameters.FRRNeighbor{}
	err = json.Unmarshal(vtyshRes.Bytes(), &neighborList)

	if err != nil {
		return 0, errors.Wrap(err, "unable to unmarshal output map")
	}

	for k, neigh := range neighborList {
		ipAdd := net.ParseIP(k)
		if ipAdd == nil {
			return 0, err
		}

		return getPrefixCounter(addressFamilyInfo, neigh)
	}

	return 0, nil
}

// getPrefixCounter returns either the number of sent prefixes from the external FRR or
// the number of received prefixes from the Speakers.
func getPrefixCounter(prefixType string, neigh netmlbparameters.FRRNeighbor) (int, error) {
	for _, prefixCounter := range neigh.AddressFamilyInfo {
		switch prefixType {
		case netmlbparameters.SentPrefixCounter:
			if prefixCounter.SentPrefixCounter == 0 {
				return 0, fmt.Errorf("no prefix are advertised")
			}

			return prefixCounter.SentPrefixCounter, nil

		case netmlbparameters.AcceptedPrefixCounter:
			if prefixCounter.AcceptedPrefixCounter != 0 {
				return 0, fmt.Errorf("the speaker received prefix from external FRR")
			}

			return prefixCounter.AcceptedPrefixCounter, nil
		}
	}

	return 0, nil
}
