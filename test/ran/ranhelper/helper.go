package ranhelper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/k8snetworkplumbingwg/sriov-network-operator/pkg/render"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	podUtil "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
)

const (
	createMode = "create"
	deleteMode = "delete"
	updateMode = "update"
)

type promQueryResponse struct {
	Status string
	Error  string
	Data   struct {
		Result []PromMetric
	}
}

// PromMetric struct to hold an item in prom query result list
type PromMetric struct {
	Metric map[string]string
	Value  []interface{}
}

// ApplyObjects adds manifests from given directory to cluster
func ApplyObjects(resDir string) error {
	return modifyObjects(createMode, resDir)
}

// DeleteObjects removes manifests from given directory from cluster
func DeleteObjects(resDir string) error {
	return modifyObjects(deleteMode, resDir)
}

// UpdateObjects updates existing resurces based on manifests from given directory
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
		if mode == createMode {
			err = helper.Apiclient.Client.Create(context.TODO(), obj)
		} else if mode == deleteMode {
			err = helper.Apiclient.Client.Delete(context.TODO(), obj)
		} else if mode == updateMode {
			err = updateObject(obj)
		}
		if err != nil {
			errorList = append(errorList, err)
		}
	}
	if len(errorList) > 0 {
		return fmt.Errorf(
			"one or more errors occured while processing resources from dir %s \n%v errors",
			resDir, errorList)
	}
	return nil
}

func updateObject(obj *unstructured.Unstructured) error {
	name := obj.GetName()
	if name == "" {
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

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// ExecAndLogCommand Execute a command locally.
// Usage: This can be used to issue any bash cmd or oc command assuming KUBECONFIG is set properly.
func ExecAndLogCommand(timeout time.Duration, name string, arg ...string) ([]byte, error) {
	// Create a new context and add a timeout to it
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel() // The cancel should be deferred so resources are cleaned up

	log.Printf("run command '%s %v'", name, arg)
	out, err := exec.CommandContext(ctx, name, arg...).Output()

	// We want to check the context error to see if the timeout was executed.
	// The error returned by cmd.Output() will be OS specific based on what
	// happens when a process is killed.
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("command '%s %v' failed because of the timeout", name, arg)
	}

	if exitError, ok := err.(*exec.ExitError); ok {
		log.Printf("run command '%s %v' (err=%v):\n  stderr=%s\n", name, arg, err, exitError.Stderr)
	}
	return out, err
}

// ExecCommandOnNode executes given command on given node and returns the result
// Usage: This can be used to issue a command on given node
func ExecCommandOnNode(node *corev1.Node, cmd []string) (string, error) {
	podName := fmt.Sprintf("%s-%s", ran.PrivPodNamespace, node.Name)
	pod, err := helper.Apiclient.Pods(ran.PrivPodNamespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	log.Printf("Exec command on %s: %v\n", node.Name, cmd)
	out, err := podUtil.ExecCommand(helper.Apiclient, *pod, cmd)
	if err != nil {
		return "", err
	}
	return strings.Trim(out.String(), "\n"), err
}

// ExecCommandOnNodeWithHostBinaries executes given command that uses host binaries on given node and returns the result
// Usage: This can be used to issue a command on given node that requires using host binaries, such as crictl, podman.
func ExecCommandOnNodeWithHostBinaries(node *corev1.Node, command []string) (string, error) {
	initialArgs := []string{
		"chroot",
		"/rootfs",
	}
	initialArgs = append(initialArgs, command...)
	return ExecCommandOnNode(node, initialArgs)
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
func ExecPromQuery(query string, logCommand bool) ([]PromMetric, error) {
	command := []string{
		"bash", "-c",
		fmt.Sprintf("curl \"-s\" '%squery' --data-urlencode 'query=%s'; echo", ran.PromLocalUrl, query),
	}
	output, err := execCommandInPromPod(command, logCommand)
	if err != nil {
		return nil, err
	}

	var response promQueryResponse
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

func IsOcExist() bool {
	_, err := ExecAndLogCommand(10*time.Second, "oc", "version")
	return err == nil
}
