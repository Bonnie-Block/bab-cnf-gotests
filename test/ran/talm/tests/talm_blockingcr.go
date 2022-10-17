package tests

import (
	. "github.com/onsi/ginkgo/v2"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
)

var _ = Describe("Talm Blocking CRs Tests", func() {

	execute.BeforeAll(func() {
	})

	BeforeEach(func() {
	})

	Describe("Single batch test", func() {
		// Context("with blocking CRs missing", func() {
		// 	It("Should report the missing CRs", func() {
		// 		// Polarion test id 47956
		// 		// https://issues.redhat.com/browse/CNF-6492
		// 	})
		// })
		// Context("with blocking CRs failed", func() {
		// 	It("should report the failed CRs", func() {
		// 		// Polarion test id 47948
		// 		// https://issues.redhat.com/browse/CNF-6493
		// 	})
		// })
		// Context("with blocking CRs successful", func() {
		// 	It("should complete the CGU", func() {
		// 		// Polarion test id 47948
		// 		// https://issues.redhat.com/browse/CNF-6494
		// 	})
		// })
	})
})
