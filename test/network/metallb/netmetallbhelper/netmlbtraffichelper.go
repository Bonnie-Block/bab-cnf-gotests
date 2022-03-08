package netmetallbhelper

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBGPTable(ipStack string, trafficPolicyName string, bgpASN int) {
	var workerNodeListString []string

	clusterIPStack := ValidateClusterIPStack()
	if clusterIPStack != netmlbparameters.SingleIPv4Stack {
		if ipStack != netmlbparameters.SingleIPv4Stack &&
			bgpASN == netmlbparameters.EBGPASN {
			Fail("Test Skipped - BZ - https://bugzilla.redhat.com/show_bug.cgi?id=2063720")
		}
	}

	metalLBIPList, err := helper.Config.GetMetallbVirtIP()
	Expect(err).ToNot(HaveOccurred())

	workerNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
	Expect(len(workerNodeList)).To(BeNumerically(">", 1))
	Expect(err).ToNot(HaveOccurred())

	for _, node := range workerNodeList {
		workerNodeListString = append(workerNodeListString, node.Name)
	}

	By("should create external FRR container")

	workerNodesAdresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)
	workerNodesV6Adresses := nethelper.NodeIPsForFamily(workerNodeList, netmlbparameters.IPV6Family)

	annotation := defineAnnotationWithIPStack(ipStack, metalLBIPList, clusterIPStack)

	if ipStack != netmlbparameters.SingleIPv4Stack {
		workerNodesAdresses = append(workerNodesAdresses, workerNodesV6Adresses...)
	}

	err = helper.Apiclient.Create(context.Background(), DefineExternalNAD())
	Expect(err).ToNot(HaveOccurred())

	masterConfigMap := DefineFRRBGPConfigMap(workerNodesAdresses,
		netparameters.MasterConfigMapName,
		bgpASN,
		netmlbparameters.BGP,
		ipStack)

	_, err = helper.Apiclient.ConfigMaps(netmlbparameters.TestNamespace).Create(
		context.TODO(),
		masterConfigMap,
		metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())

	masterNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
	Expect(err).ToNot(HaveOccurred())
	Expect(len(masterNodeList)).To(BeNumerically(">", 0))
	masterNode := masterNodeList[0]

	frrPodWithNAD := pod.RedefinePodWithNetwork(DefineFrrPodWithTestContainer(
		masterNode.Name, netmlbparameters.TestNamespace), annotation)
	masterNodeFRRPod := helper.WaitUntilPodCreatedAndRunning(frrPodWithNAD, netmlbparameters.PodWaitingTime)

	By("should create a BGP addresspool for service")

	addresspoolIPList := netmlbparameters.AddressPoolS1

	if ipStack == netmlbparameters.SingleIPv6Stack {
		addresspoolIPList = netmlbparameters.AddressPoolV6
	}

	addresspool := DefineMetalLBAddressPool(
		addresspoolIPList,
		netmlbparameters.BGP,
		ipStack,
		netmlbparameters.AddressPoolS1Name)

	err = helper.Apiclient.Create(context.Background(), addresspool)
	Expect(err).ToNot(HaveOccurred())

	By("should create service with 2 backend pods")

	err = DefineAndCreateLBService(
		netmlbparameters.TestNamespace,
		ipStack,
		netmlbparameters.AddressPoolS1Name,
		netmlbparameters.AppLabel1,
		k8sv1.ServiceExternalTrafficPolicyType(trafficPolicyName))
	Expect(err).ToNot(HaveOccurred())

	DefineAndRunMlbClientPod(workerNodeListString[0],
		helper.Config.Network.TestContainerImage,
		netmlbparameters.AppLabel1)

	DefineAndRunMlbClientPod(workerNodeListString[1],
		helper.Config.Network.TestContainerImage,
		netmlbparameters.AppLabel1)

	By("should create a BGP Peer on Speakers")

	err = createSpeakerBGPPeerIPStack(ipStack, metalLBIPList, bgpASN)
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() bool {
		return CheckNeighborsStatus(masterNodeFRRPod, ipStack,
			workerNodesAdresses)
	}, 1*time.Minute, netmlbparameters.Interval).Should(BeTrue())

	By("should validate Traffic")
	validateTraffic(masterNodeFRRPod, workerNodesAdresses, ipStack)
}

func validateTraffic(frrPod *k8sv1.Pod, nodeIPAdresses []string, ipStack string) {
	By("should validate BGP routes to service")

	var routes []string

	routes, ipFamily := defineIPRouteFamily(ipStack)

	if ipStack == netmlbparameters.DualIPStack {
		Eventually(func() error {
			return CheckBGPRoutes(frrPod, nodeIPAdresses, routes,
				netparameters.IPV4Family)
		}, 2*time.Minute, netmlbparameters.TimeoutBFDBGP).ShouldNot(HaveOccurred())
	}

	Eventually(func() error {
		return CheckBGPRoutes(frrPod, nodeIPAdresses, routes, ipFamily)
	}, 2*time.Minute, netmlbparameters.TimeoutBFDBGP).ShouldNot(HaveOccurred())

	By("should validate curl to service")

	httpOutput, err := curlService(frrPod, ipStack)
	Expect(err).ToNot(HaveOccurred(), httpOutput)
}

func curlService(frrPod *k8sv1.Pod, ipStack string) (string, error) {
	switch ipStack {
	case netmlbparameters.SingleIPv4Stack:
		return HTTPMlbPod(frrPod, netmlbparameters.AddressPoolS1[0], netmlbparameters.Curl,
			netparameters.IPV4Family, netmlbparameters.TestContainerName)

	case netmlbparameters.SingleIPv6Stack:
		return HTTPMlbPod(frrPod, netmlbparameters.AddressPoolS1[2], netmlbparameters.Curl,
			netmlbparameters.IPV6Family, netmlbparameters.TestContainerName)
	}

	httpOutput, err := HTTPMlbPod(frrPod, netmlbparameters.AddressPoolS1[2], netmlbparameters.Curl,
		netmlbparameters.IPV6Family, netmlbparameters.TestContainerName)
	if err != nil {
		return httpOutput, err
	}

	return HTTPMlbPod(frrPod, netmlbparameters.AddressPoolS1[2], netmlbparameters.Curl,
		netmlbparameters.IPV6Family, netmlbparameters.TestContainerName)
}

func defineIPRouteFamily(ipStack string) ([]string, string) {
	routesV4 := []string{netmlbparameters.AddressPoolS1[0]}
	routesV6 := []string{netmlbparameters.AddressPoolS1[2]}

	if ipStack == netmlbparameters.SingleIPv4Stack {
		return routesV4, netparameters.IPV4Family
	}

	return routesV6, netmlbparameters.IPV6Family
}

func createSpeakerBGPPeerIPStack(ipStack string, metalLBIPList []string, bgpASN int) error {
	if ipStack == netmlbparameters.SingleIPv4Stack {
		return CreateSpeakerBGPPeer(metalLBIPList[0], "", uint32(bgpASN))
	}

	if ipStack == netmlbparameters.SingleIPv6Stack {
		return CreateSpeakerBGPPeer(metalLBIPList[2], "", uint32(bgpASN))
	}

	err := CreateSpeakerBGPPeer(metalLBIPList[0], "", uint32(bgpASN))
	if err == nil {
		return err
	}

	return CreateSpeakerBGPPeer(metalLBIPList[2], "", uint32(bgpASN))
}

func defineAnnotationWithIPStack(ipStack string, metalLBIPList []string, clusterIPStack string) string {
	annotation := `[{"name": "external", "ips": `
	if ipStack == netmlbparameters.SingleIPv4Stack {
		return annotation + fmt.Sprintf(`["%s/%s"]}]`, metalLBIPList[0], netparameters.IPV4Subnet)
	}

	if clusterIPStack == netmlbparameters.SingleIPv4Stack {
		Skip("Cluster does not support IPv6")
	}

	if ipStack == netmlbparameters.SingleIPv6Stack {
		return annotation + fmt.Sprintf(`["%s/%s"]}]`, metalLBIPList[2], netparameters.IPV6Subnet)
	}

	return annotation + fmt.Sprintf(`["%s/%s","%s/%s"]}]`, metalLBIPList[0],
		netparameters.IPV4Subnet, metalLBIPList[2], netparameters.IPV6Subnet)
}
