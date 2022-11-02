package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	configurationPolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
)

var _ = Describe("Talm Canary Tests", Label("talmcanary"), func() {

	// These tests only use the hub and spoke1
	var clusterList []*testClient.ClientSet

	execute.BeforeAll(func() {
		// Initialize cluster list
		clusterList = rantalmhelper.GetAllTestClients()
	})

	BeforeEach(func() {
		// Check that the required clusters are present
		err := rantalmhelper.IsClustersPresent(clusterList)
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
				rantalmhelper.PolicySetName)
			Expect(len(errList)).To(Equal(0))

			// Create namespace
			err := namespaces.Create(rantalmhelper.Namespace, client)
			Expect(err).ToNot(HaveOccurred())
		}
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
			rantalmhelper.PolicySetName)
		Expect(len(errList)).To(Equal(0))
	})

	Describe("Canary feature used", func() {
		// Context("where first canary fails", func() {
		// 	It("should stop the CGU", func() {
		// 		// Polarion test id 47954
		// 		// https://issues.redhat.com/browse/CNF-6477
		// 	})
		// })
		Context("where all the canaries are successful", func() {
			// 47947
			It("should complete the CGU", func() {
				// Temporary namespace that will be created using the cgu
				temporaryNamespace := rantalmhelper.Namespace + "-temp"

				err := rantalmhelper.CleanupNamespace(clusterList, temporaryNamespace)
				Expect(err).ToNot(HaveOccurred())

				By("creating the enabled cgu wth canaries and associated resources", func() {
					By("creating the cgu and associated resources", func() {
						cgu := rantalmhelper.GetCguDefinition(
							rantalmhelper.CguName,
							[]string{rantalmhelper.Spoke2Name, rantalmhelper.Spoke2Name},
							[]string{rantalmhelper.Spoke2Name},
							[]string{rantalmhelper.PolicyName},
							rantalmhelper.Namespace, 1, 15)

						err := rantalmhelper.CreatePolicyAndCgu(
							rantalmhelper.HubAPIClient,
							rantalmhelper.GetNamespaceDefinition(temporaryNamespace),
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
							rantalmparameters.TalmDefaultReconcileTime,
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
						err := rantalmhelper.WaitForCguToFinishSuccessfully(rantalmhelper.CguName, rantalmhelper.Namespace)
						Expect(err).ToNot(HaveOccurred())
					})

					err = rantalmhelper.CleanupNamespace(clusterList, temporaryNamespace)
					Expect(err).ToNot(HaveOccurred())

				})
			})
		})
	})
})
