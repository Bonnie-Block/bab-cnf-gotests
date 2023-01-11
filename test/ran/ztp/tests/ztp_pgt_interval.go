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
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
)

var _ = Describe("Talm ZTP PGT Interval Tests", func() {

	// These tests use the hub and spoke
	var clusterList []*testClient.ClientSet

	execute.BeforeAll(func() {
		// Initialize cluster list
		clusterList = ranztphelper.GetAllTestClients()
	})

	BeforeEach(func() {
		// Check that the required clusters are present
		err := ranhelper.IsClustersPresent(clusterList)
		if err != nil {
			Skip(fmt.Sprintf("error occurred validating required clusters are present: %s", err.Error()))
		}
	})

	Context("override the PGT policy's compliance and non-compliance intervals", func() {
		It("should specify new intervals and verify they were applied", func() {
			// https://issues.redhat.com/browse/CNF-6305

			// The ztp test data is stored in a nested directory within the ztp repo
			testGitPath := ranztphelper.ZtpGitDir + "/ztp-test/custom-interval"

			// Update the Argo app to point to the new test kustomization
			err := ranztphelper.SetGitDetailsInArcgocd(ranztphelper.ZtpGitRepo, ranztphelper.ZtpGitBranch, testGitPath, true)
			Expect(err).ToNot(HaveOccurred())

			log.Println("Sleeping to wait for policies to be created")
			time.Sleep(1 * time.Minute)

			// Get the default policy from ACM
			defaultComplianceInterval, defaultNonComplianceInterval, err := ranztphelper.GetEvaluationIntervals("custom-interval-policy-default", ranztpparameters.ZtpTestNamespace)
			Expect(err).ToNot(HaveOccurred())

			// Assert that the policy intervals are 1m
			Expect(defaultComplianceInterval == "1m")
			Expect(defaultNonComplianceInterval == "1m")

			// Get the override policy from ACM
			overrideComplianceInterval, overrideNonComplianceInterval, err := ranztphelper.GetEvaluationIntervals("custom-interval-policy-override", ranztpparameters.ZtpTestNamespace)
			Expect(err).ToNot(HaveOccurred())

			// Assert that the policy intervals are 2m
			Expect(overrideComplianceInterval == "2m")
			Expect(overrideNonComplianceInterval == "2m")

		})
		It("should specify an invalid interval format and verify the app error", func() {
			// https://issues.redhat.com/browse/CNF-6306
			// The ztp test data is stored in a nested directory within the ztp repo
			testGitPath := ranztphelper.ZtpGitDir + "/ztp-test/invalid-interval"

			// Update the Argo app to point to the new test kustomization
			err := ranztphelper.SetGitDetailsInArcgocd(ranztphelper.ZtpGitRepo, ranztphelper.ZtpGitBranch, testGitPath, false)
			Expect(err).ToNot(HaveOccurred())

			// Argocd will refuse to create the policy since the time format is invalid
			log.Println("Sleeping to wait for policies to be attempted")
			time.Sleep(1 * time.Minute)

			// Get the conditions from the Argocd app
			expectedMessage := "evaluationInterval.compliant 'time: invalid duration"
			err = ranztphelper.WaitForConditionInArgocdApp(ranztphelper.HubAPIClient, ranztpparameters.Policies, ranztpparameters.OpenshiftGitops, expectedMessage, 5 * time.Minute)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
