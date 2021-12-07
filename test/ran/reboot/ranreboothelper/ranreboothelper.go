package ranreboothelper

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
)

// parseBmcInfo returns bmc username, password, and hosts from environment variables if exist.
func parseBmcInfo(conf *config.Config) (bmcUser, bmcPassword string, bmcHosts []string) {
	hostsEnvVar := conf.Ran.BmcHosts
	Expect(hostsEnvVar).ToNot(BeEmpty(), "Please set BMC_HOSTS environment variable.")
	hosts := strings.Split(hostsEnvVar, ",")

	return conf.Ran.BmcUser, conf.Ran.BmcPassword, hosts
}

// PowerOffAndOnSno powers off SNO node via BMC and wait for cluster to be unreachable
// Returns host power on timestamp.
func PowerOffAndOnSno() time.Time {
	user, password, hosts := parseBmcInfo(helper.Config)
	// Always attempt to power on host
	defer func() {
		errs := powerControlHosts(true, hosts, user, password)
		Expect(errs).To(BeEmpty())
	}()

	errs := powerControlHosts(false, hosts, user, password)
	Expect(errs).To(BeEmpty())
	waitForClusterUnreachable()
	// Wait for sometime before power on
	time.Sleep(30 * time.Second)
	powerOnTime := time.Now()

	return powerOnTime
}

// WaitForClusterReachable waits for cluster reachable by listing cluster nodes and expecting it to work.
func WaitForClusterReachable() {
	log.Println("Waiting for cluster to be reachable")

	apiTimeout := int64(30)

	Eventually(func() error {
		_, err := helper.Apiclient.Nodes().List(context.Background(), metav1.ListOptions{TimeoutSeconds: &apiTimeout})

		return err
	}, 15*time.Minute, 3*time.Second).ShouldNot(HaveOccurred())
	log.Println("Cluster is reachable")
}

// waitForClusterUnreachable waits for cluster unreachable by listing cluster nodes and expecting error.
func waitForClusterUnreachable() {
	timeout := 3 * time.Minute
	apiTimeout := int64(30)

	Eventually(func() bool {
		_, err := helper.Apiclient.Nodes().List(context.Background(), metav1.ListOptions{TimeoutSeconds: &apiTimeout})

		return err != nil
	}, timeout, 5*time.Second).Should(BeTrue(), fmt.Sprintf("cluster is still reachable after %s", timeout.String()))
	log.Println("Lost connection to cluster")
}

// IsIpmitoolExist returns true if ipmitool is installed on test executor, otherwise false.
func IsIpmitoolExist() bool {
	_, err := helper.ExecAndLogCommand(true, 10*time.Second, "which", "ipmitool")

	return err == nil
}

// powerControlHosts powers on or off given BMC hosts.
func powerControlHosts(powerOn bool, hosts []string, user, password string) []error {
	var (
		errs   []error
		action = "off"
	)

	if powerOn {
		action = "on"
	}

	for _, host := range hosts {
		powerStatus, _ := getHostPowerStatus(host, user, password)
		if !strings.Contains(powerStatus, fmt.Sprintf("Power is %s", action)) {
			_, err := execIpmiCommand(host, user, password, []string{"power", action})
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errs
}

// getHostPowerStatus returns host power status queried via ipmitool command.
func getHostPowerStatus(host, user, password string) (string, error) {
	output, err := execIpmiCommand(host, user, password, []string{"power", "status"})

	return string(output), err
}

// execIpmiCommand executes given ipmitool subcommands. e.g., power on, power status.
func execIpmiCommand(host, user, password string, subcommands []string) ([]byte, error) {
	args := []string{"-I", "lanplus", "-U", user, "-P", password, "-H", host, "chassis"}
	args = append(args, subcommands...)

	return helper.ExecAndLogCommand(true, 1*time.Minute, "ipmitool", args...)
}

// WaitForAllPodsHealthy waits for all pods on cluster or in given namespaces to be Completed or Running & Ready
// Returns a map of unhealthy pods if any, otherwise empty map
// When namespaces is an empty list or nil, ALL namespaces on cluster will be checked.
func WaitForAllPodsHealthy(
	namespaces []string, timeout, interval, stableDuration time.Duration) map[string]map[string]string {
	var (
		msgNs     = "in cluster"
		msgStable = ""
	)

	if len(namespaces) > 0 {
		msgNs = fmt.Sprintf("in namespaces %v", namespaces)
	}

	if stableDuration > 0 {
		msgStable = fmt.Sprintf(" for %s", stableDuration.String())
	}

	log.Printf("Waiting up to %s for all pods %s to be healthy%s\n", timeout.String(), msgNs, msgStable)

	unhealthyPods := make(map[string]map[string]string)
	apiTimeout := int64(10)
	startTime := time.Now()
	errPullInterval := wait.PollImmediate(interval, timeout, func() (bool, error) {
		var namespacesToCheck []string
		if len(namespaces) == 0 {
			namespaceList, err := helper.Apiclient.Namespaces().List(
				context.Background(),
				metav1.ListOptions{TimeoutSeconds: &apiTimeout},
			)
			if err != nil {
				startTime = time.Now()

				return false, nil
			}
			for _, ns := range namespaceList.Items {
				namespacesToCheck = append(namespacesToCheck, ns.Name)
			}
		} else {
			namespacesToCheck = append(namespacesToCheck, namespaces...)
		}
		unhealthyPods = make(map[string]map[string]string)
		for _, nsName := range namespacesToCheck {
			podsInNs, err := getUnhealthyPods(nsName)
			if err != nil {
				unhealthyPods[nsName] = map[string]string{"all pods": "failed to list pods"}
			} else if len(podsInNs) > 0 {
				unhealthyPods[nsName] = podsInNs
			}
		}
		if len(unhealthyPods) > 0 {
			startTime = time.Now()

			return false, nil
		}
		// All pods are in expected state. Check for stable duration if it's larger than zero.
		if stableDuration > 0 {
			actualStableDuration := time.Since(startTime)
			// Add an interval because the timer started before mcp became updated
			if actualStableDuration < stableDuration+interval {
				return false, nil
			}
		}

		return true, nil
	})

	if errPullInterval == nil {
		log.Printf("All pods %s are healthy%s", msgNs, msgStable)
	} else {
		log.Println(errPullInterval.Error())
	}

	return unhealthyPods
}

// getUnhealthyPods lists pods in given namespace and checks pods status.
// Returns pods in specified namespace that are neither Completed nor Running & Ready, and an error if lists pods
// failed.
func getUnhealthyPods(namespace string) (map[string]string, error) {
	unhealthyPods := make(map[string]string)
	pods, err := helper.Apiclient.Pods(namespace).List(context.Background(), metav1.ListOptions{})

	if err != nil {
		return unhealthyPods, err
	}

	for _, pod := range pods.Items {
		err = helper.IsPodHealthy(&pod)
		if err != nil && !(pod.Status.Phase == corev1.PodFailed && pod.Spec.RestartPolicy == corev1.RestartPolicyNever) {
			// Ignore failed pod with restart policy never. This could happen in image pruner or installer pods that
			// will never restart after completed. And could stuck in error in various conditions after initial completion.
			unhealthyPods[pod.Name] = err.Error()
		}
	}

	return unhealthyPods, nil
}
