package helper

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	fpgav1 "github.com/open-ness/openshift-operator/N3000/api/v1"
	fecv1 "github.com/open-ness/openshift-operator/sriov-fec/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/accelerator/n3000/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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

// GetSriovFecNodeForN3000Bitstream5G retrieves SriovFecNodeConfig
func GetSriovFecNodeForN3000Bitstream5G(cs *client.ClientSet) (*fecv1.SriovFecNodeConfig, *fecv1.SriovAccelerator, error) {
	sriovFecNodeConfigList, err := helper.GetSriovFecNodeConfigList(cs)
	if err != nil {
		return nil, nil, err
	}
	for _, sriovFecNodeConfig := range sriovFecNodeConfigList.Items {
		for _, accelerators := range sriovFecNodeConfig.Status.Inventory.SriovAccelerators {
			if accelerators.DeviceID == parameters.N3000Bitstream5G {
				return &sriovFecNodeConfig, &accelerators, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("SriovFecNodeConfigList %v doesn`t have sriovfecnodeconfig with configured nic", sriovFecNodeConfigList)
}

// GetSriovFecN30005GClusterConfigDefinition retrieves SriovFecClusterConfig definition
func GetSriovFecN30005GClusterConfigDefinition(cs *client.ClientSet, isDefaultConfig bool) *fecv1.SriovFecClusterConfig {
	var err error
	var sriovFecNodeConfig *fecv1.SriovFecNodeConfig
	var accelerator *fecv1.SriovAccelerator

	vf := 0
	vfAmount := 0
	if !isDefaultConfig {
		vf = 16
		vfAmount = 2
	}

	Eventually(func() error {
		sriovFecNodeConfig, accelerator, err = GetSriovFecNodeForN3000Bitstream5G(cs)
		return err
	}, 5*time.Minute, 1*time.Second).ShouldNot(HaveOccurred(), "there are no available SriovAccelerators")

	sriovFecClusterConfig := &fecv1.SriovFecClusterConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "config",
			Namespace: parameters.OperatorNamespace,
		}, Spec: fecv1.SriovFecClusterConfigSpec{
			Nodes: []fecv1.NodeConfig{{
				NodeName: sriovFecNodeConfig.Name,
				PhysicalFunctions: []fecv1.PhysicalFunctionConfig{{
					PCIAddress: accelerator.PCIAddress,
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
