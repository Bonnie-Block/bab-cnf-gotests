package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"

	ptpv1 "github.com/openshift/ptp-operator/pkg/apis/ptp/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/ptp/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/ptp/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
)

var _ = Describe("PTP", func() {
	execute.BeforeAll(func() {
		Expect(apiclient).NotTo(BeNil())
	})

	Describe("Multiple ptp interfaces", func() {
		var ptpRunningPods []v1core.Pod
		var masterNodeLabel, slaveNodeLabel string

		BeforeEach(func() {
			By("Configure PTP")
			configurePTP()

			masterConfigs, slaveConfigs := discoveryPTPConfiguration(operatorNamespace)

			masterNodeLabel = checkPtpProfileLabels(masterConfigs).label

			Expect(len(slaveConfigs)).ShouldNot(Equal(0), "There is no Slave configuration")

			slaveNodeLabel = checkPtpProfileLabels(slaveConfigs).label
			Expect(slaveNodeLabel).ShouldNot(Equal(""), "There is no PTP slave")

			By("Find all master and slave PTP pods")
			ptpPods, err := apiclient.Pods(operatorNamespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app=linuxptp-daemon"})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(ptpPods.Items)).To(BeNumerically(">", 0), fmt.Sprint("linuxptp-daemon is not deployed on cluster"))

			ptpSlaveRunningPods := []v1core.Pod{}
			ptpMasterRunningPods := []v1core.Pod{}

			for _, pod := range ptpPods.Items {
				if podRole(pod, slaveNodeLabel) {
					waitUntilLogIsDetected(pod, 3*time.Minute, "Profile Name:")
					ptpSlaveRunningPods = append(ptpSlaveRunningPods, pod)
				} else if podRole(pod, masterNodeLabel) {
					waitUntilLogIsDetected(pod, 3*time.Minute, "Profile Name:")
					ptpMasterRunningPods = append(ptpMasterRunningPods, pod)
				}
			}
			Expect(len(ptpMasterRunningPods)).To(BeNumerically(">=", 1), fmt.Sprint("Fail to detect PTP master pods on Cluster"))
			Expect(len(ptpSlaveRunningPods)).To(BeNumerically(">=", 1), fmt.Sprint("Fail to detect PTP slave pods on Cluster"))

			ptpRunningPods = append(ptpMasterRunningPods, ptpSlaveRunningPods...)
		})

		AfterEach(func() {
			By("Cleaning up resources after test")

			err := helper.Clean(apiclient, operatorNamespace)
			Expect(err).NotTo(HaveOccurred())
		})

		//37056
		It("from the same policy", func() {
			for _, podEntry := range ptpRunningPods {
				if podRole(podEntry, masterNodeLabel) {

					Eventually(func() map[string]int {
						ptpProcessesAndMetrics, err := amountPtpProcessesAndMetrics("master", podEntry)
						Expect(err).NotTo(HaveOccurred())
						return ptpProcessesAndMetrics
					}, 1*time.Minute, 5*time.Second).Should(Equal(map[string]int{"phc2sysMetric": 2, "ptp4lProc": 2, "phc2sysProc": 2}), "Wrong amount of ptp4l or phc2sys  on a Master")

				} else if podRole(podEntry, slaveNodeLabel) {

					Eventually(func() map[string]int {
						ptpProcessesAndMetrics, err := amountPtpProcessesAndMetrics("slave", podEntry)
						Expect(err).NotTo(HaveOccurred())
						return ptpProcessesAndMetrics
					}, 1*time.Minute, 5*time.Second).Should(Equal(map[string]int{"ptp4lMetric": 2, "phc2sysMetric": 2, "ptp4lProc": 2, "phc2sysProc": 2}), "Wrong amount of ptp4l or phc2sys  on a Slave")

				}

			}

		})

	})
})

func configurePTP() {
	err := helper.CleanAllPtpConfig(apiclient, operatorNamespace)
	Expect(err).ToNot(HaveOccurred())

	ptpNodes, err := helper.PtpEnabled(apiclient)
	Expect(err).ToNot(HaveOccurred())
	Expect(len(ptpNodes)).To(BeNumerically(">", 1), "need at least two nodes with ptp capable nics")

	By("Labeling the grandmaster node")
	ptpGrandMasterNode := ptpNodes[0]
	ptpGrandMasterNode.NodeObject, err = nodes.LabelNode(apiclient, ptpGrandMasterNode.NodeName, parameters.PtpGrandmasterNodeLabel, "")
	Expect(err).ToNot(HaveOccurred())

	By("Labeling the slave node")
	ptpSlaveNode := ptpNodes[1]
	ptpSlaveNode.NodeObject, err = nodes.LabelNode(apiclient, ptpSlaveNode.NodeName, parameters.PtpSlaveNodeLabel, "")
	Expect(err).ToNot(HaveOccurred())

	By("Creating the policy for the grandmaster node")
	config, err := config.NewConfig()
	Expect(err).ToNot(HaveOccurred())
	validPtpInterfaces, err := helper.GetPtpInterfaces(config, apiclient, 2)
	Expect(err).ToNot(HaveOccurred())
	err = createConfigMultipleInterfaces(parameters.PtpGrandMasterPolicyNameArr,
		validPtpInterfaces ,
		"-2",
		"-a -r -r",
		parameters.PtpGrandmasterNodeLabel,
		pointer.Int64Ptr(5))
	Expect(err).ToNot(HaveOccurred())

	By("Creating the policy for the slave node")
	err = createConfigMultipleInterfaces(parameters.PtpSlavePolicyNameArr,
		validPtpInterfaces ,
		"-s -2",
		"-a -r",
		parameters.PtpSlaveNodeLabel,
		pointer.Int64Ptr(5))
	Expect(err).ToNot(HaveOccurred())

	By("Restart the linuxptp-daemon pods")
	ptpPods, err := apiclient.Pods(operatorNamespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app=linuxptp-daemon"})
	Expect(err).ToNot(HaveOccurred())
	for _, pod := range ptpPods.Items {
		err = apiclient.Pods(operatorNamespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{GracePeriodSeconds: pointer.Int64Ptr(0)})
		Expect(err).ToNot(HaveOccurred())
	}

	daemonset, err := apiclient.DaemonSets(operatorNamespace).Get(context.Background(), parameters.PtpDaemonsetName, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())
	expectedNumber := daemonset.Status.DesiredNumberScheduled
	Eventually(func() int32 {
		daemonset, err = apiclient.DaemonSets(operatorNamespace).Get(context.Background(), parameters.PtpDaemonsetName, metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		return daemonset.Status.NumberReady
	}, 2*time.Minute, 2*time.Second).Should(Equal(expectedNumber))

	Eventually(func() int {
		ptpPods, err := apiclient.Pods(operatorNamespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app=linuxptp-daemon"})
		Expect(err).ToNot(HaveOccurred())
		return len(ptpPods.Items)
	}, 2*time.Minute, 2*time.Second).Should(Equal(int(expectedNumber)))

	err = wait.PollImmediate(1*time.Second, 60*time.Second, func() (done bool, err error) {
		ptpPods, err := apiclient.Pods(operatorNamespace).List(context.Background(),
			metav1.ListOptions{LabelSelector: "app=linuxptp-daemon", FieldSelector: fmt.Sprintf("spec.nodeName=%s", ptpSlaveNode.NodeName)})
		Expect(err).ToNot(HaveOccurred())
		Expect(len(ptpPods.Items)).To(Equal(1))

		logs, err := pod.GetLog(apiclient, &ptpPods.Items[0], 2*time.Second, parameters.PtpContainerName)
		Expect(err).ToNot(HaveOccurred())

		if strings.Contains(logs, "new foreign master") {
			fmt.Printf("Found valid PTP Configuration, using %s %s for master and slave\n", validPtpInterfaces[0], validPtpInterfaces[1])
			return true, nil
		}
		return false, nil
	})
	Expect(err).ToNot(HaveOccurred(), "Did not found valid PTP Configuration")

}

func discoveryPTPConfiguration(namespace string) ([]ptpv1.PtpConfig, []ptpv1.PtpConfig) {
	var masters []ptpv1.PtpConfig
	var slaves []ptpv1.PtpConfig

	configList, err := apiclient.PtpConfigs(namespace).List(context.Background(), metav1.ListOptions{})
	Expect(err).ToNot(HaveOccurred())
	for _, config := range configList.Items {
		for _, profile := range config.Spec.Profile {
			if isPtpMaster(*profile.Ptp4lOpts, *profile.Phc2sysOpts) {
				masters = append(masters, config)
			}
			if isPtpSlave(*profile.Ptp4lOpts, *profile.Phc2sysOpts) {
				slaves = append(slaves, config)
			}
		}
	}
	return masters, slaves
}

type ptpDiscoveryRes struct {
	label       string
	profileName string
}

func checkPtpProfileLabels(configs []ptpv1.PtpConfig) ptpDiscoveryRes {
	for _, config := range configs {
		for _, recommend := range config.Spec.Recommend {
			for _, match := range recommend.Match {
				label := *match.NodeLabel
				nodeCount := amountLabeledNodes(label)

				if nodeCount > 0 {
					return ptpDiscoveryRes{label, config.Name}
				}
			}
		}
	}
	return ptpDiscoveryRes{"", ""}
}

func amountLabeledNodes(label string) int {
	nodeList, err := apiclient.Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=", label)})
	Expect(err).ToNot(HaveOccurred())
	return len(nodeList.Items)
}

func isPtpSlave(ptp4lOpts string, phc2sysOpts string) bool {
	return strings.Contains(ptp4lOpts, "-s") && strings.Count(phc2sysOpts, "-a") == 1 && strings.Count(phc2sysOpts, "-r") == 1

}

func isPtpMaster(ptp4lOpts string, phc2sysOpts string) bool {
	return !strings.Contains(ptp4lOpts, "-s") && strings.Count(phc2sysOpts, "-a") == 1 && strings.Count(phc2sysOpts, "-r") == 2
}

func podRole(runningPod v1core.Pod, role string) bool {
	nodeList, err := apiclient.Nodes().List(context.Background(), metav1.ListOptions{
		LabelSelector: role,
	})
	Expect(err).NotTo(HaveOccurred())

	for _, nodeName := range nodeList.Items {
		if runningPod.Spec.NodeName == nodeName.Name {
			return true
		}
	}
	return false
}

func waitUntilLogIsDetected(podEntry v1core.Pod, timeout time.Duration, neededLog string) {
	Eventually(func() string {
		logs, err := pod.GetLog(apiclient, &podEntry, 2*time.Minute, parameters.PtpContainerName)
		Expect(err).ToNot(HaveOccurred())

		return logs
	}, timeout, 1*time.Second).Should(ContainSubstring(neededLog), fmt.Sprintf("Timeout to detect log \"%s\" in pod \"%s\"", neededLog, podEntry.Name))
}

func createConfigMultipleInterfaces(profileName []string, ifaceName []string, ptp4lOpts, phc2sysOpts, nodeLabel string, priority *int64) error {
	var ptpProfile []ptpv1.PtpProfile
	var ptpRecommend []ptpv1.PtpRecommend
	matchRule := ptpv1.MatchRule{NodeLabel: &nodeLabel}
	for i := 0; i < len(profileName); i++ {
		ptpProfile = append(ptpProfile, ptpv1.PtpProfile{Name: &profileName[i], Interface: &ifaceName[i], Phc2sysOpts: &phc2sysOpts, Ptp4lOpts: &ptp4lOpts})
		ptpRecommend = append(ptpRecommend, ptpv1.PtpRecommend{Profile: &profileName[i], Priority: priority, Match: []ptpv1.MatchRule{matchRule}})
	}
	policy := ptpv1.PtpConfig{ObjectMeta: metav1.ObjectMeta{Name: profileName[0], Namespace: operatorNamespace},
		Spec: ptpv1.PtpConfigSpec{Profile: ptpProfile, Recommend: ptpRecommend}}

	_, err := apiclient.PtpConfigs(operatorNamespace).Create(context.Background(), &policy, metav1.CreateOptions{})
	return err
}

func amountStringsByGreps(str string, stringsToGrep ...string) int {
	count := 0
	exists := false
	for _, line := range strings.Split(str, "\n") {
		for _, grep := range stringsToGrep {
			if !strings.Contains(line, grep) {
				exists = false
				break
			}
			exists = true
		}
		if exists {
			count++
		}
	}
	return count
}

func amountPtpProcessesAndMetrics(role string, podEntry v1core.Pod) (map[string]int, error) {
	ptpMetrics, _ := pod.ExecCommand(apiclient, podEntry, []string{"curl", "127.0.0.1:9091/metrics"})
	psOutput, _ := pod.ExecCommand(apiclient, podEntry, []string{"ps", "-C", "ptp4l", "-C", "phc2sys"})

	if role == "master" {
		phc2sysMetric := amountStringsByGreps(ptpMetrics.String(), "openshift_ptp_offset_from_master", "phc2sys")
		ptp4lProc := amountStringsByGreps(psOutput.String(), "ptp4l")
		phc2sysProc := amountStringsByGreps(psOutput.String(), "phc2sys")
		return map[string]int{"phc2sysMetric": phc2sysMetric, "ptp4lProc": ptp4lProc, "phc2sysProc": phc2sysProc}, nil
	} else if role == "slave" {
		ptp4lMetric := amountStringsByGreps(ptpMetrics.String(), "openshift_ptp_offset_from_master", "ptp4l")
		phc2sysMetric := amountStringsByGreps(ptpMetrics.String(), "openshift_ptp_offset_from_master", "phc2sys")
		ptp4lProc := amountStringsByGreps(psOutput.String(), "ptp4l")
		phc2sysProc := amountStringsByGreps(psOutput.String(), "phc2sys")
		return map[string]int{"ptp4lMetric": ptp4lMetric, "phc2sysMetric": phc2sysMetric, "ptp4lProc": ptp4lProc, "phc2sysProc": phc2sysProc}, nil
	}
	return nil, fmt.Errorf("Wrong PTP role: %s  of Pod", role)
}
