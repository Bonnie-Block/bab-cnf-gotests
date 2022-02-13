package tests

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/metallb/metallb-operator/api/v1beta1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	timeout       = time.Second * 5
	deployTimeout = time.Minute * 3
	interval      = time.Second * 1
)

var (
	bfdProfileDefinition    *v1beta1.BFDProfile
	bgpPeerDefinition       *v1beta1.BGPPeer
	masterNodePod           *k8sv1.Pod
	masterConfigMap         *k8sv1.ConfigMap
	firstWorkerNodeAddress  string
	secondWorkerNodeAddress string
	workerNodeList          []k8sv1.Node
	err                     error
)

var _ = Describe("BFD", func() {
	execute.BeforeAll(func() {
		By("Setup Metallb")
		netmetallbhelper.SetupMetalLB()

		By("Checking MetalLB operator is installed and running")
		Eventually(netmetallbhelper.IsMetalLBAvailable, deployTimeout, interval).ShouldNot(HaveOccurred())
	})

	Context("Single hop", func() {
		BeforeEach(func() {
			workerNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(workerNodeList)).To(BeNumerically(">", 1))
			workerNodesAddresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(workerNodesAddresses)).To(BeNumerically(">", 1))
			firstWorkerNodeAddress = workerNodesAddresses[0]
			secondWorkerNodeAddress = workerNodesAddresses[1]

			masterNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(masterNodeList)).To(BeNumerically(">", 0))
			masterNode := masterNodeList[0]

			By("Creating BFD profile")
			bfdProfileDefinition = netmetallbhelper.DefineBFDProfile(netmlbparameters.BFDProfileName)
			err = helper.Apiclient.Create(context.Background(), bfdProfileDefinition)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				return netmetallbhelper.IsProtocolConfigured(netmlbparameters.BFDConfigPrefix)
			}, deployTimeout, interval).
				Should(BeTrue(), "BFD is not configured on the Speakers")

			By("Creating BGP Peers")
			bgpPeerDefinition = netmetallbhelper.DefineBGPPeerWithBFD(masterNode.Status.Addresses[0].Address,
				netmlbparameters.Asn2, netmlbparameters.BFDProfileName)
			err = helper.Apiclient.Create(context.Background(), bgpPeerDefinition)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				return netmetallbhelper.IsProtocolConfigured(netmlbparameters.BGPConfigPrefix)
			}, deployTimeout, interval).Should(BeTrue(), "BGP is not configured on the Speakers")

			By("Creating FRR container on a Master node")
			masterConfigMap = netmetallbhelper.DefineBFDMLBConfigMap(workerNodesAddresses,
				netparameters.MasterConfigMapName,
				netmlbparameters.Asn1)
			_, err = helper.Apiclient.ConfigMaps(netmlbparameters.TestNamespace).Create(
				context.TODO(),
				masterConfigMap,
				metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			frrPod := nethelper.DefineFRRPod(masterNode.Name, netmlbparameters.TestNamespace)
			masterNodePod = helper.WaitUntilPodCreatedAndRunning(frrPod, deployTimeout)

			By("Checking that BGP and BFD sessions are established and up")
			Eventually(func() bool {
				return netmetallbhelper.IsBGPNeighborshipHasState(masterNodePod, firstWorkerNodeAddress,
					netmlbparameters.BGPStateEstablished)
			}, deployTimeout, interval).Should(BeTrue())
			Eventually(func() error {
				return nethelper.IsBFDHasStatus(masterNodePod, firstWorkerNodeAddress,
					netmlbparameters.BFDStatusUp)
			}, timeout, interval).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			By("Cleaning after test")
			err = netmetallbhelper.DeleteAllBGPPeers()
			Expect(err).ToNot(HaveOccurred())

			err = helper.Apiclient.Delete(context.Background(), masterConfigMap)
			Expect(err).ToNot(HaveOccurred())

			err = helper.Apiclient.Delete(context.Background(), masterNodePod)
			Expect(err).ToNot(HaveOccurred())

			err = netmetallbhelper.DeleteAllBFDProfiles()
			Expect(err).ToNot(HaveOccurred())
			// Failed due to BZ 2050824. The BFD configuration check should be removed after the BZ fix.
			Eventually(func() bool {
				return netmetallbhelper.IsProtocolConfigured(netmlbparameters.BFDConfigPrefix)
			}, 1*time.Minute, 2*time.Second).Should(BeFalse(), "BFD configuration is not removed")
		})

		Context("basic functionality", func() {
			AfterEach(func() {
				err = netmetallbhelper.DeleteLabelFromWorkers(netmlbparameters.SpeakerNodeTestLabel)
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

				Eventually(netmetallbhelper.AreSpeakersReady, deployTimeout, interval).
					Should(BeTrue(), "Speaker pods are not ready")
			})

			It("should provide fast link failure detection ", func() {
				By("Changing the label selector for Metallb and adding a label for Workers")
				workerNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(workerNodeList)).To(BeNumerically(">", 1))
				err = netmetallbhelper.UpdateSpeakerNodeSelector(netmlbparameters.MetalLBOperatorNameSpace,
					map[string]string{netmlbparameters.SpeakerNodeTestLabel: ""})
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() int {
					speakerPodList, _ := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).List(
						context.Background(),
						metav1.ListOptions{LabelSelector: netmlbparameters.SpeakersLabelSelector},
					)

					return len(speakerPodList.Items)
				}, 1*time.Minute, 1*time.Second).Should(BeNumerically("==", 0))

				for _, worker := range workerNodeList {
					_, err = nodes.LabelNode(helper.Apiclient, worker.Name, netmlbparameters.SpeakerNodeTestLabel, "")
					Expect(err).ToNot(HaveOccurred())
				}

				Eventually(netmetallbhelper.AreSpeakersReady, deployTimeout, interval).
					Should(BeTrue(), "Speaker pods are not ready")
				By("Checking that BGP and BFD sessions are established and up")
				Eventually(func() bool {
					return netmetallbhelper.IsBGPNeighborshipHasState(masterNodePod, firstWorkerNodeAddress,
						netmlbparameters.BGPStateEstablished)
				}, deployTimeout, interval).Should(BeTrue())
				Eventually(func() error {
					return nethelper.IsBFDHasStatus(masterNodePod, firstWorkerNodeAddress,
						netmlbparameters.BFDStatusUp)
				}, timeout, interval).ShouldNot(HaveOccurred())

				Eventually(func() bool {
					return netmetallbhelper.IsBGPNeighborshipHasState(masterNodePod, secondWorkerNodeAddress,
						netmlbparameters.BGPStateEstablished)
				}, deployTimeout, interval).Should(BeTrue())
				Eventually(func() error {
					return nethelper.IsBFDHasStatus(masterNodePod, secondWorkerNodeAddress,
						netmlbparameters.BFDStatusUp)
				}, timeout, interval).ShouldNot(HaveOccurred())

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
						metav1.ListOptions{LabelSelector: "component=speaker"},
					)

					return len(speakerPodList.Items)
				}, 1*time.Minute, 1*time.Second).Should(BeNumerically("==", len(workerNodeList)-1))

				By("Checking that BGP and BFD sessions are down with one BGPpeer and continue to work with another")
				Expect(nethelper.IsBFDHasStatus(masterNodePod, firstWorkerNodeAddress,
					netmlbparameters.BFDStatusDown)).ShouldNot(HaveOccurred())
				Expect(netmetallbhelper.IsBGPNeighborshipHasState(masterNodePod, firstWorkerNodeAddress,
					netmlbparameters.BGPStateEstablished)).ToNot(BeTrue())

				Expect(nethelper.IsBFDHasStatus(masterNodePod, secondWorkerNodeAddress,
					netmlbparameters.BFDStatusUp)).ShouldNot(HaveOccurred())
				Expect(netmetallbhelper.IsBGPNeighborshipHasState(masterNodePod, secondWorkerNodeAddress,
					netmlbparameters.BGPStateEstablished)).To(BeTrue())

				By("Bringing Speaker pod back and checking that speaker pods  are up and running")
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
				}, 1*time.Minute, 1*time.Second).Should(BeNumerically("==", len(workerNodeList)))

				By("Checking that BGP and BFD sessions are established and up")
				Eventually(func() bool {
					return netmetallbhelper.IsBGPNeighborshipHasState(masterNodePod, firstWorkerNodeAddress,
						netmlbparameters.BGPStateEstablished)
				}, deployTimeout, interval).Should(BeTrue())
				Eventually(func() error {
					return nethelper.IsBFDHasStatus(masterNodePod, firstWorkerNodeAddress,
						netmlbparameters.BFDStatusUp)
				}, timeout, interval).ShouldNot(HaveOccurred())

				Eventually(func() bool {
					return netmetallbhelper.IsBGPNeighborshipHasState(masterNodePod, secondWorkerNodeAddress,
						netmlbparameters.BGPStateEstablished)
				}, deployTimeout, interval).Should(BeTrue())
				Eventually(func() error {
					return nethelper.IsBFDHasStatus(masterNodePod, secondWorkerNodeAddress,
						netmlbparameters.BFDStatusUp)
				}, timeout, interval).ShouldNot(HaveOccurred())
			})
		})

		It("provides Prometheus BFD metrics", func() {
			_, err = namespaces.LabelNamespace(helper.Apiclient,
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
			}, deployTimeout, 2*interval).Should(Not(HaveOccurred()))
		})
	})
})
