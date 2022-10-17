package tests

import (
	. "github.com/onsi/ginkgo/v2"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
)

var _ = Describe("Talm Multibatch Tests", func() {

	execute.BeforeAll(func() {
	})

	BeforeEach(func() {
	})

	Describe("Multiple batch test", func() {
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
		// 	})
		// })
		// Context("where the second batch fails with continue option", func() {
		// 	It("should finish the remaining batches", func() {
		// 		// Polarion test id 47954
		// 		// https://issues.redhat.com/browse/CNF-6482
		// 	})
		// })
		// Context("where all batches are successful", func() {
		// 	It("should completed the CGU", func() {
		// 		// Polarion test id 47947
		// 		// https://issues.redhat.com/browse/CNF-6479
		// 	})
		// })
		// Context("where all the batches fail", func() {
		// 	It("should error the CGU", func() {
		// 		// Polarion test id 47954
		// 	})
		// })
	})
})
