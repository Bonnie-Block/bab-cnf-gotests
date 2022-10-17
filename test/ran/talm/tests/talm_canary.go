package tests

import (
	. "github.com/onsi/ginkgo/v2"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
)

var _ = Describe("Talm Canary Tests", func() {

	execute.BeforeAll(func() {
	})

	BeforeEach(func() {
	})

	Describe("Canary feature used", func() {
		// Context("where first canary fails", func() {
		// 	It("should stop the CGU", func() {
		// 		// Polarion test id 47954
		// 		// https://issues.redhat.com/browse/CNF-6477
		// 	})
		// })
		// Context("where all the canaries are successful", func() {
		// 	It("should complete the CGU", func() {
		// 		// Polarion test id 47947
		// 		// https://issues.redhat.com/browse/CNF-6478
		// 	})
		// })
	})
})
