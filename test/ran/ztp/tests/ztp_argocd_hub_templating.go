package tests

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

var _ = Describe("ZTP Argocd Hub templating Tests", Ordered, Label("ztp-hub-templating"), func() {

	// These tests use the hub and spoke
	var clusterList []*testClient.ClientSet

	policyName := "hub-templating-policy-sriov-config"
	cguName := "hub-templating"
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

	Context("using hub side acm templating", func() {
		// 54240
		It("should report an error for using autoindent function where not allowed", func() {
			// https://issues.redhat.com/browse/CNF-6304

			// The ztp test data is stored in a nested directory within the ztp repo
			testGitPath := ranztphelper.JoinGitPaths(
				[]string{
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Path,
					"ztp-test/hub-templating-autoindent",
				},
			)

			HubTemplateTestSetup(testGitPath, policyName, cguName)

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

		// 54240
		It("should create the policy successfully with a valid template", func() {

			testGitPath := ranztphelper.JoinGitPaths(
				[]string{
					ranztphelper.ArgocdApps[ranztpparameters.ArgocdPoliciesAppName].Path,
					"ztp-test/hub-templating-valid",
				},
			)

			HubTemplateTestSetup(testGitPath, policyName, cguName)

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
			// Get the network to see if it exists
			_, err := ranztphelper.
				SpokeAPIClient.
				SriovnetworkV1Interface.
				SriovNetworks("openshift-sriov-network-operator").
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
					SriovNetworks("openshift-sriov-network-operator").
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
						SriovNetworks("openshift-sriov-network-operator").
						Get(
							ranztphelper.GetZtpContext(),
							ranztpparameters.ZtpTestNamespace,
							metav1.GetOptions{},
						)

					if err != nil && strings.Contains(err.Error(), "not found") {
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
				ranztpparameters.ZtpTestNamespace,
			)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

// HubTemplateTestSetup is used to prevent duplication of the core of this particular set of tests.
func HubTemplateTestSetup(testGitPath, policyName, cguName string) {
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
				"hub-templating-policy-sriov-config",
			},
			ranztpparameters.ZtpTestNamespace,
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
	// added Eventually since logs take longer to show up in console
	Eventually(
		func() string {
			podList, err := client.
				Pods("openshift-cluster-group-upgrades").
				List(ranztphelper.GetZtpContext(), metav1.ListOptions{})

			Expect(err).To(BeNil())
			Expect(len(podList.Items)).To(BeNumerically("==", 1))

			p := podList.Items[0]
			plog, err := pod.GetLog(client, &p, 1*time.Minute, "manager")

			Expect(err).To(BeNil())

			return plog
		},
		ranztpparameters.ArgocdChangeTimeout,
		ranztpparameters.ArgocdChangeInterval,
	).Should(ContainSubstring(expectationSubString))
}
