package tests

import (
	"fmt"
	"log"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	imageregistryv1 "github.com/openshift/api/imageregistry/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	corev1 "k8s.io/api/core/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

var _ = Describe("ZTP Argocd policies Tests", Ordered, Label("ztp-argocd-policies"), func() {

	// These tests use the hub and spoke
	var clusterList []*testClient.ClientSet
	var snoNode *corev1.Node

	BeforeAll(func() {
		// Initialize cluster list
		clusterList = ranztphelper.GetAllTestClients()

		// get SNO
		nodeList, err := ranhelper.GetSnoNodes()
		Expect(err).ToNot(HaveOccurred())
		snoNode = nodeList[0]
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
				"4.11",
				"",
			) {
				Skip(fmt.Sprintf(
					"unable to run test on ztp version '%s' as it is less than minimum '%s",
					ranztphelper.ZtpVersion,
					"4.11",
				))
			}
		})
	})

	Context("override the PGT policy's compliance and non-compliance intervals", Label("ztp-pgt-interval"), func() {
		It("should specify new intervals and verify they were applied", func() {
			// https://issues.redhat.com/browse/CNF-6305

			// The ztp test data is stored in a nested directory within the ztp repo
			testGitPath := ranztphelper.JoinGitPaths(
				[]string{
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Path,
					"ztp-test/custom-interval",
				},
			)

			By("Checking if the git path exists", func() {
				if !ranztphelper.DoesGitPathExist(
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Repo,
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Branch,
					testGitPath+"/kustomization.yaml",
				) {
					Skip(fmt.Sprintf("git path '%s' could not be found", testGitPath))
				}
			})

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
					ranztpparameters.ArgocdChangeTimeout,
				)
				Expect(err).ToNot(HaveOccurred())
				err = ranztphelper.WaitForPolicyToExist(
					"custom-interval-policy-override",
					ranztpparameters.ZtpTestNamespace,
					ranztpparameters.ArgocdChangeTimeout,
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
			testGitPath := ranztphelper.JoinGitPaths(
				[]string{
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Path,
					"ztp-test/invalid-interval",
				},
			)

			By("Checking if the git path exists", func() {
				if !ranztphelper.DoesGitPathExist(
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Repo,
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Branch,
					testGitPath+"/kustomization.yaml",
				) {
					Skip(fmt.Sprintf("git path '%s' could not be found", testGitPath))
				}
			})

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
					expectedMessage, ranztpparameters.ArgocdChangeTimeout,
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("with an image registry configured on the du profile", Label("ztp-image-registry"), func() {
		var imageRegistryConfig *imageregistryv1.Config
		const imageReistryPath string = "/var/imageregistry"
		const imageRegistryConfigName string = "cluster"

		BeforeEach(func() {
			log.Printf("Checking if the image registry directory '%s' is present on test cluster\n", imageReistryPath)
			_, err := helper.ExecCommandOnNodeWithHostBinaries(snoNode,
				[]string{"ls", imageReistryPath})
			if err != nil {
				Skip("The image registry directory '%s' should be pre-created but it doesn't exist.")
			}

			log.Printf("Getting the current image registry config '%s'\n", imageRegistryConfigName)
			// The image registry config "cluster" is the default config auto-created
			// by the image registry operator after the cluster is up. We expect it to be exist.
			imageRegistryConfig, err = ranztphelper.GetImageRegistryConfig(
				imageRegistryConfigName, ranztphelper.SpokeAPIClient)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if imageRegistryConfig == nil {
				Skip("No need to restore image registry config.")
			}

			// Reset the app back to the original repo to make sure the later restore/cleanup
			// actions will not be impacted by the enforce policies
			By("Resetting the policies app back to the original settings", func() {
				err := ranztphelper.SetGitDetailsInArcgocd(
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Repo,
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Branch,
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Path,
					ranztpparameters.ArgocdPoliciesAppName,
					true,
					true,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("Restoring the image registry configs", func() {
				err := ranztphelper.RestoreImageRegistryConfig(
					imageRegistryConfigName,
					imageRegistryConfig,
					ranztphelper.SpokeAPIClient,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			// Delete any leftovers from the image registry test
			By("Removing the image registry leftovers if any exist", func() {
				err := ranztphelper.CleanupImageRegistryConfig(
					"image-registry-sc",
					"image-registry-pv-filesystem",
					"image-registry-pvc",
					ranztpparameters.ImageRegistryNamespace,
					ranztphelper.SpokeAPIClient,
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		It("should validate the image registry exists", func() {
			// https://issues.redhat.com/browse/CNF-6301

			// The ztp test data is stored in a nested directory within the ztp repo
			testGitPath := ranztphelper.JoinGitPaths(
				[]string{
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Path,
					"ztp-test/image-registry",
				},
			)

			By("Checking if the git path exists", func() {
				if !ranztphelper.DoesGitPathExist(
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Repo,
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Branch,
					testGitPath+"/kustomization.yaml",
				) {
					Skip(fmt.Sprintf("git path '%s' could not be found", testGitPath))
				}
			})

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
					err := ranztphelper.WaitForPolicyToExist(
						policy,
						ranztpparameters.ZtpTestNamespace,
						ranztpparameters.ArgocdChangeTimeout,
					)
					Expect(err).ToNot(HaveOccurred())
				}
			})

			By("Waiting for the policies to be in the Compliant state", func() {
				for _, policy := range policies {
					err := ranztphelper.WaitForPolicyToHaveComplianceState(
						policy,
						ranztpparameters.ZtpTestNamespace,
						policiesv1.Compliant,
						ranztpparameters.ArgocdChangeTimeout,
					)
					Expect(err).ToNot(HaveOccurred())
				}
			})

			By("Waiting for the image registry config to be in the Available state", func() {
				err := ranztphelper.WaitForConditionInImageRegistryConfig(
					imageRegistryConfigName,
					"Available",
					"Ready",
					"True",
					ranztphelper.SpokeAPIClient)
				Expect(err).ToNot(HaveOccurred())
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
	})
})
