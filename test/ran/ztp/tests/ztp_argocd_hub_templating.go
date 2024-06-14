package tests

import (
	"fmt"
	"log"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

var _ = Describe("ZTP Argocd Hub templating Tests", Ordered, Label("ztp-hub-templating"), func() {

	// These tests use the hub and spoke
	var clusterList []*testClient.ClientSet

	policyName := "hub-templating-policy-sriov-config"
	cguName := "hub-templating"
	cguNamespace := "default" // cgu ns should be different than the policy ns
	cguLogHubTemplateError := "policy has hub template error"

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

	Context("using hub side acm templating", func() {
		// 54240
		When("Talm is less than version 4.16", func() {
			BeforeEach(func() {
				if !ranhelper.IsVersionStringInRange(
					ranztphelper.TalmVersion,
					"",
					"4.15",
				) {
					Skip("These templating tests are only valid when using TALM version 4.15 or lower")
				}
			})

			It("should report an error for using printf function where not allowed", polarion.ID("54240"), func() {
				// https://issues.redhat.com/browse/CNF-6304

				// The ztp test data is stored in a nested directory within the ztp repo
				testGitPath := ranztphelper.JoinGitPaths(
					[]string{
						ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Path,
						"ztp-test/hub-templating-printf",
					},
				)

				HubTemplateTestSetup(testGitPath, policyName, cguName, cguNamespace)

				By("Validating TALM reported a policy error", func() {
					assertTalmPodLog(ranztphelper.HubAPIClient, "printf variable is not supported in the template function Name field")
				})
			})

			// 54240
			It("should report an error for using fromsecret function where not allowed", polarion.ID("54240"), func() {
				// https://issues.redhat.com/browse/CNF-6304

				// The ztp test data is stored in a nested directory within the ztp repo
				testGitPath := ranztphelper.JoinGitPaths(
					[]string{
						ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Path,
						"ztp-test/hub-templating-fromsecret",
					},
				)

				// Since we need to create the secret first, we need to create the namespace
				// Normally that would be handled by the site config but this test is a special case

				// Create the namespace
				err := namespaces.Create(ranztpparameters.ZtpTestNamespace, ranztphelper.HubAPIClient)
				Expect(err).ToNot(HaveOccurred())

				// Wait until the test namespace exists
				err = wait.PollImmediate(
					30*time.Second,
					5*time.Minute,
					func() (bool, error) {
						if namespaces.Exists(ranztpparameters.ZtpTestNamespace, ranztphelper.HubAPIClient) {
							return true, nil
						}

						return false, nil
					},
				)
				Expect(err).ToNot(HaveOccurred())

				// Create secret
				secret := corev1.Secret{}
				secret.Name = ranztphelper.SpokeName + "-sriovdata"
				secret.StringData = map[string]string{
					"vlan": "MTEwCg==",
				}

				_, err = ranztphelper.HubAPIClient.
					Secrets(ranztpparameters.ZtpTestNamespace).
					Create(ranztphelper.GetZtpContext(), &secret, metav1.CreateOptions{})

				Expect(err).ToNot(HaveOccurred())

				HubTemplateTestSetup(testGitPath, policyName, cguName, cguNamespace)

				By("Validating TALM reported a policy error", func() {
					assertTalmPodLog(ranztphelper.HubAPIClient, "template function is not supported in TALM")
				})
			})

			// 54240
			It("should report an error for using autoindent function where not allowed", polarion.ID("54240"), func() {
				// https://issues.redhat.com/browse/CNF-6304

				// The ztp test data is stored in a nested directory within the ztp repo
				testGitPath := ranztphelper.JoinGitPaths(
					[]string{
						ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Path,
						"ztp-test/hub-templating-autoindent",
					},
				)

				HubTemplateTestSetup(testGitPath, policyName, cguName, cguNamespace)

				By("Validating TALM reported a policy error", func() {
					assertTalmPodLog(ranztphelper.HubAPIClient, cguLogHubTemplateError)
				})

				By("Validating the specific error using the policy annotation", func() {
					err := ranztphelper.WaitForConfigPolicyMessageToContainSubstring(
						ranztpparameters.ZtpTestNamespace+"."+policyName,
						ranztphelper.SpokeName,
						"wrong type for value; expected string; got int",
					)
					Expect(err).ToNot(HaveOccurred())
				})
			})

			It("should report an error for using invalid lookup hub template function", func() {
				if !ranhelper.IsVersionStringInRange(
					ranztphelper.TalmVersion,
					"4.12",
					"",
				) {
					Skip("The minimum required TALM version for this test is 4.12")
				}

				// The ztp test data is stored in a nested directory within the ztp repo
				testGitPath := ranztphelper.JoinGitPaths(
					[]string{
						ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Path,
						"ztp-test/hub-templating-lookup-invalid",
					},
				)

				HubTemplateTestSetup(testGitPath, policyName, cguName, cguNamespace)

				By("Validating CGU reported an invalid policy error", func() {
					err := rantalmhelper.WaitForCguInCondition(
						ranztphelper.HubAPIClient,
						cguName,
						cguNamespace,
						"Validated",
						"Invalid managed policies",
						"False",
						"NotAllManagedPoliciesExist", 1*time.Minute)
					Expect(err).ToNot(HaveOccurred())
				})

				By("Validating TALM reported a policy error", func() {
					assertTalmPodLog(ranztphelper.HubAPIClient,
						"template function only allows the resource with apiVersion in"+
							" 'cluster.open-cluster-management.io', kind 'ManagedCluster' and empty namespace")
				})
			})
		})
		// 54240
		It("should create the policy successfully with a valid template", polarion.ID("54240"), func() {

			testGitRepo := "ztp-test/hub-templating-valid_4.11"
			if ranhelper.IsVersionStringInRange(
				ranztphelper.TalmVersion,
				"4.12",
				"",
			) {
				testGitRepo = "ztp-test/hub-templating-valid"
			}

			testGitPath := ranztphelper.JoinGitPaths(
				[]string{
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Path,
					testGitRepo,
				},
			)

			HubTemplateTestSetup(testGitPath, policyName, cguName, cguNamespace)

			By("Validating the policy reaches compliant status", func() {
				err := ranztphelper.WaitForPolicyToHaveComplianceState(
					policyName,
					ranztpparameters.ZtpTestNamespace,
					policiesv1.Compliant,
					ranztpparameters.ArgocdChangeTimeout,
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

		// Delete any leftovers from the templating test
		By("Removing the hub templating leftovers if any exist", func() {
			const sriovNetNamespace string = "openshift-sriov-network-operator"

			// Get the network to see if it exists
			_, err := ranztphelper.
				SpokeAPIClient.
				SriovnetworkV1Interface.
				SriovNetworks(sriovNetNamespace).
				Get(
					ranztphelper.GetZtpContext(),
					ranztpparameters.ZtpTestNamespace,
					metav1.GetOptions{},
				)

			// If it exists then delete the network
			if err == nil {
				err := ranztphelper.
					SpokeAPIClient.
					SriovnetworkV1Interface.
					SriovNetworks(sriovNetNamespace).
					Delete(
						ranztphelper.GetZtpContext(),
						ranztpparameters.ZtpTestNamespace,
						metav1.DeleteOptions{},
					)
				Expect(err).ToNot(HaveOccurred())
			}

			// Wait until its gone
			err = wait.PollImmediate(
				ranztpparameters.ArgocdChangeInterval,
				ranztpparameters.ArgocdChangeTimeout,
				func() (bool, error) {
					_, err := ranztphelper.
						SpokeAPIClient.
						SriovnetworkV1Interface.
						SriovNetworks(sriovNetNamespace).
						Get(
							ranztphelper.GetZtpContext(),
							ranztpparameters.ZtpTestNamespace,
							metav1.GetOptions{},
						)

					if err != nil && (strings.Contains(err.Error(), "could not find") || strings.Contains(err.Error(), "not found")) {
						return true, nil
					}

					return false, nil
				},
			)
			Expect(err).ToNot(HaveOccurred())
		})

		By("Removing the cgu if it exists", func() {
			err := rantalmhelper.DeleteCguAndWait(
				ranztphelper.HubAPIClient,
				cguName,
				cguNamespace,
			)
			Expect(err).ToNot(HaveOccurred())
		})

		By("Removing test secret if it exists", func() {
			secret, err := ranztphelper.HubAPIClient.
				Secrets(ranztpparameters.ZtpTestNamespace).
				Get(ranztphelper.GetZtpContext(), ranztphelper.SpokeName+"-sriovdata", metav1.GetOptions{})

			if secret != nil && err == nil {
				err = ranztphelper.HubAPIClient.
					Secrets(ranztpparameters.ZtpTestNamespace).
					Delete(ranztphelper.GetZtpContext(), secret.Name, metav1.DeleteOptions{})

				Expect(err).ToNot(HaveOccurred())
			}
		})
	})
})

// HubTemplateTestSetup is used to prevent duplication of the core of this particular set of tests.
func HubTemplateTestSetup(testGitPath, policyName, cguName, cguNamespace string) {
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

	By("Waiting for the policy to exist", func() {
		err := ranztphelper.WaitForPolicyToExist(
			policyName,
			ranztpparameters.ZtpTestNamespace,
			ranztpparameters.ArgocdChangeTimeout,
		)
		Expect(err).ToNot(HaveOccurred())
	})

	By("Creating CGU", func() {
		// Get the cgu
		cgu := rantalmhelper.GetCguDefinition(
			cguName,
			[]string{
				ranztphelper.SpokeName,
			},
			[]string{},
			[]string{
				policyName,
			},
			cguNamespace,
			1,
			10,
		)

		// Make sure cgu is enabled
		cgu.Spec.Enable = rantalmhelper.BoolAddr(true)

		// Create the cgu
		err := rantalmhelper.CreateCguAndWait(
			ranztphelper.HubAPIClient,
			cgu,
		)

		Expect(err).ToNot(HaveOccurred())
	})
}

// assertTalmPodLog retrieves the TALM pod and asserts on the log.
func assertTalmPodLog(client *testClient.ClientSet, expectationSubString string) {
	log.Printf("Waiting for TALM log to report: '%s'\n", expectationSubString)

	// added Eventually since logs take longer to show up in console
	Eventually(
		func() string {
			podList, err := client.
				Pods(rantalmparameters.OpenshiftOperatorNamespace).
				List(ranztphelper.GetZtpContext(), metav1.ListOptions{})

			Expect(err).To(BeNil())
			Expect(len(podList.Items)).To(BeNumerically(">=", 1))

			plog := ""
			for _, p := range podList.Items {
				if strings.HasPrefix(p.GetName(), rantalmparameters.TalmPodNameHub) {
					plog, err = pod.GetLog(client, &p, 1*time.Minute, rantalmparameters.TalmContainerName)
					Expect(err).To(BeNil())

					return plog
				}
			}

			return plog
		},
		ranztpparameters.ArgocdChangeTimeout,
		ranztpparameters.ArgocdChangeInterval,
	).Should(ContainSubstring(expectationSubString))
}
