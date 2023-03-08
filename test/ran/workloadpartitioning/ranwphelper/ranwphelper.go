package ranwphelper

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/workloadpartitioning/ranwpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	podhelper "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
)

type ContainerInfo struct {
	Name      string `json:"name"`
	Cpus      string `json:"cpus"`
	Namespace string `json:"namespace"`
	PodName   string `json:"podname"`
	Shares    int    `json:"shares"`
	Pid       int    `json:"pid"`
}

// GetContainersInfo returns containers info on given node via crictl.
func GetContainersInfo(node *corev1.Node) []ContainerInfo {
	var (
		containerinfos []ContainerInfo
		err            error
		output         string
	)
	// Occasionally, the return will contain mal-formatted chars. Retry im case that happens.
	for i := 1; i <= 3; i++ {
		// The selected fields needs to match ContainersInfo struct
		output, err = helper.ExecCommandOnNodeWithHostBinaries(node, []string{"bash", "-c",
			`crictl ps --state running --quiet | xargs crictl inspect -o json | jq '. | {
name: .status.metadata.name,
podname: .status.labels."io.kubernetes.pod.name",
namespace: .status.labels."io.kubernetes.pod.namespace",
pid: .info.pid,
cpus: .info.runtimeSpec.linux.resources.cpu.cpus,
shares: .info.runtimeSpec.linux.resources.cpu.shares,
}' | jq -sM`})
		if err == nil {
			err = json.Unmarshal([]byte(output), &containerinfos)
		}

		if err == nil {
			break
		} else {
			// Sleep for 1 second before next attempt.
			time.Sleep(1 * time.Second)
		}
	}
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("json unmarshal failed with output:\n%s", output))
	log.Printf("Containers count: %d\n", len(containerinfos))

	return containerinfos
}

// isKernelPid checks if a given pid is a kernel process.
func IsKernelPid(node *corev1.Node, pid int) bool {
	// ps command via container often causes SIGURG, thus redirect output to a file first
	cmd := fmt.Sprintf("ps --no-headers -o ppid -p %s > /tmp/x ; cat /tmp/x", strconv.Itoa(pid))
	parentPID, _ := helper.ExecCommandOnNodeWithHostBinaries(node, []string{"bash", "-c", cmd})

	if len(parentPID) == 0 || strings.TrimSpace(parentPID) == "2" {
		// Either the process was terminated or
		// it is a kernel process
		return true
	}

	return false
}

// getKernelPids returns list of kernel process ids.
func getKernelPids(node *corev1.Node) []int {
	// Get all kernel threads (PID 2 and children)
	return getPids(node, "ps --no-headers --ppid 2 -p 2 -o pid")
}

// getAllPids returns list of all process ids.
func getAllPids(node *corev1.Node) []int {
	return getPids(node, "ps --no-headers -e -o pid")
}

// getPids runs given ps command to get a list of pids and parse them into a list..
func getPids(node *corev1.Node, command string) []int {
	var (
		retries = 3
		output  string
		err     error
	)
	// ps command via container with long output often causes SIGURG, thus redirect output to a file first.
	command += " > /tmp/x ; cat /tmp/x"
	for i := 1; i <= retries; i++ {
		output, err = helper.ExecCommandOnNodeWithHostBinaries(node, []string{"bash", "-c", command})
		Expect(err).ToNot(HaveOccurred())

		if !strings.Contains(output, "Signal 23") {
			break
		} else {
			log.Println("Signal 23 (URG) encountered. Retry...")
			time.Sleep(time.Second)
		}
	}

	pidStrings := strings.Split(output, "\r\n")

	var pids []int

	for _, pidString := range pidStrings {
		pidString = strings.TrimSpace(pidString)
		pid, err := strconv.Atoi(pidString)
		Expect(err).ToNot(HaveOccurred())
		pids = append(pids, pid)
	}

	return pids
}

// getPidsAffinity gets pids' affinity list from taskset command and returns a map with pid as key, and affinity
// list as value.
func getPidsAffinity(node *corev1.Node, pids []int) map[int]string {
	var pidStrings []string
	for _, pid := range pids {
		pidStrings = append(pidStrings, strconv.Itoa(pid))
	}

	pidString := strings.Join(pidStrings, " ")
	cmd := fmt.Sprintf(
		"pids=\"%s\"; for pid in $pids; do if [ $pid == $$ ]; then continue; fi; taskset -pc $pid; done",
		pidString,
	)
	output, _ := helper.ExecCommandOnNodeWithHostBinaries(node, []string{"bash", "-c", cmd})
	// Allow cmd to fail for transient processes. Check return content instead.
	Expect(output).To(ContainSubstring("current affinity list"))

	affinities := make(map[int]string)
	regularEx := regexp.MustCompile(`pid (\d+)'s current affinity list: (.*)$`)

	for _, line := range strings.Split(output, "\r\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "No such process") {
			continue
		}

		match := regularEx.FindAllStringSubmatch(line, -1)
		pidInt, err := strconv.Atoi(match[0][1])
		Expect(err).ToNot(HaveOccurred())

		affinities[pidInt] = strings.TrimSpace(match[0][2])
	}

	return affinities
}

// printPidInfo prints out pids info via ps command.
func printPidInfo(node *corev1.Node, pids []int) {
	var pidStrings []string
	for _, pid := range pids {
		pidStrings = append(pidStrings, strconv.Itoa(pid))
	}

	cmd := fmt.Sprintf("ps %s", strings.Join(pidStrings, " "))
	output, _ := helper.ExecCommandOnNodeWithHostBinaries(node, []string{"bash", "-c", cmd})
	log.Println(output)
}

// CheckPodsAffinity checks given containers are pinned to specified cpus.
func CheckPodsAffinity(containersInfo []ContainerInfo, affinedCPUSet cpuset.CPUSet) {
	var errorPods []ContainerInfo

	for _, podinfo := range containersInfo {
		cpus := cpuset.MustParse(podinfo.Cpus)
		if !cpus.Equals(affinedCPUSet) {
			errorPods = append(errorPods, podinfo)
		}
	}

	Expect(errorPods).To(BeEmpty())
	log.Printf("%d cotainers checked", len(containersInfo))
}

// DefineQoSTestPod defines test pod with given cpu and memory resources.
func DefineQoSTestPod(nodeName, namespace, cpuReq, cpuLimit, memReq, memLimit string) *corev1.Pod {
	image := helper.Config.Ran.CnfTestImage
	// Create namespace if not alrady created
	if !namespaces.Exists(namespace, helper.Apiclient) {
		log.Println("Creating namespace:", namespace)
		err := namespaces.Create(namespace, helper.Apiclient)
		Expect(err).ShouldNot(HaveOccurred())
	}

	pod := ranhelper.RedefineContainerResources(
		podhelper.DefinePodOnNode(namespace, image, nodeName), cpuReq, cpuLimit, memReq, memLimit)

	return pod
}

// findInt returns true if a specific integer is in given int slice, false otherwise.
func findInt(item int, intSlice []int) bool {
	for _, i := range intSlice {
		if i == item {
			return true
		}
	}

	return false
}

// IsNonMgmtPod checks if a pod is a non-management pod. e.g., test pods created in automation, amq and bmer pods.
func IsNonMgmtPod(podName, namespace string) bool {
	if ranwpparameters.NonMgmtNamespaces.Has(namespace) || strings.HasPrefix(podName, parameters.PrivPodNamespace) ||
		strings.HasPrefix(podName, "process-exporter") {
		return true
	}

	return false
}

func GetMgmtContainersInfo(containersInfo []ContainerInfo) []ContainerInfo {
	var mgmtContainerInfo []ContainerInfo

	for _, podinfo := range containersInfo {
		// Exclude test pods
		if !IsNonMgmtPod(podinfo.PodName, podinfo.Namespace) {
			mgmtContainerInfo = append(mgmtContainerInfo, podinfo)
		}
	}

	return mgmtContainerInfo
}

// CheckCPUAffinityOnNonKernelPids checks cpus (affinity) for non kernel pids and returns a
// non nil error and a map of key:pids,value:cpus for those processes not matching the specified cpus.
func CheckCPUAffinityOnNonKernelPids(node *corev1.Node, cpus cpuset.CPUSet) (map[int]string, error) {
	pidsToExclude := getKernelPids(node)
	allPids := getAllPids(node)
	containersInfo := GetContainersInfo(node)

	for _, containerInfo := range containersInfo {
		pidsToExclude = append(pidsToExclude, containerInfo.Pid)
	}

	var pidsToCheck []int

	for _, pid := range allPids {
		if !findInt(pid, pidsToExclude) {
			pidsToCheck = append(pidsToCheck, pid)
		}
	}

	affinities := getPidsAffinity(node, pidsToCheck)
	failedMap := make(map[int]string)

	var failedPids []int

	for pid, affinity := range affinities {
		pidCpuset := cpuset.MustParse(affinity)
		if !pidCpuset.IsSubsetOf(cpus) {
			// Make sure it's not a kernel process
			// as there may have race condition in
			// the previous queries of pids
			if !IsKernelPid(node, pid) {
				failedMap[pid] = affinity
				failedPids = append(failedPids, pid)
			}
		}
	}

	var err error = nil

	if len(failedPids) > 0 {
		err = errors.New("processes not matching reserved CPU affinites found")

		printPidInfo(node, failedPids)
	}

	return failedMap, err
}
