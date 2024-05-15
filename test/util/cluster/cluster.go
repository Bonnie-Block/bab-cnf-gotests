package cluster

import (
	"context"
	"fmt"
	"strings"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	testclient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

// EnabledNodes provides info on sriov enabled nodes of the cluster.
type EnabledNodes struct {
	Nodes  []string
	States map[string]sriovv1.SriovNetworkNodeState
}

var (
	supportedDrivers = []string{"mlx5_core", "i40e", "ixgbe", "ice"}
	supportedDevices = []string{"1583", "1593", "158b", "10fb", "1015", "1017"}
)

// DiscoverSriov retrieves Sriov related information of a given cluster.
func DiscoverSriov(clients *testclient.ClientSet, operatorNamespace string) (*EnabledNodes, error) {
	nodeStates, err := clients.SriovNetworkNodeStates(operatorNamespace).List(context.Background(), metav1.ListOptions{})
	res := &EnabledNodes{}
	res.States = make(map[string]sriovv1.SriovNetworkNodeState)
	res.Nodes = make([]string, 0)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve note states %w", err)
	}

	ss, err := nodes.MatchingOptionalSelectorState(clients, nodeStates.Items)
	if err != nil {
		return nil, fmt.Errorf("failed to find matching node states %w", err)
	}

	for _, state := range ss {
		if !stateStable(state) {
			return nil, fmt.Errorf("sync status still in progress")
		}

		node := state.Name

		for _, itf := range state.Status.Interfaces {
			if IsDriverSupported(itf.Driver) {
				res.Nodes = append(res.Nodes, node)
				res.States[node] = state

				break
			}
		}
	}

	if len(res.Nodes) == 0 {
		return nil, fmt.Errorf("no sriov enabled node found")
	}

	return res, nil
}

// FindOneSriovDevice retrieves a valid sriov device for the given node.
func (n *EnabledNodes) FindOneSriovDevice(node string) (*sriovv1.InterfaceExt, error) {
	s, ok := n.States[node]
	if !ok {
		return nil, fmt.Errorf("node %s not found", node)
	}

	for _, itf := range s.Status.Interfaces {
		if IsDriverSupported(itf.Driver) && isDeviceSupported(itf.DeviceID) {
			return &itf, nil
		}
	}

	return nil, fmt.Errorf("unable to find sriov devices in node %s", node)
}

// FindSriovDevices retrieves all valid sriov devices for the given node.
func (n *EnabledNodes) FindSriovDevices(node string) ([]*sriovv1.InterfaceExt, error) {
	devices := []*sriovv1.InterfaceExt{}
	state, ok := n.States[node]

	if !ok {
		return nil, fmt.Errorf("node %s not found", node)
	}

	for i, itf := range state.Status.Interfaces {
		if IsDriverSupported(itf.Driver) {
			devices = append(devices, &state.Status.Interfaces[i])
		}
	}

	return devices, nil
}

// FindOneMellanoxSriovDevice retrieves a valid sriov device for the given node.
func (n *EnabledNodes) FindOneMellanoxSriovDevice(node string) (*sriovv1.InterfaceExt, error) {
	s, ok := n.States[node]
	if !ok {
		return nil, fmt.Errorf("node %s not found", node)
	}

	for _, itf := range s.Status.Interfaces {
		if itf.Driver == "mlx5_core" {
			return &itf, nil
		}
	}

	return nil, fmt.Errorf("unable to find a mellanox sriov devices in node %s", node)
}

// SriovStable tells if all the node states are in sync (and the cluster is ready for another round of tests).
func SriovStable(operatorNamespace string, clients *testclient.ClientSet) (bool, error) {
	nodeStates, err := clients.SriovNetworkNodeStates(operatorNamespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to fetch nodes state %w", err)
	}

	if len(nodeStates.Items) == 0 {
		return false, nil
	}

	for _, state := range nodeStates.Items {
		if !stateStable(state) {
			return false, nil
		}
	}

	return true, nil
}

func stateStable(
	state sriovv1.SriovNetworkNodeState) bool {
	switch state.Status.SyncStatus {
	case "Succeeded":
		return true
	// When the config daemon is restarted the status will be empty
	// This doesn't mean the config was applied
	case "":
		return false
	}

	return false
}

func IsDriverSupported(driver string) bool {
	for _, supportedDriver := range supportedDrivers {
		if strings.Contains(driver, supportedDriver) {
			return true
		}
	}

	return false
}

func isDeviceSupported(deviceID string) bool {
	for _, d := range supportedDevices {
		if deviceID == d {
			return true
		}
	}

	return false
}

func IsClusterStable(clients *testclient.ClientSet) (bool, error) {
	nodes, err := clients.Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return false, err
	}

	for _, node := range nodes.Items {
		if node.Spec.Unschedulable {
			return false, nil
		}
	}

	return true, nil
}
