package tests

import (
	"io/ioutil"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ = Describe("Talm Spoke Tests", func() {

	execute.BeforeAll(func() {
	})

	BeforeEach(func() {
	})

	Describe("Two spoke test", func() {
		// Context("where one of them fails", func() {
		// 	It("should report the failed spoke", func() {
		// 		// Polarion test id 47952
		// 		// https://issues.redhat.com/browse/CNF-6498
		// 	})
		// })
		Context("where one of them is missing", func() {
			It("should report the missing spoke", func() {
				// Polarion test id 47949
				// https://issues.redhat.com/browse/CNF-6497

				// Get the current directory
				pwd, err := os.Getwd()
				Expect(err).ToNot(HaveOccurred())

				// Open the yaml file
				raw, err := ioutil.ReadFile(pwd + "/tests/resources/talm-cgu-missing-cluster.yaml")
				Expect(err).ToNot(HaveOccurred())

				// Apply the cgu
				result, err := rantalmhelper.ApplyTalmResource(raw)
				Expect(err).ToNot(HaveOccurred())

				// Wait for the cgu condition to show the expected error message
				err = wait.PollImmediate(
					rantalmparameters.TalmTestPollInterval,
					rantalmparameters.TalmDefaultReconcileTime,
					func() (done bool, err error) {
						// Get the progressing condition
						condition, err := rantalmhelper.GetTalmCondition(result, "ClustersSelected")
						// The condition may not exist when we call this, so we need to wait for it to exist
						if err != nil {
							return false, err
						}

						// Check the error message
						expectedError := "Unable to select clusters: cluster non-existent-cluster is not a ManagedCluster"
						if condition.Message == expectedError {
							return true, nil
						}

						return false, nil
					})
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
