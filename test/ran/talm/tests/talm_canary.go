package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	configurationPolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
)

var _ = Describe("Talm Canary Tests", Ordered, Label("talmcanary"), func() {

	// These tests only use the hub and spoke1
	var clusterList []*testClient.ClientSet

	BeforeAll(func() {
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
			Expect(errList).To(BeEmpty())

			// Create namespace
			err := namespaces.Create(rantalmhelper.Namespace, client)
			Expect(err).ToNot(HaveOccurred())
		}

		// Cleanup the temporary namespace
		err = rantalmhelper.CleanupNamespace(clusterList, rantalmhelper.TemporaryNamespaceName)
		Expect(err).ToNot(HaveOccurred())
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
		Expect(errList).To(BeEmpty())

		// Cleanup the temporary namespace
		err := rantalmhelper.CleanupNamespace(clusterList, rantalmhelper.TemporaryNamespaceName)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("where first canary fails", func() {
		// 47954
		It("should stop the CGU", func() {
			By("verifying the temporary namespace does not exist", func() {
				result := namespaces.Exists(rantalmhelper.TemporaryNamespaceName, rantalmhelper.Spoke1APIClient)
				Expect(result).To(BeFalse())
				result = namespaces.Exists(rantalmhelper.TemporaryNamespaceName, rantalmhelper.Spoke2APIClient)
				Expect(result).To(BeFalse())
			})
			By("creating the cgu and associated resources", func() {
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
					[]string{rantalmhelper.Spoke1Name, rantalmhelper.Spoke2Name},
					[]string{rantalmhelper.Spoke2Name},
					[]string{rantalmhelper.PolicyName},
					rantalmhelper.Namespace, 1, 9)

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

			By("making sure the canary cluster (spoke2) starts first", func() {
				err := rantalmhelper.WaitForClusterInProgressInCgu(
					rantalmhelper.HubAPIClient,
					rantalmhelper.CguName,
					rantalmhelper.Spoke2Name,
					rantalmhelper.Namespace,
					3*rantalmparameters.TalmDefaultReconcileTime,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("making sure the non-canary cluster (spoke1) has not started yet", func() {
				started, err := rantalmhelper.IsClusterStartedInCgu(
					rantalmhelper.HubAPIClient,
					rantalmhelper.CguName,
					rantalmhelper.Spoke1Name,
					rantalmhelper.Namespace,
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(started).To(BeFalse())
			})

			By("validating that the timeout was due to canary failure", func() {
				// Validation here depends on the TALM version

				conditionType := rantalmhelper.SucceededType
				conditionMessage := "Policy remediation took too long on canary clusters"

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
					11*time.Minute,
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("where all the canaries are successful", func() {
		// 47947
		It("should complete the CGU", func() {

			By("creating the cgu and associated resources", func() {
				cgu := rantalmhelper.GetCguDefinition(
					rantalmhelper.CguName,
					[]string{rantalmhelper.Spoke2Name, rantalmhelper.Spoke2Name},
					[]string{rantalmhelper.Spoke2Name},
					[]string{rantalmhelper.PolicyName},
					rantalmhelper.Namespace, 1, 9)

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
					metav1.LabelSelector{},
					cgu,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("making sure the canary cluster (spoke2) starts first", func() {
				err := rantalmhelper.WaitForClusterInProgressInCgu(
					rantalmhelper.HubAPIClient,
					rantalmhelper.CguName,
					rantalmhelper.Spoke2Name,
					rantalmhelper.Namespace,
					2*rantalmparameters.TalmDefaultReconcileTime,
				)
				Expect(err).ToNot(HaveOccurred())
			})

			By("making sure the non-canary cluster (spoke1) has not started yet", func() {
				started, err := rantalmhelper.IsClusterStartedInCgu(
					rantalmhelper.HubAPIClient,
					rantalmhelper.CguName,
					rantalmhelper.Spoke1Name,
					rantalmhelper.Namespace,
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(started).To(BeFalse())
			})

			By("waiting for the cgu to finish successfully", func() {
				err := rantalmhelper.WaitForCguToFinishSuccessfully(rantalmhelper.CguName, rantalmhelper.Namespace, 10*time.Minute)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

})
