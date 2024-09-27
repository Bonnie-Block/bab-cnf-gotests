package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metallboperatorv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmetallbhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/metallb/netmlbparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"

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
		testSetupFail     = true
	)

	describeIPStack := netmetallbhelper.CreateParamIPStackInJSON
	describeIPStackPrefix := netmetallbhelper.CreateParamIPStackPrefixInJSON

	execute.BeforeAll(func() {
		var ipV6Address string

		ipv4metalLBIPList, ipv6metalLBIPList, err = netmetallbhelper.GetMetalLBIPByFamily()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("An unexpected error occurred while"+
			" determining the IP addresses from the METALLB_ADDR_LIST environment variable.: %s", err))

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
		testSetupFail = false
	})

	BeforeEach(func() {
		if testSetupFail {
			Fail("Test failed due to error in BeforeAll")
		}
		By("Setup Metallb")
		netmetallbhelper.SetupMetalLB()
	})

	AfterEach(func() {
		By("should delete AddressPool, Service, BGP Peers and test Pod after test")
		externalNadList := []string{netmlbparameters.ExternalNADName}
		masterConfigMapList := []string{netparameters.MasterConfigMapName}
		netmetallbhelper.RemoveMetallbBGPTestSetup(externalNadList, masterConfigMapList)

	})

	Context("functionality", func() {

		// 47203
		DescribeTable("Verify external FRR BGP Peer cannot propagate routes to Speaker",
			polarion.ID("47203"),
			func(ipStack string) {
				netmetallbhelper.TestBGPBlockRouteAdvertisment(
					ipStack,
					ipv4metalLBIPList,
					ipv6metalLBIPList,
					masterNodeList,
					workerNodeList)
			},
			Entry(describeIPStack, netparameters.IPV4Family,
				polarion.SetProperty("IPStack", netparameters.IPV4Family)),
			Entry(describeIPStack, netparameters.IPV6Family,
				polarion.SetProperty("IPStack", netparameters.IPV6Family)),
		)
	})

	Context("updates", func() {
		// 	47178
		DescribeTable("Verify bgp-advertisement updates", polarion.ID("47178"),
			func(ipStack string, prefixLen int32) {
				netmetallbhelper.TestBGPAdvertismentTableUpdates(
					masterNodeList,
					workerNodeList,
					ipv4metalLBIPList,
					ipv6metalLBIPList,
					ipStack,
					prefixLen)
			},
			Entry(describeIPStackPrefix, netparameters.IPV4Family, netmlbparameters.PrefixLen32,
				polarion.SetProperty("IPStack", netparameters.IPV4Family),
				polarion.SetProperty("PrefixLenght", fmt.Sprintf("%d", netmlbparameters.PrefixLen32))),
			Entry(describeIPStackPrefix, netparameters.IPV6Family, netmlbparameters.PrefixLen128,
				polarion.SetProperty("IPStack", netparameters.IPV6Family),
				polarion.SetProperty("PrefixLenght", fmt.Sprintf("%d", netmlbparameters.PrefixLen128))),
		)
		// 47180
		It("BGP Timer update", polarion.ID("47180"), func() {

			By("should create external FRR container")

			masterNodeFRRPod := netmetallbhelper.CreateFRRContainerOnMaster(
				workerNodeList,
				masterNodeList[0],
				ipv4metalLBIPList[0],
				"",
				netparameters.IPV4Family,
				netmlbparameters.IBGPASN,
				netmlbparameters.ExternalNADName,
				netparameters.MasterConfigMapName,
				netmlbparameters.PropagateFalse)

			By("should create a BGP Peer on Speakers")

			workerNodesAdresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)

			err := netmetallbhelper.CreateSpeakerBGPPeerIPStack(netparameters.IPV4Family,
				ipv4metalLBIPList[0], "", netmlbparameters.IBGPASN, netmlbparameters.BGPPeerName1v4)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				return netmetallbhelper.CheckNeighborsStatus(masterNodeFRRPod, workerNodesAdresses)
			}, 1*time.Minute, netmlbparameters.Interval).Should(BeTrue())

			By("should verify default BGP Peer timers")
			frrk8sPods, err := helper.Apiclient.Pods(netmlbparameters.MetalLBOperatorNameSpace).
				List(context.Background(), metav1.ListOptions{
					LabelSelector: netmlbparameters.FRRK8SLabelSelector,
				})
			Expect(err).ToNot(HaveOccurred())

			defaultTimerSettings := []int{netmlbparameters.BGPDefaultHoldTimer, netmlbparameters.BGPDefaultKeepAliveTimer}
			Eventually(func() error {
				return netmetallbhelper.ValidateBGPTimers(frrk8sPods.Items, defaultTimerSettings)
			}, 1*time.Minute, netmlbparameters.Interval).Should(Not(HaveOccurred()))

			By("should update default BGP Peer timers")
			updatedTimerSettings := []int{netmlbparameters.BGPUpdatedHoldTimer, netmlbparameters.BGPUpdatedKeepAliveTimer}

			err = netmetallbhelper.UpdateBGPPeerTimers()
			Expect(err).ToNot(HaveOccurred())

			err = netmetallbhelper.ResetBGPPeer(masterNodeFRRPod)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() error {
				return netmetallbhelper.ValidateBGPTimers(frrk8sPods.Items, updatedTimerSettings)
			}, 1*time.Minute, netmlbparameters.Interval).Should(Not(HaveOccurred()))

		})
	})

	Context("Log Level Feature", func() {
		BeforeEach(func() {
			By("should create external FRR container")

			masterNodeFRRPod := netmetallbhelper.CreateFRRContainerOnMaster(
				workerNodeList,
				masterNodeList[0],
				ipv4metalLBIPList[0],
				"",
				netparameters.IPV4Family,
				netmlbparameters.IBGPASN,
				netmlbparameters.ExternalNADName,
				netparameters.MasterConfigMapName,
				netmlbparameters.PropagateFalse)

			By("should create a BGP Peer on Speakers")

			workerNodesAdresses := nethelper.NodeIPsForFamily(workerNodeList, netparameters.IPV4Family)

			err := netmetallbhelper.CreateSpeakerBGPPeerIPStack(netparameters.IPV4Family,
				ipv4metalLBIPList[0], "", netmlbparameters.IBGPASN, netmlbparameters.BGPPeerName1v4)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				return netmetallbhelper.CheckNeighborsStatus(masterNodeFRRPod, workerNodesAdresses)
			}, 1*time.Minute, netmlbparameters.Interval).Should(BeTrue())
		})

		// 49810
		It("Verify FRR Speaker default Informational logs", polarion.ID("49810"), func() {
			By("should be validate default log level informational")
			err = netmetallbhelper.ValidateLogLevel(netmlbparameters.LogLevelInfo)
			Expect(err).ToNot(HaveOccurred())
		})

		// 49812
		It("Verify FRR Speaker debugging logs", polarion.ID("49812"), func() {

			By("should enable debug level logs on Speaker FRR containers")
			err = netmetallbhelper.SetLogLevel(metallboperatorv1beta1.LogLevelDebug)
			Expect(err).ToNot(HaveOccurred())

			By("should wait for Metallb speakers to stablize after update")
			helper.WaitForAllPodsHealthy([]string{netmlbparameters.MetalLBOperatorNameSpace},
				2*time.Minute,
				netmlbparameters.UpdateIntervalMetallb,
				netmlbparameters.WorkloadStableDuration)

			By("checking MetalLB daemonset is in running state after update")

			Eventually(func() error {
				return helper.IsDaemonsetReady(helper.Apiclient,
					netmlbparameters.MetalLBOperatorNameSpace, netmlbparameters.MetalLBDaemonsetName)
			}, 2*time.Minute, netmlbparameters.UpdateIntervalMetallb).ShouldNot(HaveOccurred())

			By("checking MetalLB log level is updated to debugging")

			Eventually(func() error {
				return netmetallbhelper.ValidateLogLevel(netmlbparameters.LogLevelDebug)
			}, 2*time.Minute, netmlbparameters.UpdateIntervalMetallb).ShouldNot(HaveOccurred(),
				"Log level is not configured as debug")
		})
	})
})
