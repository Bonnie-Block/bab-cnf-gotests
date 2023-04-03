package tests

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/helper/container"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/discovery/parameters"
	. "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	generalParam "gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
)

var _ = Describe("Discovery mode with all ", func() {
	var (
		cnfTestEnv               *parameters.EnvironmentConfig
		containerEngine          *exec.Cmd
		machineConfigPoolName    string
		discoverySriovPolicyList []*sriovv1.SriovNetworkNodePolicy
		isSingleNode             bool
		snoTimeoutMultiplier     time.Duration = 1
		testFail                               = ""
	)

	execute.BeforeAll(func() {
		var err error
		By("Validate env vars")
		cnfTestEnv, err = helper.NewConfig()
		if err != nil {
			testFail = fmt.Sprintf("Error to collect cnfTestEnv: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
		isSingleNode, err = nodes.IsSingleNodeCluster(Apiclient)
		if err != nil {
			testFail = fmt.Sprintf("Error to check if the cluster is single node: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
		if isSingleNode {
			snoTimeoutMultiplier = 2
			disableDrainState := GetNodeDrainState(generalParam.SriovOperatorNamespace)
			if !disableDrainState {
				SetDisableNodeDrainState(true, generalParam.SriovOperatorNamespace)
				ChangedNodeDrainState = true
			}
		}
		containerEngine, err = container.SelectEngine()
		if err != nil {
			testFail = fmt.Sprintf("Error to determine container engine: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
		machineConfigPoolName = strings.Split(Config.General.CnfNodeLabel, "/")[1]
		By("Pull cnf-test images")
		err = container.PullImage(cnfTestEnv.TestImageRegistry, cnfTestEnv.CnfTestImage)
		if err != nil {
			testFail = fmt.Sprintf("Error pulling cnf-tests image: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}

		By("Pull dpdk-based images")
		err = container.PullImage(cnfTestEnv.TestImageRegistry, cnfTestEnv.DpdkTestImage)
		if err != nil {
			testFail = fmt.Sprintf("Error pulling dpdk-base image: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}

		By("Validate Machine Configs Installed")
		mcList, err := Apiclient.MachineConfigs().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			testFail = fmt.Sprintf("Error to collect machine config list: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
		mcXU32Ready, mcSCTPReady, mcQOSEgressReady, mcQOSIngressReady := false, false, false, false
		for _, machineCfg := range mcList.Items {
			if machineCfg.Name == "load-sctp-module" {
				mcSCTPReady = true
			}
			if machineCfg.Name == "load-xt-u32-module" {
				mcXU32Ready = true
			}
			if machineCfg.Name == parameters.DiscoveryOVSQOSEgressMCName {
				mcQOSEgressReady = true
			}
			if machineCfg.Name == parameters.DiscoveryOVSQOSIngressMCName {
				mcQOSIngressReady = true
			}
		}
		if !mcSCTPReady {
			By("Deploy load-sctp-module machine-config")
			err = helper.DeployMC(helper.DefineSCTPMC(machineConfigPoolName))
			if err != nil {
				testFail = fmt.Sprintf("Error to deploy sctp MachineConfig: %s", err)
				Expect(err).ToNot(HaveOccurred(), testFail)
			}

			err = WaitForClusterToBeStable(machineConfigPoolName, snoTimeoutMultiplier)
			if err != nil {
				testFail = fmt.Sprintf("Error in wait for cluster to be stable: %s", err)
				Expect(err).ToNot(HaveOccurred(), testFail)
			}
		}

		if !mcXU32Ready {
			By("Deploy load-xt-u32-module machine-config")
			err = helper.DeployMC(helper.DefineXtu32MC(machineConfigPoolName))
			if err != nil {
				testFail = fmt.Sprintf("Error to deploy xt_u32 MachineConfig: %s", err)
				Expect(err).ToNot(HaveOccurred(), testFail)
			}
			err = WaitForClusterToBeStable(machineConfigPoolName, snoTimeoutMultiplier)
			if err != nil {
				testFail = fmt.Sprintf("Error in wait for cluster to be stable: %s", err)
				Expect(err).ToNot(HaveOccurred(), testFail)
			}
		}

		if !mcQOSEgressReady && !isSingleNode {
			By("Deploy egress-limit machine-config")
			err = helper.DeployMC(helper.DefineQOSEgressMC(machineConfigPoolName))
			if err != nil {
				testFail = fmt.Sprintf("Error to deploy egress-limit MachineConfig: %s", err)
				Expect(err).ToNot(HaveOccurred(), testFail)
			}
			err = WaitForClusterToBeStable(machineConfigPoolName, snoTimeoutMultiplier)
			if err != nil {
				testFail = fmt.Sprintf("Error in wait for cluster to be stable: %s", err)
				Expect(err).ToNot(HaveOccurred(), testFail)
			}
		}

		if !mcQOSIngressReady && !isSingleNode {
			By("Deploy ingress-limit machine-config")
			err = helper.DeployMC(helper.DefineQOSIngressMC(machineConfigPoolName))
			if err != nil {
				testFail = fmt.Sprintf("Error to deploy ingress-limit MachineConfig: %s", err)
				Expect(err).ToNot(HaveOccurred(), testFail)
			}
			err = WaitForClusterToBeStable(machineConfigPoolName, snoTimeoutMultiplier)
			if err != nil {
				testFail = fmt.Sprintf("Error in wait for cluster to be stable: %s", err)
				Expect(err).ToNot(HaveOccurred(), testFail)
			}
		}

		By("Validate sctp kernel module is Loaded")
		checkForSctpReady(
			Apiclient,
			Config.General.CnfNodeLabel,
			Config.Network.TestContainerImage)

		if !isSingleNode {
			By("Setup PTP discovery mode policy")
			definePTPDiscoveryModePolicy(Config)
		} else {
			By("Skip discovery mode policy configuration on SNO")
		}

		By("Discover SRIOV interfaces")
		sriovInfos, err := cluster.DiscoverSriov(Apiclient, generalParam.SriovOperatorNamespace)
		if err != nil {
			testFail = fmt.Sprintf("Error discover SRIOV node info: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
		sriovInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
		if err != nil {
			testFail = fmt.Sprintf("Error discover SRIOV interfaces: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
		validSriovInterfaces, err := Config.GetSriovInterfaces(sriovInterfaces, 1)
		if err != nil {
			testFail = fmt.Sprintf("Error determine SRIOV interfaces: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}

		By("Create SRIOV Policy")
		discoverySriovPolicyList = helper.DefineDiscoverySriovPolicyList(validSriovInterfaces[0])
		for _, networkPolicy := range discoverySriovPolicyList {
			err = Apiclient.Create(context.Background(), networkPolicy)
			if err != nil {
				testFail = fmt.Sprintf("Error Create SRIOV policy: %s", err)
				Expect(err).ToNot(HaveOccurred(), testFail)
			}
		}
		By("Waiting until SRIOV become stable")
		WaitForSRIOVStable(generalParam.SriovOperatorNamespace, parameters.SriovWaitingTime, snoTimeoutMultiplier)

		By("Waiting until SRIOV resources become available")
		ValidateSriovVFsAvailableOnNodes(
			sriovInfos.Nodes,
			discoverySriovPolicyList,
			5)

		By("Create Performance Profile")
		err = CreatePerformanceProfile(
			parameters.DiscoveryPerformanceProfile,
			Config.General.CnfNodeLabel)
		if err != nil {
			testFail = fmt.Sprintf("Error Create PerformanceProfile policy: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}

		err = WaitForClusterToBeStable(machineConfigPoolName, snoTimeoutMultiplier)
		if err != nil {
			testFail = fmt.Sprintf("Error waiting for cluster to be stable: %s", err)
			Expect(err).ToNot(HaveOccurred(), testFail)
		}
	})

	BeforeEach(func() {
		if testFail != "" {
			Fail(testFail)
		}
		By("Remove all existing cnftests  reports")
		removeAllFromDir(Config.General.ReportDirAbsPath)
	})

	It("features configured", func() {
		// Skip test due to the Intel bug in discovery mode
		if discoverySriovPolicyList[0].Spec.DeviceType == "vfio-pci" {
			Skip("Skip test on top of intel card due to the BZ: https://bugzilla.redhat.com/show_bug.cgi?id=1971274")
		}
		runCNFTests(Config, cnfTestEnv, containerEngine)
		reportIsValid(
			Config.General.ReportDirAbsPath,
			definePassedSkipTestsNumber(parameters.DiscoveryAllFeaturesScenario, isSingleNode),
		)
	})

	It("features configured(Except for SriovNetworkNodePolicy)", func() {
		By("Remove Sriov Policy")
		err := helper.CleanAllSriovPolicy(snoTimeoutMultiplier)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error removing all sriov policy: %s", err))
		runCNFTests(Config, cnfTestEnv, containerEngine)
		reportIsValid(
			Config.General.ReportDirAbsPath,
			definePassedSkipTestsNumber(parameters.DiscoveryExceptSriovScenario, isSingleNode),
		)
	})

	It("features configured(Except for SriovNetworkNodePolicy and Ptpconfig)", func() {
		if isSingleNode {
			Skip("PTP is not supported on Single node cluster")
		}
		By("Remove PTP policy")
		err := CleanAllPtpConfig(
			generalParam.PtpOperatorNamespace,
			parameters.DiscoveryPtpGrandmasterNodeLabel,
			parameters.DiscoveryPtpSlaveNodeLabel)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error removing all ptp configuration: %s", err))
		runCNFTests(Config, cnfTestEnv, containerEngine)
		reportIsValid(
			Config.General.ReportDirAbsPath,
			definePassedSkipTestsNumber(parameters.DiscoveryExceptSriovPtpScenario, isSingleNode),
		)
	})

	It("features configured(Except for SriovNetworkNodePolicy, Ptpconfig and PerformanceProfile)", func() {
		By("Remove Performance policy")
		err := CleanAllPerformanceProfile(machineConfigPoolName, snoTimeoutMultiplier)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error removing all Performance profiles: %s", err))
		runCNFTests(Config, cnfTestEnv, containerEngine)
		reportIsValid(Config.General.ReportDirAbsPath, definePassedSkipTestsNumber(
			parameters.DiscoveryExceptSriovPtpPerformanceScenario,
			isSingleNode,
		))
	})

	It("features configured(Except for SriovNetworkNodePolicy, Ptpconfig, PerformanceProfile and ovs_qos)", func() {
		if isSingleNode {
			Skip("OVS_QOS is not supported on Single node cluster")
		}
		By("Remove egress and ingress ovs_qos MCs")
		err := helper.DeleteOVSQOSMCs(machineConfigPoolName)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error removing all OVS_QOS MCs: %s", err))
		runCNFTests(Config, cnfTestEnv, containerEngine)
		reportIsValid(Config.General.ReportDirAbsPath, definePassedSkipTestsNumber(
			parameters.DiscoveryExceptSriovPtpPerformanceOVSQOSScenario,
			isSingleNode,
		))
	})
})

func checkForSctpReady(clientSet *client.ClientSet, sctpNodeSelector string, image string) {
	nodeList, err := clientSet.Nodes().List(context.Background(), metav1.ListOptions{
		LabelSelector: sctpNodeSelector,
	})
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Error collecting nodes based on label: %s\nError: %s", sctpNodeSelector, err))

	filtered, err := nodes.MatchingOptionalSelector(Apiclient, nodeList.Items)
	Expect(err).ToNot(
		HaveOccurred(),
		fmt.Sprintf("Error filtering nodes %s", err))
	Expect(len(filtered)).To(
		BeNumerically(">", 0),
		"Expect at least 1 ready nodes")

	args := []string{
		`set -x; x="$(checksctp 2>&1)"; echo "$x" ; if [ "$x" = "SCTP supported" ]; then echo "succeeded"; exit 0; else echo "failed"; exit 1; fi`}
	for _, n := range filtered {
		job := jobForNode(
			"testsctp-check",
			n.ObjectMeta.Labels[generalParam.LabelHostname],
			"checksctp",
			[]string{"/bin/bash", "-c"},
			args,
			image)
		_, err := clientSet.Pods("default").Create(context.Background(), job, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
	}

	Eventually(func() bool {
		pods, err := clientSet.Pods("default").List(
			context.Background(),
			metav1.ListOptions{LabelSelector: "app=checksctp"})
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
				generalParam.LabelHostname: node,
			},
		},
	}

	return &job
}

func runCNFTests(
	config *config.Config,
	cnfTestEnv *parameters.EnvironmentConfig,
	containerEngine *exec.Cmd) {
	kubeconfigFile := os.Getenv("KUBECONFIG")
	Expect(len(kubeconfigFile)).ToNot(
		Equal(0),
		"KUBECONFIG var is empty. Please set KUBECONFIG")

	_, err := os.Stat(kubeconfigFile)
	Expect(err).ToNot(HaveOccurred(), "KUBECONFIG file doesn't exists")
	// "TODO Add Gatekeeper as soon as BZ(2015836) is fixed"
	cnfTest := exec.Command(
		containerEngine.Path, "run", "-v", fmt.Sprintf(
			"%s:/kubefiles:Z", filepath.Dir(kubeconfigFile)),
		"-v", fmt.Sprintf(
			"%s:/%s:Z", config.General.ReportDirAbsPath, filepath.Base(config.General.ReportDirAbsPath)),
		"-e", "DISCOVERY_MODE=true",
		"-e", "KUBECONFIG=/kubefiles/kubeconfig",
		"-e", fmt.Sprintf("ROLE_WORKER_CNF=%s", strings.Split(config.General.CnfNodeLabel, "/")[1]),
		"-e", "IS_OPENSHIFT=true",
		"-e", fmt.Sprintf("IMAGE_REGISTRY=%s", cnfTestEnv.TestImageRegistry),
		"-e", fmt.Sprintf("CNF_TESTS_IMAGE=%s", cnfTestEnv.CnfTestImage),
		"-e", fmt.Sprintf("DPDK_TESTS_IMAGE=%s", cnfTestEnv.DpdkTestImage),
		fmt.Sprintf("%s/%s", cnfTestEnv.TestImageRegistry, cnfTestEnv.CnfTestImage),
		"/usr/bin/test-run.sh", "-ginkgo.focus=sriov|sctp|dpdk|performance|ptp|vrf|xt_u32|ovs_qos|metallb",
		fmt.Sprintf("--report=/%s", filepath.Base(config.General.ReportDirAbsPath)),
		fmt.Sprintf("--junit=/%s", filepath.Base(config.General.ReportDirAbsPath)))
	cnfTest.Stdout = os.Stdout
	cnfTest.Stderr = os.Stderr

	By("Start discovery mode testing")

	err = cnfTest.Run()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error running cnfTests: %s", err))
}

func reportIsValid(reportPath string, expectedTestNumbers []int) {
	passedTestNumber := expectedTestNumbers[0]
	skippedTestNumber := expectedTestNumbers[1]
	data, err := os.Open(path.Join(reportPath, parameters.JUnitCNFTestsReportName))
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error opening cnf-tests report file: %s", err))
	byteValue, err := io.ReadAll(data)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error reading cnf-tests report file: %s", err))

	var report parameters.Report
	err = xml.Unmarshal(byteValue, &report)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error unmarshaling cnf-tests xml: %s", err))
	Expect(report.Failures).To(BeZero(), fmt.Sprintf("Error Failures test count more then 0: %s", err))
	Expect(report.Errors).To(BeZero(), fmt.Sprintf("Error Errors test count more then 0: %s", err))
	Expect(report.Tests).To(
		BeEquivalentTo(passedTestNumber),
		"Invalid passed test number")

	skipCount := 0

	for _, tc := range report.Results {
		if tc.Skipped != nil {
			skipCount++
		}
	}

	Expect(skipCount).To(BeEquivalentTo(
		skippedTestNumber),
		"Invalid Skip test number")
}

func removeAllFromDir(dir string) {
	openDir, err := os.Open(dir)
	Expect(err).ToNot(HaveOccurred())

	defer openDir.Close()
	names, err := openDir.Readdirnames(-1)
	Expect(err).ToNot(HaveOccurred())

	for _, name := range names {
		err = os.RemoveAll(filepath.Join(dir, name))
		Expect(err).ToNot(HaveOccurred())
	}
}

func definePTPDiscoveryModePolicy(config *config.Config) {
	ptpNodes, err := PtpEnabled(generalParam.PtpOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to determine ptp enable nodes: %s", err))
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
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to label grandmaster ptp node: %s", err))

	By("Labeling the slave node")

	ptpSlaveNode := ptpNodes[1]
	ptpSlaveNode.NodeObject, err = nodes.LabelNode(
		Apiclient,
		ptpSlaveNode.NodeName,
		parameters.DiscoveryPtpSlaveNodeLabel,
		"")
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to label slave ptp node: %s", err))

	By("Creating the policy for the grandmaster node")

	var validPtpInterfaces []string

	Eventually(func() error {
		validPtpInterfaces, err = GetPtpInterfaces(
			config,
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
		parameters.DiscoveryPtpGrandmasterProfile,
		generalParam.PtpOperatorNamespace,
		validPtpInterfaces[0],
		"-2",
		"-a -r -r",
		parameters.DiscoveryPtpGrandmasterNodeLabel,
		pointer.Int64(5))
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to create PtpConfig grandmaster: %s", err))

	By("Creating the policy for the worker node")

	err = helper.CreatePTPConfig(
		parameters.DiscoveryPtpWorkerProfile,
		generalParam.PtpOperatorNamespace,
		validPtpInterfaces[0],
		"-s -2",
		"-a -r",
		parameters.DiscoveryPtpSlaveNodeLabel,
		pointer.Int64(5))
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to create PtpConfig slave: %s", err))

	By("Restart the linuxptp-daemon pods")

	ptpPods, err := Apiclient.Pods(generalParam.PtpOperatorNamespace).List(
		context.Background(),
		metav1.ListOptions{LabelSelector: generalParam.PtpDaemonsetLabelSelector})
	Expect(err).ToNot(HaveOccurred())

	for _, pod := range ptpPods.Items {
		err = Apiclient.Pods(generalParam.PtpOperatorNamespace).Delete(
			context.Background(),
			pod.Name,
			metav1.DeleteOptions{GracePeriodSeconds: pointer.Int64(0)})
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to remove ptp pod: %s, %s", pod.Name, err))
	}

	daemonset, err := Apiclient.DaemonSets(generalParam.PtpOperatorNamespace).Get(
		context.Background(),
		generalParam.PtpDaemonsetName,
		metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to collect ptp daemonSet info: %s", err))

	expectedNumber := daemonset.Status.DesiredNumberScheduled
	Eventually(func() int32 {
		daemonset, err = Apiclient.DaemonSets(generalParam.PtpOperatorNamespace).Get(
			context.Background(),
			generalParam.PtpDaemonsetName,
			metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())

		return daemonset.Status.NumberReady
	}, 4*time.Minute, 2*time.Second).Should(
		Equal(expectedNumber),
		fmt.Sprintf("Waiting interval expired during ptp daemonSet Eventually loop : %s", err))

	Eventually(func() int {
		ptpPods, err := Apiclient.Pods(generalParam.PtpOperatorNamespace).List(
			context.Background(),
			metav1.ListOptions{LabelSelector: generalParam.PtpDaemonsetLabelSelector})
		Expect(err).ToNot(HaveOccurred())

		return len(ptpPods.Items)
	}, 2*time.Minute, 2*time.Second).Should(
		Equal(int(expectedNumber)),
		fmt.Sprintf("Waiting interval expired trying to check that all ptp pods are running: %s", err))

	err = wait.PollImmediate(
		1*time.Second,
		600*time.Second,
		func() (done bool, err error) {
			ptpPods, err := Apiclient.Pods(generalParam.PtpOperatorNamespace).List(
				context.Background(),
				metav1.ListOptions{
					LabelSelector: generalParam.PtpDaemonsetLabelSelector,
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
}

func definePassedSkipTestsNumber(scenario string, isSingleNode bool) []int {
	var (
		passedTestNumber  int
		skippedTestNumber int
	)

	switch scenario {
	case parameters.DiscoveryAllFeaturesScenario:
		if isSingleNode {
			passedTestNumber = parameters.SNODiscoveryAllFeaturesPassedTest
			skippedTestNumber = parameters.SNODiscoveryAllFeaturesSkippedTest
		} else {
			passedTestNumber = parameters.DiscoveryAllFeaturesPassedTest
			skippedTestNumber = parameters.DiscoveryAllFeaturesSkippedTest
		}
	case parameters.DiscoveryExceptSriovScenario:
		if isSingleNode {
			passedTestNumber = parameters.SNODiscoveryExceptSriovPassedTest
			skippedTestNumber = parameters.SNODiscoveryExceptSriovSkippedTest
		} else {
			passedTestNumber = parameters.DiscoveryExceptSriovPassedTest
			skippedTestNumber = parameters.DiscoveryExceptSriovSkippedTest
		}
	case parameters.DiscoveryExceptSriovPtpScenario:
		passedTestNumber = parameters.DiscoveryExceptSriovPtpPassedTest
		skippedTestNumber = parameters.DiscoveryExceptSriovPtpSkippedTest
	case parameters.DiscoveryExceptSriovPtpPerformanceScenario:
		if isSingleNode {
			passedTestNumber = parameters.SNODiscoveryExceptSriovPerformancePassedTest
			skippedTestNumber = parameters.SNODiscoveryExceptSriovPerformanceSkippedTest
		} else {
			passedTestNumber = parameters.DiscoveryExceptSriovPtpPerformancePassedTest
			skippedTestNumber = parameters.DiscoveryExceptSriovPtpPerformancetSkippedTest
		}
	case parameters.DiscoveryExceptSriovPtpPerformanceOVSQOSScenario:
		passedTestNumber = parameters.DiscoveryExceptSriovPtpOVSQOSPerformancePassedTest
		skippedTestNumber = parameters.DiscoveryExceptSriovPtpOVSQOSPerformanceSkippedTest

	default:
		Fail(fmt.Sprintf("Unknown scenario %s", scenario))
	}

	return []int{passedTestNumber, skippedTestNumber}
}
