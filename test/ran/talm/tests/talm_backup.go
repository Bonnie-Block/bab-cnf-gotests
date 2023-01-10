package tests

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	configurationPolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
)

const (
	backupPath  = "/var/recovery"
	ranTestPath = "/var/ran-test-talm-recovery"
	fsSize      = "100M"
	nodeUser    = "core"
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

			// Delete temporary namespace on spoke cluster.
			spokeClusterList := []*testClient.ClientSet{rantalmhelper.Spoke1APIClient}
			err := rantalmhelper.CleanupNamespace(spokeClusterList, rantalmhelper.TemporaryNamespaceName)
			Expect(err).ToNot(HaveOccurred())
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

			// apply
			err := rantalmhelper.CreatePolicyAndCgu(
				rantalmhelper.HubAPIClient,
				rantalmhelper.GetNamespaceDefinition(rantalmhelper.TemporaryNamespaceName),
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
		})
	})

	Context("backup is enabled in CGU. ", func() {
		curName := "backupsequence"
		// created cguEnabled boolean
		cguEnabled := false
		AfterEach(func() {
			// Delete generated CRs on Hub Cluster.
			hubErrList := rantalmhelper.CleanupTestResourcesOnClient(
				rantalmhelper.HubAPIClient,
				fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmhelper.PolicyName, curName),
				rantalmhelper.Namespace,
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementBindingCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementRuleCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmhelper.PolicySetName, curName),
				"",
				false,
			)
			Expect(len(hubErrList)).To(Equal(0))

			// Delete temporary namespace on spoke cluster.
			spokeClusterList := []*testClient.ClientSet{rantalmhelper.Spoke1APIClient}
			err := rantalmhelper.CleanupNamespace(spokeClusterList, rantalmhelper.TemporaryNamespaceName)
			Expect(err).ToNot(HaveOccurred())

		})
		// ocp-54294, ocp-54295
		It("verifies backup begins and succeeds after CGU is enabled", func() {
			// Create namespace
			err := namespaces.Create(rantalmhelper.Namespace, rantalmhelper.HubAPIClient)
			Expect(err).ToNot(HaveOccurred())

			By("creating a disabled cgu with backup enabled")
			// prep cgu
			cgu := rantalmhelper.GetCguDefinition(
				fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, curName),
				[]string{rantalmhelper.Spoke1Name},
				[]string{},
				[]string{fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName)},
				rantalmparameters.TalmTestNamespace, 1, 30)

			cgu.Spec.Backup = true
			// passing reference to cguEnabled because cgu.Spec.Enable is of type BoolAddr
			cgu.Spec.Enable = &cguEnabled

			// apply cgu
			err = rantalmhelper.CreatePolicyAndCgu(
				rantalmhelper.HubAPIClient,
				rantalmhelper.GetNamespaceDefinition(rantalmhelper.TemporaryNamespaceName),
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
			Expect(err).ToNot(HaveOccurred())

			By("checking backup does not begin when CGU is disabled")

			err = rantalmhelper.WaitForBackupStart(
				rantalmhelper.HubAPIClient,
				cgu.Name,
				cgu.Namespace,
				2*time.Minute,
			)
			Expect(err).To(HaveOccurred())

			By("enalble CGU")
			err = rantalmhelper.EnableCgu(rantalmhelper.HubAPIClient, cgu)
			Expect(err).ToNot(HaveOccurred())

			By("waiting for backup to begin")
			err = rantalmhelper.WaitForBackupStart(
				rantalmhelper.HubAPIClient,
				cgu.Name,
				cgu.Namespace,
				1*time.Minute,
			)
			Expect(err).ToNot(HaveOccurred())

			// Wait for spoke cluster backup to finish and report Succeeded.
			By("waiting for cgu to indicate backup succeeded for spoke")
			assertBackupStatus(cgu.Name, rantalmhelper.Spoke1Name, "Succeeded")

		})
	})
})

var _ = Describe("Talm Backup Tests with two spokes", Ordered, func() {
	curName := "disk-full-multiple-spokes"

	BeforeAll(func() {
		// tests below requires all clusters to be present. hub + spoke1 + spoke2
		clusterList := rantalmhelper.GetAllTestClients()
		// Check that the required clusters are present
		err := ranhelper.IsClustersPresent(clusterList)
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
		// Delete temporary namespace on spoke cluster.
		spokeClusterList := []*testClient.ClientSet{rantalmhelper.Spoke1APIClient, rantalmhelper.Spoke2APIClient}
		err := rantalmhelper.CleanupNamespace(spokeClusterList, rantalmhelper.TemporaryNamespaceName)
		Expect(err).ToNot(HaveOccurred())
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

		// apply
		err := rantalmhelper.CreatePolicyAndCgu(
			rantalmhelper.HubAPIClient,
			rantalmhelper.GetNamespaceDefinition(rantalmhelper.TemporaryNamespaceName),
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
	})

})

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
				safeToDeleteBackupDir = false

				log.Printf("partition detected for %s, "+
					"will not attempt to delete the folder (only the content if any)", backupPath)
			} else if deviceType == "loop" {

				if currentlyUsingLoopDevicePath == devicePath {
					// unmount and detach the loop device
					_, err = ranhelper.ExecSSHCommand(nodeName, nodeUser,
						[]string{fmt.Sprintf("sudo umount --detach-loop %s", backupPath)})
					Expect(err).To(BeNil())

				} else {
					safeToDeleteBackupDir = false
					log.Print("WARNING: most likely cleanup didnt complete during the previous run. ")
					/*
						Assuming loop0 is the unwanted one...
						look for clues with lsblk
						$ lsblk
						NAME   MAJ:MIN RM   SIZE RO TYPE MOUNTPOINT
						loop0    7:0    0   100M  0 loop /var/recovery -----> this line should not be there

						unmount it with: `sudo umount --detach-loop /var/recovery`
						check lsblk to verify there's nothing mounted to loop0 and line is gone completely

						if line is still there (but unmounted) make use `losetup` to see the status of loopdevice (loop0)
						$ losetup
						NAME       SIZELIMIT OFFSET AUTOCLEAR RO BACK-FILE                                      DIO LOG-SEC
						/dev/loop0         0      0         1  0 /var/ran-test-talm-recovery/100M.img (deleted)   0     512

						if you see (deleted) -- reboot the node. i.e sudo reboot.
						Once back loop0 should not appear anywhere (lsblk + losetup)

					*/
					log.Printf("See comments for manual cleanup of %s\n", devicePath)
				}
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
