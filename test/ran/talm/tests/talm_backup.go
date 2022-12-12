package tests

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"

	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmparameters"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	configurationPolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
)

const (
	backupPath          = "/var/recovery"
	ranTestPath         = "/var/ran-test-talm-recovery"
	fsSize              = "100M"
	backupPodLabel      = "job-name=backup-agent"
	backupNs            = "openshift-talo-backup"
	backupContainerName = "container-image"
	nodeUser            = "core"
)

var (
	nodeName           string
	loopBackDevicePath string
)

var _ = Describe("Talm Backup Tests with single spoke", func() {

	// ocp-50835
	Context("with full disk for spoke1", func() {
		curName := "disk-full-single-spoke"
		BeforeEach(func() {
			By("setting up filesystem to simulate low space")
			nodeName = getNodeName(rantalmhelper.Spoke1APIClient)
			loopBackDevicePath = prepareEnvWithSmallMountPoint(nodeName, nodeUser)
		})

		AfterEach(func() {
			log.Println("starting disk-full env clean up")
			diskFullEnvCleanup(nodeName, nodeUser, curName, loopBackDevicePath)
		})

		It("should have a failed cgu for single spoke", func() {
			By("applying all the required CRs for backup")
			// prep cgu
			cgu := rantalmhelper.GetCguDefinition(
				fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, curName),
				[]string{rantalmhelper.Spoke1Name},
				[]string{},
				[]string{fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName)},
				rantalmparameters.TalmTestNamespace, 1, 250)
			cgu.Spec.Backup = true

			// prep clusterVersion
			clusterVersion, err := rantalmhelper.GetClusterVersionDefinition("Both",
				rantalmhelper.Spoke1APIClient)
			Expect(err).To(BeNil())

			// apply
			err = rantalmhelper.CreatePolicyAndCgu(
				rantalmhelper.HubAPIClient,
				clusterVersion,
				configurationPolicyv1.MustHave,
				configurationPolicyv1.Inform,
				fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PolicySetNameCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementBindingCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementRuleCommonName, curName),
				rantalmparameters.TalmTestNamespace,
				metav1.LabelSelector{},
				cgu,
			)
			Expect(err).To(BeNil())

			By("waiting for cgu to fail for spoke1")
			assertBackupStatus(cgu.Name, rantalmhelper.Spoke1Name, "UnrecoverableError")

			By("verifying insufficient disk error in talm backup pod log")
			assertBackupPodLog(rantalmhelper.Spoke1APIClient, "Insufficient disk space", corev1.PodFailed)
		})

	})
})

var _ = Describe("Talm Backup Tests with two spokes", Ordered, func() {
	curName := "disk-full-multiple-spokes"

	BeforeAll(func() {
		// tests below requires all clusters to be present. hub + spoke1 + spoke2
		clusterList := rantalmhelper.GetAllTestClients()
		// Check that the required clusters are present
		err := rantalmhelper.IsClustersPresent(clusterList)
		if err != nil {
			Skip(fmt.Sprintf("error occurred validating required clusters are present: %s", err.Error()))
		}
	})

	BeforeEach(func() {
		By("setting up filesystem to simulate low space")
		nodeName = getNodeName(rantalmhelper.Spoke1APIClient)
		loopBackDevicePath = prepareEnvWithSmallMountPoint(nodeName, nodeUser)
	})

	AfterEach(func() {
		log.Println("starting disk-full env clean up")
		diskFullEnvCleanup(nodeName, nodeUser, curName, loopBackDevicePath)
	})

	It("should not affect backup on second spoke in same batch", func() {
		By("applying all the required CRs for backup")
		// prep cgu
		cgu := rantalmhelper.GetCguDefinition(
			fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, curName),
			[]string{rantalmhelper.Spoke1Name, rantalmhelper.Spoke2Name},
			[]string{},
			[]string{fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName)},
			rantalmparameters.TalmTestNamespace, 100, 250)
		cgu.Spec.Backup = true

		// prep clusterVersion
		clusterVersion, err := rantalmhelper.GetClusterVersionDefinition("Both",
			rantalmhelper.Spoke1APIClient)
		Expect(err).To(BeNil())

		// apply
		err = rantalmhelper.CreatePolicyAndCgu(
			rantalmhelper.HubAPIClient,
			clusterVersion,
			configurationPolicyv1.MustHave,
			configurationPolicyv1.Inform,
			fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName),
			fmt.Sprintf("%s-%s", rantalmparameters.PolicySetNameCommonName, curName),
			fmt.Sprintf("%s-%s", rantalmparameters.PlacementBindingCommonName, curName),
			fmt.Sprintf("%s-%s", rantalmparameters.PlacementRuleCommonName, curName),
			rantalmparameters.TalmTestNamespace,
			metav1.LabelSelector{},
			cgu,
		)
		Expect(err).To(BeNil())

		By("waiting for cgu to indicate it failed for spoke1")
		assertBackupStatus(cgu.Name, rantalmhelper.Spoke1Name, "UnrecoverableError")

		By("waiting for cgu to indicate it succeeded for spoke2")
		assertBackupStatus(cgu.Name, rantalmhelper.Spoke2Name, "Succeeded")

		By("verifying insufficient disk error in talm backup pod log in spoke1")
		assertBackupPodLog(rantalmhelper.Spoke1APIClient, "Insufficient disk space", corev1.PodFailed)

		By("verifying no error talm backup pod log in spoke2")
		assertBackupPodLog(rantalmhelper.Spoke2APIClient, "successfully finished", corev1.PodSucceeded)
	})

})

// assertBackupPodLog retrieves the backup pod generated by job and asserts on the log.
func assertBackupPodLog(client *testClient.ClientSet, expectationSubString string, phase corev1.PodPhase) {
	// added Eventually since logs take longer to show up in console
	Eventually(func() string {
		podList, err := client.Pods(backupNs).List(context.Background(), metav1.ListOptions{
			LabelSelector: backupPodLabel,
		})
		Expect(err).To(BeNil())
		Expect(len(podList.Items)).To(BeNumerically("==", 1))
		p := podList.Items[0]
		Expect(p.Status.Phase).To(Equal(phase))
		plog, err := pod.GetLog(client, &p, -time.Until(p.CreationTimestamp.Time), backupContainerName)
		Expect(err).To(BeNil())
		log.Println("generated pod logs: \n", plog)

		return plog
	}, 1*time.Minute, 5*time.Second).Should(ContainSubstring(expectationSubString))
}

// assertBackupPodLog asserts status of backup struct.
func assertBackupStatus(cguName, spokeName, expectation string) {
	Eventually(func() string {
		cgu, err := rantalmhelper.HubAPIClient.ClustergroupupgradesoperatorV1alpha1Interface.
			ClusterGroupUpgrades(rantalmparameters.TalmTestNamespace).
			Get(context.Background(), cguName, metav1.GetOptions{})
		Expect(err).To(BeNil())

		if cgu.Status.Backup == nil {
			log.Println("backup struct not ready yet")

			return ""
		}

		_, ok := cgu.Status.Backup.Status[spokeName]
		if !ok {
			log.Println("cluster name as key did not appear yet")

			return ""
		}

		log.Printf("[%s] %s backup status: %s\n", cgu.Name, spokeName, cgu.Status.Backup.Status[spokeName])

		return cgu.Status.Backup.Status[spokeName]
	}, 10*time.Minute, 10*time.Second).Should(Equal(expectation))
}

// getNodeName get the name of the node to use for ssh.
func getNodeName(client *testClient.ClientSet) string {
	nodeList, err := client.CoreV1Interface.Nodes().List(context.Background(), metav1.ListOptions{})
	Expect(err).To(BeNil())
	Expect(len(nodeList.Items)).To(BeNumerically("==", 1))

	return nodeList.Items[0].Name
}

// diskFullEnvCleanup clean all the resources created for single cluster backup fail.
func diskFullEnvCleanup(nodeName, nodeUser, curName, currentlyUsingLoopDevicePath string) {
	// delete generated CRs
	rantalmhelper.CleanupTestResourcesOnClient(
		rantalmhelper.HubAPIClient,
		fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, curName),
		fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName),
		rantalmparameters.TalmTestNamespace,
		fmt.Sprintf("%s-%s", rantalmparameters.PlacementBindingCommonName, curName),
		fmt.Sprintf("%s-%s", rantalmparameters.PlacementRuleCommonName, curName),
		fmt.Sprintf("%s-%s", rantalmparameters.PolicySetNameCommonName, curName),
		"",
		false,
	)

	// check where backup dir is mounted and start clean up
	safeToDeleteBackupDir := true
	// retrieve all mounts for backup dir
	output, err := ranhelper.ExecSSHCommand(nodeName, nodeUser,
		[]string{fmt.Sprintf("findmnt -n -o SOURCE --target %s", backupPath)})
	Expect(err).To(BeNil())

	output = strings.TrimSuffix(output, "\n")
	if output != "" {
		outputArr := strings.Split(output, "\n")
		for _, devicePath := range outputArr {
			// retrieve all devices e.g part or loop
			deviceType, err := ranhelper.ExecSSHCommand(nodeName, nodeUser,
				[]string{fmt.Sprintf("lsblk %s -o TYPE -n", devicePath)})
			Expect(err).To(BeNil())

			deviceType = strings.TrimSuffix(deviceType, "\n")

			if deviceType == "part" {
				log.Printf("partition detected for %s, "+
					"will not attempt to delete the folder (only the content if any)", backupPath)

				safeToDeleteBackupDir = false
			} else if deviceType == "loop" && currentlyUsingLoopDevicePath == devicePath {
				// unmount and detach the loop device
				_, err = ranhelper.ExecSSHCommand(nodeName, nodeUser,
					[]string{fmt.Sprintf("sudo umount --detach-loop %s", backupPath)})
				Expect(err).To(BeNil())
			}
		}
	}

	// if true there was a partition (most likely ZTP /w MC) so delete content instead of the whole thing
	if safeToDeleteBackupDir {
		_, err = ranhelper.ExecSSHCommand(nodeName, nodeUser,
			[]string{fmt.Sprintf("sudo rm -rf %s", backupPath)})
		Expect(err).To(BeNil())
	} else {
		_, err = ranhelper.ExecSSHCommand(nodeName, nodeUser,
			[]string{fmt.Sprintf("sudo rm -rf %s/*", backupPath)})
		Expect(err).To(BeNil())
	}

	// delete ran-test-talm-recovery folder
	_, err = ranhelper.ExecSSHCommand(nodeName, nodeUser,
		[]string{fmt.Sprintf("sudo rm -rf %s", ranTestPath)})
	Expect(err).To(BeNil())
}

// prepareEnvWithSmallMountPoint use loopback device,
// a virtual file system backed by a file, to create a small partition
// helpful links https://stackoverflow.com/q/16044204 and https://youtu.be/r9CQhwci4tE
func prepareEnvWithSmallMountPoint(nodeName, nodeUser string) string {
	// create a dir for backup if not already there
	_, err := ranhelper.ExecSSHCommand(nodeName, nodeUser,
		[]string{fmt.Sprintf("sudo mkdir -p %s", backupPath)})
	Expect(err).To(BeNil())

	// create a dir for ran test dir if not already there
	_, err = ranhelper.ExecSSHCommand(nodeName, nodeUser,
		[]string{fmt.Sprintf("sudo mkdir -p %s", ranTestPath)})
	Expect(err).To(BeNil())

	// find the next available loopback device (OS takes care of creating a new one if needed)
	loopBackDevicePath, err := ranhelper.ExecSSHCommand(nodeName, nodeUser,
		[]string{"sudo losetup -f"})
	Expect(err).To(BeNil())

	loopBackDevicePath = strings.TrimSpace(loopBackDevicePath)
	log.Println("loopback device path: ", loopBackDevicePath)

	// create a file with desired size. It's where the file-system will live
	_, err = ranhelper.ExecSSHCommand(nodeName, nodeUser,
		[]string{fmt.Sprintf("sudo fallocate -l %s %s/%s.img", fsSize, ranTestPath, fsSize)})
	Expect(err).To(BeNil())

	// create the loop device by assigning it with the file. tip: use losetup -a to check the status
	_, err = ranhelper.ExecSSHCommand(nodeName, nodeUser,
		[]string{fmt.Sprintf("sudo losetup %s %s/%s.img", loopBackDevicePath, ranTestPath, fsSize)})
	Expect(err).To(BeNil())

	// format to your desired fs type. xfs is RH preferred but ext4 works too.
	_, err = ranhelper.ExecSSHCommand(nodeName, nodeUser,
		[]string{fmt.Sprintf("sudo mkfs.xfs -f -q %s", loopBackDevicePath)})
	Expect(err).To(BeNil())

	// mount the fs to backup dir
	_, err = ranhelper.ExecSSHCommand(nodeName, nodeUser,
		[]string{fmt.Sprintf("sudo mount %s %s", loopBackDevicePath, backupPath)})
	Expect(err).To(BeNil())

	return loopBackDevicePath
}
