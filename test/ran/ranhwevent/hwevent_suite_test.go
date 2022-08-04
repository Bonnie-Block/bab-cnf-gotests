package ranhwevent

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/onsi/ginkgo/v2/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stmcginnis/gofish/redfish"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventhelper/consumers"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventhelper/nodevendor"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventhelper/ocp"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventhelper/rfclient"
	corev1 "k8s.io/api/core/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/tests"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

var (
	_, currentFile, _, _ = runtime.Caller(0)
	subscriptionURI      rfclient.SubscriptionURI
	eventService         *redfish.EventService
	ConsumersList        *corev1.PodList
	PrivilegedPods       map[string]*corev1.Pod
	LocalNodeVendor      string
	err                  error
)

func TestHwEvent(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()

	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)

	RegisterFailHandler(Fail)

	reporterConfig.SlowSpecThreshold = 1500.0
	RunSpecs(t, "RAN hw event tests", reporterConfig)
	reporterConfig.SlowSpecThreshold = 5.0
}

var _ = BeforeSuite(func() {
	By("Verify redfish hostname is defined")
	Expect(ranhweventparameters.Redfish.Hostname).ToNot(BeEmpty(),
		"Please set BMC_HOSTS environment variable to the URL for the tested node.")

	By("Verify redfish username is defined")
	Expect(ranhweventparameters.Redfish.Username).ToNot(BeEmpty(),
		"Please set BMC_USER environment variable to the node's redfish username.")

	By("Verify redfish password is defined")
	Expect(ranhweventparameters.Redfish.Password).ToNot(BeEmpty(),
		"Please set BMC_PASSWORD environment variable to the node's redfish password.")

	By("Connect to redfish")
	ranhweventparameters.Redfish.Session, err = rfclient.GetClient(ranhweventparameters.Redfish)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("failed to connect to redfish: %v using user: %v and pass: %v due to %v",
			ranhweventparameters.Redfish.RedfishURL,
			ranhweventparameters.Redfish.Username,
			ranhweventparameters.Redfish.Password,
			err))

	By("Creating privileged pods in order to query node vendor")
	PrivilegedPods = helper.CreatePrivilegedPods("")
	Expect(len(PrivilegedPods)).ToNot(Equal(0),
		"Missing Privileged pods")

	By("Query the node under test redfish vendor")
	LocalNodeVendor, err = nodevendor.GetRedfishVendor(ranhweventparameters.Redfish.Session)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("On redfish vendor query, got this error: %v\n", err))

	By("Verify that redfish has a HTTPS target to send the events defined")
	// This info can be queried form the open shift cluster, but will be added in a separate commit
	Expect(helper.Config.Ran.EventReceiver).ToNot(BeEmpty(),
		fmt.Sprintf("Please set EVENT_RECEIVER environment variable."+
			" This is the output of $ get route -n %v", ranhweventparameters.NamespaceConsumer))
	// check this HTTPS is alive
	// check operator is active

	By("Check consumers exist")
	ConsumersList, err = consumers.GetConsumers()
	Expect(err).ToNot(HaveOccurred(), err)
	Expect(ConsumersList.Items).ToNot(Equal(0), "Missing consumers")

	By("Purge previous redfish subscriptions")
	err = rfclient.ClearSubscriptions(ranhweventparameters.Redfish)
	Expect(err).ToNot(HaveOccurred(), err)

	By("Subscribe to events")
	subscriptionURI, eventService, err = rfclient.Subscribe(LocalNodeVendor, ranhweventparameters.Redfish)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(subscriptionURI).ToNot(Equal(nil))

})

var _ = AfterSuite(func() {
	By("Purge privileged pods that were created for test")
	err = ocp.PurgePrivPodNamespace()
	Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("Failed to purge privileged pods due to: %v\n",
		err))

	By("Unsubscribe events")
	err := rfclient.Unsubscribe(subscriptionURI, eventService)
	Expect(err).ToNot(HaveOccurred(), err)

	By("End redfish session.")
	ranhweventparameters.Redfish.Session.Logout()
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, ranhweventparameters.ReporterNamespacesToDump,
		ranhweventparameters.ReporterCrds)
})
