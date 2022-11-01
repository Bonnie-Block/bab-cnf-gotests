package tests

import (
	. "github.com/onsi/ginkgo/v2"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
)

var _ = Describe("Talm Backup Tests", func() {
	// Backup tests require 2 spokes and a hub, and TALM must be 4.11+
	execute.BeforeAll(func() {
		// Check that the required clusters are present
		err := rantalmhelper.IsClustersPresent(rantalmhelper.GetAllTestClients())
		if err != nil {
			Skip(err.Error())
		}
	})

	Describe("Backup test", func() {
		// Context("with 2 spokes but 1 fails", func() {
		// 	It("should have only completed one spoke", func() {
		// 		// Polarion test id tbd
		// 		// https://issues.redhat.com/browse/CNF-6498
		// 	})
		// })
		// Context("with full disk", func() {
		// 	It("should have a failed cgu with specific error message", func() {
		// 		// Polarion test id 50835
		// 		// https://issues.redhat.com/browse/CNF-6500
		// 	})
		// })
	})
})
