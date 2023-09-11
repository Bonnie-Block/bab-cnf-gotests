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
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

var _ = Describe("ZTP Argocd ACM CR Tests", Ordered, Label("ztp-argocd-acm-crs"), func() {

	// These tests use the hub and spoke
	var clusterList []*testClient.ClientSet

	var acmPolicyGeneratorImage string
	var oldAcmPolicyGeneratorContainerConfiguration corev1.Container

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
				"4.12",
				"",
			) {
				Skip(fmt.Sprintf(
					"unable to run test on ztp version '%s' as it is less than minimum '%s",
					ranztphelper.ZtpVersion,
					"4.12",
				))
			}
		})

		By("determine the container image for ACM CR integration", func() {
			// The following oc command gets the same object:
			/*
			 oc get deployments -n open-cluster-management multiclusterhub-operator -ojson \
			 | jq '.spec.template.spec.containers[] | select(.name == "multiclusterhub-operator")' \
			 | jq '.env[] | select(.name == "OPERAND_IMAGE_MULTICLUSTER_OPERATORS_SUBSCRIPTION") .value'
			*/

			// Get the deployment
			deployment, err := ranztphelper.HubAPIClient.
				Deployments(ranztpparameters.AcmOperatorNamespace).
				Get(ranztphelper.GetZtpContext(), ranztpparameters.MulticlusterhubOperator, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Get the tag from the neested configuration
			acmPolicyGeneratorImage = GetContainerImageFromDeploymentEnvironment(
				deployment,
				ranztpparameters.MulticlusterhubOperator,
				"OPERAND_IMAGE_MULTICLUSTER_OPERATORS_SUBSCRIPTION",
			)

			// This needs to be defined or else we can't do the test
			Expect(acmPolicyGeneratorImage).ToNot(BeEmpty())

			log.Printf("Found acm policy generator container image: '%s'\n", acmPolicyGeneratorImage)
		})

		By("patching argocd to allow ACM CRs", func() {
			// Get the argocd instance from the hub
			// The following oc command gets the same object:
			// oc get argocd -n openshift-gitops openshift-gitops -ojson | jq
			argocd, err := ranztphelper.GetArgocdInstance(ranztpparameters.OpenshiftGitops, ranztpparameters.OpenshiftGitops)
			Expect(err).ToNot(HaveOccurred())

			// The default payload here is based off the documentation:
			//nolint:lll
			// https://github.com/openshift-kni/cnf-features-deploy/blob/master/ztp/gitops-subscriptions/ACMPolicyGeneratorIntergration.md#openshift-gitopsargocd
			acmPolicyGeneratorContainer := corev1.Container{
				Name: ranztpparameters.AcmPolicyGeneratorName,
				Command: []string{
					"/bin/bash",
				},
				Args: []string{
					"-c",
					"cp -r /etc/kustomize /.config",
				},
				ImagePullPolicy: "Always",
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "kustomize",
						MountPath: "/.config",
					},
				},
				Image: acmPolicyGeneratorImage,
			}

			// Then we need to check the init containers, which can be found here:
			// oc get argocd -n openshift-gitops openshift-gitops -ojson | jq '.spec.repo.initContainers'
			for index, container := range argocd.Spec.Repo.InitContainers {
				// If the container already exists then we will just save the existing config and make sure the image is correct
				if container.Name == ranztpparameters.AcmPolicyGeneratorName {
					// Save the original configuration so we can restore it later
					oldAcmPolicyGeneratorContainerConfiguration = container

					// Save the configuration and update the image
					acmPolicyGeneratorContainer = container
					acmPolicyGeneratorContainer.Image = acmPolicyGeneratorImage

					// We also want to strip this container out of the init containers list because we're going to add it back below
					argocd.Spec.Repo.InitContainers = append(
						argocd.Spec.Repo.InitContainers[:index],
						argocd.Spec.Repo.InitContainers[index+1:]...,
					)

					break
				}
			}

			// Now that we have our payload we need to update the argocd's init containers list
			// However we want the acm container to be first so use append here
			argocd.Spec.Repo.InitContainers = append(
				[]corev1.Container{acmPolicyGeneratorContainer},
				argocd.Spec.Repo.InitContainers...,
			)

			// Now save the argocd configuration
			err = ranztphelper.UpdateArgocdInstance(argocd)
			Expect(err).ToNot(HaveOccurred())

			// Give argocd a moment to catch up
			log.Println("Sleeping to wait for changes to take effect")
			time.Sleep(1 * time.Minute)
		})
	})

	// 54236
	Context("should use ACM CRs to template a policy", func() {
		It("should deploy the policy and validate it was successful", polarion.ID("54236"), func() {
			// https://issues.redhat.com/browse/CNF-6297

			// The ztp test data is stored in a nested directory within the ztp repo
			testGitPath := ranztphelper.JoinGitPaths(
				[]string{
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Path,
					"ztp-test/acm-crs",
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
					"acm-crs-policy",
					ranztpparameters.ZtpTestNamespace,
					ranztpparameters.ArgocdChangeTimeout,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("Validing the policy was created & wait for it to finish", func() {
				// Wait until the policy is non compliant
				err := ranztphelper.WaitForPolicyToHaveComplianceState(
					"acm-crs-policy",
					ranztpparameters.ZtpTestNamespace,
					policiesv1.NonCompliant,
					1*time.Minute,
				)
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

		By("reverting argocd patch for ACM CRs", func() {
			argocd, err := ranztphelper.GetArgocdInstance(ranztpparameters.OpenshiftGitops, ranztpparameters.OpenshiftGitops)
			Expect(err).ToNot(HaveOccurred())

			// Then we need to check the init containers, which can be found here:
			// oc get argocd -n openshift-gitops openshift-gitops -ojson | jq '.spec.repo.initContainers'
			for index, container := range argocd.Spec.Repo.InitContainers {
				// If the container already exists then we will just save the existing config and make sure the image is correct
				if container.Name == ranztpparameters.AcmPolicyGeneratorName {
					// Remove the existing container
					argocd.Spec.Repo.InitContainers = append(
						argocd.Spec.Repo.InitContainers[:index],
						argocd.Spec.Repo.InitContainers[index+1:]...,
					)

					// If there was originally this container then restore the original configuration
					if oldAcmPolicyGeneratorContainerConfiguration.Name != "" {
						argocd.Spec.Repo.InitContainers = append(
							[]corev1.Container{oldAcmPolicyGeneratorContainerConfiguration},
							argocd.Spec.Repo.InitContainers...,
						)
					}

					// Now save the argocd configuration
					err = ranztphelper.UpdateArgocdInstance(argocd)
					Expect(err).ToNot(HaveOccurred())

					// Give argocd a moment to catch up
					log.Println("Sleeping to wait for changes to take effect")
					time.Sleep(1 * time.Minute)

					// Break out of the loop since we are now done
					break
				}
			}
		})
	})
})

// GetcontainerImageFromDeploymentEnvironment is used to search a deployment for a particular
// container image in its container environments.
func GetContainerImageFromDeploymentEnvironment(deployment *v1.Deployment, containerName, envName string) string {
	// Loop over all the containers
	for _, container := range deployment.Spec.Template.Spec.Containers {
		// Find the matching container
		if container.Name == containerName {
			// Now loop over all the environment configuration
			for _, env := range container.Env {
				// Until we find the one for the image we care about
				if env.Name == envName {
					// Return the image tag
					return env.Value
				}
			}
		}
	}

	return ""
}
