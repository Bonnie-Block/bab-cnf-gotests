package tests

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	performancev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/cpu/rancpuhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/workloadpartitioning/ranwphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/workloadpartitioning/ranwpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
)

var _ = Describe("SNO management workload partitioning", func() {
	var (
		node        *corev1.Node
		isSNO       bool
		perfProfile *performancev2.PerformanceProfile
		mgmtCPUSet  cpuset.CPUSet
		isolCPUSet  cpuset.CPUSet
		pod         *corev1.Pod
	)

	execute.BeforeAll(func() {
		isSNO, _ = nodes.IsSingleNodeCluster(helper.Apiclient)
		perfProfile, _ = rancpuhelper.GetPerformanceProfileWithCPUSet(nil)
		// Get node for testing
		workers, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
		Expect(err).ToNot(HaveOccurred())
		node = &workers[0]
		mgmtCPUSet = cpuset.MustParse(string(*perfProfile.Spec.CPU.Reserved))
		isolCPUSet = cpuset.MustParse(string(*perfProfile.Spec.CPU.Isolated))

		// Print out kernel version with best effort
		output, err := helper.ExecCommandOnNode(node, []string{"uname", "-r"})
		if err == nil {
			log.Println("Kernel version: " + output)
		}
	})

	BeforeEach(func() {
		if !isSNO {
			Skip("Test is only applicable to Single Node cluster")
		}
		if perfProfile == nil {
			Skip("No performance profile with reserved and isolated cpu set configuration found on cluster")
		}
		Expect(node).ToNot(Equal(nil))
	})

	// 41236
	It("should have management workload cpu resource added to node", polarion.ID("41236"), func() {
		By("Checking node capacity and allocatable resources", func() {
			cpuCapacity := node.Status.Capacity["cpu"]
			wpCapacity := node.Status.Capacity[ranwpparameters.AnnotationWpResource]
			wpAllocatable := node.Status.Allocatable[ranwpparameters.AnnotationWpResource]
			Expect(wpCapacity.Value()).To(BeEquivalentTo(cpuCapacity.Value() * 1000))
			Expect(wpAllocatable.Value()).To(BeEquivalentTo(cpuCapacity.Value() * 1000))
		})
	})

	It("should pin OS daemon to reserved cpus", func() {
		failedPids, _ := ranwphelper.CheckCPUAffinityOnNonKernelPids(node, mgmtCPUSet)
		Expect(failedPids).To(BeEmpty())
	})

	// 41230
	It("should have management pods pinned to reserved cpus", polarion.ID("41230"), func() {
		By("Checking cpuset for all running containers via crictl inspect on container host", func() {
			containersInfo := ranwphelper.GetContainersInfo(node)
			ranwphelper.CheckPodsAffinity(ranwphelper.GetMgmtContainersInfo(containersInfo), mgmtCPUSet)
		})
	})

	// 41232,41238,41229
	It("should have the correct cpushares for management pods in crio", polarion.ID("41232"), func() {
		By("Comparing container cpu shares in crio and pod annotation", func() {
			containersInfo := ranwphelper.GetContainersInfo(node)
			checkCPUShares(containersInfo)
		})
	})

	// 41233
	It("should mutate burstable pod with cpu and memory requests", polarion.ID("41233"), func() {
		createsTestMgmtNamespace()
		By("Creating a wp annotated burtable pod under test management namespace", func() {
			pod = ranwphelper.DefineQoSTestPod(node.Name, ran.NamespaceTesting, "1", "", "100M", "")
			pod = ranhelper.RedefineWithNewAnnotations(pod,
				map[string]string{ranwpparameters.AnnotationWpPodKey: ranwpparameters.AnnotationWpPodValue})
			pod = helper.WaitUntilPodCreatedAndRunning(pod, 5*time.Minute)
		})
		By("Checking pod resources are mutated", func() {
			resourceLimits := pod.Spec.Containers[0].Resources.Limits
			resourceRequests := pod.Spec.Containers[0].Resources.Requests
			Expect(resourceLimits.Cpu().Value()).To(Equal(int64(0)))
			Expect(resourceRequests.Cpu().Value()).To(Equal(int64(0)))
			Expect(resourceLimits.Memory().Value()).To(Equal(int64(0)))
			Expect(resourceRequests.Memory().String()).To(Equal("100M"))
			mutatedLimit := resourceLimits[ranwpparameters.AnnotationWpResource]
			mutatedReq := resourceRequests[ranwpparameters.AnnotationWpResource]
			Expect(mutatedLimit.Value()).To(BeEquivalentTo(int64(1000)))
			Expect(mutatedReq.Value()).To(BeEquivalentTo(int64(1000)))
		})
		By("Checking pod is mutated with cpushares annotation added", func() {
			podShares := make(map[string]int)
			podShares = getPodsCPUShares([]corev1.Pod{*pod}, podShares)
			Expect(podShares).ToNot(BeEmpty())
			for _, val := range podShares {
				// 1 cpu request = 1024 mi request = 1024 cpushares
				Expect(val).To(BeEquivalentTo(1024))
			}
		})
		By("Checking correct cpushares and affinity in crio", func() {
			containersInfo := ranwphelper.GetContainersInfo(node)
			for _, containerInfo := range containersInfo {
				if containerInfo.Namespace == pod.Namespace && containerInfo.PodName == pod.Name {
					Expect(containerInfo.Shares).To(BeEquivalentTo(1024))
					podCPUSet := cpuset.MustParse(containerInfo.Cpus)
					Expect(podCPUSet.String()).To(BeEquivalentTo(mgmtCPUSet.String()))

					return
				}
			}
			Fail(fmt.Sprintf("test pod %s is not found in crictl ps", pod.Name))
		})
	})

	// 41536
	It("should mutate best effort pod", polarion.ID("41536"), func() {
		createsTestMgmtNamespace()
		By("Creating a wp annotated best effort pod under test management namespace", func() {
			pod = ranwphelper.DefineQoSTestPod(node.Name, ran.NamespaceTesting, "", "", "", "")
			pod = ranhelper.RedefineWithNewAnnotations(pod,
				map[string]string{ranwpparameters.AnnotationWpPodKey: ranwpparameters.AnnotationWpPodValue})
			pod = helper.WaitUntilPodCreatedAndRunning(pod, 5*time.Minute)
		})
		By("Checking pod resource requests are not mutated", func() {
			resourceLimits := pod.Spec.Containers[0].Resources.Limits
			resourceRequests := pod.Spec.Containers[0].Resources.Requests
			Expect(resourceLimits.Cpu().Value()).To(Equal(int64(0)))
			Expect(resourceRequests.Cpu().Value()).To(Equal(int64(0)))
			Expect(resourceLimits.Memory().Value()).To(Equal(int64(0)))
			Expect(resourceRequests.Memory().Value()).To(Equal(int64(0)))
			mutatedLimit := resourceLimits[ranwpparameters.AnnotationWpResource]
			mutatedReq := resourceRequests[ranwpparameters.AnnotationWpResource]
			Expect(mutatedLimit.Value()).To(BeEquivalentTo(int64(0)))
			Expect(mutatedReq.Value()).To(BeEquivalentTo(int64(0)))
		})
		By("Checking pod has wp cpushares annotation added with 2 cpushares", func() {
			podShares := make(map[string]int)
			podShares = getPodsCPUShares([]corev1.Pod{*pod}, podShares)
			Expect(podShares).ToNot(BeEmpty())
			for _, val := range podShares {
				// Best effort pod is mutated with cpushares 2
				Expect(val).To(BeEquivalentTo(2))
			}
		})
		By("Checking correct cpushares and affinity in crio", func() {
			containersInfo := ranwphelper.GetContainersInfo(node)
			for _, containerInfo := range containersInfo {
				if containerInfo.Namespace == pod.Namespace && containerInfo.PodName == pod.Name {
					Expect(containerInfo.Shares).To(BeEquivalentTo(2))
					podCPUSet := cpuset.MustParse(containerInfo.Cpus)
					Expect(podCPUSet.String()).To(BeEquivalentTo(mgmtCPUSet.String()))

					return
				}
			}
			Fail(fmt.Sprintf("test pod %s is not found in crictl ps", pod.Name))
		})
	})

	It("should mutate burstable pod with only memory request&limit", func() {
		createsTestMgmtNamespace()
		By("Creating a burstable pod under test management namespace with only memory request", func() {
			pod = ranwphelper.DefineQoSTestPod(node.Name, ran.NamespaceTesting, "", "", "100M", "100M")
			pod = ranhelper.RedefineWithNewAnnotations(pod,
				map[string]string{ranwpparameters.AnnotationWpPodKey: ranwpparameters.AnnotationWpPodValue})
			pod = helper.WaitUntilPodCreatedAndRunning(pod, 5*time.Minute)
		})
		By("Checking pod resources are not mutated", func() {
			resourceLimits := pod.Spec.Containers[0].Resources.Limits
			resourceRequests := pod.Spec.Containers[0].Resources.Requests
			Expect(resourceRequests.Cpu().Value()).To(Equal(int64(0)))
			Expect(resourceLimits.Cpu().Value()).To(Equal(int64(0)))
			Expect(resourceRequests.Memory().String()).To(Equal("100M"))
			Expect(resourceLimits.Memory().String()).To(Equal("100M"))
		})
		By("Checking correct cpushares and cpu affinity in crio", func() {
			containersInfo := ranwphelper.GetContainersInfo(node)
			for _, containerInfo := range containersInfo {
				if containerInfo.Namespace == pod.Namespace && containerInfo.PodName == pod.Name {
					Expect(containerInfo.Shares).To(BeEquivalentTo(2))
					podCPUSet := cpuset.MustParse(containerInfo.Cpus)
					Expect(podCPUSet.String()).To(BeEquivalentTo(mgmtCPUSet.String()))

					return
				}
			}
			Fail(fmt.Sprintf("test pod %s is not found in crictl ps", pod.Name))
		})
	})

	It("should not mutate burstable pod with cpu request and limit", func() {
		createsTestMgmtNamespace()
		cpuReq, cpuLimit := 1, 2
		By("Creating a burstable pod under test management namespace with cpu request and limit", func() {
			pod = ranwphelper.DefineQoSTestPod(
				node.Name,
				ran.NamespaceTesting,
				strconv.Itoa(cpuReq),
				strconv.Itoa(cpuLimit),
				"",
				"",
			)
			pod = ranhelper.RedefineWithNewAnnotations(pod,
				map[string]string{ranwpparameters.AnnotationWpPodKey: ranwpparameters.AnnotationWpPodValue})
			pod = helper.WaitUntilPodCreatedAndRunning(pod, 5*time.Minute)
		})
		By("Checking pod resources are not mutated with warning annotated", func() {
			resourceRequests := pod.Spec.Containers[0].Resources.Requests
			resourceLimits := pod.Spec.Containers[0].Resources.Limits
			Expect(resourceLimits.Cpu().Value()).To(Equal(int64(cpuLimit)))
			Expect(resourceRequests.Cpu().Value()).To(Equal(int64(cpuReq)))
			warning, warningAnnotated := pod.Annotations[ranwpparameters.AnnotationWpMutationWarning]
			Expect(warningAnnotated).To(BeTrue())
			Expect(warning).To(MatchRegexp(ranwpparameters.WarningCPUReqAndLimitRegExp))
		})
		By("Checking correct cpushares and affinity in crio", func() {
			containersInfo := ranwphelper.GetContainersInfo(node)
			for _, containerInfo := range containersInfo {
				if containerInfo.Namespace == pod.Namespace && containerInfo.PodName == pod.Name {
					Expect(containerInfo.Shares).To(BeEquivalentTo(cpuReq * 1024))
					podCPUSet := cpuset.MustParse(containerInfo.Cpus)
					Expect(podCPUSet.String()).To(BeEquivalentTo(isolCPUSet.String()))

					return
				}
			}
			Fail(fmt.Sprintf("test pod %s is not found in crictl ps", pod.Name))
		})
	})

	// 41269
	It("should not mutate burstable pod if mutation changes pod QoS class", polarion.ID("41269"), func() {
		createsTestMgmtNamespace()
		By("Creating a burstable pod under test management namespace with only cpu request", func() {
			pod = ranwphelper.DefineQoSTestPod(node.Name, ran.NamespaceTesting, "1", "", "", "")
			pod = ranhelper.RedefineWithNewAnnotations(pod,
				map[string]string{ranwpparameters.AnnotationWpPodKey: ranwpparameters.AnnotationWpPodValue})
			pod = helper.WaitUntilPodCreatedAndRunning(pod, 5*time.Minute)
		})
		By("Checking pod resources are not mutated with warning annotated", func() {
			resourceLimits := pod.Spec.Containers[0].Resources.Limits
			resourceRequests := pod.Spec.Containers[0].Resources.Requests
			Expect(resourceLimits.Cpu().Value()).To(Equal(int64(0)))
			Expect(resourceRequests.Cpu().Value()).To(Equal(int64(1)))
			Expect(resourceLimits.Memory().Value()).To(Equal(int64(0)))
			Expect(resourceRequests.Memory().Value()).To(Equal(int64(0)))
			warning, warningAnnotated := pod.Annotations[ranwpparameters.AnnotationWpMutationWarning]
			Expect(warningAnnotated).To(BeTrue())
			Expect(warning).To(ContainSubstring(ranwpparameters.WarningQoSChange))
		})
		By("Checking correct cpushares and affinity in crio", func() {
			containersInfo := ranwphelper.GetContainersInfo(node)
			for _, containerInfo := range containersInfo {
				if containerInfo.Namespace == pod.Namespace && containerInfo.PodName == pod.Name {
					Expect(containerInfo.Shares).To(BeEquivalentTo(1024))
					podCPUSet := cpuset.MustParse(containerInfo.Cpus)
					Expect(podCPUSet.String()).To(BeEquivalentTo(isolCPUSet.String()))

					return
				}
			}
			Fail(fmt.Sprintf("test pod %s is not found in crictl ps", pod.Name))
		})
	})

	// 41541
	It("should not mutate guaranteed pod", polarion.ID("41541"), func() {
		createsTestMgmtNamespace()
		By("Creating a guaranteed test pod under test management namespace", func() {
			pod = ranwphelper.DefineQoSTestPod(node.Name, ran.NamespaceTesting, "1", "1", "100M", "100M")
			pod = ranhelper.RedefineWithNewAnnotations(pod,
				map[string]string{ranwpparameters.AnnotationWpPodKey: ranwpparameters.AnnotationWpPodValue})
			pod = helper.WaitUntilPodCreatedAndRunning(pod, 5*time.Minute)
		})
		By("Checking pod resources are not mutated with warning annotated", func() {
			resourceLimits := pod.Spec.Containers[0].Resources.Limits
			resourceRequests := pod.Spec.Containers[0].Resources.Requests
			println(pod.Annotations[ranwpparameters.AnnotationWpMutationWarning])
			Expect(resourceLimits.Cpu().Value()).To(Equal(int64(1)))
			Expect(resourceRequests.Cpu().Value()).To(Equal(int64(1)))
			Expect(resourceLimits.Memory().String()).To(Equal("100M"))
			Expect(resourceRequests.Memory().String()).To(Equal("100M"))
			warning, warningAnnotated := pod.Annotations[ranwpparameters.AnnotationWpMutationWarning]
			Expect(warningAnnotated).To(BeTrue())
			Expect(warning).To(ContainSubstring(ranwpparameters.WarningQoSGuaranteed))
		})
		By("Checking correct cpushares and affinity in crio", func() {
			containersInfo := ranwphelper.GetContainersInfo(node)
			for _, containerInfo := range containersInfo {
				if containerInfo.Namespace == pod.Namespace && containerInfo.PodName == pod.Name {
					Expect(containerInfo.Shares).To(BeEquivalentTo(1024))
					podCPUSet := cpuset.MustParse(containerInfo.Cpus)
					// For Guaranteed pod with 1 cpu requested, expect it to affine to 1 isolated cpu.
					Expect(podCPUSet.IsSubsetOf(isolCPUSet)).To(BeTrue())
					Expect(len(podCPUSet.ToSliceNoSort())).To(BeEquivalentTo(1))

					return
				}
			}
			Fail(fmt.Sprintf("test pod %s is not found in crictl ps", pod.Name))
		})
	})
})

// checkCPUShares checks cpu shares for all running management containers match with the wp pod annotation.
// For management pods without explicit cpu request, the expected cpushares is 2. Cpushare annotation should be
// added to Best Effort pod but not Burstable Pod.
func checkCPUShares(containersInfo []ranwphelper.ContainerInfo) {
	// Gather cpu share information from pod annotation
	allNamespaces, err := helper.Apiclient.Namespaces().List(context.Background(), metav1.ListOptions{})
	Expect(err).ToNot(HaveOccurred())

	containerShares := make(map[string]int)

	for _, ns := range allNamespaces.Items {
		if ranwpparameters.NonMgmtNamespaces.Has(ns.Name) {
			continue
		}

		pods, err := helper.Apiclient.Pods(ns.Name).List(context.Background(), metav1.ListOptions{})
		Expect(err).ToNot(HaveOccurred())

		containerShares = getPodsCPUShares(pods.Items, containerShares)
	}
	// Check cpu shares for all running containers by comparing the value with pod annotation
	for _, containerInfo := range containersInfo {
		if ranwphelper.IsNonMgmtPod(containerInfo.PodName, containerInfo.Namespace) {
			continue
		}

		searchKey := fmt.Sprintf("%s*%s*%s", containerInfo.Namespace, containerInfo.PodName, containerInfo.Name)
		valInAnnotation, ok := containerShares[searchKey]

		if ok {
			// Check crio cpu shares matches with pod annotation if annotation exists.
			Expect(valInAnnotation).To(BeEquivalentTo(
				containerInfo.Shares),
				"cpu shares value in crio mismatches pod annotation")
		} else {
			// For a burstable management pod, if a container has no cpu request, then no annotation would be
			// added for this container. In this case, expected cpushares in crio should be 2.
			pod, err := helper.Apiclient.Pods(containerInfo.Namespace).Get(
				context.Background(),
				containerInfo.PodName,
				metav1.GetOptions{},
			)
			Expect(err).ToNot(HaveOccurred())

			annotationExpectations := getMgmtCPUShareAnnotationExpectations(*pod)
			Expect(annotationExpectations[searchKey]).To(BeFalse(), "cpu share annotation is not found for %s", searchKey)
			Expect(containerInfo.Shares).To(BeEquivalentTo(2), "cpu share in crio is not 2 for %s", searchKey)
		}
	}
}

// getMgmtCPUShareAnnotationExpectations returns a map with a formatted container name as key, and cpu share
// expectation as value. A management container is not expected to have cpushare annotation if it has no explicit
// cpu request and pod is Burstable.
func getMgmtCPUShareAnnotationExpectations(pod corev1.Pod) map[string]bool {
	containerSharesExpectations := make(map[string]bool)

	for _, container := range pod.Spec.Containers {
		expectation := true
		if pod.Status.QOSClass == corev1.PodQOSBurstable && container.Resources.Requests.Cpu().Value() == int64(0) {
			expectation = false

			for k, v := range container.Resources.Requests {
				if k == ranwpparameters.AnnotationWpResource && v.Value() > int64(0) {
					// If cpushares resource request exists, then we expect the annotation to be added.
					expectation = true
				}
			}
		}

		containerSharesExpectations[fmt.Sprintf("%s*%s*%s", pod.Namespace, pod.Name, container.Name)] = expectation
	}

	return containerSharesExpectations
}

// getPodsCPUShares retrieves containers' wp cpu shares from pod annotations.
func getPodsCPUShares(pods []corev1.Pod, containerShares map[string]int) map[string]int {
	for _, pod := range pods {
		for key, val := range pod.Annotations {
			if strings.HasPrefix(key, ranwpparameters.AnnotationPrefixCPUShare) {
				r := regexp.MustCompile(`"cpushares":\s?([0-9]+),?.*\z`)
				cpushare, err := strconv.Atoi(strings.TrimSpace(r.FindStringSubmatch(val)[1]))
				Expect(err).ToNot(HaveOccurred())

				containerName := strings.TrimSpace(strings.Split(key, "/")[1])
				containerShares[fmt.Sprintf("%s*%s*%s", pod.Namespace, pod.Name, containerName)] = cpushare
			}
		}
	}

	return containerShares
}

// createsTestMgmtNamespace creates a test namespace with management workload partitioning annotation.
func createsTestMgmtNamespace() {
	if namespaces.Exists(ran.NamespaceTesting, helper.Apiclient) {
		return
	}

	By("creating a management test pod under test management namespace", func() {
		namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: ran.NamespaceTesting,
			Annotations: map[string]string{
				ranwpparameters.AnnotationWpNamespaceKey: ranwpparameters.AnnotationWpNamespaceValue},
			Labels: map[string]string{
				// Required for privileged pods in OCP 4.12 and newer
				"pod-security.kubernetes.io/audit":               "privileged",
				"pod-security.kubernetes.io/enforce":             "privileged",
				"pod-security.kubernetes.io/warn":                "privileged",
				"security.openshift.io/scc.podSecurityLabelSync": "false",
			},
		}}
		_, err := helper.Apiclient.Namespaces().Create(context.Background(), namespace, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
	})
}
