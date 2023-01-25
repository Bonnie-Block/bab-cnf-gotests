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

var _ = Describe("ZTP PGT Tests", Ordered, Label("ztp-pgt"), func() {

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

	Context("override the PGT policy's compliance and non-compliance intervals", Label("ztp-pgt-interval"), func() {
		It("should specify new intervals and verify they were applied", func() {
			// https://issues.redhat.com/browse/CNF-6305

			// The ztp test data is stored in a nested directory within the ztp repo
			testGitPath := ranztphelper.ZtpGitDir + "/ztp-test/custom-interval"

			By("Updating the Argocd app", func() {
				// Update the Argo app to point to the new test kustomization
				err := ranztphelper.SetGitDetailsInArcgocd(
					ranztphelper.ZtpGitRepo,
					ranztphelper.ZtpGitBranch,
					testGitPath,
					true,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("Waiting for policies to be created", func() {
				log.Println("Sleeping for 1 minute")
				time.Sleep(1 * time.Minute)
			})

			By("Validing the interval on the default policy", func() {
				// Get the default policy from ACM
				defaultComplianceInterval, defaultNonComplianceInterval, err := ranztphelper.
					GetEvaluationIntervals(
						"custom-interval-policy-default",
						ranztpparameters.ZtpTestNamespace,
					)
				Expect(err).ToNot(HaveOccurred())

				// Assert that the policy intervals are 1m
				Expect(defaultComplianceInterval == "1m")
				Expect(defaultNonComplianceInterval == "1m")
			})

			By("Validating the interval on the overridden policy", func() {
				// Get the override policy from ACM
				overrideComplianceInterval, overrideNonComplianceInterval, err := ranztphelper.
					GetEvaluationIntervals(
						"custom-interval-policy-override",
						ranztpparameters.ZtpTestNamespace,
					)
				Expect(err).ToNot(HaveOccurred())

				// Assert that the policy intervals are 2m
				Expect(overrideComplianceInterval == "2m")
				Expect(overrideNonComplianceInterval == "2m")
			})
		})
		It("should specify an invalid interval format and verify the app error", func() {
			// https://issues.redhat.com/browse/CNF-6306
			// The ztp test data is stored in a nested directory within the ztp repo
			testGitPath := ranztphelper.ZtpGitDir + "/ztp-test/invalid-interval"

			By("Updating the Argocd app", func() {
				// Update the Argo app to point to the new test kustomization
				err := ranztphelper.SetGitDetailsInArcgocd(ranztphelper.ZtpGitRepo, ranztphelper.ZtpGitBranch, testGitPath, false)
				Expect(err).ToNot(HaveOccurred())
			})

			By("Waiting for the policies to be attempted", func() {
				// Argocd will refuse to create the policy since the time format is invalid
				log.Println("Sleeping for 1 minute")
				time.Sleep(1 * time.Minute)
			})

			By("Checking the Argocd conditions for the expected error", func() {
				// Get the conditions from the Argocd app
				expectedMessage := "evaluationInterval.compliant 'time: invalid duration"
				err := ranztphelper.WaitForConditionInArgocdApp(
					ranztphelper.HubAPIClient,
					ranztpparameters.Policies,
					ranztpparameters.ZtpDeployedNamespace,
					expectedMessage, 5*time.Minute,
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
