package ranhelper

import (
	"context"
	"fmt"
	"os"

	testclient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/wait"

	"log"
	"time"

	"github.com/k8snetworkplumbingwg/sriov-network-operator/pkg/render"
	"github.com/pkg/errors"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
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
