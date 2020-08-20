package sriov

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/parameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/cluster"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
)

const (
	timeout = 1800 * time.Second
)

func TestSriov(t *testing.T) {
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
	RunSpecsWithDefaultAndCustomReporters(t, "SRIOV Operator conformance tests", rr)
}

var _ = BeforeSuite(func() {
	clients, err := config.DefineClients()
	Expect(err).ToNot(HaveOccurred())
	sriovInfos, err := cluster.DiscoverSriov(clients, parameters.OperatorNamespace)
	err = helper.CompareNodeSriovInterfaces(sriovInfos)
	Expect(err).ToNot(HaveOccurred())
	namespaces.Clean(parameters.OperatorNamespace, parameters.OperatorTestNamespace, clients, false)
	helper.WaitForSRIOVStable(clients, parameters.OperatorNamespace, timeout)
})

var _ = AfterSuite(func() {
	clients, err := config.DefineClients()
	Expect(err).ToNot(HaveOccurred())
	namespaces.Clean(parameters.OperatorNamespace, parameters.OperatorTestNamespace, clients, false)
	err = namespaces.DeleteAndWait(clients, parameters.OperatorTestNamespace, timeout)
	Expect(err).ToNot(HaveOccurred())
	helper.WaitForSRIOVStable(clients, parameters.OperatorNamespace, timeout)
})
