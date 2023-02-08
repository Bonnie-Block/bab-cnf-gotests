package tests

import (
	"fmt"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
)

var _ = Describe("ZTP Argocd clusters Tests", Ordered, Label("ztp-argocd-clusters"), func() {

	// These tests use the hub and spoke
	var clusterList []*testClient.ClientSet

	BeforeAll(func() {
		// Initialize cluster list
		clusterList = ranztphelper.GetAllTestClients()
	})

	BeforeEach(func() {
		// Check that the required clusters are present
		err := ranhelper.IsClustersPresent(clusterList)
		if err != nil {
			Skip(fmt.Sprintf("error occurred validating required clusters are present: %s", err.Error()))
		}
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

	Context("override the klusterlet addon configuration", Label("ztp-klusterlet"), func() {
		It("should override the klusterlet addon configuration and verify the change", func() {
			// https://issues.redhat.com/browse/CNF-6299
			// The ztp test data is stored in a nested directory within the ztp repo
			testGitPath := ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Path + "/ztp-test/klusterlet-addon"

			By("updating the argo app", func() {
				// Update the Argo app to point to the new test kustomization
				err := ranztphelper.SetGitDetailsInArcgocd(
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Repo,
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Branch,
					testGitPath,
					ranztpparameters.ArgocdClustersAppName,
					false,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("waiting for the change to take effect", func() {
				log.Println("Sleeping to wait for changes to take effect")
				time.Sleep(1 * time.Minute)
			})

			By("Validating the klusterlet addon change occurred", func() {
				// Get the klusterlet addon configuration
				kac, err := ranztphelper.GetKlusterletConfiguration(ranztphelper.SpokeName, ranztphelper.SpokeName)
				Expect(err).ToNot(HaveOccurred())

				// Validate that the search collector is enabled now
				Expect(kac.Spec.SearchCollectorConfig.Enabled).To(Equal(true))
			})
		})
	})

	AfterEach(func() {
		// Reset the clusters app back to default after each test
		By("Resetting the clusters app back to the original settings", func() {
			err := ranztphelper.SetGitDetailsInArcgocd(
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Repo,
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Branch,
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdClustersAppName].Path,
				ranztpparameters.ArgocdClustersAppName,
				false,
			)
			Expect(err).ToNot(HaveOccurred())
		})

		By("waiting for the change to take effect", func() {
			log.Println("Sleeping to wait for changes to take effect")
			time.Sleep(1 * time.Minute)
		})
	})
})
