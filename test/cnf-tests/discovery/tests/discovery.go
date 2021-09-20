package tests

import (
	"context"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	. "github.com/onsi/ginkgo"
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
	utilNode "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
)

var _ = Describe("Discovery mode with all ", func() {
	config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())
	var (
		cnfTestEnv               *parameters.EnvironmentConfig
		containerEngine          *exec.Cmd
		machineConfigPoolName    string
		discoverySriovPolicyList []*sriovv1.SriovNetworkNodePolicy
		isSingleNode             bool
		snoTimeoutMultiplier     time.Duration = 1
	)

	execute.BeforeAll(func() {
		By("Validate env vars")
		cnfTestEnv, err = helper.NewConfig()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to collect cnfTestEnv: %s", err))
		isSingleNode, err = nodes.IsSingleNodeCluster(Apiclient)
		Expect(err).ToNot(HaveOccurred())
		if isSingleNode {
			snoTimeoutMultiplier = 2
			disableDrainState := GetNodeDrainState(generalParam.SriovOperatorNamespace)
			if !disableDrainState {
				SetDisableNodeDrainState(true, generalParam.SriovOperatorNamespace)
				ChangedNodeDrainState = true
			}
		}
		containerEngine, err = container.SelectEngine()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to determine container engine: %s", err))
		machineConfigPoolName = strings.Split(config.General.CnfNodeLabel, "/")[1]
		By("Pull cnf-test images")
		err = container.PullImage(cnfTestEnv.TestImageRegistry, cnfTestEnv.CnfTestImage)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error pulling cnf-tests image: %s", err))

		By("Pull dpdk-based images")
		err = container.PullImage(cnfTestEnv.TestImageRegistry, cnfTestEnv.DpdkTestImage)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error pulling dpdk-base image: %s", err))

		By("Validate load-sctp-module and load-xt-u32-module Machine Configs Installed")
		mcList, err := Apiclient.MachineConfigs().List(context.TODO(), metav1.ListOptions{})
		Expect(err).ToNot(HaveOccurred(), "Error to collect machine config list: %s", err)
		mcXU32Ready, mcSCTPReady := false, false
		for _, mc := range mcList.Items {
			if mc.Name == "load-sctp-module" {
				mcSCTPReady = true
			}
			if mc.Name == "load-xt-u32-module" {
				mcXU32Ready = true
			}
		}
		if !mcSCTPReady {
			By("Deploy load-sctp-module machine-config")
			err := helper.DeployMC(helper.DefineSCTPMC(machineConfigPoolName))
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to deploy sctp MachineConfig: %s", err))

			err = WaitForClusterToBeStable(machineConfigPoolName, snoTimeoutMultiplier)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error in wait for cluster to be stable: %s", err))
		}

		if !mcXU32Ready {
			By("Deploy load-xt-u32-module machine-config")
			err := helper.DeployMC(helper.DefineXtu32MC(machineConfigPoolName))
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to deploy xt_u32 MachineConfig: %s", err))
			err = WaitForClusterToBeStable(machineConfigPoolName, snoTimeoutMultiplier)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error in wait for cluster to be stable: %s", err))
		}

		By("Validate sctp kernel module is Loaded")
		checkForSctpReady(
			Apiclient,
			config.General.CnfNodeLabel,
			config.Network.TestContainerImage)

		if !isSingleNode {
			By("Setup PTP discovery mode policy")
			definePTPDiscoveryModePolicy(config)
		} else {
			By("Skip discovery mode policy configuration on SNO")
		}

		By("Discover SRIOV interfaces")
		sriovInfos, err := cluster.DiscoverSriov(Apiclient, generalParam.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error discover SRIOV node info: %s", err))
		sriovInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error discover SRIOV interfaces: %s", err))
		validSriovInterfaces, err := config.GetSriovInterfaces(sriovInterfaces, 1)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error determine SRIOV interfaces: %s", err))

		By("Create SRIOV Policy")
		discoverySriovPolicyList = helper.DefineDiscoverySriovPolicyList(validSriovInterfaces[0])
		for _, networkPolicy := range discoverySriovPolicyList {
			err = Apiclient.Create(context.Background(), networkPolicy)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error Create SRIOV policy: %s", err))
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
			config.General.CnfNodeLabel)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error Create PerformanceProfile policy: %s", err))

		err = WaitForClusterToBeStable(machineConfigPoolName, snoTimeoutMultiplier)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error waiting for cluster to be stable: %s", err))

	})

	BeforeEach(func() {
		By("Remove all existing cnftests  reports")
		removeAllFromDir(config.General.ReportDirAbsPath)
	})

	It("features configured", func() {
		// Skip test due to the Intel bug in discovery mode
		if discoverySriovPolicyList[0].Spec.DeviceType == "vfio-pci" {
			Skip("Skip test on top of intel card due to the BZ: https://bugzilla.redhat.com/show_bug.cgi?id=1971274")
		}
		runCNFTests(config, cnfTestEnv, containerEngine)
		reportIsValid(config.General.ReportDirAbsPath, definePassedSkipTestsNumber(parameters.DiscoveryAllFeaturesScenario, isSingleNode))
	})

	It("features configured(Except for SriovNetworkNodePolicy)", func() {
		By("Remove Sriov Policy")
		err := helper.CleanAllSriovPolicy(snoTimeoutMultiplier)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error removing all sriov policy: %s", err))
		runCNFTests(config, cnfTestEnv, containerEngine)
		reportIsValid(config.General.ReportDirAbsPath, definePassedSkipTestsNumber(parameters.DiscoveryExceptSriovScenario, isSingleNode))
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
		runCNFTests(config, cnfTestEnv, containerEngine)
		reportIsValid(config.General.ReportDirAbsPath, definePassedSkipTestsNumber(parameters.DiscoveryExceptSriovPtpScenario, isSingleNode))
	})

	It("features configured(Except for SriovNetworkNodePolicy, Ptpconfig and PerformanceProfile)", func() {
		By("Remove Performance policy")
		err := CleanAllPerformanceProfile(machineConfigPoolName, snoTimeoutMultiplier)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error removing all Performance profiles: %s", err))
		runCNFTests(config, cnfTestEnv, containerEngine)
		reportIsValid(config.General.ReportDirAbsPath, definePassedSkipTestsNumber(parameters.DiscoveryExceptSriovPtpPerformanceScenario, isSingleNode))
	})
})

func checkForSctpReady(cs *client.ClientSet, sctpNodeSelector string, image string) {
	nodeList, err := cs.Nodes().List(context.Background(), metav1.ListOptions{
		LabelSelector: sctpNodeSelector,
	})
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Error collecting nodes based on label: %s\nError: %s", sctpNodeSelector, err))

	filtered, err := utilNode.MatchingOptionalSelector(Apiclient, nodeList.Items)
	Expect(err).ToNot(
		HaveOccurred(),
		fmt.Sprintf("Error filtering nodes %s", err))
	Expect(len(filtered)).To(
		BeNumerically(">", 0),
		fmt.Sprintf("Expect at least 1 ready nodes"))

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
		cs.Pods("default").Create(context.Background(), job, metav1.CreateOptions{})
	}

	Eventually(func() bool {
		pods, err := cs.Pods("default").List(
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
		fmt.Sprintf("KUBECONFIG var is empty. Please set KUBECONFIG"))
	_, err := os.Stat(kubeconfigFile)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("KUBECONFIG file doesn't exists"))
	cnfTest := exec.Command(
		containerEngine.Path, "run", "-v", fmt.Sprintf(
			"%s:/kubefiles", filepath.Dir(kubeconfigFile)),
		"-v", fmt.Sprintf("%s:/%s", config.General.ReportDirAbsPath, filepath.Base(config.General.ReportDirAbsPath)),
		"-e", "DISCOVERY_MODE=true",
		"-e", "KUBECONFIG=/kubefiles/kubeconfig",
		"-e", fmt.Sprintf("ROLE_WORKER_CNF=%s", strings.Split(config.General.CnfNodeLabel, "/")[1]),
		"-e", fmt.Sprintf("IMAGE_REGISTRY=%s", cnfTestEnv.TestImageRegistry),
		"-e", fmt.Sprintf("CNF_TESTS_IMAGE=%s", cnfTestEnv.CnfTestImage),
		"-e", fmt.Sprintf("DPDK_TESTS_IMAGE=%s", cnfTestEnv.DpdkTestImage),
		fmt.Sprintf("%s/%s", cnfTestEnv.TestImageRegistry, cnfTestEnv.CnfTestImage),
		"/usr/bin/test-run.sh", "-ginkgo.focus=sriov|sctp|dpdk|performance|ptp|vrf|xt_u32",
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
	data, err := os.Open(fmt.Sprintf(path.Join(reportPath, parameters.JUnitCNFTestsReportName)))
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error opening cnf-tests report file: %s", err))
	byteValue, err := ioutil.ReadAll(data)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error reading cnf-tests report file: %s", err))
	var report parameters.Report
	err = xml.Unmarshal(byteValue, &report)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error unmarshaling cnf-tests xml: %s", err))
	Expect(report.Failures).To(BeZero(), fmt.Sprintf("Error Failures test count more then 0: %s", err))
	Expect(report.Errors).To(BeZero(), fmt.Sprintf("Error Errors test count more then 0: %s", err))
	Expect(report.Tests).To(
		BeEquivalentTo(passedTestNumber),
		fmt.Sprintf("Invalid passed test number"))
	skipCount := 0
	for _, tc := range report.Results {
		if tc.Skipped != nil {
			skipCount++
		}
	}
	Expect(skipCount).To(BeEquivalentTo(
		skippedTestNumber),
		fmt.Sprintf("Invalid Skip test number"))
}

func removeAllFromDir(dir string) {
	openDir, err := os.Open(dir)
	Expect(err).ToNot(HaveOccurred())
	defer openDir.Close()
	names, err := openDir.Readdirnames(-1)
	Expect(err).ToNot(HaveOccurred())
	for _, name := range names {
		err = os.RemoveAll(filepath.Join(dir, name))
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
		pointer.Int64Ptr(5))
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to create PtpConfig grandmaster: %s", err))

	By("Creating the policy for the worker node")
	err = helper.CreatePTPConfig(
		parameters.DiscoveryPtpWorkerProfile,
		generalParam.PtpOperatorNamespace,
		validPtpInterfaces[0],
		"-s -2",
		"-a -r",
		parameters.DiscoveryPtpSlaveNodeLabel,
		pointer.Int64Ptr(5))
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error to create PtpConfig slave: %s", err))

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
	}, 2*time.Minute, 2*time.Second).Should(
		Equal(expectedNumber),
		fmt.Sprintf("Waiting interval expired during ptp daemonSet Eventually loop : %s", err))

	Eventually(func() int {
		ptpPods, err := Apiclient.Pods(generalParam.PtpOperatorNamespace).List(
			context.Background(),
			metav1.ListOptions{LabelSelector: "app=linuxptp-daemon"})
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
	default:
		Fail(fmt.Sprintf("Unknown scenario %s", scenario))
	}
	return []int{passedTestNumber, skippedTestNumber}
}
