package tests

import (
	"context"
	"log"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
)

// execSSHCommand retrieves mount point ionode from node via ssh.  Assumes rsa key is imported.
func execSSHCommand(host string, user string, subcommands []string) (string, error) {
	args := []string{"-o", "ConnectTimeout=10", "-o", "ControlMaster=auto", "-o", "ControlPersist=60s", "-o",
		"BatchMode=yes", "-o", "StrictHostKeyChecking=no", "-l", user, host}
	args = append(args, subcommands...)
	output, err := helper.ExecAndLogCommand(true, 1*time.Minute, "ssh", args...)

	return string(output), err
}

var _ = Describe("Container Namespace Hiding", func() {
	var (
		nodes *corev1.NodeList
		user  = "core"
		err   error
	)

	execute.BeforeAll(func() {

		// Get nodes for connection host
		nodes, err = helper.Apiclient.Nodes().List(context.Background(), metav1.ListOptions{})
		Expect(err).ToNot(HaveOccurred())

	})

	It("should not have kubelet and crio using the same ionode as systemd", func() {

		for _, node := range nodes.Items {
			host := node.Name

			log.Printf("Check systemd ionode on host %s... ", host)
			systemdionode, err := execSSHCommand(host, user, []string{"sudo", "readlink", "/proc/1/ns/mnt"})
			Expect(err).ToNot(HaveOccurred())
			log.Printf("systemd ionode: %s\n", systemdionode)

			log.Printf("Check kubelet ionode on host %s... ", host)
			kubeletionode, err := execSSHCommand(
				host, user, []string{"sudo", "readlink", "/proc/$(pgrep", "-n", "kubelet)/ns/mnt"})
			Expect(err).ToNot(HaveOccurred())
			log.Printf("kubelet ionode: %s\n", kubeletionode)

			log.Printf("Check crio ionode on host %s... ", host)
			crioionode, err := execSSHCommand(
				host, user, []string{"sudo", "readlink", "/proc/$(pgrep", "-n", "crio)/ns/mnt"})
			Expect(err).ToNot(HaveOccurred())
			log.Printf("crio ionode: %s\n", crioionode)

			Expect(kubeletionode).To(Equal(crioionode), "kubelet and crio ionodes do not match.")
			Expect(systemdionode).NotTo(Equal(kubeletionode),
				"systemd and kubelet ionodes do not match - namespace is not hidden.")
			Expect(systemdionode).NotTo(Equal(crioionode),
				"systemd and crio ionodes do not match - namespace is not hidden.")
		}
	})
})
