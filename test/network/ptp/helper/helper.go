package helper

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/wait"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/ptp/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"

	ptpv1 "github.com/openshift/ptp-operator/pkg/apis/ptp/v1"
	corev1 "k8s.io/api/core/v1"
)

// CleanAllPtpConfig removes any configuration applied by ptp tests.
func CleanAllPtpConfig(cs *client.ClientSet, operatorNamespace string) error {
	err := Clean(cs, operatorNamespace)
	if err != nil {
		return err
	}

	nodeList, err := nodes.GetByLabel(cs, parameters.PtpGrandmasterNodeLabel)
	if err != nil {
		return fmt.Errorf("helper.CleanAllPtpConfig: Failed to retrieve grandmaster node list %v", err)
	}
	for _, node := range nodeList.Items {
		delete(node.Labels, parameters.PtpGrandmasterNodeLabel)
		_, err = cs.Nodes().Update(context.Background(), &node, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("helper.CleanAllPtpConfig: Failed to remove label from %s %v", node.Name, err)
		}
	}

	nodeList, err = nodes.GetByLabel(cs, parameters.PtpSlaveNodeLabel)
	if err != nil {
		return fmt.Errorf("helper.CleanAllPtpConfig: Failed to retrieve slave node list %v", err)
	}
	for _, node := range nodeList.Items {
		delete(node.Labels, parameters.PtpSlaveNodeLabel)
		_, err = cs.Nodes().Update(context.Background(), &node, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("helper.CleanAllPtpConfig: Failed to remove label from %s %v", node.Name, err)
		}
	}
	return nil
}

func Clean(cs *client.ClientSet, operatorNamespace string) error {
	ptpConfigList, err := getPtpConfigsByNamespace(cs, operatorNamespace)
	if err != nil {
		return fmt.Errorf("helper.CleanAllPtpConfig: Failed to retrieve ptp config list %v", err)
	}
	for _, ptpConfig := range ptpConfigList.Items {
		err = cs.PtpConfigs(operatorNamespace).Delete(context.Background(), ptpConfig.Name, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("helper.CleanAllPtpConfig: Failed to delete ptp config %s %v", ptpConfig.Name, err)
		}
	}
	err = wait.PollImmediate(5, 20, func() (done bool, err error) {
		ptpConfigList, err = getPtpConfigsByNamespace(cs, operatorNamespace)
		if err != nil {
			return true, err
		}
		if len(ptpConfigList.Items) == 0 {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("helper.CleanAllPtpConfig: Failed to list ptp config  %v", err)
	}
	return nil
}

type NodeTopology struct {
	NodeName      string
	InterfaceList []string
	NodeObject    *corev1.Node
}

// PtpEnabled returns the topology of a given node, filtering using the given selector.
func PtpEnabled(client *client.ClientSet) ([]NodeTopology, error) {
	nodeDevicesList, err := client.NodePtpDevices(parameters.OperatorNamespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	if len(nodeDevicesList.Items) == 0 {
		return nil, fmt.Errorf("Zero nodes found")
	}

	nodeTopologyList := []NodeTopology{}

	nodesList, err := MatchingOptionalSelectorPTP(client, nodeDevicesList.Items)
	for _, node := range nodesList {
		if len(node.Status.Devices) > 0 {
			interfaceList := []string{}
			for _, iface := range node.Status.Devices {
				interfaceList = append(interfaceList, iface.Name)
			}
			nodeTopology := NodeTopology{NodeName: node.Name, InterfaceList: interfaceList}
			nodeTopologyList = append(nodeTopologyList, nodeTopology)
		}
	}

	return nodeTopologyList, nil
}

// MatchingOptionalSelectorPTP filter the given slice with only the nodes matching the optional selector.
// If no selector is set, it returns the same list.
// The NODES_SELECTOR must be set with a labelselector expression.
// For example: NODES_SELECTOR="sctp=true"
func MatchingOptionalSelectorPTP(cs *client.ClientSet, toFilter []ptpv1.NodePtpDevice) ([]ptpv1.NodePtpDevice, error) {
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

	res := make([]ptpv1.NodePtpDevice, 0)
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

// FindPtpConfigsByNamespace retrieves PtpConfig list by namespace
func getPtpConfigsByNamespace(cs *client.ClientSet, namespace string) (*ptpv1.PtpConfigList, error) {
	ptpConfigList, err := cs.PtpConfigs(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return ptpConfigList, nil
}

// GetPtpInterfaces returns list of requested interfaces
func  GetPtpInterfaces(config *config.Config, apiclient *client.ClientSet, requestedNumber int)([] string, error) {
	var validPtpInterfacesList []string
	sriovInfos, err := cluster.DiscoverSriov(apiclient, "openshift-sriov-network-operator")
	if err != nil {
		return nil, err
	}
	sriovInterfaces, err := sriovInfos.FindSriovDevices(sriovInfos.Nodes[0])
	if err != nil {
		return nil, err
	}

	validSriovInterfaces, err := config.GetSriovInterfaces(sriovInterfaces, requestedNumber)
	if err != nil {
		return nil, err
	}
	for _, validSriovInterface := range validSriovInterfaces {
		validPtpInterfacesList = append(validPtpInterfacesList, validSriovInterface.Name)
	}

	return validPtpInterfacesList, nil
}
