package ranwphelper

import (
	"encoding/json"
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
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
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

// GetContainersInfo returns containers info on given node via crictl
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
			time.Sleep(1)
		}
	}
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("json unmarshal failed with output:\n%s", output))
	log.Printf("Containers count: %d\n", len(containerinfos))
	return containerinfos
}

// GetKernelPids returns list of kernel process ids
func GetKernelPids(node *corev1.Node) []int {
	// Get all kernel threads (PID 2 and children)
	return getPids(node, "ps --no-headers --ppid 2 -p 2 -o pid")
}

// GetAllPids returns list of all process ids
func GetAllPids(node *corev1.Node) []int {
	return getPids(node, "ps --no-headers -e -o pid")
}

// getPids runs given ps command to get a list of pids and parse them into a list
func getPids(node *corev1.Node, command string) []int {
	retries := 3
	var output string
	var err error
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
		pids = append(pids, int(pid))
	}
	return pids
}

// GetPidsAffinity gets pids' affinity list from taskset command and returns a map with pid as key, and affinity list as value
func GetPidsAffinity(node *corev1.Node, pids []int) map[int]string {
	var pidStrings []string
	for _, pid := range pids {
		pidStrings = append(pidStrings, strconv.Itoa(pid))
	}
	pidString := strings.Join(pidStrings, " ")
	cmd := fmt.Sprintf("pids=\"%s\"; for pid in $pids; do if [ $pid == $$ ]; then continue; fi; taskset -pc $pid; done", pidString)
	output, _ := helper.ExecCommandOnNodeWithHostBinaries(node, []string{"bash", "-c", cmd})
	// Allow cmd to fail for transient processes. Check return content instead.
	Expect(output).To(ContainSubstring("current affinity list"))

	affinities := make(map[int]string)
	re := regexp.MustCompile(`pid (\d+)'s current affinity list: (.*)$`)
	for _, line := range strings.Split(output, "\r\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "No such process") {
			continue
		}
		match := re.FindAllStringSubmatch(line, -1)
		pidInt, err := strconv.Atoi(match[0][1])
		Expect(err).ToNot(HaveOccurred())
		affinities[pidInt] = strings.TrimSpace(match[0][2])
	}
	return affinities
}

// PrintPidInfo prints out pids info via ps command
func PrintPidInfo(node *corev1.Node, pids []int) {
	var pidStrings []string
	for _, pid := range pids {
		pidStrings = append(pidStrings, strconv.Itoa(pid))
	}
	cmd := fmt.Sprintf("ps %s", strings.Join(pidStrings, " "))
	output, _ := helper.ExecCommandOnNodeWithHostBinaries(node, []string{"bash", "-c", cmd})
	log.Println(output)
}

// CheckPodsAffinity checks given containers are pinned to specified cpus
func CheckPodsAffinity(containersInfo []ContainerInfo, affinedCpuSet cpuset.CPUSet) {
	var errorPods []ContainerInfo
	for _, podinfo := range containersInfo {
		cpus := cpuset.MustParse(podinfo.Cpus)
		if !cpus.Equals(affinedCpuSet) {
			errorPods = append(errorPods, podinfo)
		}
	}
	Expect(errorPods).To(BeEmpty())
	log.Printf("%d cotainers checked", len(containersInfo))
}

// DefineQoSTestPod defines test pod with given cpu and memory resources
func DefineQoSTestPod(nodeName, namespace, cpuReq, cpuLimit, memReq, memLimit string) *corev1.Pod {
	config_, err := config.NewConfig()
	Expect(err).ShouldNot(HaveOccurred())
	image := config_.Ran.CnfTestImage
	// Create namespace if not alrady created
	if !namespaces.Exists(namespace, helper.Apiclient) {
		log.Println("Creating namespace:", namespace)
		err := namespaces.Create(namespace, helper.Apiclient)
		Expect(err).ShouldNot(HaveOccurred())
	}
	pod := ranhelper.RedefineContainerResources(podhelper.DefinePodOnNode(namespace, image, nodeName), cpuReq, cpuLimit, memReq, memLimit)
	return pod
}
