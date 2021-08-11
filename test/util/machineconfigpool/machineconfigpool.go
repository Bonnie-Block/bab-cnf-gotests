package machineconfigpool

import (
	"context"
	"time"

	performancev2 "github.com/openshift-kni/performance-addon-operators/api/v2"
	"github.com/openshift-kni/performance-addon-operators/pkg/controller/performanceprofile/components"
	mcov1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	testclient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
)

type Contents struct {
	Source       string
	Verification []string `json:"verification:"`
}
type Files struct {
	Contents   *Contents
	Filesystem string
	Mode       int
	Path       string
}
type Ingnition struct {
	Version string
}
type Storage struct {
	Files []Files
}
type McpConfig struct {
	Ingnition *Ingnition
	Storage   *Storage
}

// WaitForCondition waits until the machine config pool will have specified condition type with the expected status
func WaitForCondition(
	cs *testclient.ClientSet,
	mcp *mcov1.MachineConfigPool,
	conditionType mcov1.MachineConfigPoolConditionType,
	conditionStatus corev1.ConditionStatus,
	timeout time.Duration,
) error {
	return wait.PollImmediate(10*time.Second, timeout, func() (bool, error) {
		mcpUpdated, err := cs.MachineConfigPools().Get(context.Background(), mcp.Name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}

		for _, c := range mcpUpdated.Status.Conditions {
			if c.Type == conditionType && c.Status == conditionStatus {
				return true, nil
			}
		}
		return false, nil
	})
}

// GetByLabel returns all MCPs with the specified label
func GetByLabel(cs *testclient.ClientSet, key, value string) ([]mcov1.MachineConfigPool, error) {
	selector := labels.NewSelector()
	req, err := labels.NewRequirement(key, selection.Equals, []string{value})
	if err != nil {
		return nil, err
	}
	selector = selector.Add(*req)
	mcps := &mcov1.MachineConfigPoolList{}
	if err := cs.List(context.TODO(), mcps, &client.ListOptions{LabelSelector: selector}); err != nil {
		return nil, err
	}
	if len(mcps.Items) > 0 {
		return mcps.Items, nil
	}
	// fallback to look for a mcp with the same nodeselector.
	// key value may come from a node selector, so looking for a mcp
	// that targets the same nodes is legit
	if err := cs.List(context.TODO(), mcps); err != nil {
		return nil, err
	}
	var res []mcov1.MachineConfigPool
	for _, item := range mcps.Items {
		if item.Spec.NodeSelector.MatchLabels[key] == value {
			res = append(res, item)
		}
		nodeRoleKey := components.NodeRoleLabelPrefix + value

		if _, ok := item.Spec.NodeSelector.MatchLabels[nodeRoleKey]; ok {
			res = append(res, item)
		}
	}
	return res, nil
}

// GetByProfile returns the MCP by a given performance profile
func GetByProfile(cs *testclient.ClientSet, performanceProfile *performancev2.PerformanceProfile) (*mcov1.MachineConfigPool, error) {
	mcpLabel := performanceProfile.Spec.MachineConfigLabel
	key, value := components.GetFirstKeyAndValue(mcpLabel)
	mcpsByLabel, err := GetByLabel(cs, key, value)
	if err != nil {
		return nil, err
	}
	return &mcpsByLabel[0], nil
}
