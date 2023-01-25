package tests

import (
	"fmt"
	"log"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ZTP Machine Config Tests", Ordered, Label("ztp-machine-config"), func() {

	const annotationString string = "ran.openshift.io/ztp-gitops-generated"

	// These tests use the hub and spoke
	var clusterList []*testClient.ClientSet

	BeforeAll(func() {
		// Initialize cluster list
		clusterList = ranztphelper.GetAllTestClients()
	})

	BeforeEach(func() {
		// Check that the required clusters are present
		By("Checking that required clusters are present", func() {
			err := ranhelper.IsClustersPresent(clusterList)
			if err != nil {
				Skip(fmt.Sprintf("error occurred validating required clusters are present: %s", err.Error()))
			}
		})
		// Check for minimum ztp version
		By("Checking the ZTP version", func() {
			if !ranhelper.IsVersionStringInRange(
				ranztphelper.ZtpVersion,
				ranztpparameters.MinimumZtpVersion,
				"",
			) {
				Skip(fmt.Sprintf(
					"unable to run test on ztp version '%s' as it is less than minimum '%s",
					ranztphelper.ZtpVersion,
					ranztpparameters.MinimumZtpVersion,
				))
			}
		})
	})

	Context("should check machine config for ztp annotation", func() {
		// https://issues.redhat.com/browse/CNF-6300
		It("should find the annotation present in the machine configs", func() {

			// We need to check all of the machine configs that match these names
			var machineConfigs = [...]string{
				"02-master-workload-partitioning",
				"container-mount-namespace-and-kubelet-conf-master",
				"container-mount-namespace-and-kubelet-conf-worker",
			}

			// Keep track of how many machine configs we checked
			checkedConfigs := 0

			By("Checking all the available machine configs for ones that were deployed by ztp", func() {
				// Get the full list of machine configs
				machineConfigList, err := ranztphelper.SpokeAPIClient.MachineconfigurationV1Interface.MachineConfigs().List(
					ranztphelper.GetZtpContext(),
					v1.ListOptions{},
				)
				Expect(err).ToNot(HaveOccurred())

				// Loop over all the machine configs
				for _, machineConfig := range machineConfigList.Items {
					// Check if this machine config matches any of the ones we need to check
					for _, matchString := range machineConfigs {
						// If it does not match then continue to the next check
						if !strings.Contains(machineConfig.Name, matchString) {

							continue
						}

						// Increment this counter so we can do an additional check at the end
						checkedConfigs++

						log.Printf("Checking mc '%s' for annotation '%s'\n", machineConfig.Name, annotationString)

						// Expect the annotation to be defined
						Expect(machineConfig.Annotations[annotationString]).To(Equal("{}"))
					}
				}
			})

			By("Checking if there were any matches", func() {
				// If we did not find any matching machine configs then consider that a skip
				if checkedConfigs == 0 {
					Skip("no matching machine configs were found")
				}
			})
		})
	})
})
