package ranhelper

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
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

// IsVersionStringInRange can be used to check if a version string is between a specified min and max value.
// All the string inputs to this function should be dot separated positive intergers, e.g. "1.0.0" or "4.10".
// Each string inputs must be at least two dot separarted integers but may also be 3 or more.
func IsVersionStringInRange(version, minimum, maximum string) bool {
	// First we will define a helper to validate that a string is at least two dot separated positive integers
	ValidateInputString := func(input string) (bool, []int) {
		// Split the input string on the dot character
		versionSplits := strings.Split(input, ".")

		// We need at least two digits to verify
		if len(versionSplits) < 2 {
			return false, []int{}
		}

		// Prepare a list of digits to return later
		digits := []int{}

		// Check if the first two splits are valid integers
		for i := 0; i < 2; i++ {
			// Attempt to convert the string to an integer
			digit, err := strconv.Atoi(versionSplits[i])
			if err != nil {
				return false, []int{}
			}

			// Save the digit
			digits = append(digits, digit)
		}

		// Otherwise if we've passed all the checks it is a valid version string
		return true, digits
	}

	versionValid, versionDigits := ValidateInputString(version)
	minimumValid, minimumDigits := ValidateInputString(minimum)
	maximumValid, maximumDigits := ValidateInputString(maximum)

	if !minimumValid {
		// We don't want any bad minimums to be passed at all, so panic if minimum wasn't an empty string
		if minimum != "" {
			panic(fmt.Errorf("invalid minimum provided: '%s'", minimum))
		}

		// Assume the minimum digits are [0,0] for later comparison
		minimumDigits = []int{0, 0}
	}

	if !maximumValid {
		// We don't want any bad maximums to be passed at all, so panic if maximum wasn't an empty string
		if maximum != "" {
			panic(fmt.Errorf("invalid maximum provided: '%s'", maximum))
		}

		// Assume the maximum digits are [math.MaxInt, math.MaxInt] for later comparison
		maximumDigits = []int{math.MaxInt, math.MaxInt}
	}

	// If the version was not valid then we need to check the min and max
	if !versionValid {
		// If no min or max was defined then return true
		if !minimumValid && !maximumValid {
			return true
		}

		// Otherwise return whether the input maximum was an empty string or not
		return maximum == ""
	}

	// Otherwise the versions were valid so compare the digits
	for i := 0; i < 2; i++ {
		// The version bit should be between the minimum and maximum
		if versionDigits[i] < minimumDigits[i] || versionDigits[i] > maximumDigits[i] {
			return false
		}
	}

	// At the end if we never returned then all the digits were in valid range
	return true
}

// GetOperatorVersionFromCSV parses the ClusterServiceVersions resource to obtain the installed operator version.
// This resource will be populated only when installing operators from the Operator Hub.
// The returned value here is the same as you would see in the Operator Hub, e.g. "4.11.2".
func GetOperatorVersionFromCSV(client *testclient.ClientSet, operatorName, operatorNamespace string) (string, error) {
	// Check if the client is valid
	if client == nil {
		return "", fmt.Errorf("provided nil client")
	}

	// Get the CSV objects
	csvs, err := client.ClusterServiceVersions(operatorNamespace).
		List(context.TODO(), metav1.ListOptions{})

	// Check for any error getting the CSVs
	if err != nil {
		return "", err
	}

	// Find the CSV that matches the operator
	for _, csv := range csvs.Items {
		if strings.Contains(csv.Name, operatorName) {
			return csv.Spec.Version.String(), nil
		}
	}

	return "", nil
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

// GetNodeByName gets a node by its name.
func GetNodeByName(name string) (*k8sv1.Node, error) {
	return helper.Apiclient.Nodes().Get(context.Background(), name, metav1.GetOptions{})
}
