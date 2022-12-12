package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztphelper"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
)

var _ = Describe("Talm ZTP PGT Interval Tests", func() {

	// These tests use the hub and spoke
	var clusterList []*testClient.ClientSet

	execute.BeforeAll(func() {
		// Initialize cluster list
		clusterList = ranztphelper.GetAllTestClients()
	})

	BeforeEach(func() {
		// Check that the required clusters are present
		err := ranhelper.IsClustersPresent(clusterList)
		if err != nil {
			Skip(fmt.Sprintf("error occurred validating required clusters are present: %s", err.Error()))
		}
	})

	Context("override the PGT policy's compliance and non-compliance intervals", func() {
		It("should specify new intervals and verify they were applied", func() {
			// https://issues.redhat.com/browse/CNF-6305

			Skip("not implemented yet")
		})
		It("should specify an invalid interval format and verify the app error", func() {
			// https://issues.redhat.com/browse/CNF-6306

			Skip("not implemented yet")
		})
	})
})
