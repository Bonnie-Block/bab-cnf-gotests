package helper

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	k8s "sigs.k8s.io/controller-runtime/pkg/client"

	fecv1 "github.com/open-ness/openshift-operator/sriov-fec/api/v1"

	globalHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
)

const (
	DeploymentSriovFecName        = "sriov-fec-controller-manager"
	TestNamespace                 = "vran-acceleration-operators-test"
	AcceleratorDiscoveryDaemonset = "accelerator-discovery"
	SriovDevicePlugin             = "sriov-device-plugin"
	SriovFecDaemonset             = "sriov-fec-daemonset"
)

// IsSriovFecDeploymentReady checks if Sriov Fec deployment is ready
func IsSriovFecDeploymentReady(cs *client.ClientSet, operatorNamespace string) (bool, error) {
	deploymentSriovFec, err := cs.Deployments(operatorNamespace).Get(context.Background(), DeploymentSriovFecName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	if deploymentSriovFec.Status.ReadyReplicas > 0 {
		return true, nil
	}
	return false, nil
}

//IsSriovFecDeploymentInstalled checks if Sriov Fec deployment is installed
func IsSriovFecDeploymentInstalled(cs *client.ClientSet, operatorNamespace string) (bool, error) {
	_, err := cs.Deployments(operatorNamespace).Get(context.Background(), DeploymentSriovFecName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	return true, nil
}

// InstallSriovFecClusterNodeConfig creates a new SriovFecClusterConfig and waits for the cluster to become stable
func InstallSriovFecClusterNodeConfig(cs *client.ClientSet, fecConfig *fecv1.SriovFecClusterConfig, isSingleNode bool) {
	CleanAllSriovFecClusterConfig(cs)
	createSriovFecClusterConfig(cs, fecConfig)
	fmt.Println("Waiting for the cluster to become stable")
	if !isSingleNode {
		err := nodes.WaitForClusterToBeStable(cs)
		Expect(err).NotTo(HaveOccurred())
	}

	Eventually(func() string {
		sriovFecNodeConfigList, err := GetSriovFecNodeConfigList(cs)
		Expect(err).NotTo(HaveOccurred())
		return sriovFecNodeConfigList.Items[0].Status.Conditions[0].Reason
	}, 2*time.Minute, 5*time.Second).Should(Equal("Succeeded"), "SriovFecNodeConfig resource is not configured successfully ")
}

// CountSriovFecDaemonsets counts total number of  Ready  and Desired sriov-fec daemonsets
func CountSriovFecDaemonsets(cs *client.ClientSet, operatorNamespace string) (countRunningDaemonsets int32, countDesiredDaemonsets int32) {
	acceleratorDiscovery, err := cs.DaemonSets(operatorNamespace).Get(context.Background(), AcceleratorDiscoveryDaemonset, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	sriovfecDamonset, err := cs.DaemonSets(operatorNamespace).Get(context.Background(), SriovFecDaemonset, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	sriovfecDevicePlugin, err := cs.DaemonSets(operatorNamespace).Get(context.Background(), SriovDevicePlugin, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	countRunningDaemonsets = acceleratorDiscovery.Status.NumberReady +
		sriovfecDamonset.Status.NumberReady +
		sriovfecDevicePlugin.Status.NumberReady
	countDesiredDaemonsets = acceleratorDiscovery.Status.DesiredNumberScheduled +
		sriovfecDamonset.Status.DesiredNumberScheduled +
		sriovfecDevicePlugin.Status.DesiredNumberScheduled
	return countRunningDaemonsets, countDesiredDaemonsets
}

// GetSriovFecNodeConfigList retrieves SriovFecNodeList
func GetSriovFecNodeConfigList(cs *client.ClientSet) (*fecv1.SriovFecNodeConfigList, error) {
	sriovFecNodeConfigList := &fecv1.SriovFecNodeConfigList{}
	err := cs.List(context.Background(), sriovFecNodeConfigList)
	if err != nil {
		return nil, err
	}
	sriovFecNodeConfigList.Items, err = matchingOptionalSelectorSriovFec(cs, sriovFecNodeConfigList.Items)
	if err != nil {
		return nil, err
	}
	return sriovFecNodeConfigList, nil
}

// GetSriovFecClusterConfigList retrieves SriovFecClusterConfigList
func GetSriovFecClusterConfigList(cs *client.ClientSet) (*fecv1.SriovFecClusterConfigList, error) {
	sriovFecClusterConfigList := &fecv1.SriovFecClusterConfigList{}
	err := cs.List(context.Background(), sriovFecClusterConfigList)
	if err != nil {
		return nil, err
	}
	return sriovFecClusterConfigList, nil
}

//createSriovFecClusterConfig creates a new SriovFecClusterConfig
func createSriovFecClusterConfig(cs *client.ClientSet, sriovFecClusterConfig *fecv1.SriovFecClusterConfig) {
	err := cs.Create(context.Background(), sriovFecClusterConfig)
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() fecv1.SyncStatus {
		sriovFecClusterConfigList, err := GetSriovFecClusterConfigList(cs)
		Expect(err).NotTo(HaveOccurred())
		return sriovFecClusterConfigList.Items[0].Status.SyncStatus
	}, 5*time.Minute, 5*time.Second).Should(Equal(fecv1.SucceededSync), "SriovFecClusterConfig resource is not applied")
}

// CleanAllN3000Cluster removes all N3000Cluster resources
func CleanAllSriovFecClusterConfig(cs *client.ClientSet) {
	sriovFecClusterConfigList := &fecv1.SriovFecClusterConfigList{}
	err := cs.List(context.Background(), sriovFecClusterConfigList)
	Expect(err).ToNot(HaveOccurred())
	if len(sriovFecClusterConfigList.Items) > 0 {
		for _, sriovFecClusterConfig := range sriovFecClusterConfigList.Items {
			err = cs.Delete(context.Background(), &sriovFecClusterConfig)
			Expect(err).ToNot(HaveOccurred())
		}
	}
}

// DeleteSriovFecPods remove all the sriov fec daemonset pods
func DeleteSriovFecPods(cs *client.ClientSet, operatorNamespace string) {
	podList := &corev1.PodList{}
	err := cs.List(context.Background(), podList, &k8s.ListOptions{LabelSelector: labels.SelectorFromSet(labels.Set{"app": "sriov-fec-daemonset"}), Namespace: operatorNamespace})
	Expect(err).ToNot(HaveOccurred())
	for _, podObj := range podList.Items {
		err = cs.Delete(context.Background(), &podObj)
		Expect(err).ToNot(HaveOccurred())
	}
}

type RemoveStringValue struct {
	Op   string `json:"op"`
	Path string `json:"path"`
}

// CleanSriovFecNodeSpec use patch to clean the spec section of the sriovFecNode object
// Not possible with update only patch
func CleanSriovFecNodeSpec(cs *client.ClientSet, nodeName, operatorNamespace string) {
	sriovFecNodeConfig := &fecv1.SriovFecNodeConfig{}
	err := cs.Get(context.Background(), k8s.ObjectKey{Name: nodeName, Namespace: operatorNamespace}, sriovFecNodeConfig)
	Expect(err).ToNot(HaveOccurred())

	var payloads []interface{}
	payload := RemoveStringValue{
		Op:   "remove",
		Path: "/spec",
	}
	payloads = append(payloads, payload)
	payloadBytes, _ := json.Marshal(payloads)

	err = cs.Patch(context.Background(), sriovFecNodeConfig, k8s.RawPatch(types.JSONPatchType, payloadBytes))
	Expect(err).ToNot(HaveOccurred())
}

// matchingOptionalSelectorSriovFec filter the given slice with only the nodes matching the optional selector.
// If no selector is set, it returns the same list.
// The NODES_SELECTOR must be set with a labelselector expression.
// For example: NODES_SELECTOR="sctp=true"
func matchingOptionalSelectorSriovFec(cs *client.ClientSet, toFilter []fecv1.SriovFecNodeConfig) ([]fecv1.SriovFecNodeConfig, error) {
	if nodes.NodesSelector == "" {
		return toFilter, nil
	}
	toMatch, err := nodes.GetByLabel(cs, nodes.NodesSelector)
	if err != nil {
		return nil, fmt.Errorf("Error in getting nodes matching the %s label selector, %v", nodes.NodesSelector, err)
	}
	if len(toMatch.Items) == 0 {
		return nil, fmt.Errorf("Failed to get nodes matching %s label selector", nodes.NodesSelector)
	}

	res := make([]fecv1.SriovFecNodeConfig, 0)
	for _, n := range toFilter {
		for _, m := range toMatch.Items {
			if n.Name == m.Name {
				res = append(res, n)
				break
			}
		}
	}
	if len(res) == 0 {
		return nil, fmt.Errorf("Failed to find matching nodes with %s label selector", nodes.NodesSelector)
	}
	return res, nil
}

// CreateBbdevPod creates bbdev pod
func CreateBbdevPod(cs *client.ClientSet, namespace, acceleratorResourceName string, config *config.Config) *corev1.Pod {
	podBbdevDefinition := getBbdevPodDefinition(namespace, acceleratorResourceName, config)
	pod := globalHelper.WaitUntilPodCreatedAndRunning(podBbdevDefinition, 5*time.Minute)
	return pod
}

// getBbdevPodDefinition retrieves bbdev pod definition
func getBbdevPodDefinition(namespace, acceleratorResourceName string, config *config.Config) *corev1.Pod {
	podObject := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "pod-bbdev-sample-app",
			Namespace:    namespace},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				SecurityContext: &corev1.SecurityContext{
					RunAsUser:  pointer.Int64Ptr(0),
					Privileged: pointer.BoolPtr(false),
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

// RunBbdevTests executes bbdev tests in bbdev pod
func RunBbdevTests(cs *client.ClientSet, bbdevPod *corev1.Pod) string {
	pcideviceIntelComIntelFec5GBuff, err := pod.ExecCommand(cs, *bbdevPod, []string{"bash", "-c", "printenv | grep INTEL"})
	Expect(err).NotTo(HaveOccurred())
	pcideviceIntelComIntelFec5GString := strings.TrimSpace(strings.Split(pcideviceIntelComIntelFec5GBuff.String(), "=")[1])
	command := fmt.Sprintf("/usr/bbdev/test-bbdev.py"+
		" -e \"-w %v -d /usr/bbdev/\"  -c validation"+
		" -p /usr/bbdev/dpdk-test-bbdev"+
		" -n 64 -b 8"+
		" -v /usr/bbdev/test_vectors/*", pcideviceIntelComIntelFec5GString)

	bbdevTestsOutput, _ := pod.ExecCommand(cs, *bbdevPod, []string{"bash", "-c", command})
	return bbdevTestsOutput.String()
}

// IsBbdevFailedTests  checks if  any  bbdev test has failed
func IsBbdevFailedTests(str string) bool {
	for _, line := range strings.Split(str, "\n") {
		if strings.Contains(line, "Tests Failed") && !strings.Contains(line, "0") {
			return true
		}
	}
	return false
}
