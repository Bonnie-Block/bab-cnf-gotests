package tests

import (
	"fmt"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	configurationPolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
)

var _ = Describe("Talm Batching Tests", func() {

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

	Describe("Single batch test", Label("talmsinglebatch"), func() {
		// Context("where the CGU times out", func() {
		// 	It("reports the timeout value", func() {
		// 		// Polarion test id 47954
		// 		// https://issues.redhat.com/browse/CNF-6491
		// 	})
		// })
		Context("where a managed policy is missing", func() {
			// 47955
			It("reports the missing policy", func() {

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
					// This should immediately error out so we don't need a long timeout
					err := rantalmhelper.WaitForCguInCondition(
						rantalmhelper.HubAPIClient,
						rantalmhelper.CguName,
						rantalmhelper.Namespace,
						"Validated",
						"Missing managed policies: [non-existent-policy] ",
						"",
						1*time.Minute,
					)
					Expect(err).ToNot(HaveOccurred())
				})

			})
		})
	})

	Describe("Multiple batch test", Label("talmmultibatch"), func() {
		// Context("where the first batch fails with continue option", func() {
		// 	It("should finish the remaining batches", func() {
		// 		// Polarion test id 47952
		// 		// https://issues.redhat.com/browse/CNF-6480
		// 	})
		// })
		// Context("where the first batch fails with abort option", func() {
		// 	It("should abort the remaining batches", func() {
		// 		// Polarion test id 47952
		// 		// https://issues.redhat.com/browse/CNF-6481
		//		// This test requires TALM 4.11+
		// 	})
		// })
		// Context("where the second batch fails with continue option", func() {
		// 	It("should finish the remaining batches", func() {
		// 		// Polarion test id 47954
		// 		// https://issues.redhat.com/browse/CNF-6482
		// 	})
		// })
		Context("where all batches are successful", func() {
			// 47947
			It("should completed the CGU", func() {
				log.Println("starting test")

				temporaryNamespace := rantalmhelper.Namespace + "-temp"
				err := rantalmhelper.CleanupNamespace(clusterList, temporaryNamespace)
				Expect(err).ToNot(HaveOccurred())

				By("creating the cgu and associated resources", func() {
					cgu := rantalmhelper.GetCguDefinition(
						rantalmhelper.CguName,
						[]string{rantalmhelper.Spoke2Name, rantalmhelper.Spoke2Name},
						[]string{},
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

				By("waiting for the cgu to finish successfully", func() {
					err := rantalmhelper.WaitForCguToFinishSuccessfully(rantalmhelper.CguName, rantalmhelper.Namespace)
					Expect(err).ToNot(HaveOccurred())
				})

				err = rantalmhelper.CleanupNamespace(clusterList, temporaryNamespace)
				Expect(err).ToNot(HaveOccurred())

			})
		})
		// Context("where all the batches fail", func() {
		// 	It("should error the CGU", func() {
		// 		// Polarion test id 47954
		// 	})
		// })
	})
})
