package tests

import (
	"fmt"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	configurationPolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
)

var _ = Describe("Talm Batching Tests", Label("talmbatching"), func() {

	// These tests only use the hub and spoke1
	var clusterList []*testClient.ClientSet

	execute.BeforeAll(func() {
		// Initialize cluster list
		clusterList = rantalmhelper.GetAllTestClients()
	})

	BeforeEach(func() {
		// Check that the required clusters are present
		err := ranhelper.IsClustersPresent(clusterList)
		if err != nil {
			Skip(fmt.Sprintf("error occurred validating required clusters are present: %s", err.Error()))
		}

		// Cleanup state to make it consistent
		for _, client := range clusterList {

			// Cleanup everything
			errList := rantalmhelper.CleanupTestResourcesOnClients(
				[]*testClient.ClientSet{
					client,
				},
				rantalmhelper.CguName,
				rantalmhelper.PolicyName,
				rantalmhelper.Namespace,
				rantalmhelper.PlacementBindingName,
				rantalmhelper.PlacementRule,
				rantalmhelper.PolicySetName,
				rantalmhelper.CatalogSourceName)
			Expect(len(errList)).To(Equal(0))

			// Create namespace
			err := namespaces.Create(rantalmhelper.Namespace, client)
			Expect(err).ToNot(HaveOccurred())
		}
	})

	Context("with a single spoke that is missing", Label("talmmissingspoke"), func() {
		// 47949
		It("should report the missing spoke", func() {
			By("validating the talm version meets the test minimum", func() {
				// TALM 4.11 does not set any conditions for a non managed cluster error
				// We are unable to verify the state in 4.11 therefore we cannot run this test
				if !ranhelper.IsVersionStringInRange(
					rantalmhelper.TalmHubVersion,
					rantalmparameters.TalmUpdatedConditionsVersion,
					"",
				) {
					Skip("test requires talm 4.12 or higher")
				}
			})
			By("creating the cgu", func() {
				cgu := rantalmhelper.GetCguDefinition(
					rantalmhelper.CguName,
					[]string{"non-existent-cluster"},
					[]string{},
					[]string{"non-existent-policy"},
					rantalmhelper.Namespace, 1, 1)

				err := rantalmhelper.CreateCguAndWait(
					rantalmhelper.HubAPIClient,
					cgu,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			// Wait for the cgu condition to show the expected error message
			By("waiting for the error condition to match", func() {
				err := rantalmhelper.WaitForCguInCondition(
					rantalmhelper.HubAPIClient,
					rantalmhelper.CguName,
					rantalmhelper.Namespace,
					"ClustersSelected",
					"Unable to select clusters: cluster non-existent-cluster is not a ManagedCluster",
					"",
					"",
					rantalmparameters.TalmDefaultReconcileTime*3,
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("with a missing policy", Label("talmmissingpolicy"), func() {
		// 49755
		It("should report the missing policy", func() {
			By("create and enable a cgu with a managed policy that does not exist", func() {

				cgu := rantalmhelper.GetCguDefinition(
					rantalmhelper.CguName,
					[]string{rantalmhelper.Spoke1Name},
					[]string{},
					[]string{"non-existent-policy"},
					rantalmhelper.Namespace, 1, 1)

				err := rantalmhelper.CreateCguAndWait(
					rantalmhelper.HubAPIClient,
					cgu,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("waiting for the cgu status to report the missing policy", func() {
				// Validation here depends on the TALM version

				conditionType := rantalmhelper.ValidatedType
				conditionMessage := "Missing managed policies: [non-existent-policy]"

				if !ranhelper.IsVersionStringInRange(
					rantalmhelper.TalmHubVersion,
					rantalmparameters.TalmUpdatedConditionsVersion,
					"",
				) {
					conditionType = rantalmhelper.ReadyType
					conditionMessage = "The ClusterGroupUpgrade CR has: missing managed policies: [non-existent-policy]"
				}

				// This should immediately error out so we don't need a long timeout
				err := rantalmhelper.WaitForCguInCondition(
					rantalmhelper.HubAPIClient,
					rantalmhelper.CguName,
					rantalmhelper.Namespace,
					conditionType,
					conditionMessage,
					"",
					"",
					1*time.Minute,
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("using a catalog source", Label("talmcatalogsource"), func() {
		// 47952
		It("should abort the CGU when the first batch fails with the Abort batch timeout action", func() {

			if !ranhelper.IsVersionStringInRange(
				rantalmhelper.TalmHubVersion,
				"4.12",
				"",
			) {
				Skip("configurable batchtimeoutaction requires talm 4.12 or higher")
			}

			By("verifying the temporary namespace does not exist on spoke1", func() {
				result := namespaces.Exists(rantalmhelper.TemporaryNamespaceName, rantalmhelper.Spoke1APIClient)
				Expect(result).To(BeFalse())
			})

			By("creating the temporary namespace on spoke2 only", func() {
				err := namespaces.Create(rantalmhelper.TemporaryNamespaceName, rantalmhelper.Spoke2APIClient)
				Expect(err).ToNot(HaveOccurred())
			})

			By("verifying the temporary namespace exists on spoke2", func() {
				result := namespaces.Exists(rantalmhelper.TemporaryNamespaceName, rantalmhelper.Spoke2APIClient)
				Expect(result).To(BeTrue())
			})

			By("creating the cgu and associated resources", func() {
				// This test uses a max concurrency of 1
				// This way we can verify the CGU aborts after the first batch fails
				cgu := rantalmhelper.GetCguDefinition(
					rantalmhelper.CguName,
					[]string{
						rantalmhelper.Spoke1Name,
						rantalmhelper.Spoke2Name,
					},
					[]string{},
					[]string{
						rantalmhelper.PolicyName,
					},
					rantalmhelper.Namespace,
					1,
					9,
				)

				cgu.Spec.Enable = rantalmhelper.BoolAddr(false)
				cgu.Spec.BatchTimeoutAction = "Abort"

				catsrc := rantalmhelper.GetCatsrcDefinition(
					rantalmhelper.CatalogSourceName,
					rantalmhelper.TemporaryNamespaceName,
					operatorsv1alpha1.SourceTypeInternal,
					1,
					"",
					"",
					"",
					rantalmhelper.CatalogSourceName,
				)

				err := rantalmhelper.CreatePolicyAndCgu(
					rantalmhelper.HubAPIClient,
					&catsrc,
					configurationPolicyv1.MustHave,
					configurationPolicyv1.Inform,
					rantalmhelper.PolicyName,
					rantalmhelper.PolicySetName,
					rantalmhelper.PlacementBindingName,
					rantalmhelper.PlacementRule,
					rantalmhelper.Namespace,
					metav1.LabelSelector{},
					cgu,
				)

				Expect(err).ToNot(HaveOccurred())
			})

			By("waiting for the system to settle", func() {
				time.Sleep(rantalmparameters.TalmSystemStablizationTime)
			})

			By("enabling the CGU", func() {
				cgu, err := rantalmhelper.GetCgu(
					rantalmhelper.HubAPIClient,
					rantalmhelper.CguName,
					rantalmhelper.Namespace,
				)
				Expect(err).ToNot(HaveOccurred())

				err = rantalmhelper.EnableCgu(
					rantalmhelper.HubAPIClient,
					cgu,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("waiting for the cgu to timeout", func() {
				err := rantalmhelper.WaitForCguToTimeout(
					rantalmhelper.CguName,
					rantalmhelper.Namespace,
					11*time.Minute,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("validating that the policy failed on spoke1", func() {
				result, err := rantalmhelper.IsCatsrcExist(
					rantalmhelper.Spoke1APIClient,
					rantalmhelper.CatalogSourceName,
					rantalmhelper.TemporaryNamespaceName,
				)
				err = rantalmhelper.FilterMissingResourceErrors(err)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeFalse())
			})

			By("validating that the policy failed on spoke2", func() {
				result, err := rantalmhelper.IsCatsrcExist(
					rantalmhelper.Spoke2APIClient,
					rantalmhelper.CatalogSourceName,
					rantalmhelper.TemporaryNamespaceName,
				)
				err = rantalmhelper.FilterMissingResourceErrors(err)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeFalse())
			})

			By("validating that the timeout should have occurred after just the first reconcile", func() {
				// We need to get the cgu so we can get the timestamps from it
				cgu, err := rantalmhelper.GetCgu(rantalmhelper.HubAPIClient, rantalmhelper.CguName, rantalmhelper.Namespace)
				Expect(err).ToNot(HaveOccurred())

				// Get the start and end time from the cgu status
				startTime := cgu.Status.Status.StartedAt
				endTime := cgu.Status.Status.CompletedAt

				// Get the runtime in minutes
				// We only really care about the minutes here since the test is relatively short
				runtime := endTime.Minute() - startTime.Minute()

				expectedDeviation := int(10 * time.Second)
				expectedTimeout := int(rantalmparameters.TalmDefaultReconcileTime)

				// We expect that the total runtime should be about equal to the expected timeout
				// In particular we expect it to be just about one reconcile loop for this test
				Expect(runtime+expectedDeviation >= expectedTimeout)
				Expect(runtime-expectedDeviation <= expectedTimeout)

			})

			By("validating that the timeout message matched the abort message", func() {

				conditionType := rantalmhelper.SucceededType
				conditionMessage := "Policy remediation took too long on some clusters"

				if !ranhelper.IsVersionStringInRange(
					rantalmhelper.TalmHubVersion,
					rantalmparameters.TalmUpdatedConditionsVersion,
					"",
				) {
					conditionType = rantalmhelper.ReadyType
					conditionMessage = rantalmhelper.Talm411TimeoutMessage
				}

				err := rantalmhelper.WaitForCguInCondition(
					rantalmhelper.HubAPIClient,
					rantalmhelper.CguName,
					rantalmhelper.Namespace,
					conditionType,
					conditionMessage,
					"",
					"",
					1*time.Minute,
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})
		// 47952
		It("should report the failed spoke when one spoke in a batch times out", func() {
			By("creating the temporary namespace on spoke1 only", func() {
				err := namespaces.Create(rantalmhelper.TemporaryNamespaceName, rantalmhelper.Spoke1APIClient)
				Expect(err).ToNot(HaveOccurred())
			})
			By("verifying the temporary namespace exists on spoke1", func() {
				result := namespaces.Exists(rantalmhelper.TemporaryNamespaceName, rantalmhelper.Spoke1APIClient)
				Expect(result).To(BeTrue())
			})
			By("verifying the temporary namespace does not exist on spoke2", func() {
				result := namespaces.Exists(rantalmhelper.TemporaryNamespaceName, rantalmhelper.Spoke2APIClient)
				Expect(result).To(BeFalse())
			})
			By("creating the cgu and associated resources", func() {
				// This test uses a max concurrency of 2
				// This way both spokes are in the same batch
				cgu := rantalmhelper.GetCguDefinition(
					rantalmhelper.CguName,
					[]string{
						rantalmhelper.Spoke1Name,
						rantalmhelper.Spoke2Name,
					},
					[]string{},
					[]string{
						rantalmhelper.PolicyName,
					},
					rantalmhelper.Namespace,
					2,
					9,
				)

				cgu.Spec.Enable = rantalmhelper.BoolAddr(false)

				catsrc := rantalmhelper.GetCatsrcDefinition(
					rantalmhelper.CatalogSourceName,
					rantalmhelper.TemporaryNamespaceName,
					operatorsv1alpha1.SourceTypeInternal,
					1,
					"",
					"",
					"",
					rantalmhelper.CatalogSourceName,
				)

				err := rantalmhelper.CreatePolicyAndCgu(
					rantalmhelper.HubAPIClient,
					&catsrc,
					configurationPolicyv1.MustHave,
					configurationPolicyv1.Inform,
					rantalmhelper.PolicyName,
					rantalmhelper.PolicySetName,
					rantalmhelper.PlacementBindingName,
					rantalmhelper.PlacementRule,
					rantalmhelper.Namespace,
					metav1.LabelSelector{},
					cgu,
				)

				Expect(err).ToNot(HaveOccurred())
			})

			By("waiting for the system to settle", func() {
				time.Sleep(rantalmparameters.TalmSystemStablizationTime)
			})

			By("enabling the CGU", func() {
				cgu, err := rantalmhelper.GetCgu(
					rantalmhelper.HubAPIClient,
					rantalmhelper.CguName,
					rantalmhelper.Namespace,
				)
				Expect(err).ToNot(HaveOccurred())

				err = rantalmhelper.EnableCgu(
					rantalmhelper.HubAPIClient,
					cgu,
				)
				Expect(err).ToNot(HaveOccurred())

			})

			By("waiting for the cgu to timeout", func() {
				err := rantalmhelper.WaitForCguToTimeout(
					rantalmhelper.CguName,
					rantalmhelper.Namespace,
					16*time.Minute,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("validating that the policy was successful on spoke1", func() {
				result, err := rantalmhelper.IsCatsrcExist(
					rantalmhelper.Spoke1APIClient,
					rantalmhelper.CatalogSourceName,
					rantalmhelper.TemporaryNamespaceName,
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeTrue())
			})

			By("validating that the policy failed on spoke2", func() {
				result, err := rantalmhelper.IsCatsrcExist(
					rantalmhelper.Spoke2APIClient,
					rantalmhelper.CatalogSourceName,
					rantalmhelper.TemporaryNamespaceName,
				)
				err = rantalmhelper.FilterMissingResourceErrors(err)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})

		It("should continue the CGU when the first batch fails with the Continue batch timeout action", func() {
			By("creating the temporary namespace on spoke2 only", func() {
				err := namespaces.Create(rantalmhelper.TemporaryNamespaceName, rantalmhelper.Spoke2APIClient)
				Expect(err).ToNot(HaveOccurred())
			})
			By("verifying the temporary namespace exists on spoke2", func() {
				result := namespaces.Exists(rantalmhelper.TemporaryNamespaceName, rantalmhelper.Spoke2APIClient)
				Expect(result).To(BeTrue())
			})
			By("verifying the temporary namespace does not exist on spoke1", func() {
				result := namespaces.Exists(rantalmhelper.TemporaryNamespaceName, rantalmhelper.Spoke1APIClient)
				Expect(result).To(BeFalse())
			})
			By("creating the cgu and associated resources", func() {
				// Max concurrency of one to ensure two batches are used
				cgu := rantalmhelper.GetCguDefinition(
					rantalmhelper.CguName,
					[]string{
						rantalmhelper.Spoke1Name,
						rantalmhelper.Spoke2Name,
					},
					[]string{},
					[]string{
						rantalmhelper.PolicyName,
					},
					rantalmhelper.Namespace,
					1,
					9,
				)

				cgu.Spec.Enable = rantalmhelper.BoolAddr(false)

				catsrc := rantalmhelper.GetCatsrcDefinition(
					rantalmhelper.CatalogSourceName,
					rantalmhelper.TemporaryNamespaceName,
					operatorsv1alpha1.SourceTypeInternal,
					1,
					"",
					"",
					"",
					rantalmhelper.CatalogSourceName,
				)

				err := rantalmhelper.CreatePolicyAndCgu(
					rantalmhelper.HubAPIClient,
					&catsrc,
					configurationPolicyv1.MustHave,
					configurationPolicyv1.Inform,
					rantalmhelper.PolicyName,
					rantalmhelper.PolicySetName,
					rantalmhelper.PlacementBindingName,
					rantalmhelper.PlacementRule,
					rantalmhelper.Namespace,
					metav1.LabelSelector{},
					cgu,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("waiting for the system to settle", func() {
				time.Sleep(rantalmparameters.TalmSystemStablizationTime)
			})

			By("enabling the CGU", func() {
				cgu, err := rantalmhelper.GetCgu(
					rantalmhelper.HubAPIClient,
					rantalmhelper.CguName,
					rantalmhelper.Namespace,
				)
				Expect(err).ToNot(HaveOccurred())

				err = rantalmhelper.EnableCgu(
					rantalmhelper.HubAPIClient,
					cgu,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("waiting for the cgu to timeout", func() {
				err := rantalmhelper.WaitForCguToTimeout(rantalmhelper.CguName, rantalmhelper.Namespace, 16*time.Minute)
				Expect(err).ToNot(HaveOccurred())
			})

			By("validating that the policy was successful on spoke2", func() {
				result, err := rantalmhelper.IsCatsrcExist(
					rantalmhelper.Spoke2APIClient,
					rantalmhelper.CatalogSourceName,
					rantalmhelper.TemporaryNamespaceName,
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeTrue())
			})

			By("validating that the policy was not successful on spoke1", func() {
				result, err := rantalmhelper.IsCatsrcExist(
					rantalmhelper.Spoke1APIClient,
					rantalmhelper.CatalogSourceName,
					rantalmhelper.TemporaryNamespaceName,
				)
				err = rantalmhelper.FilterMissingResourceErrors(err)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})
		// 54926
		It("should continue the CGU when the second batch fails with the Continue batch timeout action", func() {
			expectedTimeout := 16

			By("creating the temporary namespace on spoke1 only", func() {
				err := namespaces.Create(rantalmhelper.TemporaryNamespaceName, rantalmhelper.Spoke1APIClient)
				Expect(err).ToNot(HaveOccurred())
			})
			By("verifying the temporary namespace exists on spoke1", func() {
				result := namespaces.Exists(rantalmhelper.TemporaryNamespaceName, rantalmhelper.Spoke1APIClient)
				Expect(result).To(BeTrue())
			})
			By("verifying the temporary namespace does not exist on spoke2", func() {
				result := namespaces.Exists(rantalmhelper.TemporaryNamespaceName, rantalmhelper.Spoke2APIClient)
				Expect(result).To(BeFalse())
			})
			By("creating the cgu and associated resources", func() {
				// Max concurrency of one to ensure two batches are used
				cgu := rantalmhelper.GetCguDefinition(
					rantalmhelper.CguName,
					[]string{
						rantalmhelper.Spoke1Name,
						rantalmhelper.Spoke2Name,
					},
					[]string{},
					[]string{
						rantalmhelper.PolicyName,
					},
					rantalmhelper.Namespace,
					1,
					expectedTimeout,
				)

				cgu.Spec.Enable = rantalmhelper.BoolAddr(false)

				catsrc := rantalmhelper.GetCatsrcDefinition(
					rantalmhelper.CatalogSourceName,
					rantalmhelper.TemporaryNamespaceName,
					operatorsv1alpha1.SourceTypeInternal,
					1,
					"",
					"",
					"",
					rantalmhelper.CatalogSourceName,
				)

				err := rantalmhelper.CreatePolicyAndCgu(
					rantalmhelper.HubAPIClient,
					&catsrc,
					configurationPolicyv1.MustHave,
					configurationPolicyv1.Inform,
					rantalmhelper.PolicyName,
					rantalmhelper.PolicySetName,
					rantalmhelper.PlacementBindingName,
					rantalmhelper.PlacementRule,
					rantalmhelper.Namespace,
					metav1.LabelSelector{},
					cgu,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("waiting for the system to settle", func() {
				time.Sleep(rantalmparameters.TalmSystemStablizationTime)
			})

			By("enabling the CGU", func() {
				cgu, err := rantalmhelper.GetCgu(
					rantalmhelper.HubAPIClient,
					rantalmhelper.CguName,
					rantalmhelper.Namespace,
				)
				Expect(err).ToNot(HaveOccurred())

				err = rantalmhelper.EnableCgu(
					rantalmhelper.HubAPIClient,
					cgu,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("waiting for the cgu to timeout", func() {
				err := rantalmhelper.WaitForCguToTimeout(rantalmhelper.CguName, rantalmhelper.Namespace, 21*time.Minute)
				Expect(err).ToNot(HaveOccurred())
			})

			By("validating that the policy was successful on spoke1", func() {
				result, err := rantalmhelper.IsCatsrcExist(
					rantalmhelper.Spoke1APIClient,
					rantalmhelper.CatalogSourceName,
					rantalmhelper.TemporaryNamespaceName,
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeTrue())
			})

			By("validating that the policy failed on spoke2", func() {
				result, err := rantalmhelper.IsCatsrcExist(
					rantalmhelper.Spoke2APIClient,
					rantalmhelper.CatalogSourceName,
					rantalmhelper.TemporaryNamespaceName,
				)
				err = rantalmhelper.FilterMissingResourceErrors(err)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeFalse())
			})

			By("validating that cgu timeout is recalculated for later batches after earlier batches complete", func() {

				// We need to get the cgu so we can get the timestamps from it
				cgu, err := rantalmhelper.GetCgu(rantalmhelper.HubAPIClient, rantalmhelper.CguName, rantalmhelper.Namespace)
				Expect(err).ToNot(HaveOccurred())

				// Get runtime in minutes from the cgu status
				startTime := cgu.Status.Status.StartedAt
				endTime := cgu.Status.Status.CompletedAt
				runtime := endTime.Minute() - startTime.Minute()

				// We expect that the total runtime should be about equal to the expected timeout
				// In particular we expect it to be +/- one reconcile loop time (5 minutes)
				// The first batch will complete successfully, so the second should use the entire remaining expected timout.
				Expect(runtime >= expectedTimeout)
				Expect(runtime <= expectedTimeout+int(rantalmparameters.TalmDefaultReconcileTime))
			})
		})
	})

	Context("using a temporary namespace", Label("talmtempnamespace"), func() {
		// 47954, 54292
		It("should report the timeout value when one cluster is in a batch and it times out", func() {
			// We will be verifying that the actual timeout is close to this value
			expectedTimeout := 8

			By("verifying the temporary namespace does not exist", func() {
				result := namespaces.Exists(rantalmhelper.TemporaryNamespaceName, rantalmhelper.Spoke1APIClient)
				Expect(result).To(BeFalse())
			})

			By("creating the enabled cgu with a catalog source in a namespace that does not exist", func() {

				catsrc := rantalmhelper.GetCatsrcDefinition(
					rantalmhelper.CatalogSourceName,
					rantalmhelper.TemporaryNamespaceName,
					operatorsv1alpha1.SourceTypeInternal,
					1,
					"",
					"",
					"",
					rantalmhelper.CatalogSourceName,
				)

				cgu := rantalmhelper.GetCguDefinition(
					rantalmhelper.CguName,
					[]string{
						rantalmhelper.Spoke1Name,
					},
					[]string{},
					[]string{
						rantalmhelper.PolicyName,
					},
					rantalmhelper.Namespace,
					1,
					expectedTimeout,
				)

				cgu.Spec.Enable = rantalmhelper.BoolAddr(false)

				err := rantalmhelper.CreatePolicyAndCgu(
					rantalmhelper.HubAPIClient,
					&catsrc,
					configurationPolicyv1.MustHave,
					configurationPolicyv1.Inform,
					rantalmhelper.PolicyName,
					rantalmhelper.PolicySetName,
					rantalmhelper.PlacementBindingName,
					rantalmhelper.PlacementRule,
					rantalmhelper.Namespace,
					metav1.LabelSelector{},
					cgu,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("waiting for the system to settle", func() {
				time.Sleep(rantalmparameters.TalmSystemStablizationTime)
			})

			By("enabling the CGU", func() {
				cgu, err := rantalmhelper.GetCgu(
					rantalmhelper.HubAPIClient,
					rantalmhelper.CguName,
					rantalmhelper.Namespace,
				)
				Expect(err).ToNot(HaveOccurred())

				err = rantalmhelper.EnableCgu(
					rantalmhelper.HubAPIClient,
					cgu,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("waiting for the cgu to timeout", func() {
				err := rantalmhelper.WaitForCguToTimeout(
					rantalmhelper.CguName,
					rantalmhelper.Namespace,
					11*time.Minute,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("validating the timeout value was approximately correct", func() {
				// We need to get the cgu so we can get the timestamps from it
				cgu, err := rantalmhelper.GetCgu(rantalmhelper.HubAPIClient, rantalmhelper.CguName, rantalmhelper.Namespace)
				Expect(err).ToNot(HaveOccurred())

				// Get the start and end time from the cgu status
				startTime := cgu.Status.Status.StartedAt
				endTime := cgu.Status.Status.CompletedAt

				// Get the runtime in minutes
				// We only really care about the minutes here since the test is relatively short
				runtime := endTime.Minute() - startTime.Minute()

				// We expect that the total runtime should be about equal to the expected timeout
				// In particular we expect it to be +/- one reconcile loop time (5 minutes)
				Expect(runtime+int(rantalmparameters.TalmDefaultReconcileTime) >= expectedTimeout)
				Expect(runtime-int(rantalmparameters.TalmDefaultReconcileTime) <= expectedTimeout)
			})

			// Deletion of TALM-generated policy requires 4.12 or higher
			if ranhelper.IsVersionStringInRange(
				rantalmhelper.TalmHubVersion,
				rantalmparameters.TalmUpdatedConditionsVersion,
				"",
			) {
				By("verifying the test policy was deleted upon CGU expiration", func() {
					TalmPolicyPrefix := rantalmhelper.CguName + "-" + rantalmhelper.PolicyName
					talmGeneratedPolicyName, err := rantalmhelper.GetPolicyNameWithPrefix(
						rantalmhelper.HubAPIClient,
						TalmPolicyPrefix,
						rantalmhelper.Namespace)
					Expect(err).ToNot(HaveOccurred())
					log.Printf("Checking for existence of test policy %s", talmGeneratedPolicyName)

					if talmGeneratedPolicyName != "" {
						log.Printf("Test policy %s still exists. Waiting for deletion.", talmGeneratedPolicyName)
						err = rantalmhelper.WaitUntilObjectDoesNotExist(
							rantalmhelper.HubAPIClient,
							talmGeneratedPolicyName,
							rantalmhelper.Namespace,
							rantalmhelper.IsPolicyExist,
						)
						Expect(err).ToNot(HaveOccurred())
					}
				})
			}
		})

		// 47947, 54288, 54289, 54559, 54292
		It("should complete the CGU when two clusters are successful in a single batch", func() {
			By("creating the cgu and associated resources", func() {
				cgu := rantalmhelper.GetCguDefinition(
					rantalmhelper.CguName,
					[]string{rantalmhelper.Spoke2Name, rantalmhelper.Spoke2Name},
					[]string{},
					[]string{rantalmhelper.PolicyName},
					rantalmhelper.Namespace, 1, 15)

				cgu.Spec.Enable = rantalmhelper.BoolAddr(false)

				policyLabelSelector := metav1.LabelSelector{}

				// ocp-54288, ocp-54289, ocp-54559 - Test k8s matchLabels and matchExpressions selectors  (4.12 feature)
				if ranhelper.IsVersionStringInRange(
					rantalmhelper.TalmHubVersion,
					rantalmparameters.TalmUpdatedConditionsVersion,
					"",
				) {
					log.Printf("Test using MatchLabels with name %s and MatchExpressions with name %s...",
						rantalmhelper.Spoke1Name, rantalmhelper.Spoke2Name)
					policyLabelSelector = metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{{
							Key:      "common",
							Operator: "In",
							Values:   []string{"true"},
						}},
					}
					cgu.Spec.Clusters = nil
					cgu.Spec.ClusterLabelSelectors = []metav1.LabelSelector{
						{MatchLabels: map[string]string{"name": rantalmhelper.Spoke1Name}},
						{MatchExpressions: []metav1.LabelSelectorRequirement{{
							Key:      "name",
							Operator: "In",
							Values:   []string{rantalmhelper.Spoke2Name},
						}}},
					}
				}
				err := rantalmhelper.CreatePolicyAndCgu(
					rantalmhelper.HubAPIClient,
					rantalmhelper.GetNamespaceDefinition(rantalmhelper.TemporaryNamespaceName),
					configurationPolicyv1.MustHave,
					configurationPolicyv1.Inform,
					rantalmhelper.PolicyName,
					rantalmhelper.PolicySetName,
					rantalmhelper.PlacementBindingName,
					rantalmhelper.PlacementRule,
					rantalmhelper.Namespace,
					policyLabelSelector,
					cgu,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("waiting for the system to settle", func() {
				time.Sleep(rantalmparameters.TalmSystemStablizationTime)
			})

			By("enabling the CGU", func() {
				cgu, err := rantalmhelper.GetCgu(
					rantalmhelper.HubAPIClient,
					rantalmhelper.CguName,
					rantalmhelper.Namespace,
				)
				Expect(err).ToNot(HaveOccurred())

				err = rantalmhelper.EnableCgu(
					rantalmhelper.HubAPIClient,
					cgu,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("waiting for the cgu to finish successfully", func() {
				err := rantalmhelper.WaitForCguToFinishSuccessfully(rantalmhelper.CguName, rantalmhelper.Namespace, 21*time.Minute)
				Expect(err).ToNot(HaveOccurred())
			})

			By("verifying the test policy was deleted upon CGU expiration", func() {

				TalmPolicyPrefix := rantalmhelper.CguName + "-" + rantalmhelper.PolicyName

				talmGeneratedPolicyName, err := rantalmhelper.GetPolicyNameWithPrefix(
					rantalmhelper.HubAPIClient,
					TalmPolicyPrefix,
					rantalmhelper.Namespace)
				Expect(err).ToNot(HaveOccurred())
				log.Printf("Checking for existence of test policy %s", talmGeneratedPolicyName)

				if talmGeneratedPolicyName != "" {
					log.Printf("Test policy %s still exists. Waiting for deletion.", talmGeneratedPolicyName)
					err = rantalmhelper.WaitUntilObjectDoesNotExist(
						rantalmhelper.HubAPIClient,
						talmGeneratedPolicyName,
						rantalmhelper.Namespace,
						rantalmhelper.IsPolicyExist,
					)
					Expect(err).ToNot(HaveOccurred())
				}
			})
		})
	})

	AfterEach(func() {
		// Cleanup everything
		errList := rantalmhelper.CleanupTestResourcesOnClients(
			clusterList,
			rantalmhelper.CguName,
			rantalmhelper.PolicyName,
			rantalmhelper.Namespace,
			rantalmhelper.PlacementBindingName,
			rantalmhelper.PlacementRule,
			rantalmhelper.PolicySetName,
			rantalmhelper.CatalogSourceName)
		Expect(len(errList)).To(Equal(0))

		// Cleanup the temporary namespace
		err := rantalmhelper.CleanupNamespace(clusterList, rantalmhelper.TemporaryNamespaceName)
		Expect(err).ToNot(HaveOccurred())
	})
})
