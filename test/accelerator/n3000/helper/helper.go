package helper

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	fpgav1 "github.com/open-ness/openshift-operator/N3000/api/v1"
	fecv1 "github.com/open-ness/openshift-operator/sriov-fec/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/n3000/parameters"
	networkHelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
)

// createN3000ClusterConfig creates N3000Cluster resource
func createN3000ClusterConfig(cs *client.ClientSet, nodeName string, image string, bitstreamStore string, checksum string, pciAddr string) error {
	clusterConfig := &fpgav1.N3000Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "n3000",
			Namespace: parameters.OperatorNamespace,
		},
		Spec: fpgav1.N3000ClusterSpec{
			Nodes: []fpgav1.N3000ClusterNode{{
				NodeName: nodeName,
				FPGA: []fpgav1.N3000Fpga{{
					UserImageURL: "http://" + bitstreamStore + "/" + image,
					PCIAddr:      pciAddr,
					CheckSum:     checksum,
				}},
			}},
		},
	}
	err := cs.Create(context.Background(), clusterConfig)
	if err != nil {
		return err
	}

	Eventually(func() string {
		n3000Node, err := getN3000NodeByName(cs, nodeName)
		Expect(err).ToNot(HaveOccurred())
		condition, err := getN3000NodeCondition(n3000Node)
		Expect(err).ToNot(HaveOccurred())
		return condition.Reason
	}, 1*time.Minute, 5*time.Second).Should(Equal("InProgress"), "N3000Cluster resource is not applied")
	return nil
}

// createPodWithPort creates a pod with configured port
func createPodWithPort(cs *client.ClientSet, namespace string, image string, port int32) *corev1.Pod {
	podDefenition := pod.GetPodDefinitionWithPortAndLabel(namespace, image, port, parameters.TestLabel)
	pod, err := cs.Pods(namespace).Create(context.Background(), podDefenition, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
	Eventually(func() corev1.PodPhase {
		pod, _ = cs.Pods(namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
		return pod.Status.Phase
	}, 1*time.Minute, time.Second).Should(Equal(corev1.PodRunning))
	return pod
}

// CleanAllN3000Cluster removes all N3000Cluster resources
func CleanAllN3000Cluster(cs *client.ClientSet) {
	n3000ClusterConfigList := &fpgav1.N3000ClusterList{}
	err := cs.List(context.Background(), n3000ClusterConfigList)
	Expect(err).ToNot(HaveOccurred())
	if len(n3000ClusterConfigList.Items) > 0 {
		for _, n3000ClusterConfig := range n3000ClusterConfigList.Items {
			err = cs.Delete(context.Background(), &n3000ClusterConfig)
			Expect(err).ToNot(HaveOccurred())
		}
	}
}

// CountN3000Daemonsets counts total number of  Ready  and Desired n3000 daemonsets
func CountN3000Daemonsets(cs *client.ClientSet, operatorNamespace string) (countRunningDaemonsets int32, countDesiredDaemonsets int32) {
	daemonsetDriver, err := cs.DaemonSets(operatorNamespace).Get(context.Background(), parameters.DaemonsetDriverName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	daemonsetTelemetry, err := cs.DaemonSets(operatorNamespace).Get(context.Background(), parameters.DaemonsetTelemetryName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	daemonsetN3000Daemon, err := cs.DaemonSets(operatorNamespace).Get(context.Background(), parameters.DaemonsetN3000DaemonName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	countRunningDaemonsets = daemonsetDriver.Status.NumberReady +
		daemonsetTelemetry.Status.NumberReady +
		daemonsetN3000Daemon.Status.NumberReady
	countDesiredDaemonsets = daemonsetDriver.Status.DesiredNumberScheduled +
		daemonsetTelemetry.Status.DesiredNumberScheduled +
		daemonsetN3000Daemon.Status.DesiredNumberScheduled
	return countRunningDaemonsets, countDesiredDaemonsets
}

// GetN3000NodeList retrieves n3000NodeList
func GetN3000NodeList(cs *client.ClientSet) (*fpgav1.N3000NodeList, error) {
	n3000NodeList := &fpgav1.N3000NodeList{}
	err := cs.List(context.Background(), n3000NodeList)
	if err != nil {
		return nil, err
	}
	n3000NodeList.Items, err = matchingOptionalSelectorN3000(cs, n3000NodeList.Items)
	if err != nil {
		return nil, err
	}
	return n3000NodeList, nil
}

// GetN3000Node retrieves N3000Node
func GetN3000Node(cs *client.ClientSet) (*fpgav1.N3000Node, error) {
	n3000NodeList, err := GetN3000NodeList(cs)
	if err != nil {
		return nil, err
	}
	for _, n3000Node := range n3000NodeList.Items {
		for _, fortville := range n3000Node.Status.Fortville {
			if len(fortville.NICs) > 0 {
				return &n3000Node, nil
			}
		}
	}
	return nil, fmt.Errorf("N3000NodeList %v doesn`t have n3000node with configured nic", n3000NodeList)
}

// getN3000NodeByName retrieves N3000Node by Node name
func getN3000NodeByName(cs *client.ClientSet, nodeName string) (*fpgav1.N3000Node, error) {
	n3000NodeList, err := GetN3000NodeList(cs)
	if err != nil {
		return nil, err
	}
	for _, n3000Node := range n3000NodeList.Items {
		if n3000Node.Name == nodeName {
			for _, fortville := range n3000Node.Status.Fortville {
				if len(fortville.NICs) > 0 {
					return &n3000Node, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("N3000NodeList %v doesn`t have n3000node with configured nic  and name %s", n3000NodeList, nodeName)
}

// GetN3000FpgaStatus retrieves N3000FpgaStatus of N3000Node
func GetN3000FpgaStatus(n3000Node *fpgav1.N3000Node) (*fpgav1.N3000FpgaStatus, error) {
	if n3000Node.Status.FPGA == nil || len(n3000Node.Status.FPGA) < 1 {
		return nil, fmt.Errorf("N3000Node %s doesn`t have FPGA configuration", n3000Node.Name)
	}
	fpgaStatus := n3000Node.Status.FPGA[0]
	return &fpgaStatus, nil
}

// getN3000NodeCondition retrieves condition of N3000Node
func getN3000NodeCondition(n3000Node *fpgav1.N3000Node) (*metav1.Condition, error) {
	if n3000Node.Status.Conditions == nil || len(n3000Node.Status.Conditions) < 1 {
		return nil, fmt.Errorf("N3000Node %s doesn`t have Condition configuration", n3000Node.Name)
	}
	n3000NodeCondition := n3000Node.Status.Conditions[0]
	return &n3000NodeCondition, nil
}

// IsSriovFecDeploymentReady checks if Sriov Fec deployment is ready
func IsSriovFecDeploymentReady(cs *client.ClientSet, operatorNamespace string) (bool, error) {
	deploymentSriovFec, err := cs.Deployments(operatorNamespace).Get(context.Background(), parameters.DeploymentSriovFecName, metav1.GetOptions{})
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
	_, err := cs.Deployments(operatorNamespace).Get(context.Background(), parameters.DeploymentSriovFecName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	return true, nil
}

// InstallSriovFecClusterNodeConfig creates a new SriovFecClusterConfig and waits for the cluster to become stable
func InstallSriovFecClusterNodeConfig(cs *client.ClientSet, IsDefaultConfig bool) {
	CleanAllSriovFecClusterConfig(cs)
	createSriovFecClusterConfig(cs, IsDefaultConfig)
	fmt.Println("Waiting for the cluster to become stable")
	err := nodes.WaitForClusterToBeStable(cs)
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() string {
		sriovFecNodeConfigList, err := getSriovFecNodeConfigList(cs)
		Expect(err).NotTo(HaveOccurred())
		return sriovFecNodeConfigList.Items[0].Status.Conditions[0].Reason
	}, 2*time.Minute, 5*time.Second).Should(Equal("Succeeded"), "SriovFecNodeConfig resource is not configured successfully ")
}

// getSriovFecNodeConfigList retrieves SriovFecNodeConfigList
func getSriovFecNodeConfigList(cs *client.ClientSet) (*fecv1.SriovFecNodeConfigList, error) {
	sriovFecNodeConfigList := &fecv1.SriovFecNodeConfigList{}
	err := cs.List(context.Background(), sriovFecNodeConfigList)
	if err != nil {
		return nil, err
	}
	return sriovFecNodeConfigList, nil
}

// getSriovFecClusterConfigList retrieves SriovFecClusterConfigList
func getSriovFecClusterConfigList(cs *client.ClientSet) (*fecv1.SriovFecClusterConfigList, error) {
	sriovFecClusterConfigList := &fecv1.SriovFecClusterConfigList{}
	err := cs.List(context.Background(), sriovFecClusterConfigList)
	if err != nil {
		return nil, err
	}
	return sriovFecClusterConfigList, nil
}

//createSriovFecClusterConfig creates a new SriovFecClusterConfig
func createSriovFecClusterConfig(cs *client.ClientSet, IsDefaultConfig bool) {
	sriovFecClusterConfig := getSriovFecClusterConfigDefinition(cs, IsDefaultConfig)
	err := cs.Create(context.Background(), sriovFecClusterConfig)
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() fecv1.SyncStatus {
		sriovFecClusterConfigList, err := getSriovFecClusterConfigList(cs)
		Expect(err).NotTo(HaveOccurred())
		return sriovFecClusterConfigList.Items[0].Status.SyncStatus
	}, 10*time.Minute, 5*time.Second).Should(Equal(fecv1.SucceededSync), "SriovFecClusterConfig resource is not applied")
}

// CleanAllN3000Cluster removes all N3000Cluster resources
func CleanAllSriovFecClusterConfig(cs *client.ClientSet) {
	sriovFecClusterConfigList := &fecv1.SriovFecClusterConfigList{}
	cs.List(context.Background(), sriovFecClusterConfigList)
	if len(sriovFecClusterConfigList.Items) > 0 {
		for _, sriovFecClusterConfig := range sriovFecClusterConfigList.Items {
			err := cs.Delete(context.Background(), &sriovFecClusterConfig)
			Expect(err).ToNot(HaveOccurred())
		}
	}
}

// CreateBbdevPod creates bbdev pod
func CreateBbdevPod(cs *client.ClientSet, namespace string) *corev1.Pod {
	podBbdevDefinition := getBbdevPodDefinition(namespace)
	pod := networkHelper.WaitUntilPodCreatedAndRunning(cs, podBbdevDefinition, parameters.TestNamespace, 1*time.Minute)
	return pod
}

// getBbdevPodDefinition retrieves bbdev pod definition
func getBbdevPodDefinition(namespace string) *corev1.Pod {
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
				Image:           parameters.ImageTestCMD,
				ImagePullPolicy: "IfNotPresent",
				Command:         []string{"/bin/bash", "-c", "--"},
				Args:            []string{"while true; do sleep 300000; done;"},
				VolumeMounts: []corev1.VolumeMount{{
					MountPath: "mnt/huge",
					Name:      "hugepage"}},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceName("cpu"):                    resource.MustParse("4"),
						corev1.ResourceName("intel.com/intel_fec_5g"): resource.MustParse("1"),
						corev1.ResourceName("hugepages-1Gi"):          resource.MustParse("2Gi"),
						corev1.ResourceName("memory"):                 resource.MustParse("1Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceName("cpu"):                    resource.MustParse("4"),
						corev1.ResourceName("intel.com/intel_fec_5g"): resource.MustParse("1"),
						corev1.ResourceName("hugepages-1Gi"):          resource.MustParse("2Gi"),
						corev1.ResourceName("memory"):                 resource.MustParse("1Gi"),
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

// getSriovFecClusterConfigDefinition retrieves SriovFecClusterConfig definition
func getSriovFecClusterConfigDefinition(cs *client.ClientSet, isDefaultConfig bool) *fecv1.SriovFecClusterConfig {
	var sriovFecNodeConfig *fecv1.SriovFecNodeConfig
	vf := 0
	vfAmount := 0
	if !isDefaultConfig {
		vf = 16
		vfAmount = 2
	}

	Eventually(func() int {
		sriovFecNodeConfigList, err := getSriovFecNodeConfigList(cs)
		Expect(err).NotTo(HaveOccurred())
		sriovFecNodeConfig = &sriovFecNodeConfigList.Items[0]
		return len(sriovFecNodeConfig.Status.Inventory.SriovAccelerators)
	}, 2*time.Minute, 1*time.Second).Should(BeNumerically(">", 0), "there are no available SriovAccelerators")

	sriovFecClusterConfig := &fecv1.SriovFecClusterConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "config",
			Namespace: parameters.OperatorNamespace,
		}, Spec: fecv1.SriovFecClusterConfigSpec{
			Nodes: []fecv1.NodeConfig{{
				NodeName: sriovFecNodeConfig.Name,
				PhysicalFunctions: []fecv1.PhysicalFunctionConfig{{
					PCIAddress: sriovFecNodeConfig.Status.Inventory.SriovAccelerators[0].PCIAddress,
					PFDriver:   "pci-pf-stub",
					VFDriver:   "vfio-pci",
					VFAmount:   vfAmount,
					BBDevConfig: fecv1.BBDevConfig{
						N3000: &fecv1.N3000BBDevConfig{
							NetworkType: "FPGA_5GNR",
							PFMode:      false,
							FLRTimeOut:  610,
							Downlink: fecv1.UplinkDownlink{
								Bandwidth:   3,
								LoadBalance: 128,
								Queues: fecv1.UplinkDownlinkQueues{
									VF0: vf,
									VF1: vf,
								},
							},
							Uplink: fecv1.UplinkDownlink{
								Bandwidth:   3,
								LoadBalance: 128,
								Queues: fecv1.UplinkDownlinkQueues{
									VF0: vf,
									VF1: vf,
								},
							},
						},
					},
				},
				}},
			}},
	}
	return sriovFecClusterConfig
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

// isBbdevFailedTests  checks if  any  bbdev test has failed
func IsBbdevFailedTests(str string) bool {
	for _, line := range strings.Split(str, "\n") {
		if strings.Contains(line, "Tests Failed") && !strings.Contains(line, "0") {
			return true
		}
	}
	return false
}

// matchingOptionalSelectorN3000 filter the given slice with only the nodes matching the optional selector.
// If no selector is set, it returns the same list.
// The NODES_SELECTOR must be set with a labelselector expression.
// For example: NODES_SELECTOR="sctp=true"
func matchingOptionalSelectorN3000(cs *client.ClientSet, toFilter []fpgav1.N3000Node) ([]fpgav1.N3000Node, error) {
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

	res := make([]fpgav1.N3000Node, 0)
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

// CreateService creates Service resource
func CreateService(cs *client.ClientSet, namespace string) *corev1.Service {
	serviceDefinition := getServiceDefinition(parameters.TestLabel)
	service, err := cs.Services(namespace).Create(context.Background(), serviceDefinition, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	return service
}

// getServiceDefinition retrieves definition for Service with specific selector
func getServiceDefinition(selector map[string]string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "testservice-",
		},
		Spec: corev1.ServiceSpec{
			Selector: selector,
			Ports: []corev1.ServicePort{{
				NodePort: 0,
				Port:     80,
				Protocol: "TCP",
				TargetPort: intstr.IntOrString{
					IntVal: 80,
				},
			}},
		},
	}
	return service
}

// InstallNewN3000Image creates a pod with n3000 images, creates a new N3000cluster config  and waits for the cluster to become stable
func InstallNewN3000Image(cs *client.ClientSet, n3000NodeName string, fpgaStatus *fpgav1.N3000FpgaStatus, image string, checksum string, port int32, service *corev1.Service) {
	CleanAllN3000Cluster(cs)
	podWithImages := createPodWithPort(cs, parameters.TestNamespace, parameters.ImageTestCMD, port)
	installWebServerInPod(cs, podWithImages)
	err := createN3000ClusterConfig(cs, n3000NodeName, image, service.Spec.ClusterIP, checksum, fpgaStatus.PciAddr)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting until cluster become stable")
	Eventually(func() string {
		n3000Node, err := getN3000NodeByName(cs, n3000NodeName)
		Expect(err).NotTo(HaveOccurred())
		n3000NodeCondition, err := getN3000NodeCondition(n3000Node)
		Expect(err).NotTo(HaveOccurred())
		return n3000NodeCondition.Message
	}, 50*time.Minute, 15*time.Second).Should(Equal("Flashed successfully"), "Bitstream flashing failed")
}

// installWebServerInPod executes scrypt to enable web server.
func installWebServerInPod(cs *client.ClientSet, webPod *corev1.Pod) {
	_, err := pod.ExecCommand(cs, *webPod, []string{"./usr/scripts/install-nginx.sh"})
	Expect(err).ToNot(HaveOccurred())
}
