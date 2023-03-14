package bmer

import (
	"fmt"
	"log"
	"runtime"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stmcginnis/gofish/redfish"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/bmer/ranbmerhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/bmer/ranbmerhelper/nodevendor"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/bmer/ranbmerhelper/rfclient"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/bmer/ranbmerparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/bmer/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranparameters"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
	corev1 "k8s.io/api/core/v1"
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

func TestBmer(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()

	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)

	RegisterFailHandler(Fail)

	RunSpecs(t, "RAN Bare Metal Event Relay Tests", reporterConfig)
}

var _ = BeforeSuite(func() {
	// Openshift related pre-test checks
	err = ranbmerhelper.CheckCustomResourceDefinition()
	if err != nil {
		Skip(fmt.Sprintf("Got this error when query feature custom resource definition: %v , skip testing", err))
	}

	// Redfish related pre-test checks
	By("Verify redfish hostname is defined")
	Expect(ranbmerparameters.Redfish.Hostname).ToNot(BeEmpty(),
		"Please set BMC_HOSTS environment variable to the URL for the tested node.")

	By("Verify redfish username is defined")
	Expect(ranbmerparameters.Redfish.Username).ToNot(BeEmpty(),
		"Please set BMC_USER environment variable to the node's redfish username.")

	By("Verify redfish password is defined")
	Expect(ranbmerparameters.Redfish.Password).ToNot(BeEmpty(),
		"Please set BMC_PASSWORD environment variable to the node's redfish password.")

	By("Connect to redfish")
	ranbmerparameters.Redfish.Session, err = rfclient.GetClient(ranbmerparameters.Redfish)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("failed to connect to redfish: %v using user: %v and pass: %v due to %v",
			ranbmerparameters.Redfish.RedfishURL,
			ranbmerparameters.Redfish.Username,
			ranbmerparameters.Redfish.Password,
			err))

	// Define the redfish access to the kubernetes operator using a secret
	err = ranbmerhelper.CreateHwEventSecret(
		ranbmerparameters.SecretName,
		parameters.BmerNamespace,
		ranbmerparameters.Redfish.Hostname,
		ranbmerparameters.Redfish.Username,
		ranbmerparameters.Redfish.Password)

	// In case the secret is already defined, do not fail the test
	if err != nil && err.Error() == "HwEvent secret already exist. skip creating it" {
		log.Println("Secret already defined. skip creating it.")
	} else {
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create secret due to: %v", err))
	}
	err = ranhelper.WaitForDeploymentReady(
		helper.Apiclient, parameters.BmerNamespace, ranbmerparameters.AppName)
	Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf(
		"Hardware event deployment is not ready after creating secret due to: %v", err))

	transportType, err := ranhelper.GetTransportType(
		helper.Apiclient, parameters.BmerNamespace, ranbmerparameters.AppName)
	Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf(
		"GetTransportType error: %v", err))
	if transportType != "" {
		ranparameters.TransportType = transportType
	} else {
		log.Printf("WARNING: failed to get transportType from hw-event-proxy, use default tranportType %v\n",
			ranparameters.TransportType)
	}

	By("Check tranportType: ", func() {
		fmt.Fprintln(GinkgoWriter, "***", ranparameters.TransportType, "***")
	})

	By("Check ClusterServiceVersions mirrored images necessary for consumer deploy")
	mirroredImages, err := ranhelper.GetDeployImages(parameters.BmerNamespace)
	Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf(
		"failed to get images to be used from ClusterServiceVersions due to: %v", err))

	By("Check consumer image is defined")
	Expect(helper.Config.Ran.BmerConsumerImage).ToNot(BeEmpty(),
		"BMER_CONSUMER_IMAGE environment is missing")

	By("Configure the cluster objects for hardware event proxy")
	err = ranbmerhelper.ConfigHwEventProxyObjects()
	Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("failed to config app due to: %v", err))

	By("Check routing to app service exist")
	ranbmerparameters.Redfish.EventReceiver, err = ranbmerhelper.GetAppRoute()

	Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf(
		"failed to find routing to application, so it can not receive events due to: %v", err))
	By("Verify that redfish has a HTTPS target to send the events defined")
	// check this HTTPS is alive retry if deployment is in progress.
	err = ranbmerhelper.GetHTTPS(ranbmerparameters.Redfish.EventReceiver)
	Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf(
		"failed to verify HTTPS is ready to recive events due to: %v", err))

	By("Deploy consumers")
	err = ranhelper.DeployConsumers(mirroredImages, ranparameters.TransportType, parameters.BmerNamespace)

	// In case the consumer already exist on the cluster an error with the string skip will be returned.
	if err != nil && err.Error() == "consumers already deployed in cluster. skipping creating them" {
		log.Printf("Consumers creating skipped: %v", err)
	} else {
		Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("failed to deploy consumers due to: %v", err))
	}

	By("Check consumers exist")
	ConsumersList, err = ranhelper.GetConsumers(parameters.BmerNamespace)
	Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("failed to check consumers exist due to: %v", err))

	By("Creating privileged pods in order to query node vendor")
	PrivilegedPods = helper.CreatePrivilegedPods("")

	By("Query the node under test redfish vendor")
	LocalNodeVendor, err = nodevendor.GetRedfishVendor(ranbmerparameters.Redfish.Session)
	Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("On redfish vendor query, got this error: %v\n", err))

	By("Purge previous redfish subscriptions")
	err = rfclient.ClearSubscriptions(ranbmerparameters.Redfish)
	if err != nil {
		log.Printf("failed to purge previous redfish subscriptions due to: %v", err)
	}

	By("Subscribe to events")
	subscriptionURI, eventService, err = rfclient.Subscribe(LocalNodeVendor, ranbmerparameters.Redfish)
	Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("failed to subscribe to redfish events due to: %v", err))
	Expect(subscriptionURI).ToNot(Equal(nil), "failed to get subscription URI replay")

	if ranparameters.TransportType == ranparameters.TransportHTTP {
		log.Printf("Add 5 seconds delay for HTTP transport to be ready")
		time.Sleep(5 * time.Second)
	}
})

var _ = AfterSuite(func() {
	var teardownErrors []error
	if eventService != nil {
		By("Unsubscribe events")
		teardownErrors = append(teardownErrors, rfclient.Unsubscribe(subscriptionURI, eventService))
	}
	By("Remove consumer pods")
	destroyErrors := ranhelper.DestroyConsumers()
	teardownErrors = append(teardownErrors, destroyErrors...)
	By("Remove Hw event secret")
	teardownErrors = append(teardownErrors, ranbmerhelper.DeleteHwEventSecret(
		parameters.BmerNamespace, ranbmerparameters.SecretName))
	By("End redfish session.")
	ranbmerparameters.Redfish.Session.Logout()
	By("Purge privileged pods that were created for test")
	teardownErrors = append(teardownErrors, ranbmerhelper.PurgePrivPodNamespace())

	By("Check errors in tear-down")
	for _, err := range teardownErrors {
		Expect(err).ShouldNot(HaveOccurred())
	}
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, ranbmerparameters.ReporterNamespacesToDump,
		ranbmerparameters.ReporterCrds)
})
