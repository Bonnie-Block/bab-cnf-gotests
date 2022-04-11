package netmetallbhelper

import (
	"context"
	"time"

	"github.com/metallb/metallb-operator/api/v1beta1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestBGPAdvertismentTable(ipStack string, metalLBIPList []string, workerNodeList []k8sv1.Node,
	masterNodeList []k8sv1.Node, prefixLenght int32) {
	By("should create external FRR container")

	masterNodeFRRPod := CreateFRRContainerOnMaster(workerNodeList, masterNodeList, metalLBIPList, ipStack,
		netmlbparameters.IBGPASN)

	By("should create a BGP addresspool")

	addresspoolIPList := netmlbparameters.AddressPoolV4Prefix32

	if ipStack == netparameters.IPV6Family {
		addresspoolIPList = netmlbparameters.AddressPoolV6Prefix128
	}

	err := helper.Apiclient.Create(
		context.Background(),
		DefineMetalLBAddressPool(
			addresspoolIPList,
			netmlbparameters.BGP,
			ipStack,
			netmlbparameters.AddressPoolS1Name,
			prefixLenght),
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
	workerNodesAdresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)

	if ipStack != netparameters.IPV4Family {
		workerNodesV6Adresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV6Family)
		workerNodesAdresses = append(workerNodesAdresses, workerNodesV6Adresses...)
		ipFamily = netparameters.IPV6Family
	}

	err = CreateSpeakerBGPPeerIPStack(ipStack, metalLBIPList, netmlbparameters.IBGPASN)
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
	metalLBIPList []string, ipStack string, prefixLenght int32) {
	By("should create external FRR container")

	masterNodeFRRPod := CreateFRRContainerOnMaster(workerNodeList, masterNodeList, metalLBIPList, ipStack,
		netmlbparameters.IBGPASN)

	By("should create a BGP addresspool")

	addresspoolIPList := netmlbparameters.AddressPoolV4Prefix32

	if ipStack == netparameters.IPV6Family {
		addresspoolIPList = netmlbparameters.AddressPoolV6Prefix128
	}

	addresspool := DefineMetalLBAddressPool(
		addresspoolIPList,
		netmlbparameters.BGP,
		ipStack,
		netmlbparameters.AddressPoolS1Name,
		prefixLenght)

	err := helper.Apiclient.Create(context.Background(), addresspool)
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
	workerNodesAdresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)

	if ipStack != netparameters.IPV4Family {
		workerNodesV6Adresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV6Family)
		workerNodesAdresses = append(workerNodesAdresses, workerNodesV6Adresses...)
		ipFamily = netparameters.IPV6Family
	}

	err = CreateSpeakerBGPPeerIPStack(ipStack, metalLBIPList, netmlbparameters.IBGPASN)
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
		updateBGPAdvertisement(addresspool, netmlbparameters.PrefixLen28)
		prefixLenght = netmlbparameters.PrefixLen28

	case netmlbparameters.PrefixLen128:
		updateBGPAdvertisement(addresspool, netmlbparameters.PrefixLen126)
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
	bgpPeerList := v1beta1.BGPPeerList{}

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

func updateBGPAdvertisement(addresspool *v1beta1.AddressPool, prefixLen int32) {
	addresspool.Spec.BGPAdvertisements = []v1beta1.BgpAdvertisement{
		{
			Communities:       []string{netmlbparameters.CustomCommunity},
			AggregationLength: &prefixLen,
			LocalPref:         netmlbparameters.LocalPref500,
		},
	}

	err := helper.Apiclient.Update(context.Background(), addresspool)
	Expect(err).ToNot(HaveOccurred())
}
