package ranhelper

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/k8snetworkplumbingwg/sriov-network-operator/pkg/render"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	testclient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
)

const (
	createMode = "create"
	deleteMode = "delete"
	updateMode = "update"
)

// ApplyObjects adds manifests from given directory to cluster.
func ApplyObjects(resDir string) error {
	return modifyObjects(createMode, resDir)
}

// DeleteObjects removes manifests from given directory from cluster.
func DeleteObjects(resDir string) error {
	return modifyObjects(deleteMode, resDir)
}

// DefineAPIClient creates new api client instance connected to given cluster.
func DefineAPIClient(kubeconfigEnvVar string) (*testclient.ClientSet, error) {
	kubeFilePath, present := os.LookupEnv(kubeconfigEnvVar)
	if !present {
		return nil, fmt.Errorf("can not load api client. Please check %s env var", kubeconfigEnvVar)
	}

	clients := testclient.New(kubeFilePath)
	if clients == nil {
		return nil, fmt.Errorf("client is not set please check %s env variable", kubeconfigEnvVar)
	}

	return clients, nil
}

// GetClusterName extracts the cluster name from provided kubeconfig. It assumes the there's exactly 1 cluster.
func GetClusterName(kubeconfigEnvVar string) (string, error) {
	kubeFilePath, present := os.LookupEnv(kubeconfigEnvVar)
	if !present {
		return "", fmt.Errorf("can not load api client. Please check '%s' env var", kubeconfigEnvVar)
	}

	rawConfig, _ := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeFilePath},
		&clientcmd.ConfigOverrides{
			CurrentContext: "",
		}).RawConfig()

	for _, cluster := range rawConfig.Clusters {
		// The cluster data looks like this:
		/*
			    "clusters": [
					{
						"name": "local-cluster",
						"cluster": {
							"server": "https://api.local-cluster.karmalabs.local:6443",
							"certificate-authority-data": "DATA+OMITTED"
						}
					}
				],
		*/
		// However the "name" field is not always correct so it is more consistent to instead
		// parse the name out from the server url
		splits := strings.Split(cluster.Server, ".")

		clusterName := splits[1]

		log.Println("cluster name: ", clusterName)

		return clusterName, nil
	}

	return "", fmt.Errorf("can not load api client. Please check '%s' env var", kubeconfigEnvVar)
}

// GetClusterVersion can be used to get the Openshift version from the provided cluster.
func GetClusterVersion(clusterClient *testclient.ClientSet) (string, error) {
	// Check if the client was even defined first
	if clusterClient == nil {
		return "", fmt.Errorf("provided client was not defined")
	}

	result, err := clusterClient.ConfigV1Interface.ClusterVersions().
		Get(context.Background(), "version", metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	histories := result.Status.History
	for i := len(histories) - 1; i >= 0; i-- {
		history := histories[i]
		if history.State == "Completed" {
			return history.Version, nil
		}
	}

	log.Println("Warning: No completed version found in clusterversion. Returning desired version")

	return result.Status.Desired.Version, nil
}

// IsClustersPresent can be used to check for the presence of specific clusters.
func IsClustersPresent(clients []*testclient.ClientSet) error {
	// Log the cluster list
	log.Println(clients)

	for _, client := range clients {
		if client == nil {
			return errors.New("provided nil client in cluster list")
		}
	}

	return nil
}

// IsVersionStringAtLeastVersionSpecified can be used to check if the provided version string is at least as high
// as the expected version string. Whether or not equality is permitted can also be specified.
// The input versions should be of the form of dot separated integers, e.g. "1.0.0".
func IsVersionStringAtLeastVersionSpecified(actualVersion string, expectedVersion string, allowEqual bool) bool {
	// If no actual version was provided then assume it would not match
	if actualVersion == "" {
		return false
	}

	// If no expected version was provided then assume it did match
	if expectedVersion == "" {
		return true
	}

	// Split the strings on the periods separating the version digits
	actualSplits := strings.Split(actualVersion, ".")
	expectedSplits := strings.Split(expectedVersion, ".")

	// Compare them digit by digit
	for splitIndex := 0; splitIndex < len(expectedSplits); splitIndex++ {
		// Check whether we allow equality as well as greater then
		if !allowEqual {
			if actualSplits[splitIndex] <= expectedSplits[splitIndex] {
				return false
			}
		} else {
			if actualSplits[splitIndex] < expectedSplits[splitIndex] {
				return false
			}
		}
	}

	return true
}

// PrintCr print any CR.
func PrintCr(b interface{}) error {
	customResource, err := yaml.Marshal(b)

	if err != nil {
		return err
	}

	log.Printf("--- generated CR dump:\n%s\n", string(customResource))

	return nil
}

// UpdateObjects updates existing resources based on manifests from given directory.
func UpdateObjects(resDir string) error {
	return modifyObjects(updateMode, resDir)
}

func modifyObjects(mode string, resDir string) error {
	data := render.MakeRenderData()
	objs, err := render.RenderDir(resDir, &data)

	if err != nil {
		return err
	}

	var errorList []error

	for _, obj := range objs {
		switch mode {
		case createMode:
			err = helper.Apiclient.Client.Create(context.TODO(), obj)
		case deleteMode:
			err = helper.Apiclient.Client.Delete(context.TODO(), obj)
		case updateMode:
			err = updateObject(obj)
		}

		if err != nil {
			errorList = append(errorList, err)
		}
	}

	if len(errorList) > 0 {
		return fmt.Errorf(
			"one or more errors occurred while processing resources from dir %s \n%v errors",
			resDir, errorList)
	}

	return nil
}

func updateObject(obj *unstructured.Unstructured) error {
	if obj.GetName() == "" {
		return errors.Errorf("Object %s has no name", obj.GroupVersionKind().String())
	}

	gvk := obj.GroupVersionKind()
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(gvk)
	err := helper.Apiclient.Client.Get(
		context.TODO(),
		types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()},
		existing)

	if err != nil {
		return err
	}

	obj.SetCreationTimestamp(existing.GetCreationTimestamp())
	obj.SetResourceVersion(existing.GetResourceVersion())
	obj.SetUID(existing.GetUID())
	obj.SetGeneration(existing.GetGeneration())
	obj.SetManagedFields(existing.GetManagedFields())
	obj.SetFinalizers(existing.GetFinalizers())

	return helper.Apiclient.Client.Update(context.TODO(), obj)
}

// IsContainerExistInPod check if a given contained, 'containerName', exists in a given pod, 'pod'.
// the function return 'true' if the container exists and 'false' if not.
func IsContainerExistInPod(pod k8sv1.Pod, containerName string) bool {
	containers := pod.Status.ContainerStatuses

	for _, container := range containers {
		if container.Name == containerName {
			log.Printf("found %s container\n", containerName)

			return true
		}
	}

	return false
}

// WaitForClusterReachable waits for cluster reachable by listing cluster nodes and expecting it to work.
func WaitForClusterReachable() error {
	log.Println("Waiting for cluster to be reachable")

	cond := conditionFunc
	err := wait.Poll(3*time.Second, 15*time.Minute, cond)

	if nil != err {
		return err
	}

	log.Println("Cluster is reachable")

	return nil
}

func conditionFunc() (bool, error) {
	apiTimeout := int64(30)

	_, err := helper.Apiclient.Nodes().List(context.Background(), metav1.ListOptions{TimeoutSeconds: &apiTimeout})
	if nil == err {
		return true, nil
	}

	return false, nil
}

// WaitForClusterRecover waits up to 45 minutes for all pods
// in `namespaces` projects in a given node `node` to recover.
func WaitForClusterRecover(node *k8sv1.Node, namespaces []string) error {
	// Wait for linux to be reachable via ping and record time
	interval := 5 * time.Second

	helper.WaitForNodeReachable(node)

	err := WaitForClusterReachable()
	if nil != err {
		return err
	}

	workloadStableDuration := 40 * time.Second

	unhealthyWorkloadPods := helper.WaitForAllPodsHealthy(
		namespaces,
		45*time.Minute,
		interval,
		workloadStableDuration,
	)

	if len(unhealthyWorkloadPods) != 0 {
		return fmt.Errorf("at least one pod was not recovered after 45 minutes")
	}

	return nil
}

// Assumes rsa key is imported.
func ExecSSHCommand(host string, user string, subcommands []string) (string, error) {
	// tip: for jumphost prepend args with ["-J","<jumpUser>>@<jumpIP>","-i", "<Final host's private key path>"]
	args := []string{"-o", "ConnectTimeout=10", "-o", "ControlMaster=auto", "-o", "ControlPersist=60s", "-o",
		"BatchMode=yes", "-o", "StrictHostKeyChecking=no", "-l", user, host}
	args = append(args, subcommands...)
	output, err := helper.ExecAndLogCommand(true, 1*time.Minute, "ssh", args...)

	return string(output), err
}

// GetWorker returns worker node.
// If preferMaster=true, it will try to find a node with both worker and master roles with best effort.
// If preferMaster=false, it will try to find a node without master role with best effort.
func GetWorker(preferSno bool) (*k8sv1.Node, error) {
	workers, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
	if err != nil {
		return nil, err
	}

	node := &workers[0]

	if len(workers) > 1 {
		masters, err := nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
		if err != nil {
			return nil, err
		}

		for _, worker := range workers {
			isSno := false

			for _, master := range masters {
				if worker.Name == master.Name {
					isSno = true

					break
				}
			}

			if isSno == preferSno {
				node = &worker

				break
			}
		}
	}

	return node, nil
}

// GetSnoNodes returns sno nodes with both master and worker roles in cluster.
func GetSnoNodes() ([]*k8sv1.Node, error) {
	masters, err := nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
	if err != nil {
		return nil, err
	}

	workers, err := nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
	if err != nil {
		return nil, err
	}

	var snoNodes []*k8sv1.Node

	for _, master := range masters {
		for _, worker := range workers {
			if worker.Name == master.Name {
				snoNodes = append(snoNodes, &worker)
			}
		}
	}

	return snoNodes, nil
}
