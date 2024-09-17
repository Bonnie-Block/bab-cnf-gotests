package netacceleratorhelper

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/acc100/netacc100parameters"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
	"k8s.io/utils/pointer"
	k8s "sigs.k8s.io/controller-runtime/pkg/client"

	performancev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	fecv2 "github.com/smart-edge-open/sriov-fec-operator/sriov-fec/api/v2"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
)

const (
	AcceleratorDiscoveryDaemonset = "accelerator-discovery"
	SriovDevicePlugin             = "sriov-device-plugin"
	SriovFecDaemonset             = "sriov-fec-daemonset"
	PerformanceProfileName        = "performance-profile-dpdk"
)

// InstallSriovFecClusterNodeConfig creates a new SriovFecClusterConfig and waits for the cluster to become stable.
func InstallSriovFecClusterNodeConfig(
	clientSet *client.ClientSet,
	fecConfig *fecv2.SriovFecClusterConfig,
	isSingleNode bool, cnfNodeLabel string) {
	CleanAllSriovFecClusterConfig(clientSet)
	createSriovFecClusterConfig(clientSet, fecConfig)

	Eventually(func() string {
		sriovFecNodeConfigList, err := GetSriovFecNodeConfigList(clientSet)
		Expect(err).NotTo(HaveOccurred())

		return sriovFecNodeConfigList.Items[0].Status.Conditions[0].Reason
	}, 20*time.Minute, 5*time.Second).Should(
		Equal("Succeeded"),
		"SriovFecNodeConfig resource is not configured successfully ",
	)
}

// CountSriovFecDaemonsets counts total number of  Ready  and Desired sriov-fec daemonsets.
func CountSriovFecDaemonsets(
	clientSet *client.ClientSet,
	operatorNamespace string) (countRunningDaemonsets int32, countDesiredDaemonsets int32) {
	acceleratorDiscovery, err := clientSet.DaemonSets(operatorNamespace).Get(
		context.Background(),
		AcceleratorDiscoveryDaemonset,
		metav1.GetOptions{},
	)
	Expect(err).NotTo(HaveOccurred())
	sriovfecDamonset, err := clientSet.DaemonSets(operatorNamespace).Get(
		context.Background(),
		SriovFecDaemonset,
		metav1.GetOptions{},
	)
	Expect(err).NotTo(HaveOccurred())
	sriovfecDevicePlugin, err := clientSet.DaemonSets(operatorNamespace).Get(
		context.Background(),
		SriovDevicePlugin,
		metav1.GetOptions{},
	)
	Expect(err).NotTo(HaveOccurred())

	countRunningDaemonsets = acceleratorDiscovery.Status.NumberReady +
		sriovfecDamonset.Status.NumberReady +
		sriovfecDevicePlugin.Status.NumberReady
	countDesiredDaemonsets = acceleratorDiscovery.Status.DesiredNumberScheduled +
		sriovfecDamonset.Status.DesiredNumberScheduled +
		sriovfecDevicePlugin.Status.DesiredNumberScheduled

	return countRunningDaemonsets, countDesiredDaemonsets
}

// GetSriovFecNodeConfigList retrieves SriovFecNodeList.
func GetSriovFecNodeConfigList(clientSet *client.ClientSet) (*fecv2.SriovFecNodeConfigList, error) {
	sriovFecNodeConfigList := &fecv2.SriovFecNodeConfigList{}
	err := clientSet.List(context.Background(), sriovFecNodeConfigList)

	if err != nil {
		return nil, err
	}

	sriovFecNodeConfigList.Items, err = matchingOptionalSelectorSriovFec(clientSet, sriovFecNodeConfigList.Items)

	if err != nil {
		return nil, err
	}

	return sriovFecNodeConfigList, nil
}

// createSriovFecClusterConfig creates a new SriovFecClusterConfig.
func createSriovFecClusterConfig(clientSet *client.ClientSet, sriovFecClusterConfig *fecv2.SriovFecClusterConfig) {
	err := clientSet.Create(context.Background(), sriovFecClusterConfig)
	Expect(err).ToNot(HaveOccurred())
}

// CleanAllSriovFecClusterConfig removes all FECCluster resources.
func CleanAllSriovFecClusterConfig(clientSet *client.ClientSet) {
	sriovFecClusterConfigList := &fecv2.SriovFecClusterConfigList{}
	err := clientSet.List(context.Background(), sriovFecClusterConfigList)
	Expect(err).ToNot(HaveOccurred())

	if len(sriovFecClusterConfigList.Items) > 0 {
		for _, sriovFecClusterConfig := range sriovFecClusterConfigList.Items {
			err = clientSet.Delete(context.Background(), &sriovFecClusterConfig)
			Expect(err).ToNot(HaveOccurred())
		}
	}
}

type RemoveStringValue struct {
	Op   string `json:"op"`
	Path string `json:"path"`
}

// matchingOptionalSelectorSriovFec filter the given slice with only the nodes matching the optional selector.
// If no selector is set, it returns the same list.
// The NODES_SELECTOR must be set with a labelselector expression.
// For example: NODES_SELECTOR="sctp=true".
func matchingOptionalSelectorSriovFec(
	cs *client.ClientSet, toFilter []fecv2.SriovFecNodeConfig) ([]fecv2.SriovFecNodeConfig, error) {
	if nodes.NodesSelector == "" {
		return toFilter, nil
	}

	toMatch, err := nodes.GetByLabel(cs, nodes.NodesSelector)

	if err != nil {
		return nil, fmt.Errorf(
			"error in getting nodes matching the %s label selector, %w",
			nodes.NodesSelector,
			err,
		)
	}

	if len(toMatch.Items) == 0 {
		return nil, fmt.Errorf("failed to get nodes matching %s label selector", nodes.NodesSelector)
	}

	res := make([]fecv2.SriovFecNodeConfig, 0)

	for _, n := range toFilter {
		for _, m := range toMatch.Items {
			if n.Name == m.Name {
				res = append(res, n)

				break
			}
		}
	}

	if len(res) == 0 {
		return nil, fmt.Errorf("failed to find matching nodes with %s label selector", nodes.NodesSelector)
	}

	return res, nil
}

// CreateBbdevPod creates bbdev pod.
func CreateBbdevPod(
	namespace, acceleratorResourceName string, config *config.Config) *corev1.Pod {
	podBbdevDefinition := getBbdevPodDefinition(namespace, acceleratorResourceName, config)
	bbdevPod := helper.WaitUntilPodCreatedAndRunning(podBbdevDefinition, 5*time.Minute)

	return bbdevPod
}

// getBbdevPodDefinition retrieves bbdev pod definition.
func getBbdevPodDefinition(namespace, acceleratorResourceName string, config *config.Config) *corev1.Pod {
	podObject := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "pod-bbdev-sample-app",
			Namespace:    namespace},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				SecurityContext: &corev1.SecurityContext{
					RunAsUser:  pointer.Int64(0),
					Privileged: pointer.Bool(false),
					Capabilities: &corev1.Capabilities{
						Add: []corev1.Capability{"IPC_LOCK", "SYS_RESOURCE"},
					},
				},
				Name:            "bbdev-sample-app",
				Image:           config.Network.TestContainerImage,
				ImagePullPolicy: "IfNotPresent",
				Command:         []string{"/bin/bash", "-c", "--"},
				Args:            []string{"while true; do sleep 300000; done;"},
				VolumeMounts: []corev1.VolumeMount{{
					MountPath: "mnt/huge",
					Name:      "hugepage"}},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceName("cpu"):                   resource.MustParse("4"),
						corev1.ResourceName(acceleratorResourceName): resource.MustParse("1"),
						corev1.ResourceName("hugepages-1Gi"):         resource.MustParse("2Gi"),
						corev1.ResourceName("memory"):                resource.MustParse("1Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceName("cpu"):                   resource.MustParse("4"),
						corev1.ResourceName(acceleratorResourceName): resource.MustParse("1"),
						corev1.ResourceName("hugepages-1Gi"):         resource.MustParse("2Gi"),
						corev1.ResourceName("memory"):                resource.MustParse("1Gi"),
					},
				},
			}},
			Volumes: []corev1.Volume{{
				Name: "hugepage",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{
						Medium: corev1.StorageMedium("HugePages"),
					},
				},
			}},
		},
	}

	return podObject
}

// RunBbdevTests executes bbdev tests in bbdev pod.
func RunBbdevTests(clientSet *client.ClientSet, bbdevPod *corev1.Pod, isSecureEnabled bool) string {
	printEnvCommand := []string{"bash", "-c", "printenv | grep INTEL"}

	if isSecureEnabled {
		printEnvCommand = []string{"bash", "-c", "printenv | grep INTEL | grep -v INFO"}
	}

	pcideviceIntelComIntelFec5GBuff, err := pod.ExecCommand(
		clientSet, *bbdevPod, printEnvCommand)
	Expect(err).NotTo(HaveOccurred())

	pcideviceIntelComIntelFec5GString := strings.TrimSpace(
		strings.Split(pcideviceIntelComIntelFec5GBuff.String(), "=")[1])

	testCommand := fmt.Sprintf("/usr/bbdev/test-bbdev.py"+
		" -e \"-w %v -d /usr/bbdev/\"  -c validation"+
		" -p /usr/bbdev/dpdk-test-bbdev"+
		" -n 64 -b 8"+
		" -v /usr/bbdev/test_vectors/*", pcideviceIntelComIntelFec5GString)

	if isSecureEnabled {
		token, err := getVFIOToken(clientSet, bbdevPod, pcideviceIntelComIntelFec5GString)
		Expect(err).NotTo(HaveOccurred())

		testCommand = fmt.Sprintf("/usr/bbdev/test-bbdev.py"+
			" -e \"-w %v --vfio-vf-token %s -d /usr/bbdev/\"  -c validation"+
			" -p /usr/bbdev/dpdk-test-bbdev"+
			" -n 64 -b 8 "+
			" -v /usr/bbdev/test_vectors/*", pcideviceIntelComIntelFec5GString, token)
	}

	bbdevTestsOutput, _ := pod.ExecCommand(clientSet, *bbdevPod, []string{"bash", "-c", testCommand})

	return bbdevTestsOutput.String()
}

// IsBbdevFailedTests  checks if  any  bbdev test has failed.
func IsBbdevFailedTests(str string) bool {
	for _, line := range strings.Split(str, "\n") {
		if strings.Contains(line, "Tests Failed") && !strings.Contains(line, "0") {
			return true
		}
	}

	return false
}

func FindAndValidateOrOverridePerformanceProfile(
	clientSet *client.ClientSet, nodeLabel string, snoTimeoutMultiplier time.Duration) {
	var (
		valid                 = true
		performanceProfile    = &performancev2.PerformanceProfile{}
		machineConfigPoolName = strings.Split(nodeLabel, "/")[1]
	)

	err := clientSet.Get(context.Background(), k8s.ObjectKey{Name: PerformanceProfileName}, performanceProfile)
	if err != nil {
		if !errors.IsNotFound(err) {
			Expect(err).ToNot(HaveOccurred())
		}

		valid = false
		performanceProfile = nil
	}

	if valid {
		valid, err = validatePerformanceProfile(performanceProfile)
		Expect(err).ToNot(HaveOccurred())
	}

	if !valid {
		if performanceProfile != nil {
			fmt.Println("Installed Performance Profile is not suitable for the test\n" +
				"Deleting profiles")

			err = helper.CleanAllPerformanceProfile(machineConfigPoolName, snoTimeoutMultiplier)
			Expect(err).ToNot(HaveOccurred())
		}

		fmt.Println("Creating Performance Profile")

		err = helper.CreatePerformanceProfile(PerformanceProfileName, nodeLabel)
		Expect(err).ToNot(HaveOccurred())
		err = helper.WaitForClusterToBeStable(machineConfigPoolName, snoTimeoutMultiplier)
		Expect(err).ToNot(HaveOccurred())
	}
}

func validatePerformanceProfile(performanceProfile *performancev2.PerformanceProfile) (bool, error) {
	cpuSet, err := cpuset.Parse(string(*performanceProfile.Spec.CPU.Isolated))
	if err != nil {
		return false, err
	}

	cpuSetSlice := cpuSet.ToSlice()
	if len(cpuSetSlice) < 6 {
		return false, nil
	}

	if performanceProfile.Spec.HugePages == nil {
		return false, nil
	}

	if *performanceProfile.Spec.HugePages.DefaultHugePagesSize != "1G" {
		return false, nil
	}

	if len(performanceProfile.Spec.HugePages.Pages) == 0 {
		return false, nil
	}

	if performanceProfile.Spec.HugePages.Pages[0].Count < 10 {
		return false, nil
	}

	if performanceProfile.Spec.HugePages.Pages[0].Size != "1G" {
		return false, nil
	}

	if performanceProfile.Spec.HugePages.Pages[0].Node != nil {
		return false, nil
	}

	return true, nil
}

// GetNodeSecureBootState checks if the workers have secure boot enabled.
func GetNodeSecureBootState(nodeName []string, namespace string) (bool, error) {
	podDefinition := defineTestPod(nodeName[0], helper.Config.Network.TestContainerImage,
		namespace)

	testPod := helper.WaitUntilPodCreatedAndRunning(podDefinition, 2*time.Minute)

	defer func() {
		err := helper.Apiclient.Pods(namespace).Delete(context.Background(), testPod.Name,
			metav1.DeleteOptions{GracePeriodSeconds: pointer.Int64(0)})
		Expect(err).ToNot(HaveOccurred())
	}()

	stdout, err := pod.ExecCommand(helper.Apiclient, *testPod, []string{"cat", "/host/sys/kernel/security/lockdown"})

	if strings.Contains(stdout.String(), "No such file or directory") {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return strings.Contains(stdout.String(), "[integrity]") || strings.Contains(stdout.String(),
		"[confidentiality]"), nil
}

func defineTestPod(node, image, namespace string) *corev1.Pod {
	podDefinition := pod.DefinePodOnNode(namespace, image, node)
	pod.RedefineWithVolume(podDefinition, "host", "/host",
		corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/"}}, true)

	podDefinition = pod.RedefineAsPrivileged(podDefinition)

	return podDefinition
}

// getVFIOToken returns the vfio-token.
func getVFIOToken(clientSet *client.ClientSet, bbdevPod *corev1.Pod, pci string) (string, error) {
	vfioTokenJSONBuff, err := pod.ExecCommand(
		clientSet, *bbdevPod, []string{"bash", "-c", "printenv | grep INFO"})

	if err != nil {
		return "", err
	}

	vfioTokenJSONString := strings.TrimSpace(
		strings.Split(vfioTokenJSONBuff.String(), "=")[1])
	result := map[string]netacc100parameters.VFIOToken{}

	err = json.Unmarshal([]byte(vfioTokenJSONString), &result)

	if err != nil {
		return "", err
	}

	return result[pci].Extra.VfioToken, nil
}
