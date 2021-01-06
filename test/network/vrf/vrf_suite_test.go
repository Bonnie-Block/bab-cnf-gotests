package vrf

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/parameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/vrf/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
)

const (
	timeout = 1800 * time.Second
)

func TestVrf(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	configSuite, err := config.NewConfig()
	if err != nil {
		fmt.Print(err)
		return
	}
	junitPath := configSuite.GetReportPath(currentFile)
	dumpFile := configSuite.GetDumpFailedTestReportLocation(currentFile)
	RegisterFailHandler(Fail)
	rr := append([]Reporter{}, reporters.NewJUnitReporter(junitPath))
	if dumpFile != nil {
		clients, err := config.DefineClients()
		Expect(err).ToNot(HaveOccurred())
		rr = append(rr, k8sreporter.New(clients, dumpFile))
		defer dumpFile.Close()
	}
	RunSpecsWithDefaultAndCustomReporters(t, "VRF tests", rr)
}

var _ = AfterSuite(func() {
	clients, err := config.DefineClients()
	Expect(err).ToNot(HaveOccurred())
	err = namespaces.DeleteAndWait(clients, parameters.TestNamespace, timeout)
	Expect(err).ToNot(HaveOccurred())
})
