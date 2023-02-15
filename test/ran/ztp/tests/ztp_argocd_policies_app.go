package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

var _ = Describe("ZTP Argocd policies Tests", Ordered, Label("ztp-argocd-policies"), func() {

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
			testGitPath := ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Path + "/ztp-test/custom-interval"

			By("Updating the Argocd app", func() {
				// Update the Argo app to point to the new test kustomization
				err := ranztphelper.SetGitDetailsInArcgocd(
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Repo,
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Branch,
					testGitPath,
					ranztpparameters.ArgocdPoliciesAppName,
					true,
					true,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("Waiting for policies to be created", func() {
				err := ranztphelper.WaitForPolicyToExist(
					"custom-interval-policy-default",
					ranztpparameters.ZtpTestNamespace,
					5*time.Minute,
				)
				Expect(err).ToNot(HaveOccurred())
				err = ranztphelper.WaitForPolicyToExist(
					"custom-interval-policy-override",
					ranztpparameters.ZtpTestNamespace,
					5*time.Minute,
				)
				Expect(err).ToNot(HaveOccurred())
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
			testGitPath := ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Path + "/ztp-test/invalid-interval"

			By("Updating the Argocd app", func() {
				// Update the Argo app to point to the new test kustomization
				err := ranztphelper.SetGitDetailsInArcgocd(
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Repo,
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Branch,
					testGitPath,
					ranztpparameters.ArgocdPoliciesAppName,
					true,
					false,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("Checking the Argocd conditions for the expected error", func() {
				// Get the conditions from the Argocd app
				expectedMessage := "evaluationInterval.compliant 'time: invalid duration"
				err := ranztphelper.WaitForConditionInArgocdApp(
					ranztphelper.HubAPIClient,
					ranztpparameters.ArgocdPoliciesAppName,
					ranztpparameters.OpenshiftGitops,
					expectedMessage, 5*time.Minute,
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("with an image registry configured on the du profile", Label("ztp-image-registry"), func() {
		It("should validate the image registry exists", func() {
			// https://issues.redhat.com/browse/CNF-6301

			// The ztp test data is stored in a nested directory within the ztp repo
			testGitPath := ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Path + "/ztp-test/image-registry"

			By("Updating the Argocd app", func() {
				// Update the Argo app to point to the new test kustomization
				err := ranztphelper.SetGitDetailsInArcgocd(
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Repo,
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Branch,
					testGitPath,
					ranztpparameters.ArgocdPoliciesAppName,
					true,
					true,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			// The list of policies that should be created by the test
			policies := []string{
				"image-registry-policy-sc",
				"image-registry-policy-pvc",
				"image-registry-policy-pv",
				"image-registry-policy-config",
			}

			By("Waiting for policies to exist", func() {
				for _, policy := range policies {
					err := ranztphelper.WaitForPolicyToExist(policy, ranztpparameters.ZtpTestNamespace, 3*time.Minute)
					Expect(err).ToNot(HaveOccurred())
				}
			})

			By("Waiting for the policies to be in valid state", func() {
				for _, policy := range policies {
					err := ranztphelper.WaitForPolicyToHaveComplianceState(
						policy,
						ranztpparameters.ZtpTestNamespace,
						policiesv1.Compliant,
						3*time.Minute,
					)
					Expect(err).ToNot(HaveOccurred())
				}
			})

			By("Getting the image registry")

			imageRegistry, err := ranztphelper.SpokeAPIClient.ImageregistryV1Interface.Configs().Get(
				ranztphelper.GetZtpContext(),
				"cluster",
				v1.GetOptions{},
			)
			Expect(err).ToNot(HaveOccurred())

			By("Validating the image registry", func() {
				Expect(imageRegistry.Spec.Storage.PVC.Claim).To(Equal("image-registry-pvc"))
			})
		})
	})

	AfterEach(func() {
		// Reset the policies app back to default after each test
		By("Resetting the policies app back to the original settings", func() {
			err := ranztphelper.SetGitDetailsInArcgocd(
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Repo,
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Branch,
				ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Path,
				ranztpparameters.ArgocdPoliciesAppName,
				true,
				false,
			)
			Expect(err).ToNot(HaveOccurred())
		})

		// Delete any leftovers from the image registry test
		By("Removing the image registry leftovers if any exist", func() {
			err := ranztphelper.CleanupImageRegistryConfig(
				"image-registry-sc",
				ranztpparameters.ImageRegistryNamespace,
				"image-registry-pv-filesystem",
				"image-registry-pvc",
				"default",
				"cluster",
				ranztphelper.SpokeAPIClient,
			)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
