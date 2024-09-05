package helper

import (
	"context"
	"fmt"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/schemes/ptp/ptpv1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

type NodeTopology struct {
	NodeName      string
	InterfaceList []string
	NodeObject    *corev1.Node
}

// CleanAllPtpConfig removes any configuration applied by ptp tests.
func CleanAllPtpConfig(
	operatorNamespace string,
	ptpGrandmasterNodeLabel string,
	ptpSlaveNodeLabel string) error {
	err := PTPClean(operatorNamespace)
	if err != nil {
		return err
	}

	nodeList, err := nodes.GetByLabel(Apiclient, ptpGrandmasterNodeLabel)
	if err != nil {
		return fmt.Errorf("helper.CleanAllPtpConfig: Failed to retrieve grandmaster node list %w", err)
	}

	for _, node := range nodeList.Items {
		delete(node.Labels, ptpGrandmasterNodeLabel)

		_, err = Apiclient.Nodes().Update(context.Background(), &node, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("helper.CleanAllPtpConfig: Failed to remove label from %s %w", node.Name, err)
		}
	}

	nodeList, err = nodes.GetByLabel(Apiclient, ptpSlaveNodeLabel)
	if err != nil {
		return fmt.Errorf("helper.CleanAllPtpConfig: Failed to retrieve slave node list %w", err)
	}

	for _, node := range nodeList.Items {
		delete(node.Labels, ptpSlaveNodeLabel)

		_, err = Apiclient.Nodes().Update(context.Background(), &node, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("helper.CleanAllPtpConfig: Failed to remove label from %s %w", node.Name, err)
		}
	}

	return nil
}

// PtpEnabled returns the topology of a given node, filtering using the given selector.
func PtpEnabled(ptpOperatorNamespace string) ([]NodeTopology, error) {
	nodeDevicesList, err := Apiclient.NodePtpDevices(ptpOperatorNamespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	if len(nodeDevicesList.Items) == 0 {
		return nil, fmt.Errorf("zero nodes found")
	}

	nodeTopologyList := []NodeTopology{}

	nodesList, err := MatchingOptionalSelectorPTP(nodeDevicesList.Items)
	if err != nil {
		return nil, err
	}

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
// For example: NODES_SELECTOR="sctp=true".
func MatchingOptionalSelectorPTP(toFilter []ptpv1.NodePtpDevice) ([]ptpv1.NodePtpDevice, error) {
	if nodes.NodesSelector == "" {
		return toFilter, nil
	}

	toMatch, err := nodes.GetByLabel(Apiclient, nodes.NodesSelector)

	if err != nil {
		return nil, fmt.Errorf("error in getting nodes matching the %s label selector, %w", nodes.NodesSelector, err)
	}

	if len(toMatch.Items) == 0 {
		return nil, fmt.Errorf("failed to get nodes matching %s label selector", nodes.NodesSelector)
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
		return nil, fmt.Errorf("failed to find matching nodes with %s label selector", nodes.NodesSelector)
	}

	return res, nil
}

// PTPClean removes all PtpConfig from cluster.
func PTPClean(operatorNamespace string) error {
	ptpConfigList, err := getPtpConfigsByNamespace(operatorNamespace)
	if err != nil {
		return fmt.Errorf("helper.CleanAllPtpConfig: Failed to retrieve ptp config list %w", err)
	}

	for _, ptpConfig := range ptpConfigList.Items {
		err = Apiclient.PtpConfigs(operatorNamespace).Delete(context.Background(), ptpConfig.Name, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("helper.CleanAllPtpConfig: Failed to delete ptp config %s %w", ptpConfig.Name, err)
		}
	}

	err = wait.PollImmediate(5, 20, func() (done bool, err error) {
		ptpConfigList, err = getPtpConfigsByNamespace(operatorNamespace)
		if err != nil {
			return true, err
		}
		if len(ptpConfigList.Items) == 0 {
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		return fmt.Errorf("helper.CleanAllPtpConfig: Failed to list ptp config  %w", err)
	}

	return nil
}

// GetPtpInterfaces collects and sorts ptp interfaces based on input.
func GetPtpInterfaces(
	config *config.Config,
	requestedNumber int,
	operatorNamespace string) ([]string, error) {
	var validPtpInterfacesList []string

	sriovInfos, err := cluster.DiscoverSriov(Apiclient, operatorNamespace)

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

// getPtpConfigsByNamespace retrieves PtpConfig list by namespace.
func getPtpConfigsByNamespace(namespace string) (*ptpv1.PtpConfigList, error) {
	ptpConfigList, err := Apiclient.PtpConfigs(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return ptpConfigList, nil
}
