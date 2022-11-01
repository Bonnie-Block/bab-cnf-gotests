package tests

import (
	"fmt"
	"log"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
)

var _ = Describe("Talm Spoke Tests", func() {

	// These tests only use the hub and spoke1
	var clusterList []*testClient.ClientSet

	execute.BeforeAll(func() {
		// Initialize cluster list
		clusterList = []*testClient.ClientSet{
			rantalmhelper.HubAPIClient,
			rantalmhelper.Spoke1APIClient,
		}

		// Check that the required clusters are present
		err := rantalmhelper.IsClustersPresent(clusterList)
		if err != nil {
			Skip(fmt.Sprintf("error occurred validating required clusters are present: %s", err.Error()))
		}
	})

	BeforeEach(func() {
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

	Describe("Two spoke test", func() {
		// Context("where one of them fails", func() {
		// 	It("should report the failed spoke", func() {
		// 		// Polarion test id 47952
		// 		// https://issues.redhat.com/browse/CNF-6498
		// 	})
		// })
	})
	Describe("One spoke test", func() {
		Context("where the spoke is missing", func() {
			It("should report the missing spoke", func() {
				// Polarion test id 47949
				// https://issues.redhat.com/browse/CNF-6497

				By("creating the cgu", func() {
					err := rantalmhelper.CreateCguAndWait(
						rantalmhelper.HubAPIClient,
						[]string{
							"non-existent-cluster",
						},
						[]string{
							"non-existent-policy",
						},
						rantalmhelper.CguName,
						rantalmhelper.Namespace,
						1,
						1,
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
						rantalmparameters.TalmDefaultReconcileTime*3,
					)
					Expect(err).ToNot(HaveOccurred())
				})

				log.Println("completed test")
			})
		})
	})
})
