package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

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

var _ = Describe("MetalLB BGP", func() {
	var (
		ipv4metalLBIPList []string
		ipv6metalLBIPList []string
		workerNodeList    []k8sv1.Node
		masterNodeList    []k8sv1.Node
		err               error
	)

	execute.BeforeAll(func() {
		var ipV6Address string

		ipv4metalLBIPList, ipv6metalLBIPList, err = netmetallbhelper.GetMetalLBIPByFamily()
		Expect(err).ToNot(HaveOccurred())

		By(fmt.Sprintf("should select nodes by role %s ", parameters.RoleWorker))
		workerNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
		Expect(len(workerNodeList)).To(BeNumerically(">", 1))
		Expect(err).ToNot(HaveOccurred())

		masterNodeList, err = nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(masterNodeList)).To(BeNumerically(">", 0))

		clusterIPStack := netmetallbhelper.ValidateClusterIPStack()

		By(fmt.Sprintf("Running test on %s cluster", clusterIPStack))

		if clusterIPStack == netparameters.DualIPFamily {
			Expect(len(ipv6metalLBIPList)).To(BeNumerically(">", 0))
			ipV6Address = ipv6metalLBIPList[0]
		}

		netmetallbhelper.IsEnvVarMetallbIPinNodeExtNetRange(strings.Split(
			helper.Config.General.CnfNodeLabel, "/")[1],
			clusterIPStack,
			ipv4metalLBIPList[0],
			ipV6Address)
	})

	BeforeEach(func() {
		By("Setup Metallb")
		netmetallbhelper.SetupMetalLB()
	})

	AfterEach(func() {
		By("should delete AddressPool, Service, BGP Peers and test Pod after test")
		netmetallbhelper.RemoveMetallbBGPTestSetup()

	})

	Context("functionality", func() {
		// 	47174
		DescribeTable("Creating AddressPool with bgp-advertisement",
			func(ipStack string, prefixLen int32) {
				netmetallbhelper.TestBGPAdvertismentTable(
					ipStack,
					ipv4metalLBIPList,
					ipv6metalLBIPList,
					workerNodeList,
					masterNodeList,
					prefixLen)
			},
			Entry("IPv4 with Prefix 32", netparameters.IPV4Family, netmlbparameters.PrefixLen32),
			Entry("IPv4 with Prefix 28", netparameters.IPV4Family, netmlbparameters.PrefixLen28),
			Entry("IPv6 with Prefix 128", netparameters.IPV6Family, netmlbparameters.PrefixLen128),
			Entry("IPv6 with Prefix 64", netparameters.IPV6Family, netmlbparameters.PrefixLen64),
		)

		// 47203
		DescribeTable("Verify external FRR BGP Peer cannot propagate routes to Speaker",
			func(ipStack string) {
				netmetallbhelper.TestBGPBlockRouteAdvertisment(
					ipStack,
					ipv4metalLBIPList,
					ipv6metalLBIPList,
					masterNodeList,
					workerNodeList)
			},
			Entry("IPv4 propagate route", netparameters.IPV4Family),
			Entry("IPv6 propagate route", netparameters.IPV6Family),
		)
	})

	Context("updates", func() {
		// 	47174
		DescribeTable("Functional Verify bgp-advertisement updates",
			func(ipStack string, prefixLen int32) {
				netmetallbhelper.TestBGPAdvertismentTableUpdates(
					masterNodeList,
					workerNodeList,
					ipv4metalLBIPList,
					ipv6metalLBIPList,
					ipStack,
					prefixLen)
			},
			Entry("IPv4 update Prefix to 28", netparameters.IPV4Family, netmlbparameters.PrefixLen32),
			Entry("IPv6 update Prefix to 64", netparameters.IPV6Family, netmlbparameters.PrefixLen128),
		)
		// 47202
		It("BGP Timer update", func() {

			By("should create external FRR container")

			masterNodeFRRPod := netmetallbhelper.CreateFRRContainerOnMaster(
				workerNodeList,
				masterNodeList,
				ipv4metalLBIPList,
				ipv6metalLBIPList,
				netparameters.IPV4Family,
				netmlbparameters.IBGPASN,
				netmlbparameters.PropagateFalse)

			By("should create a BGP Peer on Speakers")

			workerNodesAdresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)

			err := netmetallbhelper.CreateSpeakerBGPPeerIPStack(netparameters.IPV4Family,
				ipv4metalLBIPList, ipv6metalLBIPList, netmlbparameters.IBGPASN)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				return netmetallbhelper.CheckNeighborsStatus(masterNodeFRRPod, netparameters.IPV4Family,
					workerNodesAdresses)
			}, 1*time.Minute, netmlbparameters.Interval).Should(BeTrue())

			By("should verify default BGP Peer timers")
			speakerPods, err := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).
				List(context.Background(), metav1.ListOptions{
					LabelSelector: netmlbparameters.SpeakersLabelSelector,
				})
			Expect(err).ToNot(HaveOccurred())

			defaultTimerSettings := []int{netmlbparameters.BGPDefaultHoldTimer, netmlbparameters.BGPDefaultKeepAliveTimer}
			Eventually(func() error {
				return netmetallbhelper.ValidateBGPTimers(speakerPods.Items, defaultTimerSettings)
			}, 1*time.Minute, netmlbparameters.Interval).Should(Not(HaveOccurred()))

			By("should update default BGP Peer timers")
			updatedTimerSettings := []int{netmlbparameters.BGPUpdatedHoldTimer, netmlbparameters.BGPUpdatedKeepAliveTimer}

			err = netmetallbhelper.UpdateBGPPeerTimers()
			Expect(err).ToNot(HaveOccurred())

			err = netmetallbhelper.ResetBGPPeer(masterNodeFRRPod)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() error {
				return netmetallbhelper.ValidateBGPTimers(speakerPods.Items, updatedTimerSettings)
			}, 1*time.Minute, netmlbparameters.Interval).Should(Not(HaveOccurred()))

		})

		Context("metrics", func() {
			BeforeEach(func() {
				By("should create external FRR container")

				masterNodeFRRPod := netmetallbhelper.CreateFRRContainerOnMaster(
					workerNodeList,
					masterNodeList,
					ipv4metalLBIPList,
					ipv6metalLBIPList,
					netparameters.IPV4Family,
					netmlbparameters.IBGPASN,
					netmlbparameters.PropagateFalse)

				By("should create a BGP Peer on Speakers")

				workerNodesAdresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)

				err := netmetallbhelper.CreateSpeakerBGPPeerIPStack(netparameters.IPV4Family,
					ipv4metalLBIPList, ipv6metalLBIPList, netmlbparameters.IBGPASN)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() bool {
					return netmetallbhelper.CheckNeighborsStatus(masterNodeFRRPod, netparameters.IPV4Family,
						workerNodesAdresses)
				}, 1*time.Minute, netmlbparameters.Interval).Should(BeTrue())
			})

			// 47202
			It("provides Prometheus BGP metrics", func() {
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
					"metallb_bgp_")

				Eventually(func() error {
					podsPerPrometheusMetricKey := netmetallbhelper.CollectPrometheusMetrics(uniqueMetricKeys)

					return netmetallbhelper.ContainSameMetrics(metalLBMonitoredEntriesByPod, podsPerPrometheusMetricKey)
				}, netmlbparameters.Timeout, 2*netmlbparameters.Interval).Should(Not(HaveOccurred()))
			})
		})
	})
})
