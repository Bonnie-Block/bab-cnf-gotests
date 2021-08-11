package rancpuhelper

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	performancev2 "github.com/openshift-kni/performance-addon-operators/api/v2"
	"github.com/openshift-kni/performance-addon-operators/pkg/controller/performanceprofile/components"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/machineconfigpool"
)

func GetThreadSiblingsList(cpu int, node *corev1.Node) ([]int, error) {
	cmd := []string{
		"cat",
		fmt.Sprintf(ran.ThreadSiblingsListPath, cpu),
	}
	output, err := ranhelper.ExecCommandOnNode(node, cmd)
	if err != nil {
		return nil, err
	}

	// Parse string output to int list. e.g., "0,40" > [0 40]
	cpuStringList := strings.Split(output, ",")
	var cpuList []int
	for _, cpuString := range cpuStringList {
		cpu, err := strconv.Atoi(strings.TrimSpace(cpuString))
		if err != nil {
			return nil, err
		}
		cpuList = append(cpuList, cpu)
	}

	return cpuList, err
}

func GetRTPerformanceProfile() (*performancev2.PerformanceProfile, error) {
	profiles := &performancev2.PerformanceProfileList{}
	if err := helper.Apiclient.List(context.TODO(), profiles); err != nil {
		return nil, err
	}

	for _, profile := range profiles.Items {
		if profile.Spec.RealTimeKernel != nil &&
			profile.Spec.RealTimeKernel.Enabled != nil &&
			*profile.Spec.RealTimeKernel.Enabled == true {
			return &profile, nil
		}
	}

	// FIXME: workaround to get a performance profile without rt kernel set. As rt kernel is now side-loaded on SNO
	// deployment pipeline due to blocking kernel bug.
	for _, profile := range profiles.Items {
		nodes, _ := GetNodesFromPerformanceProfile(&profile)
		if len(nodes) > 0 {
			output, err := ranhelper.ExecCommandOnNode(nodes[0], []string{"uname", "-r"})
			if err != nil {
				return nil, err
			}
			if strings.Contains(strings.Trim(output, "\n"), ".rt") {
				return &profile, nil
			}
		}
	}

	return nil, fmt.Errorf("RT Profile is not found")
}

// GetNodesFromPerformanceProfile returns list of nodes that are applicable to the given performance profile
func GetNodesFromPerformanceProfile(profile *performancev2.PerformanceProfile) ([]*corev1.Node, error) {
	var nodes []*corev1.Node

	// Check machineConfigPoolSelector in performance profile
	var mcpNodes *corev1.NodeList
	var mcpNodesErr error
	mcp, _ := machineconfigpool.GetByProfile(helper.Apiclient, profile)
	if mcp != nil {
		mcpNodeSelector := mcp.Spec.NodeSelector.String()
		mcpNodes, mcpNodesErr = helper.Apiclient.Nodes().List(context.TODO(), metav1.ListOptions{LabelSelector: mcpNodeSelector})
		if mcpNodes == nil {
			return nil, mcpNodesErr
		} else if len(mcpNodes.Items) == 0 {
			return nil, fmt.Errorf("No nodes found in machine config pool %s\n", mcp.Name)
		}
	}

	// Check nodeSelector in performance profile
	var profileNodes *corev1.NodeList
	var profileNodesErr error
	profileNodeSelector := profile.Spec.NodeSelector
	if profileNodeSelector != nil {
		key, value := components.GetFirstKeyAndValue(profileNodeSelector)
		profileNodes, profileNodesErr = helper.Apiclient.Nodes().List(context.TODO(),
			metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", key, value)})

		if profileNodes == nil {
			return nil, profileNodesErr
		} else if len(profileNodes.Items) == 0 {
			return nil, fmt.Errorf("No nodes found using profile node selector %s\n", profileNodeSelector)
		}

		if mcpNodes == nil {
			for _, node := range profileNodes.Items {
				nodes = append(nodes, &node)
			}
		} else {
			for _, profileNode := range profileNodes.Items {
				for _, mcpNode := range mcpNodes.Items {
					if profileNode.Name == mcpNode.Name {
						nodes = append(nodes, &mcpNode)
					}
				}
			}
			if len(nodes) == 0 {
				return nil, fmt.Errorf("No common nodes between profile Node Selector and MCP Selector\n")
			}
		}
	}

	return nodes, nil
}
