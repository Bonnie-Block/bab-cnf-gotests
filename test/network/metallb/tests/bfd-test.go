package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metallboperatorv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	metallbutils "github.com/metallb/metallb-operator/test/e2e/metallb"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"

	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	firstWorkerNodeAddress  string
	secondWorkerNodeAddress string
	workerNodeList          []k8sv1.Node
	workerAddresses         []string
	masterNode              k8sv1.Node
	ipv4metalLBIPList       []string
	err                     error
	testSetupFail           = true
)

var _ = Describe("BFD", func() {
	execute.BeforeAll(func() {
		masterNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(masterNodeList)).To(BeNumerically(">", 0))
		masterNode = masterNodeList[0]
		workerNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(workerNodeList)).To(BeNumerically(">", 1))
		workerAddresses = nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)
		Expect(len(workerAddresses)).To(BeNumerically(">", 1))
		firstWorkerNodeAddress = workerAddresses[0]
		secondWorkerNodeAddress = workerAddresses[1]
		testSetupFail = false
	})
	BeforeEach(func() {
		if testSetupFail {
			Fail("Test failed due to error in BeforeAll")
		}

		By("Setup Metallb")
		netmetallbhelper.SetupMetalLB()

		By("Checking if MetalLB operator is installed and running")
		Eventually(netmetallbhelper.IsMetalLBAvailable, netmlbparameters.Timeout, netmlbparameters.Interval).
			ShouldNot(HaveOccurred())

		By("Validating parameters")
		ipv4metalLBIPList, _, err = netmetallbhelper.GetMetalLBIPByFamily()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("An unexpected error occurred while"+
			" determining the IP addresses from the METALLB_ADDR_LIST environment variable.: %s", err))
		if len(ipv4metalLBIPList) < 2 {
			Skip("There are not enough IPv4 addresses configured in env variables METALLB_ADDR_LIST")
		}

		netmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
			helper.Config.General.CnfNodeLabel, "/")[1],
			netparameters.IPV4Family,
			ipv4metalLBIPList[0],
			"")

		By("Creating external br-ex NetworkAttachmentDefinition")
		err = helper.Apiclient.Create(context.Background(),
			netmetallbhelper.DefineMacVlanNAD(netmlbparameters.ExternalNADName, netmlbparameters.BREXInterface))
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("An unexpected error occurred during br-ex NetworkAttachmentDefinition creation: %s", err))
	})

	AfterEach(func() {
		By("Deleting MetalLB configuration")
		_ = netmetallbhelper.DeleteLabelFromWorkers(netmlbparameters.SpeakerNodeTestLabel)
		metallb := &metallboperatorv1beta1.MetalLB{}
		err = helper.Apiclient.Get(context.Background(), types.NamespacedName{Name: netmlbparameters.MetalLBCRName,
			Namespace: netmlbparameters.MetalLBOperatorNameSpace}, metallb)
		Expect(err).ToNot(HaveOccurred())

		metallbutils.Delete(metallb)

		err = nethelper.DeleteNADs(netmlbparameters.TestNamespace, netmlbparameters.ExternalNADName)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to delete NADs.: %s", err))
	})

	Context("Single hop", func() {
		var clientPodOnMasterNode *k8sv1.Pod
		BeforeEach(func() {
			netmetallbhelper.CreateBGPWithBFD(netmlbparameters.EBGPProtocol, ipv4metalLBIPList[0])
			clientPodOnMasterNode = netmetallbhelper.CreateClientOnMaster(netmlbparameters.EBGPProtocol,
				workerAddresses,
				ipv4metalLBIPList[0],
				masterNode.Name,
				netmlbparameters.ExternalNADName)
			By("Checking that BGP and BFD sessions are established and up")

			Eventually(func() bool {
				return netmetallbhelper.IsBGPNeighborshipHasState(clientPodOnMasterNode, firstWorkerNodeAddress,
					netmlbparameters.BGPStateEstablished)
			}, netmlbparameters.Timeout, netmlbparameters.Interval).Should(BeTrue())
			Eventually(func() error {
				return nethelper.IsBFDHasStatus(clientPodOnMasterNode, firstWorkerNodeAddress,
					netmlbparameters.BFDStatusUp)
			}, netmlbparameters.TimeoutBFDBGP, netmlbparameters.Interval).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			By("Cleaning after test")
			masterConfigMapList := []string{netparameters.MasterConfigMapName}
			err := netmetallbhelper.DeleteAllBGPPeers()
			Expect(err).ToNot(HaveOccurred())
			err = netmetallbhelper.DeleteConfigMaps(masterConfigMapList, netmlbparameters.TestNamespace)
			Expect(err).ToNot(HaveOccurred())

			err = helper.Apiclient.Delete(context.Background(), clientPodOnMasterNode)
			Expect(err).ToNot(HaveOccurred())

			By("Delete all BFD Profiles")
			err = netmetallbhelper.DeleteAllBFDProfiles()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should remove BGP and BFD configuration", func() {
			By("Delete all BGP Peers")
			err := netmetallbhelper.DeleteAllBGPPeers()
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() bool {
				return netmetallbhelper.IsProtocolConfigured(netmlbparameters.BGPConfigPrefix)
			}, 1*time.Minute, 2*time.Second).Should(BeFalse(), "BGP configuration is not removed")

			// Failed due to BZ 2050824. The BFD configuration check should be added after the BZ fix.
			// By("Delete all BFD Profiles")
			// err = netmetallbhelper.DeleteAllBFDProfiles()
			// Expect(err).ToNot(HaveOccurred())

			// Eventually(func() bool {
			//	return netmetallbhelper.IsProtocolConfigured(netmlbparameters.BFDConfigPrefix)
			// }, 1*time.Minute, 2*time.Second).Should(BeFalse(), "BFD configuration is not removed")
		})

		Context("basic functionality", func() {
			AfterEach(func() {
				err := netmetallbhelper.DeleteLabelFromWorkers(netmlbparameters.SpeakerNodeTestLabel)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() int {
					speakerPodList, _ := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).List(
						context.Background(),
						metav1.ListOptions{LabelSelector: netmlbparameters.SpeakersLabelSelector},
					)

					return len(speakerPodList.Items)
				}, 1*time.Minute, 1*time.Second).Should(BeNumerically("==", 0))

				err = netmetallbhelper.UpdateToDefaultSpeakerNodeSelector()
				Expect(err).ToNot(HaveOccurred())

				Eventually(netmetallbhelper.AreSpeakersReady, netmlbparameters.Timeout, netmlbparameters.Interval).
					Should(BeTrue(), "Speaker pods are not ready")
			})

			It("should provide fast link failure detection ", func() {
				netmetallbhelper.TestMetalLBBFD(netmlbparameters.ScenarioSingleHop,
					clientPodOnMasterNode,
					firstWorkerNodeAddress, secondWorkerNodeAddress, netmlbparameters.ExtTrafPolLocal)
			})
		})

		It("provides Prometheus BFD metrics", func() {
			_, err := namespaces.LabelNamespace(helper.Apiclient,
				netmlbparameters.MetalLBOperatorNameSpace,
				netmlbparameters.MonitoringLabel,
				"true")
			Expect(err).ToNot(HaveOccurred())
			speakerPods, err := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).
				List(context.Background(), metav1.ListOptions{
					LabelSelector: netmlbparameters.SpeakersLabelSelector,
				})
			Expect(err).ToNot(HaveOccurred())
			metalLBMonitoredEntriesByPod, uniqueMetricKeys := netmetallbhelper.CollectMetalLBMetricsByPod(speakerPods.Items,
				"metallb_bfd_")

			Eventually(func() error {
				podsPerPrometheusMetricKey := netmetallbhelper.CollectPrometheusMetrics(uniqueMetricKeys)

				return netmetallbhelper.ContainSameMetrics(metalLBMonitoredEntriesByPod, podsPerPrometheusMetricKey)
			}, netmlbparameters.Timeout, 2*netmlbparameters.Interval).Should(Not(HaveOccurred()))
		})
	})

	Context("Multihop", func() {
		speakerRoutesMap := make(map[string]string)
		describe := netmetallbhelper.DescribeBFDParameters

		BeforeEach(func() {
			By("Collecting information before test")
			speakerPodList, err := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).List(
				context.Background(),
				metav1.ListOptions{LabelSelector: netmlbparameters.SpeakersLabelSelector},
			)
			Expect(err).ToNot(HaveOccurred())

			speakerRoutesMap, err = netmetallbhelper.CreateRoutesMap(*speakerPodList, ipv4metalLBIPList)
			Expect(err).ToNot(HaveOccurred())

			localGWMode := netmetallbhelper.GetGWMode()
			// if false - share GW, if true - local GW
			if !localGWMode {
				By("Configuring Local GW mode")
				netmetallbhelper.SetLocalGWMode(true)
				netmetallbhelper.WaitNetworkOperator()
				netmetallbhelper.ChangedGWMode = true
			}
			Expect(netmetallbhelper.GetGWMode()).To(BeTrue())
		})

		AfterEach(func() {
			By("Cleaning after test")
			outputString, err := netmetallbhelper.AddOrDeleteSpeakerStaticRoute("del", speakerRoutesMap,
				netmlbparameters.ClientIpv4IP)
			Expect(err).ToNot(HaveOccurred(), outputString)

			err = netmetallbhelper.DeleteAllLBServices(netmlbparameters.TestNamespace)
			Expect(err).ToNot(HaveOccurred())

			err = netmetallbhelper.DeleteAllBGPPeers()
			Expect(err).ToNot(HaveOccurred())

			masterConfigMapList := []string{netparameters.MasterConfigMapName}
			err = netmetallbhelper.DeleteConfigMaps(masterConfigMapList, netmlbparameters.TestNamespace)
			Expect(err).ToNot(HaveOccurred())

			netmetallbhelper.DeleteAllIPAddressPools()

			err = netmetallbhelper.DeleteAllBGPAdvertisements()
			Expect(err).ToNot(HaveOccurred())

			err = nethelper.DeleteNADs(netmlbparameters.TestNamespace, netmlbparameters.InternalNADName)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to delete NADs.: %s", err))

			err = namespaces.CleanPods(netmlbparameters.TestNamespace, helper.Apiclient)
			Expect(err).ToNot(HaveOccurred())

			err = netmetallbhelper.DeleteAllBFDProfiles()
			Expect(err).ToNot(HaveOccurred())

			// Failed due to BZ 2050824. The BFD configuration check should be added after the BZ fix.
			// Eventually(func() bool {
			//	return netmetallbhelper.IsProtocolConfigured(netmlbparameters.BFDConfigPrefix)
			// }, 1*time.Minute, 2*time.Second).Should(BeFalse(), "BFD configuration is not removed")

			err = netmetallbhelper.DeleteLabelFromWorkers(netmlbparameters.SpeakerNodeTestLabel)
			Expect(err).ToNot(HaveOccurred())

			netmetallbhelper.RestoreNodeGWMode()
		})

		DescribeTable("should provide fast link failure detection",
			func(bgpProtocol string, ipStack string, externalTrafficPolicy k8sv1.ServiceExternalTrafficPolicyType) {

				err = netmetallbhelper.ValidateIPs(append(ipv4metalLBIPList, firstWorkerNodeAddress), ipStack)
				if err != nil {
					Skip(err.Error())
				}

				netmetallbhelper.CreateBGPWithBFD(bgpProtocol, netmlbparameters.ClientIpv4IP)

				By("Creating an IPAddressPool and BGPAdvertisement")
				ipAddressPoolDefinition := netmetallbhelper.DefineMetalLBIPAddressPool(netmlbparameters.IPv4AddressesLBList,
					ipStack,
					netmlbparameters.AddressPoolName)

				err := helper.Apiclient.Create(context.Background(), ipAddressPoolDefinition)
				Expect(err).ToNot(HaveOccurred())

				bgpAdvertisementDefinition := netmetallbhelper.DefineBGPAdvertisement(
					netmlbparameters.BGPAdvertisementName,
					netmlbparameters.CommunityNoAdv,
					ipStack,
					[]string{ipAddressPoolDefinition.Name},
					netmlbparameters.PrefixLen32,
					netmlbparameters.LocalPref100)
				err = helper.Apiclient.Create(context.Background(), bgpAdvertisementDefinition)
				Expect(err).ToNot(HaveOccurred())

				By("Creating a MetalLB service")
				_, err = netmetallbhelper.DefineAndCreateLBService(
					netmlbparameters.TestNamespace,
					ipStack,
					netmlbparameters.AddressPoolName,
					netmlbparameters.AppLabel1,
					netmlbparameters.ProtocolTCP,
					externalTrafficPolicy)
				Expect(err).ToNot(HaveOccurred())

				By("Creating nginx test pod")
				netmetallbhelper.DefineAndRunMlbClientPod(workerNodeList[0].Name,
					helper.Config.Network.TestContainerImage,
					netmlbparameters.AppLabel1, []string{netmlbparameters.ArgCommandNGINX})

				By("Creating FRR router pods on a Master node")
				internalNADDefinition := netmetallbhelper.DefineBridgeNAD()
				err = helper.Apiclient.Create(context.Background(), internalNADDefinition)
				Expect(err).ToNot(HaveOccurred())

				frrRouterPodOnMaster1 := netmetallbhelper.DefineRouterPod(masterNode.Name,
					netmlbparameters.IPv4AddressesLBList[0], firstWorkerNodeAddress,
					netmlbparameters.ExternalNADName, internalNADDefinition.Name,
					ipv4metalLBIPList[0], netmlbparameters.InternalRouter1IPv4)
				helper.WaitUntilPodCreatedAndRunning(frrRouterPodOnMaster1, netmlbparameters.PodWaitingTime)

				frrRouterPodOnMaster2 := netmetallbhelper.DefineRouterPod(masterNode.Name,
					netmlbparameters.IPv4AddressesLBList[0], secondWorkerNodeAddress,
					netmlbparameters.ExternalNADName, internalNADDefinition.Name,
					ipv4metalLBIPList[1], netmlbparameters.InternalRouter2IPv4)
				helper.WaitUntilPodCreatedAndRunning(frrRouterPodOnMaster2, netmlbparameters.PodWaitingTime)

				clientPodOnMasterNode := netmetallbhelper.CreateClientOnMaster(bgpProtocol,
					workerAddresses,
					netmlbparameters.ClientIpv4IP,
					masterNode.Name,
					internalNADDefinition.Name)

				// Add static routes from client towards Speaker via router internal IPs
				for num, routerInternalIP := range []string{netmlbparameters.InternalRouter1IPv4,
					netmlbparameters.InternalRouter2IPv4} {
					buffer, err := pod.ExecCommand(helper.Apiclient, *clientPodOnMasterNode,
						[]string{"ip", "route", "add", workerAddresses[num], "via", routerInternalIP})
					Expect(err).ToNot(HaveOccurred(), buffer.String())
				}

				By("Adding static routes to the speakers")
				outputString, err := netmetallbhelper.AddOrDeleteSpeakerStaticRoute("add", speakerRoutesMap,
					netmlbparameters.ClientIpv4IP)
				Expect(err).ToNot(HaveOccurred(), outputString)

				By("Checking that BGP and BFD sessions are established and up")
				Eventually(func() bool {
					return netmetallbhelper.IsBGPNeighborshipHasState(clientPodOnMasterNode, firstWorkerNodeAddress,
						netmlbparameters.BGPStateEstablished)
				}, netmlbparameters.Timeout, netmlbparameters.Interval).Should(BeTrue())
				Eventually(func() error {
					return nethelper.IsBFDHasStatus(clientPodOnMasterNode, firstWorkerNodeAddress,
						netmlbparameters.BFDStatusUp)
				}, netmlbparameters.TimeoutBFDBGP, netmlbparameters.Interval).ShouldNot(HaveOccurred())

				httpOutput, err := netmetallbhelper.HTTPMlbPod(clientPodOnMasterNode,
					netmlbparameters.ClientIpv4IP,
					netmlbparameters.IPv4AddressesLBList[0],
					netparameters.IPV4Family, netmlbparameters.TestContainerName, netmlbparameters.BGP)
				Expect(err).ToNot(HaveOccurred(), httpOutput)

				netmetallbhelper.TestMetalLBBFD(netmlbparameters.ScenarioMultihop,
					clientPodOnMasterNode,
					firstWorkerNodeAddress,
					secondWorkerNodeAddress,
					externalTrafficPolicy)
			},
			Entry(describe, netmlbparameters.IBPGPProtocol, netparameters.IPV4Family,
				k8sv1.ServiceExternalTrafficPolicyTypeCluster),
			Entry(describe, netmlbparameters.IBPGPProtocol, netparameters.IPV4Family,
				k8sv1.ServiceExternalTrafficPolicyTypeLocal),
			Entry(describe, netmlbparameters.EBGPProtocol, netparameters.IPV4Family,
				k8sv1.ServiceExternalTrafficPolicyTypeCluster),
			Entry(describe, netmlbparameters.EBGPProtocol, netparameters.IPV4Family,
				k8sv1.ServiceExternalTrafficPolicyTypeLocal),
		)
	})
})
