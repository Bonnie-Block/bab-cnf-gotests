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
	configurationPolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
)

var _ = Describe("Talm Batching Tests", func() {

	// These tests only use the hub and spoke1
	var clusterList []*testClient.ClientSet

	execute.BeforeAll(func() {
		// Initialize cluster list
		clusterList = rantalmhelper.GetAllTestClients()

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

	Describe("Single batch test", Label("talmsinglebatch"), func() {
		// Context("where the CGU times out", func() {
		// 	It("reports the timeout value", func() {
		// 		// Polarion test id 47954
		// 		// https://issues.redhat.com/browse/CNF-6491
		// 	})
		// })
		// Context("where a managed policy is missing", func() {
		// 	It("reports the missing policy", func() {
		// 		// Polarion test id 47955
		// 		// https://issues.redhat.com/browse/CNF-6496

		// 		// Initialize the cgu using a policy that doesn't exist
		// 		// cgu := `
		// 		// 	apiVersion: ran.openshift.io/v1alpha1
		// 		// 	kind: ClusterGroupUpgrade
		// 		// 	metadata:
		// 		// 	name: talm-single-batch-test-policy-missing
		// 		// 	namespace: default
		// 		// 	spec:
		// 		// 	clusterLabelSelectors:
		// 		// 		- matchLabels:
		// 		// 			vendor: OpenShift
		// 		// 	managedPolicies:
		// 		// 		- non-existent-policy
		// 		// 	enable: true
		// 		// `

		// 		// Apply the cgu
		// 		// rantalmhelper.ApplyTalmResource(cgu)

		// 		// Wait for the cgu condition to show the expected error message
		// 		// err := wait.PollImmediate(
		// 		// 	parameters.TalmTestPollInterval,
		// 		// 	parameters.TalmDefaultReconcileTime,
		// 		// 	func() (done bool, err error) {
		// 		// 		// Get the progressing condition
		// 		// 		condition, err := rantalmhelper.GetTalmCondition("talm-single-batch-test-policy-missing", "Validated")
		// 		// 		// The condition may not exist when we call this, so we need to wait for it to exist
		// 		// 		if err != nil {
		// 		// 			return false, nil
		// 		// 		}

		// 		// 		// Check the error message
		// 		// 		expectedError := "The ClusterGroupUpgrade CR has: missing managed policies: [non-existent-policy]"
		// 		// 		if condition.Message == expectedError {
		// 		// 			return true, nil
		// 		// 		}
		// 		// 		return false, nil
		// 		// 	})
		// 		// Expect(err).ToNot(HaveOccurred())

		// 		// Update cgu to use a real policy

		// 		// Wait for cgu to finish
		// 	})
		// })
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
			It("should completed the CGU", func() {
				// Polarion test id 47947
				// https://issues.redhat.com/browse/CNF-6479
				log.Println("starting test")

				temporaryNamespace := rantalmhelper.Namespace + "-temp"

				By("creating the cgu and associated resources", func() {
					err := rantalmhelper.CreateSimplePolicyAndCgu(
						rantalmhelper.HubAPIClient,
						rantalmhelper.GetNamespaceDefinition(temporaryNamespace),
						configurationPolicyv1.MustHave,
						configurationPolicyv1.Inform,
						rantalmhelper.PolicyName,
						rantalmhelper.PolicySetName,
						rantalmhelper.PlacementBindingName,
						rantalmhelper.PlacementRule,
						rantalmhelper.Namespace,
						[]string{
							rantalmhelper.Spoke1Name,
							rantalmhelper.Spoke2Name,
						},
						rantalmhelper.CguName,
						15,
						1,
					)
					Expect(err).ToNot(HaveOccurred())
				})

				By("waiting for the cgu to finish successfully", func() {
					err := rantalmhelper.WaitForCguToFinishSuccessfully(rantalmhelper.CguName, rantalmhelper.Namespace)
					Expect(err).ToNot(HaveOccurred())
				})

				By("deleting the temporary namespace", func() {
					for _, client := range clusterList {
						if namespaces.Exists(temporaryNamespace, client) {
							err := namespaces.DeleteAndWait(client, temporaryNamespace, 5*time.Minute)
							Expect(err).ToNot(HaveOccurred())
						}
					}
				})

				log.Println("completed test")
			})
		})
		// Context("where all the batches fail", func() {
		// 	It("should error the CGU", func() {
		// 		// Polarion test id 47954
		// 	})
		// })
	})
})
