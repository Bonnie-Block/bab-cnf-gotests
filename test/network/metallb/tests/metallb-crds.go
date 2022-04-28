package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	metallboperatorv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	metallbutils "github.com/metallb/metallb-operator/test/e2e/metallb"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	"golang.org/x/net/context"

	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("MetalLb New CRDs", func() {
	var (
		firstMasterNode   k8sv1.Node
		secondMasterNode  k8sv1.Node
		l3Client          *k8sv1.Pod
		workerNodeList    []k8sv1.Node
		ipv4metalLBIPList []string
	)

	execute.BeforeAll(func() {
		By("Collecting information before test")
		masterNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
		Expect(err).ToNot(HaveOccurred(), "An unexpected error occurred while getting master nodes.")
		Expect(len(masterNodeList)).To(BeNumerically(">", 0),
			"Master node list is empty")
		firstMasterNode = masterNodeList[0]
		secondMasterNode = masterNodeList[1]
		workerNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
		Expect(err).ToNot(HaveOccurred(), "An unexpected error occurred while getting worker nodes.")
		Expect(len(workerNodeList)).To(BeNumerically(">", 1),
			"Worker node list is empty")

		ipv4metalLBIPList, _, err = netmetallbhelper.GetMetalLBIPByFamily()
		Expect(err).ToNot(HaveOccurred(), "An unexpected error occurred while"+
			" determining the IP addresses from the METALLB_ADDR_LIST environment variable.")
		if len(ipv4metalLBIPList) < 2 {
			Skip("There are not enough IPv4 addresses configured in env variables METALLB_ADDR_LIST")
		}
	})

	BeforeEach(func() {
		By("Setup Metallb")
		netmetallbhelper.SetupMetalLB()

		By("Checking if MetalLB operator is installed and running")
		Eventually(netmetallbhelper.IsMetalLBAvailable, netmlbparameters.Timeout, netmlbparameters.Interval).
			ShouldNot(HaveOccurred(), "MetalLB operator is not installed")

		By("Creating nginx test pod")
		netmetallbhelper.DefineAndRunMlbClientPod(workerNodeList[0].Name,
			helper.Config.Network.TestContainerImage,
			netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandNGINX})

		By("Creating an IPAddresspool and BGPAdvertisement")
		err = helper.Apiclient.Create(context.Background(),
			netmetallbhelper.DefineMetalLBIPAddressPool(netmlbparameters.IPv4AddressesLBList, netparameters.IPV4Family,
				netmlbparameters.AddressPoolName))
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("An unexpected error occurred while"+
			" creating IPAddressPool %s.", netmlbparameters.AddressPoolName))

		bgpAdvertisementDefinition := netmetallbhelper.DefineBGPAdvertisement(
			netmlbparameters.BGPAdvertisementName,
			[]string{netmlbparameters.AddressPoolName},
			netparameters.IPV4Family,
			netmlbparameters.PrefixLen32)
		err = helper.Apiclient.Create(context.Background(), bgpAdvertisementDefinition)
		Expect(err).ToNot(HaveOccurred(), "An unexpected error occurred while creating BGPAdvertisement.")

		By("Creating FRR-L3client pod on a Master node")
		err = helper.Apiclient.Create(context.Background(),
			netmetallbhelper.DefineMacVlanNAD(netmlbparameters.ExternalNADName,
				netmlbparameters.BREXInterface))
		Expect(err).ToNot(HaveOccurred(), "An unexpected error occurred during br-ex NetworkAttachmentDefinition creation.")

		workerAddresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)
		Expect(len(workerAddresses)).To(BeNumerically(">", 1),
			"The number of worker IP addresses is less than 2")

		l3Client = netmetallbhelper.CreateClientOnMaster(netmlbparameters.EBGPProtocol,
			workerAddresses,
			ipv4metalLBIPList[0],
			firstMasterNode.Name,
			netmlbparameters.ExternalNADName)

		By("Configuring BGP and BFD on Speaker pods")
		netmetallbhelper.CreateBGPWithBFD(netmlbparameters.EBGPProtocol, ipv4metalLBIPList[0])

		Eventually(func() bool {
			return netmetallbhelper.IsBGPNeighborshipHasState(l3Client, workerAddresses[0],
				netmlbparameters.BGPStateEstablished)
		}, netmlbparameters.Timeout, netmlbparameters.Interval).Should(BeTrue(),
			fmt.Sprintf("BGP neighborship is not established between"+
				" %s and worker with IP address %s", l3Client.Name, workerAddresses[0]))
		Eventually(func() error {
			return nethelper.IsBFDHasStatus(l3Client, workerAddresses[0],
				netmlbparameters.BFDStatusUp)
		}, netmlbparameters.TimeoutBFDBGP, netmlbparameters.Interval).ShouldNot(HaveOccurred(),
			fmt.Sprintf("BFD is not established between"+
				" %s and worker with IP address %s", l3Client.Name, workerAddresses[0]))
	})

	AfterEach(func() {
		By("Deleting MetalLB configuration")
		err = netmetallbhelper.DeleteAllBGPPeers()
		Expect(err).ToNot(HaveOccurred(), "Failed to delete all BGPPeers.")

		err = netmetallbhelper.DeleteConfigMap(netparameters.MasterConfigMapName, netmlbparameters.TestNamespace)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to delete DeleteConfigMap %s.",
			netparameters.MasterConfigMapName))

		err = netmetallbhelper.DeleteAllBFDProfiles()
		Expect(err).ToNot(HaveOccurred(), "Failed to delete all BFDProfiles.")

		err = nethelper.DeleteNADs([]string{netmlbparameters.ExternalNADName},
			netmlbparameters.TestNamespace)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to delete NAD %s.",
			netmlbparameters.ExternalNADName))

		netmetallbhelper.DeleteAllIPAddressPools()

		err = netmetallbhelper.DeleteAllBGPAdvertisements()
		Expect(err).ToNot(HaveOccurred(), "Failed to delete all BGPAdvertisements.")

		err = namespaces.CleanPods(netmlbparameters.TestNamespace, helper.Apiclient)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to remove all pods from the namespace %s.",
			netmlbparameters.TestNamespace))

		metallb := &metallboperatorv1beta1.MetalLB{}
		err = helper.Apiclient.Get(context.Background(), types.NamespacedName{Name: netmlbparameters.MetalLBCRName,
			Namespace: netmlbparameters.MetalLBOperatorNameSpace}, metallb)
		Expect(err).ToNot(HaveOccurred(), "An unexpected error occurred while getting CR metalLB.")

		metallbutils.Delete(metallb)
	})

	Context("two advertisement modes", func() {
		BeforeEach(func() {
			if len(ipv4metalLBIPList) < 3 {
				Skip("There are not enough IPv4 addresses (3) configured in env variables METALLB_ADDR_LIST")
			}

			By("Create an additional server nginx test pod for second service")
			netmetallbhelper.DefineAndRunMlbClientPod(workerNodeList[1].Name,
				helper.Config.Network.TestContainerImage,
				netmlbparameters.AppLabel2, []string{netmlbparameters.ArgCommandNGINX})

			By("Creating a Layer2 Addresspool for the new server nginx pod")
			err = helper.Apiclient.Create(context.Background(),
				netmetallbhelper.DefineMetalLBAddressPool([]string{fmt.Sprintf(ipv4metalLBIPList[1] + "/32")},
					netmlbparameters.Layer2,
					netmlbparameters.AddressPoolL2))
			Expect(err).ToNot(HaveOccurred(), "An unexpected error occurred during Layer-2 Addresspool creation.")
		})

		AfterEach(func() {
			err = netmetallbhelper.DeleteAllLBServices(netmlbparameters.TestNamespace)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to delete all services in the namespace %s.",
				netmlbparameters.TestNamespace))

			netmetallbhelper.DeleteAllAddressPools()
		})

		// OCP-50060
		DescribeTable("should work together",
			func(externalTrafficPolicy string) {
				By("Creating 2 MetalLB services for L2 and L3 server nginx pods")
				err = netmetallbhelper.DefineAndCreateLBService(
					netmlbparameters.TestNamespace,
					netparameters.IPV4Family,
					netmlbparameters.AddressPoolName,
					netmlbparameters.AppLabel1,
					netmlbparameters.ProtocolTCP,
					k8sv1.ServiceExternalTrafficPolicyType(externalTrafficPolicy))
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("An unexpected error occurred during service %s creation.",
						netmlbparameters.AddressPoolName))

				err = netmetallbhelper.DefineAndCreateLBService(
					netmlbparameters.TestNamespace,
					netparameters.IPV4Family,
					netmlbparameters.AddressPoolL2,
					netmlbparameters.AppLabel2,
					netmlbparameters.ProtocolTCP,
					k8sv1.ServiceExternalTrafficPolicyType(externalTrafficPolicy))
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("An unexpected error occurred during service %s creation.",
						netmlbparameters.AddressPoolL2))

				By("Creating L2 client")
				l2ClientDefinition, err := netmetallbhelper.DefineMlbPodWithNetwork(secondMasterNode.Name,
					netmlbparameters.TestNamespace,
					helper.Config.Network.TestContainerImage,
					netmlbparameters.ExternalNADName,
					ipv4metalLBIPList[2])
				Expect(err).ToNot(HaveOccurred(), "An unexpected error occurred during l2 client definition.")
				l2Client := helper.WaitUntilPodCreatedAndRunning(l2ClientDefinition, netmlbparameters.PodWaitingTime)

				By("Validating that L3 client can curl both LB IP addresses")
				httpOutput, err := netmetallbhelper.HTTPMlbPod(l3Client, ipv4metalLBIPList[0],
					netmlbparameters.IPv4AddressesLBList[0],
					netparameters.IPV4Family, netmlbparameters.TestContainerName, netmlbparameters.BGP)
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("Failed to curl LB address %s from l3Client %s.: %s",
						netmlbparameters.IPv4AddressesLBList[0], l3Client.Name, httpOutput))

				Eventually(func() error {
					httpOutput, err = netmetallbhelper.HTTPMlbPod(l3Client, ipv4metalLBIPList[0],
						ipv4metalLBIPList[1],
						netparameters.IPV4Family, netmlbparameters.TestContainerName, netmlbparameters.Layer2)

					return err
				}, 1*time.Minute, 2*time.Second).ShouldNot(HaveOccurred(),
					fmt.Sprintf("L3client %s can not curl LB IP address %s: %s",
						l3Client.Name, ipv4metalLBIPList[1], httpOutput))

				By("Validating that L2 client can curl L2 LB IP address")
				Eventually(func() error {
					httpOutput, err = netmetallbhelper.HTTPMlbPod(l2Client, ipv4metalLBIPList[2], ipv4metalLBIPList[1],
						netparameters.IPV4Family, parameters.MainContainerName, netmlbparameters.Layer2)

					return err
				}, 1*time.Minute, 2*time.Second).ShouldNot(HaveOccurred(),
					fmt.Sprintf("L2client %s can not curl LB IP address %s: %s",
						l2Client.Name, ipv4metalLBIPList[1], httpOutput))

				By("Validating that L2 client cannot curl L3 LB IP address")
				Eventually(func() error {
					httpOutput, err = netmetallbhelper.HTTPMlbPod(l2Client,
						ipv4metalLBIPList[2], netmlbparameters.IPv4AddressesLBList[0],
						netparameters.IPV4Family, parameters.MainContainerName, netmlbparameters.Layer2)

					return err
				}, 1*time.Minute, 2*time.Second).Should(HaveOccurred(),
					fmt.Sprintf("L2client %s can curl LB IP address %s which is not expected: %s",
						l2Client.Name, netmlbparameters.IPv4AddressesLBList[0], httpOutput))
			},
			Entry("Service with Local externalTrafficPolicy", netmlbparameters.ExtTrafPolLocal),
			Entry("Service with Cluster externalTrafficPolicy", netmlbparameters.ExtTrafPolCluster),
		)
	})
	Context("Concurrent Layer2 and Layer3", func() {
		var (
			secInterfaces []*sriovv1.InterfaceExt
		)
		BeforeEach(func() {
			localGWMode := netmetallbhelper.GetGWMode()
			// if false - share GW, if true - local GW
			if !localGWMode {
				By("Configuring Local GW mode")
				netmetallbhelper.SetLocalGWMode(true)
				netmetallbhelper.WaitNetworkOperator()
				netmetallbhelper.ChangedGWMode = true
			}
			Expect(netmetallbhelper.GetGWMode()).To(BeTrue())

			By("Creating a L2Advertisement")
			err = helper.Apiclient.Create(context.Background(),
				netmetallbhelper.DefineL2Advertisement(netmlbparameters.L2AdvertisementName,
					[]string{netmlbparameters.AddressPoolName}))
			Expect(err).ToNot(HaveOccurred(), "An unexpected error occurred while creating L2Advertisement.")

			By(fmt.Sprintf("Adding IP to a secondary interface on the worker-0 %s", workerNodeList[0].Name))
			sriovInfos, err := cluster.DiscoverSriov(helper.Apiclient, parameters.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to find sriov supported nodes")
			sriovInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
			Expect(err).ToNot(HaveOccurred(), "Failed to find sriov supported interfaces")
			secInterfaces, err = helper.Config.GetSriovInterfaces(sriovInterfaces, 1)
			Expect(err).ToNot(HaveOccurred(), "Failed to find valid sriov interfaces")

			outputString, err := netmetallbhelper.AddOrDeleteNodeSecIPAddViaSpeaker("add", workerNodeList[0].Name,
				netmlbparameters.IPSecondaryInterface1, secInterfaces[0].Name)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error occurred while"+
				" adding IP address to the secondary interface %s.:%s", secInterfaces[0].Name, outputString))

			By(fmt.Sprintf("Creating macvlan NAD with the secondary interface %s", secInterfaces[0].Name))
			err = helper.Apiclient.Create(context.Background(),
				netmetallbhelper.DefineMacVlanNAD(netmlbparameters.InternalNADName, secInterfaces[0].Name))
			Expect(err).ToNot(HaveOccurred(), "An unexpected error occurred during"+
				" secondary interface NetworkAttachmentDefinition creation.")
		})

		AfterEach(func() {
			err = netmetallbhelper.DeleteAllL2Advertisements()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete all L2Advertisements.")

			outputString, err := netmetallbhelper.AddOrDeleteNodeSecIPAddViaSpeaker("del", workerNodeList[0].Name,
				netmlbparameters.IPSecondaryInterface1, secInterfaces[0].Name)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error occurred while"+
				" deleting IP address from the secondary interface %s.:%s", secInterfaces[0].Name, outputString))

			err = nethelper.DeleteNADs([]string{netmlbparameters.InternalNADName},
				netmlbparameters.TestNamespace)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to delete NAD %s.",
				netmlbparameters.InternalNADName))

			netmetallbhelper.RestoreNodeGWMode()
		})
		// 50059
		It("should work concurrently Layer 2 and Layer 3", func() {
			By("Creating MetalLB service")
			err = netmetallbhelper.DefineAndCreateLBService(
				netmlbparameters.TestNamespace,
				netparameters.IPV4Family,
				netmlbparameters.AddressPoolName,
				netmlbparameters.AppLabel1,
				netmlbparameters.ProtocolTCP,
				k8sv1.ServiceExternalTrafficPolicyTypeLocal)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("An unexpected error occurred during service %s creation.",
				netmlbparameters.AddressPoolName))

			By("Creating L2 client")
			l2ClientDefinition, err := netmetallbhelper.DefineMlbPodWithNetwork(workerNodeList[1].Name,
				netmlbparameters.TestNamespace,
				helper.Config.Network.TestContainerImage,
				netmlbparameters.InternalNADName,
				netmlbparameters.IPSecondaryInterface2)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("An unexpected error occurred during service %s creation.",
				netmlbparameters.AddressPoolName))
			l2Client := helper.WaitUntilPodCreatedAndRunning(l2ClientDefinition, netmlbparameters.PodWaitingTime)

			By("Validating that both clients can curl to LB address")
			httpOutput, err := netmetallbhelper.HTTPMlbPod(l3Client,
				ipv4metalLBIPList[0], netmlbparameters.IPv4AddressesLBList[0],
				netparameters.IPV4Family, netmlbparameters.TestContainerName, netmlbparameters.BGP)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to curl LB address %s from l3Client %s.: %s",
					netmlbparameters.IPv4AddressesLBList[0], l3Client.Name, httpOutput))

			Eventually(func() error {
				httpOutput, err = netmetallbhelper.HTTPMlbPod(l2Client, netmlbparameters.IPSecondaryInterface2,
					netmlbparameters.IPv4AddressesLBList[0],
					netparameters.IPV4Family, parameters.MainContainerName, netmlbparameters.Layer2)

				return err
			}, 1*time.Minute, 2*time.Second).ShouldNot(HaveOccurred(),
				fmt.Sprintf("L2client %s can not curl LB IP address %s: %s",
					l2Client.Name, netmlbparameters.IPv4AddressesLBList[0], httpOutput))
		})
	})
})
