package nodes

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
)

// NodesSelector represent the label selector used to filter impacted nodes.
var NodesSelector string

const (
	// LabelRole contains the key for the role label.
	LabelRole = "node-role.kubernetes.io"
)

func init() {
	cfgData, err := config.NewConfig()
	if err != nil {
		log.Fatalf("Error in getting configuration: %v", err)
	}

	NodesSelector = cfgData.General.CnfNodeLabel
}

// NodeInterface represent the interface connected to node.
type NodeInterface struct {
	Name     string
	Physical bool
	UP       bool
	Bridge   bool
	DefRoute bool
}

// MatchingOptionalSelectorState filter the given slice with only the nodes matching the optional selector.
// If no selector is set, it returns the same list.
// The NODES_SELECTOR must be in the form of label=value.
// For example: NODES_SELECTOR="sctp=true".
func MatchingOptionalSelectorState(
	clients *client.ClientSet, toFilter []sriovv1.SriovNetworkNodeState) ([]sriovv1.SriovNetworkNodeState, error) {
	if NodesSelector == "" {
		return toFilter, nil
	}

	toMatch, err := clients.Nodes().List(context.Background(), metav1.ListOptions{
		LabelSelector: NodesSelector,
	})

	if err != nil {
		return nil, fmt.Errorf("error in getting nodes matching %s, %w", NodesSelector, err)
	}

	if len(toMatch.Items) == 0 {
		return nil, fmt.Errorf("failed to get nodes matching %s, %w", NodesSelector, err)
	}

	res := make([]sriovv1.SriovNetworkNodeState, 0)

	for _, n := range toFilter {
		for _, m := range toMatch.Items {
			if n.Name == m.Name {
				res = append(res, n)
			}
		}
	}

	if len(res) == 0 {
		return nil, fmt.Errorf("failed to find matching nodes with %s", NodesSelector)
	}

	return res, nil
}

// MatchingOptionalSelector filter the given slice with only the nodes matching the optional selector.
// If no selector is set, it returns the same list.
// The NODES_SELECTOR must be set with a labelselector expression.
// For example: NODES_SELECTOR="sctp=true".
func MatchingOptionalSelector(clients *client.ClientSet, toFilter []corev1.Node) ([]corev1.Node, error) {
	if NodesSelector == "" {
		return toFilter, nil
	}

	toMatch, err := clients.Nodes().List(context.Background(), metav1.ListOptions{
		LabelSelector: NodesSelector,
	})

	if err != nil {
		return nil, fmt.Errorf("error in getting nodes matching the %s label selector, %w", NodesSelector, err)
	}

	if len(toMatch.Items) == 0 {
		return nil, fmt.Errorf("failed to get nodes matching %s label selector", NodesSelector)
	}

	res := make([]corev1.Node, 0)

	for _, n := range toFilter {
		for _, m := range toMatch.Items {
			if n.Name == m.Name {
				res = append(res, n)

				break
			}
		}
	}

	if len(res) == 0 {
		return nil, fmt.Errorf("failed to find matching nodes with %s label selector", NodesSelector)
	}

	return res, nil
}

// GetByRole returns all nodes with the specified role.
func GetByRole(cs *client.ClientSet, role string) ([]corev1.Node, error) {
	nodes, err := cs.Nodes().List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s/%s=", LabelRole, role),
	})
	if err != nil {
		return nil, err
	}

	return nodes.Items, nil
}

// GetPhysicalNodeInterfaces return list of interfaces.
func GetPhysicalNodeInterfaces(clientSet *client.ClientSet, node string) ([]NodeInterface, error) {
	defer func() {
		_ = namespaces.CleanPods("default", clientSet)
	}()

	cfgData, err := config.NewConfig()

	if err != nil {
		return nil, err
	}

	privilegedPod := pod.RedefineWithHostNetwork(pod.DefinePodOnNode("default", cfgData.Network.TestContainerImage, node))
	runningPrivilegedPod, err := clientSet.Pods("default").Create(
		context.Background(),
		privilegedPod,
		metav1.CreateOptions{},
	)

	if err != nil {
		return nil, err
	}

	var podStatus *corev1.Pod

	for start := time.Now(); time.Since(start) < time.Second*120; {
		podStatus, _ = clientSet.Pods("default").Get(context.Background(), runningPrivilegedPod.Name, metav1.GetOptions{})
		if podStatus.Status.Phase == corev1.PodRunning {
			break
		}

		time.Sleep(3 * time.Second)
	}

	if podStatus.Status.Phase != corev1.PodRunning {
		return nil, fmt.Errorf("can not run privileged pod")
	}

	interfaces, err := pod.ExecCommand(clientSet, *runningPrivilegedPod, []string{"ls", "-l", "/sys/class/net/"})
	if err != nil {
		return nil, err
	}

	interfaceLinksStatus, err := pod.ExecCommand(clientSet, *runningPrivilegedPod, []string{"ip", "link", "show"})
	if err != nil {
		return nil, err
	}

	defaultRoute, err := pod.ExecCommand(clientSet, *runningPrivilegedPod, []string{"ip", "route", "show", "0.0.0.0/0"})
	if err != nil {
		return nil, err
	}

	var nodeInterfaces []NodeInterface

	for _, oneInterface := range strings.Split(interfaces.String(), "\n") {
		nodeInterface := new(NodeInterface)
		splitedInterfaceSting := strings.Split(oneInterface, "/")
		interfaceName := strings.ReplaceAll(splitedInterfaceSting[len(splitedInterfaceSting)-1], "\r", "")
		nodeInterface.Name = interfaceName

		if !strings.Contains(oneInterface, "virtual") {
			nodeInterface.Physical = true
		}

		if len(splitedInterfaceSting) > 1 {
			for _, interfaceInfo := range strings.Split(interfaceLinksStatus.String(), "ff:ff:ff:ff:ff:ff") {
				if strings.Contains(interfaceInfo, interfaceName) {
					if strings.Contains(interfaceInfo, "state UP") {
						nodeInterface.UP = true
					}

					if strings.Contains(interfaceInfo, "master") {
						nodeInterface.Bridge = true
					}

					if strings.Contains(defaultRoute.String(), interfaceName) {
						nodeInterface.DefRoute = true
					}
				}
			}
		}
		nodeInterfaces = append(nodeInterfaces, *nodeInterface)
	}

	return nodeInterfaces, nil
}

// LabelNode set label (key & value) to a node.
func LabelNode(clientSet *client.ClientSet, nodeName, key, value string) (*corev1.Node, error) {
	NodeObject, err := clientSet.Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	NodeObject.Labels[key] = value
	NodeObject.CreationTimestamp = metav1.Time{}
	NodeObject.ResourceVersion = ""
	NodeObject, err = clientSet.Nodes().Update(context.Background(), NodeObject, metav1.UpdateOptions{})

	if err != nil {
		return nil, err
	}

	return NodeObject, nil
}

// GetByLabel retrieves Node list by label.
func GetByLabel(cs *client.ClientSet, label string) (*corev1.NodeList, error) {
	nodeList, err := cs.Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: label})
	if err != nil {
		return nil, err
	}

	return nodeList, nil
}

func IsSingleNodeCluster(clientSet *client.ClientSet) (bool, error) {
	// Check if cluster contains one node only which has both master and worker roles.
	masters, err := GetByRole(clientSet, parameters.RoleMaster)
	if err != nil || len(masters) != 1 {
		return false, err
	}

	workers, err := GetByRole(clientSet, parameters.RoleWorker)

	if err != nil || len(workers) != 1 || workers[0].Name != masters[0].Name {
		return false, err
	}

	return true, nil
}

// WaitForNodesReady waits for all nodes become ready.
func WaitForNodesReady(clientSet *client.ClientSet, timeout, interval time.Duration) error {
	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		nodesList, err := clientSet.Nodes().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		for _, node := range nodesList.Items {
			if !IsNodeInCondition(&node, corev1.NodeReady) {
				return false, nil
			}
		}
		log.Println("All nodes are Ready")

		return true, nil
	})
}

// IsNodeInCondition parses node conditions. Returns true if node is in given condition, otherwise false.
func IsNodeInCondition(node *corev1.Node, condition corev1.NodeConditionType) bool {
	for _, c := range node.Status.Conditions {
		if c.Type == condition && c.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}
