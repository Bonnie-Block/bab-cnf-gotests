package ranreboothelper

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
