package powersave

import (
	"log"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/powersave/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	corev1 "k8s.io/api/core/v1"
)

const (
	timeout = 10 * time.Minute
)

var (
	_, currentFile, _, _ = runtime.Caller(0)
	PrivilegedPods       map[string]*corev1.Pod
)

func TestPowerSave(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Power Save Test Suite", reporterConfig)
}

var _ = BeforeSuite(func() {
	ranhelper.CleanupRanTestResources()
	PrivilegedPods = helper.CreatePrivilegedPods("")
})

var _ = AfterSuite(func() {
	if namespaces.Exists(ran.NamespaceTesting, helper.Apiclient) {
		log.Println("Deleting test namespace", ran.NamespaceTesting)
		err := namespaces.DeleteAndWait(helper.Apiclient, ran.NamespaceTesting, timeout)
		Expect(err).ToNot(HaveOccurred())
	}
})
