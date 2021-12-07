package helper

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	v1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/machineconfigpool"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sigClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var podWaitingTime = 5 * time.Minute

// PullTestImage pulls test image on all relevant nodes.
func PullTestImage(cnfNodeLabel string, image string) {
	nodesList, err := nodes.GetByLabel(Apiclient, cnfNodeLabel)
	Expect(err).ToNot(HaveOccurred())

	for _, node := range nodesList.Items {
		pullPodDefenition := pod.RedefineWithRestartPolicy(
			pod.RedefineWithCommand(
				pod.DefinePodOnNode("default", image, node.Name),
				[]string{"echo", "image pulled Successfully && exit 0"}, []string{}), k8sv1.RestartPolicyNever)
		pullPod, err := Apiclient.Pods("default").Create(
			context.Background(),
			pullPodDefenition,
			metav1.CreateOptions{},
		)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() k8sv1.PodPhase {
			pullPod, _ = Apiclient.Pods("default").Get(
				context.Background(),
				pullPod.Name,
				metav1.GetOptions{},
			)

			return pullPod.Status.Phase
		}, podWaitingTime, time.Second).Should(Equal(k8sv1.PodSucceeded), "Invalid pulling image")
	}

	err = namespaces.CleanPods("default", Apiclient)
	Expect(err).ToNot(HaveOccurred())
}

// CountLinesByMatches returns match count int based on match pattern.
func CountLinesByMatches(str string, stringsToGrep ...string) int {
	count := 0
	exists := false

	for _, line := range strings.Split(str, "\n") {
		for _, grep := range stringsToGrep {
			if !strings.Contains(line, grep) {
				exists = false

				break
			}

			exists = true
		}

		if exists {
			count++
		}
	}

	return count
}

// WaitUntilPodCreatedAndRunning waits until pod created and running. Returns running pod.
func WaitUntilPodCreatedAndRunning(podStruct *k8sv1.Pod, waitingTime time.Duration) *k8sv1.Pod {
	return waitUntilPodCreatedAndInPhase(podStruct, waitingTime, k8sv1.PodRunning)
}

func waitUntilPodCreatedAndInPhase(podStruct *k8sv1.Pod, waitingTime time.Duration, status k8sv1.PodPhase) *k8sv1.Pod {
	err := Apiclient.Create(context.Background(), podStruct)
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() k8sv1.PodPhase {
		tempPod, _ := Apiclient.Pods(podStruct.Namespace).Get(
			context.Background(),
			podStruct.Name,
			metav1.GetOptions{})

		return tempPod.Status.Phase
	}, waitingTime, time.Second).Should(Equal(status))

	runningPod, err := Apiclient.Pods(podStruct.Namespace).Get(
		context.Background(),
		podStruct.Name,
		metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())

	return runningPod
}

// IsDeploymentInstalled checks if deployment is installed.
func IsDeploymentInstalled(
	cs *client.ClientSet, operatorNamespace string, operatorDeploymentName string) (bool, error) {
	_, err := cs.Deployments(operatorNamespace).Get(context.Background(), operatorDeploymentName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	return true, nil
}

// IsDeploymentReady checks if deployment is ready.
func IsDeploymentReady(cs *client.ClientSet, operatorNamespace string, deploymentName string) (bool, error) {
	deployment, err := cs.Deployments(operatorNamespace).Get(context.Background(), deploymentName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	if deployment.Status.ReadyReplicas > 0 {
		if deployment.Status.Replicas == deployment.Status.ReadyReplicas {
			return true, nil
		}
	}

	return false, nil
}

// CountDaemonsets counts total number of Ready and Desired daemonsets and returns int count.
func CountDaemonsets(
	cs *client.ClientSet,
	operatorNamespace string,
	daemonsetName string) (countRunningDaemonsets int32, countDesiredDaemonsets int32) {
	daemonSetDiscovery, err := cs.DaemonSets(operatorNamespace).Get(
		context.Background(),
		daemonsetName,
		metav1.GetOptions{},
	)
	Expect(err).NotTo(HaveOccurred())

	countRunningDaemonsets = daemonSetDiscovery.Status.NumberReady
	countDesiredDaemonsets = daemonSetDiscovery.Status.DesiredNumberScheduled

	return countRunningDaemonsets, countDesiredDaemonsets
}

// WaitForClusterToBeStable validates if MCP is stable
// "TODO: Fix snoTimeoutMultiplier parameter. Convert it in to int32".
func WaitForClusterToBeStable(machineConfigPoolName string, snoTimeoutMultiplier time.Duration) error {
	mcp := &v1.MachineConfigPool{}

	err := Apiclient.Client.Get(context.TODO(), sigClient.ObjectKey{Name: machineConfigPoolName}, mcp)
	if err != nil {
		return err
	}

	err = machineconfigpool.WaitForCondition(
		Apiclient,
		&v1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Name: machineConfigPoolName}},
		v1.MachineConfigPoolUpdating,
		2*time.Minute)
	if err != nil {
		return err
	}

	// We need to wait a long time here for the node to reboot
	err = machineconfigpool.WaitForCondition(
		Apiclient,
		&v1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Name: machineConfigPoolName}},
		v1.MachineConfigPoolUpdated,
		time.Duration(20*mcp.Status.MachineCount*int32(snoTimeoutMultiplier))*time.Minute)

	return err
}

// GetNodeListStringByLabel returns node names in list format.
func GetNodeListStringByLabel(labelNodeRole string) []string {
	var nodeListString []string

	nodesList, err := nodes.GetByRole(Apiclient, labelNodeRole)
	Expect(err).ToNot(HaveOccurred())

	for _, node := range nodesList {
		nodeListString = append(nodeListString, node.Name)
	}

	return nodeListString
}

// ExecAndLogCommand Execute a command locally.
// Usage: This can be used to issue any bash cmd or oc command assuming KUBECONFIG is set properly.
func ExecAndLogCommand(logCommand bool, timeout time.Duration, name string, arg ...string) ([]byte, error) {
	// Create a new context and add a timeout to it
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}

	ctx, cancel := context.WithTimeout(context.TODO(), timeout)

	defer cancel() // The cancel should be deferred so resources are cleaned up

	if logCommand {
		log.Printf("run command '%s %v'", name, arg)
	}

	out, err := exec.CommandContext(ctx, name, arg...).Output()

	// We want to check the context error to see if the timeout was executed.
	// The error returned by cmd.Output() will be OS specific based on what
	// happens when a process is killed.
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("command '%s %v' failed because of the timeout", name, arg)
	}

	if logCommand {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			log.Printf("err=%v:\n  stderr=%s\n  output=%s\n", err, exitError.Stderr, string(out))
		}
	}

	return out, err
}

// ExecCommandOnNode executes given command on given node and returns the result
// Usage: This can be used to issue a command on given node.
func ExecCommandOnNode(node *k8sv1.Node, cmd []string) (string, error) {
	podName := fmt.Sprintf("%s-%s", parameters.PrivPodNamespace, node.Name)
	runningPod, err := Apiclient.Pods(parameters.PrivPodNamespace).Get(
		context.Background(),
		podName,
		metav1.GetOptions{},
	)

	if err != nil {
		return "", err
	}

	log.Printf("Exec command on %s: %v\n", node.Name, cmd)
	out, err := pod.ExecCommand(Apiclient, *runningPod, cmd)

	return strings.Trim(out.String(), "\n"), err
}

// ExecCommandOnNodeWithHostBinaries executes given command that uses host binaries on given node and returns the result
// Usage: This can be used to issue a command on given node that requires using host binaries, such as crictl, podman.
func ExecCommandOnNodeWithHostBinaries(node *k8sv1.Node, command []string) (string, error) {
	initialArgs := []string{
		"chroot",
		"/rootfs",
	}
	initialArgs = append(initialArgs, command...)

	return ExecCommandOnNode(node, initialArgs)
}

// SoftRebootNodeAndWaitForDisconnect soft reboots given node and wait for node to be unreachable.
func SoftRebootNodeAndWaitForDisconnect(node *k8sv1.Node) {
	output, err := ExecCommandOnNodeWithHostBinaries(node, []string{"systemctl", "reboot"})
	if err != nil {
		// Allows 143 return code. The privileged pod we sent cmd from may have started terminating before cmd returns.
		if !strings.Contains(err.Error(), "exit code 143") {
			Expect(err).ShouldNot(HaveOccurred(), output)
		}
	}

	waitForNodeUnreachable(node)
}

// waitForNodeUnreachable waits for ping node to fail.
func waitForNodeUnreachable(node *k8sv1.Node) {
	log.Printf("Waiting for node %s to be unreachable via ping", node.Name)

	timeout := 5 * time.Minute
	Eventually(func() bool {

		return isNodeReachable(node)
	}, timeout, 3*time.Second).ShouldNot(
		BeTrue(),
		fmt.Sprintf("Node %s is still reachable after %s", node.Name, timeout.String()),
	)
	log.Printf("Node %s is unreachable\n", node.Name)
}

// WaitForNodeReachable waits for node to be reachable via ping and returns UTC timestamp.
func WaitForNodeReachable(node *k8sv1.Node) {
	log.Println("Waiting for node to be reachable via ping")

	timeout := 15 * time.Minute
	Eventually(func() bool {

		return isNodeReachable(node)
	}, timeout, 3*time.Second).Should(BeTrue(),
		fmt.Sprintf("Node %s is still unreachable after %s", node.Name, timeout.String()))
	log.Printf("Node %s is reachable\n", node.Name)
}

func isNodeReachable(node *k8sv1.Node) bool {
	_, err := ExecAndLogCommand(false, 20*time.Second, "ping", "-c", "3", "-W", "10", node.Name)

	return err == nil
}

// CreatePrivilegedPods creates privileged test pods on all nodes to assist testing
// Returns a map with nodeName as key and pod pointer as value, and error if occurred.
func CreatePrivilegedPods(image string) map[string]*k8sv1.Pod {
	if image == "" {
		configData, err := config.NewConfig()
		Expect(err).ShouldNot(HaveOccurred())

		image = configData.Ran.CnfTestImage
	}
	// Create ranpriv namespace if not alrady created
	if !namespaces.Exists(parameters.PrivPodNamespace, Apiclient) {
		log.Println("Creating namespace:", parameters.PrivPodNamespace)
		err := namespaces.Create(parameters.PrivPodNamespace, Apiclient)
		Expect(err).ShouldNot(HaveOccurred())
	}

	// Launch priv pods on nodes with worker role so it can be successfully scheduled.
	workerNodes, err := nodes.GetByRole(Apiclient, parameters.RoleWorker)
	Expect(err).ShouldNot(HaveOccurred())

	privPods := make(map[string]*k8sv1.Pod)
	volumeType := k8sv1.HostPathUnset
	volSource := k8sv1.VolumeSource{
		HostPath: &k8sv1.HostPathVolumeSource{Path: "/", Type: &volumeType}}

	for _, workerNode := range workerNodes {
		podName := fmt.Sprintf("%s-%s", parameters.PrivPodNamespace, workerNode.Name)
		privilegedPod, err := Apiclient.Pods(parameters.PrivPodNamespace).Get(
			context.Background(),
			podName, metav1.GetOptions{},
		)

		if err != nil {
			privilegedPod = pod.RedefineAsPrivileged(pod.DefinePodOnNode(
				parameters.PrivPodNamespace, image, workerNode.Name))
			privilegedPod = pod.RedefineWithVolume(
				pod.RedefineWithHostPid(privilegedPod),
				"rootfs", "/rootfs", volSource, false,
			)
			privilegedPod = WaitUntilPodCreatedAndRunning(
				pod.RedefineWithObjectMeta(privilegedPod, podName, "", nil), 10*time.Minute)
		}

		privPods[workerNode.Name] = privilegedPod
		WaitForPodsHealthy([]*k8sv1.Pod{privilegedPod}, 5*time.Minute)
	}

	return privPods
}

// WaitForPodsHealthy waits for given pods to appear and healthy.
func WaitForPodsHealthy(pods []*k8sv1.Pod, timeout time.Duration) {
	Eventually(func() error {
		for _, pod := range pods {
			tempPod, err := Apiclient.Pods(pod.Namespace).Get(
				context.Background(),
				pod.Name,
				metav1.GetOptions{})
			if err != nil {
				return err
			}
			err = IsPodHealthy(tempPod)
			if err != nil &&
				!(pod.Status.Phase == k8sv1.PodFailed && pod.Spec.RestartPolicy == k8sv1.RestartPolicyNever) {
				// Ignore failed pod with restart policy never. This could happen in image pruner or installer
				// pods that will never restart after completed. And could stuck in error in various conditions
				// after initial completion.
				return err
			}
		}

		return nil
	}, timeout, 3*time.Second).ShouldNot(HaveOccurred())
}

// IsPodHealthy returns nil if given pod is healthy, otherwise an error.
func IsPodHealthy(pod *k8sv1.Pod) error {
	if pod.Status.Phase == k8sv1.PodRunning {
		// Check if running pod is ready
		if !isPodInCondition(pod, k8sv1.PodReady) {
			return fmt.Errorf("pod condition is not Ready. Message: %s", pod.Status.Message)
		}
	} else if pod.Status.Phase != k8sv1.PodSucceeded {
		// Pod is not running or completed.
		return fmt.Errorf("pod phase is %s. Message: %s", pod.Status.Phase, pod.Status.Message)
	}

	return nil
}

// isPodInCondition returns true if given pod is in expected condition, otherwise false.
func isPodInCondition(pod *k8sv1.Pod, condition k8sv1.PodConditionType) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == condition && c.Status == k8sv1.ConditionTrue {
			return true
		}
	}

	return false
}
