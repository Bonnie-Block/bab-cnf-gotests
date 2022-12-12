package machineconfigpool

import (
	"context"
	"fmt"
	"log"
	"time"

	performancev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"github.com/openshift/cluster-node-tuning-operator/pkg/performanceprofile/controller/performanceprofile/components"
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

// WaitForCondition waits until the machine config pool will have specified condition type with the expected status.
func WaitForCondition(
	clientSet *testclient.ClientSet,
	mcp *mcov1.MachineConfigPool,
	conditionType mcov1.MachineConfigPoolConditionType,
	timeout time.Duration,
) error {
	return wait.PollImmediate(3*time.Second, timeout, func() (bool, error) {
		mcpUpdated, err := clientSet.MachineConfigPools().Get(context.Background(), mcp.Name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}

		return isMcpInCondition(mcpUpdated, conditionType), nil
	})
}

// GetByLabel returns all MCPs with the specified label.
func GetByLabel(clientSet *testclient.ClientSet, key, value string) ([]mcov1.MachineConfigPool, error) {
	selector := labels.NewSelector()
	req, err := labels.NewRequirement(key, selection.Equals, []string{value})

	if err != nil {
		return nil, err
	}

	selector = selector.Add(*req)
	mcps := &mcov1.MachineConfigPoolList{}

	if err := clientSet.List(context.TODO(), mcps, &client.ListOptions{LabelSelector: selector}); err != nil {
		return nil, err
	}

	if len(mcps.Items) > 0 {
		return mcps.Items, nil
	}
	// fallback to look for a mcp with the same nodeselector.
	// key value may come from a node selector, so looking for a mcp
	// that targets the same nodes is legit
	if err := clientSet.List(context.TODO(), mcps); err != nil {
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

// GetByProfile returns the MCP by a given performance profile.
func GetByProfile(
	cs *testclient.ClientSet,
	performanceProfile *performancev2.PerformanceProfile) (*mcov1.MachineConfigPool, error) {
	mcpLabel := performanceProfile.Spec.MachineConfigLabel
	key, value := components.GetFirstKeyAndValue(mcpLabel)
	mcpsByLabel, err := GetByLabel(cs, key, value)

	if err != nil {
		return nil, err
	}

	return &mcpsByLabel[0], nil
}

// WaitForMcpUpdate waits for a mcp to be updating and then updated.
func WaitForMcpUpdate(clientSet *testclient.ClientSet, nodeLabel string) error {
	mcp := &mcov1.MachineConfigPool{}
	err := clientSet.Get(context.TODO(), client.ObjectKey{Name: nodeLabel}, mcp)

	if err != nil {
		return err
	}

	err = WaitForCondition(
		clientSet,
		&mcov1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Name: nodeLabel}},
		mcov1.MachineConfigPoolUpdating,
		2*time.Minute)
	if err != nil {
		return err
	}

	// We need to wait a long time here for the node to reboot
	err = WaitForCondition(
		clientSet,
		&mcov1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Name: nodeLabel}},
		mcov1.MachineConfigPoolUpdated,
		time.Duration(45*mcp.Status.MachineCount)*time.Minute)

	return err
}

// WaitForClusterStable waits for all machine config pools to stay in updated state for given stableDuration
// Set stableDuration to 0 to return immediately when all mcps are updated.
func WaitForClusterStable(cs *testclient.ClientSet, timeout, interval, stableDuration time.Duration) error {
	startTime := time.Now()
	errMcp := wait.PollImmediate(interval, timeout, func() (bool, error) {
		mcpList, err := cs.MachineConfigPools().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		for _, mcp := range mcpList.Items {
			if err != nil {
				return false, nil
			}
			if !isMcpInCondition(&mcp, mcov1.MachineConfigPoolUpdated) {
				// Reset timer for stable duration if mcp is not in expected state
				startTime = time.Now()

				return false, nil
			}
		}

		// All given MCPs are in expected state. Check for stable duration if it's larger than zero.
		extraMsg := ""
		if stableDuration > 0 {
			actualStableDuration := time.Since(startTime)
			// Add an interval because the timer started before mcp became updated
			if actualStableDuration < stableDuration+interval {
				return false, nil
			}
			extraMsg = fmt.Sprintf("for at least %s", stableDuration.String())
		}
		log.Println("All mcps are updated", extraMsg)

		return true, nil
	})

	return errMcp
}

// isMcpInCondition parses MCP conditions. Returns true if given MCP is in given condition, otherwise false.
func isMcpInCondition(mcp *mcov1.MachineConfigPool, condition mcov1.MachineConfigPoolConditionType) bool {
	for _, c := range mcp.Status.Conditions {
		if c.Type == condition && c.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}
