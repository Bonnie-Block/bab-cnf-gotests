package ptp

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/onsi/ginkgo/reporters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/ptp/parameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/ptp/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
)

func TestPtp(t *testing.T) {
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
	RunSpecsWithDefaultAndCustomReporters(t, "PTP tests", rr)
}

var _ = BeforeSuite(func() {
	clients, err := config.DefineClients()
	Expect(err).ToNot(HaveOccurred())
	err = namespaces.Create(parameters.TestNamespace, clients)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	clients, err := config.DefineClients()
	Expect(err).ToNot(HaveOccurred())
	err = namespaces.DeleteAndWait(clients, parameters.TestNamespace, 5*time.Minute)
	Expect(err).ToNot(HaveOccurred())
})
