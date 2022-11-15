package containernshidetest

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/containernshide/tests"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestContainerNsHide(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Container Mount Namespace Hiding Test Suite", reporterConfig)
}
