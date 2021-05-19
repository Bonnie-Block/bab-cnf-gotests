package tests

import (
	"context"
	"fmt"
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/parameters"
	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	generalParam "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	utilNode "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
	"strings"
	"time"
)
var (
	hostnameLabel = "kubernetes.io/hostname"
	sriovWaitingTime                time.Duration = 35 * time.Minute
)


var _ = Describe("Discovery mode with all ", func() {

	config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())
	execute.BeforeAll(func() {

		By("Validate env vars")
		myEnv, err := helper.NewConfig()
		Expect(err).ToNot(HaveOccurred())

		By("Pull cnf-test images")
		err = helper.PullImage(myEnv.TestImageRegistry, myEnv.CnfTestImage)
		Expect(err).ToNot(HaveOccurred())

		By("Pull dpdk-based images")
		err = helper.PullImage(myEnv.TestImageRegistry, myEnv.DpdkTestImage)
		Expect(err).ToNot(HaveOccurred())

		By("Validate load-sctp-module Machine Config Installed")
		mcList, err := Apiclient.MachineConfigs().List(context.TODO(), metav1.ListOptions{})
		Expect(err).ToNot(HaveOccurred())

		mcSCTPReady:=false
		for _, mc := range mcList.Items {
			if mc.Name == "load-sctp-module" {
				mcSCTPReady=true
			}
		}

		if !mcSCTPReady {
			By("Deploy load-sctp-module machine-config")
			err := helper.DeploySCTPMc(
				Apiclient,
				strings.Split(config.General.CnfNodeLabel,"/")[1])
			Expect(err).ToNot(HaveOccurred())
			err = helper.WaitForClusterToBeStable(
				Apiclient,
				strings.Split(config.General.CnfNodeLabel,"/")[1])
		}

		By("Validate sctp kernel module is Loaded")
		checkForSctpReady(
			Apiclient,
			config.General.CnfNodeLabel,
			config.Network.TestContainerImage)

		By("Configure PTP")
		ptpNodes, err := PtpEnabled(Apiclient, generalParam.PtpOperatorNamespace)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(ptpNodes)).To(
			BeNumerically(">", 1),
			"need at least two nodes with ptp capable nics")

		By("Labeling the grandmaster node")
		ptpGrandMasterNode := ptpNodes[0]
		ptpGrandMasterNode.NodeObject, err = nodes.LabelNode(
			Apiclient,
			ptpGrandMasterNode.NodeName,
			parameters.DiscoveryPtpGrandmasterNodeLabel,
			"")
		Expect(err).ToNot(HaveOccurred())


		By("Labeling the slave node")
		ptpSlaveNode := ptpNodes[1]
		ptpSlaveNode.NodeObject, err = nodes.LabelNode(
			Apiclient,
			ptpSlaveNode.NodeName,
			parameters.DiscoveryPtpSlaveNodeLabel,
			"")
		Expect(err).ToNot(HaveOccurred())

		By("Creating the policy for the grandmaster node")

		var validPtpInterfaces []string
		Eventually(func() error {
			validPtpInterfaces, err = GetPtpInterfaces(
				config,
				Apiclient,
				1,
				generalParam.SriovOperatorNamespace)
			return err
		}, 5*time.Minute, 2*time.Second).ShouldNot(
			HaveOccurred(),
			"Error to collect ptp supported interfaces")
		Expect(len(validPtpInterfaces)).To(
			Equal(1),
			"Expect 2 ptp supported interfaces")


		err = helper.CreatePTPConfig(
			Apiclient,
			parameters.DiscoveryPtpGrandmasterProfile,
			generalParam.PtpOperatorNamespace,
			validPtpInterfaces[0],
			"-2",
			"-a -r -r",
			parameters.DiscoveryPtpGrandmasterNodeLabel,
			pointer.Int64Ptr(5))
		Expect(err).ToNot(HaveOccurred())

		By("Creating the policy for the worker node")
		err = helper.CreatePTPConfig(
			Apiclient,
			parameters.DiscoveryPtpWorkerProfile,
			generalParam.PtpOperatorNamespace,
			validPtpInterfaces[0],
			"-s -2",
			"-a -r",
			parameters.DiscoveryPtpSlaveNodeLabel,
			pointer.Int64Ptr(5))
		Expect(err).ToNot(HaveOccurred())

		By("Restart the linuxptp-daemon pods")
		ptpPods, err := Apiclient.Pods(generalParam.PtpOperatorNamespace).List(
			context.Background(),
			metav1.ListOptions{LabelSelector: "app=linuxptp-daemon"})
		Expect(err).ToNot(HaveOccurred())
		for _, pod := range ptpPods.Items {
			err = Apiclient.Pods(generalParam.PtpOperatorNamespace).Delete(
				context.Background(),
				pod.Name,
				metav1.DeleteOptions{GracePeriodSeconds: pointer.Int64Ptr(0)})
			Expect(err).ToNot(HaveOccurred())
		}
		daemonset, err := Apiclient.DaemonSets(generalParam.PtpOperatorNamespace).Get(
			context.Background(),
			generalParam.PtpDaemonsetName,
			metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		expectedNumber := daemonset.Status.DesiredNumberScheduled
		Eventually(func() int32 {
			daemonset, err = Apiclient.DaemonSets(generalParam.PtpOperatorNamespace).Get(
				context.Background(),
				generalParam.PtpDaemonsetName,
				metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			return daemonset.Status.NumberReady
		}, 2*time.Minute, 2*time.Second).Should(Equal(expectedNumber))

		Eventually(func() int {
			ptpPods, err := Apiclient.Pods(generalParam.PtpOperatorNamespace).List(
				context.Background(),
				metav1.ListOptions{LabelSelector: "app=linuxptp-daemon"})
			Expect(err).ToNot(HaveOccurred())
			return len(ptpPods.Items)
		}, 2*time.Minute, 2*time.Second).Should(Equal(int(expectedNumber)))

		err = wait.PollImmediate(
			1*time.Second,
			600*time.Second,
			func() (done bool, err error) {

			ptpPods, err := Apiclient.Pods(generalParam.PtpOperatorNamespace).List(
				context.Background(),
				metav1.ListOptions{
				LabelSelector: "app=linuxptp-daemon",
				FieldSelector: fmt.Sprintf(
					"spec.nodeName=%s",
					ptpSlaveNode.NodeName)})
			Expect(err).ToNot(HaveOccurred())
			Expect(len(ptpPods.Items)).To(Equal(1))
			logs, err := pod.GetLog(
				Apiclient,
				&ptpPods.Items[0],
				2*time.Second,
				generalParam.PtpContainerName)
			Expect(err).ToNot(HaveOccurred())
			if strings.Contains(logs, "new foreign master") {
				fmt.Printf(
					"Found valid PTP Configuration, using %s for master and slave\n",
					validPtpInterfaces[0])
				return true, nil
			}
			return false, nil
		})
		Expect(err).ToNot(HaveOccurred(), "Did not found valid PTP Configuration")

		By("Create test namespace")
		err = namespaces.Create(parameters.TestNamespace, Apiclient)
		Expect(err).ToNot(HaveOccurred())

		By("Discover SRIOV interfaces")
		sriovInfos, err := cluster.DiscoverSriov(Apiclient, generalParam.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred())

		sriovInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
		Expect(err).ToNot(HaveOccurred())

		validSriovInterfaces, err := config.GetSriovInterfaces(sriovInterfaces, 1)
		Expect(err).ToNot(HaveOccurred())

		By("Create SRIOV Policy")
		discoverySriovPolicy := DefineSriovPolicy(
			"discovery-policy",
			generalParam.SriovOperatorNamespace,
			validSriovInterfaces[0],
			5,
			"#0-4",
			1500,
			"sriovnic",
			"netdevice")
		err = Apiclient.Create(context.Background(), discoverySriovPolicy)
		Expect(err).ToNot(HaveOccurred())
		By("Waiting until SRIOV become stable")
		WaitForSRIOVStable(Apiclient, generalParam.SriovOperatorNamespace, sriovWaitingTime)

		By("Waiting until SRIOV resources become available")
		ValidateSriovVFsAvailableOnNodes(
			Apiclient,
			sriovInfos.Nodes,
			[]*sriovv1.SriovNetworkNodePolicy{discoverySriovPolicy},
			5)

		By("Create Performance Profile")
		err = helper.CreatePerformanceProfile(
			Apiclient,
			parameters.DiscoveryPerformanceProfileName,
			config.General.CnfNodeLabel)
		Expect(err).ToNot(HaveOccurred())
		err = helper.WaitForClusterToBeStable(
			Apiclient,
			strings.Split(config.General.CnfNodeLabel,"/")[1])
		Expect(err).ToNot(HaveOccurred())
	})



	It("features configured", func() {
	})

	It("features configured(Except sriov)", func(){
	})

	It("features configured(Except for sriov and ptp)", func(){
	})

	It("features configured(Except for sriov, ptp and performance)", func(){
	})
})

func checkForSctpReady(cs *client.ClientSet, sctpNodeSelector string, image string) {
	nodes, err := cs.Nodes().List(context.Background(), metav1.ListOptions{
		LabelSelector: sctpNodeSelector,
	})
	Expect(err).ToNot(HaveOccurred())

	filtered, err := utilNode.MatchingOptionalSelector(Apiclient, nodes.Items)
	Expect(err).ToNot(HaveOccurred())
	Expect(len(filtered)).To(BeNumerically(">", 0))

	args := []string{`set -x; x="$(checksctp 2>&1)"; echo "$x" ; if [ "$x" = "SCTP supported" ]; then echo "succeeded"; exit 0; else echo "failed"; exit 1; fi`}
	for _, n := range filtered {
		job := jobForNode("testsctp-check", n.ObjectMeta.Labels[hostnameLabel], "checksctp", []string{"/bin/bash", "-c"}, args, image)
		cs.Pods("default").Create(context.Background(), job, metav1.CreateOptions{})
	}

	Eventually(func() bool {
		pods, err := cs.Pods("default").List(context.Background(), metav1.ListOptions{LabelSelector: "app=checksctp"})
		ExpectWithOffset(1, err).ToNot(HaveOccurred())

		for _, p := range pods.Items {
			if p.Status.Phase != k8sv1.PodSucceeded {
				return false
			}
		}
		return true
	}, 10*time.Minute, 10*time.Second).Should(Equal(true))
}

func jobForNode(name, node, app string, cmd []string, args []string, image string) *k8sv1.Pod {
	job := k8sv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name,
			Labels: map[string]string{
				"app": app,
			},
			Namespace: "default",
		},
		Spec: k8sv1.PodSpec{
			RestartPolicy: k8sv1.RestartPolicyNever,
			Containers: []k8sv1.Container{
				{
					Name:    name,
					Image:   image,
					Command: cmd,
					Args:    args,
				},
			},
			NodeSelector: map[string]string{
				hostnameLabel: node,
			},
		},
	}

	return &job
}
