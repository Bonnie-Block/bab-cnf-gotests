package tests

import (
	. "github.com/onsi/ginkgo/v2"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
)

var _ = Describe("Talm Single batch Tests", func() {

	execute.BeforeAll(func() {
	})

	BeforeEach(func() {
	})

	Describe("Single batch test", func() {
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
})
