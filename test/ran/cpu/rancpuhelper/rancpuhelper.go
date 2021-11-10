package rancpuhelper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	performancev2 "github.com/openshift-kni/performance-addon-operators/api/v2"
	"github.com/openshift-kni/performance-addon-operators/pkg/controller/performanceprofile/components"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/cpu/rancpuparameters"
	podUtil "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/machineconfigpool"
)

func GetThreadSiblingsList(cpu int, node *corev1.Node) ([]int, error) {
	cmd := []string{
		"cat",
		fmt.Sprintf(ran.ThreadSiblingsListPath, cpu),
	}
	output, err := helper.ExecCommandOnNode(node, cmd)
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
			output, err := helper.ExecCommandOnNode(nodes[0], []string{"uname", "-r"})
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

// RunMustGather runs must-gather and returns the dir the cmd gets executed from, must-gather output, and error if any.
func RunMustGather() (mustGatherExecDir string, mustGatherOutput []byte, err error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", nil, err
	}
	output, err := helper.ExecAndLogCommand(true, 45*time.Minute, "oc", "adm", "must-gather")
	return dir, output, err
}

// DeleteMustGathers deletes given must-gather dir.
// If mustGatherExecDir is an empty string, then current work directory will be checked.
func DeleteMustGathers(mustGatherExecDir string) error {
	// Look for must-gathers under current dir if mustGatherExecDir is empty
	if mustGatherExecDir == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return err
		}
		mustGatherExecDir = currentDir
	}

	matches, err := filepath.Glob(fmt.Sprintf("%s/must-gather.local.*", mustGatherExecDir))
	if err != nil {
		return err
	}
	if matches != nil {
		log.Println("Must-gather dirs to be removed:", matches)
	}
	for _, match := range matches {
		// Best effort
		err = os.RemoveAll(match)
	}
	// Returns last error only
	return err
}

// Execute a command in Prometheus pod and returns output and error.
func execCommandInPromPod(command []string, logCommand bool) ([]byte, error) {
	promPod, err := helper.Apiclient.Pods(ran.PromNamespace).Get(context.TODO(), ran.PromPodName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if logCommand {
		log.Println("Command in prom pod:", command)
	}
	bytes, err := podUtil.ExecCommand(helper.Apiclient, *promPod, command)
	if err != nil {
		log.Printf("err: %v", err)
		return nil, err
	} else {
		if logCommand {
			log.Printf("output: %s", bytes.String())
		}
		return bytes.Bytes(), err
	}
}

// ExecPromQuery returns rest response for given prom query
// Note: "bash -c" is used, thus even if curl command failed, the error will be in the first return value
func ExecPromQuery(query string, logCommand bool) ([]rancpuparameters.PromMetric, error) {
	command := []string{
		"bash", "-c",
		fmt.Sprintf("curl \"-s\" '%squery' --data-urlencode 'query=%s'; echo", ran.PromLocalUrl, query),
	}
	output, err := execCommandInPromPod(command, logCommand)
	if err != nil {
		return nil, err
	}

	var response rancpuparameters.PromQueryResponse
	err = json.Unmarshal(output, &response)
	if err != nil {
		return nil, err
	}
	if response.Status != "success" {
		return nil, fmt.Errorf(response.Error)
	}

	result := response.Data.Result
	return result, nil
}

// GetEnv retrieves the value of the environment variable named by the key.
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func IsOcExist() bool {
	_, err := helper.ExecAndLogCommand(true, 10*time.Second, "oc", "version")
	return err == nil
}
