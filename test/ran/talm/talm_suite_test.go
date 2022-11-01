package talm

import (
	"context"
	"errors"
	"log"
	"os"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _, currentFile, _, _ = runtime.Caller(0)

var (
	err      error
	talmPods *corev1.PodList
)

func TestTalm(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)
	reporterConfig.SlowSpecThreshold = 1500.0

	RegisterFailHandler(Fail)
	// Stop ginkgo complaining about slow tests

	RunSpecs(t, "RAN TALM tests", reporterConfig)

	reporterConfig.SlowSpecThreshold = 5.0
}

var _ = BeforeSuite(func() {
	// Create an ApiClient for each node
	err = InitializeTalmClients()
	Expect(err).ToNot(HaveOccurred())

	// Make sure TALM is present
	err = VerifyTalmIsInstalled()
	Expect(err).ToNot(HaveOccurred())

	// Delete the namespace before creating it to ensure it is in a consistent blank state
	err = DeleteTalmTestNamespace()
	err = CreateTalmTestNamespace()
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	// Deleting the namespace after the suite finishes ensures all the CGUs created are deleted
	err = DeleteTalmTestNamespace()
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, rantalmparameters.TalmNamespaces, rantalmparameters.TalmCrds)
})

// VerifyTalmIsInstalled checks that talm pod+container is present and that CGUs can be fetched.
func VerifyTalmIsInstalled() error {
	// Check for talm pods
	talmPods, err = rantalmhelper.HubAPIClient.
		Pods(rantalmparameters.TalmOperatorNamespace).
		List(context.Background(),
			metav1.ListOptions{
				LabelSelector: rantalmparameters.TalmPodLabelSelector})
	if err != nil {
		return err
	}

	// Check if any pods exist
	if talmPods.Size() == 0 {
		return errors.New("unable to find talm pod")
	}

	// Check each pod for the talm container
	for _, talmPod := range talmPods.Items {
		err = helper.IsPodHealthy(&talmPod)
		if err != nil {
			return err
		}

		if !ranhelper.IsContainerExistInPod(talmPod, rantalmparameters.TalmContainerName) {
			return errors.New("talm pod defined but talm container does not exist")
		}
	}

	// Fetch a list of CGUs (which should be empty) to verify that the CRD is present
	_, err := rantalmhelper.HubAPIClient.
		ClusterGroupUpgrades("default").
		List(rantalmhelper.GetTestContext(), metav1.ListOptions{})
	Expect(err).ToNot(HaveOccurred())

	return nil
}

// InitializeTalmClients is used to create the three API clients for the two spokes and hub.
func InitializeTalmClients() error {
	// Hub may be optional depending on what tests are running
	if os.Getenv(rantalmparameters.HubKubeEnvKey) != "" {
		rantalmhelper.HubAPIClient, err = ranhelper.DefineAPIClient(rantalmparameters.HubKubeEnvKey)
		if err != nil {
			return err
		}

		rantalmhelper.HubName, err = rantalmhelper.GetClusterName(rantalmparameters.HubKubeEnvKey)
		if err != nil {
			return err
		}

		ocpVersion, err := rantalmhelper.GetClusterVersion(rantalmhelper.HubAPIClient)
		if err != nil {
			return err
		}

		log.Printf("cluster %s has OCP version %s", rantalmhelper.HubName, ocpVersion)
	}

	// Spoke1 is the default kubeconfig
	if os.Getenv(rantalmparameters.Spoke1KubeEnvKey) != "" {
		rantalmhelper.Spoke1APIClient, err = ranhelper.DefineAPIClient(rantalmparameters.Spoke1KubeEnvKey)
		if err != nil {
			return err
		}

		rantalmhelper.Spoke1Name, err = rantalmhelper.GetClusterName(rantalmparameters.Spoke1KubeEnvKey)
		if err != nil {
			return err
		}

		ocpVersion, err := rantalmhelper.GetClusterVersion(rantalmhelper.Spoke1APIClient)
		if err != nil {
			return err
		}

		log.Printf("cluster %s has OCP version %s", rantalmhelper.Spoke1Name, ocpVersion)
	}

	// Spoke2 may be optional depending on what tests are running
	if os.Getenv(rantalmparameters.Spoke2KubeEnvKey) != "" {
		rantalmhelper.Spoke2APIClient, err = ranhelper.DefineAPIClient(rantalmparameters.Spoke2KubeEnvKey)
		if err != nil {
			return err
		}

		rantalmhelper.Spoke2Name, err = rantalmhelper.GetClusterName(rantalmparameters.Spoke2KubeEnvKey)
		if err != nil {
			return err
		}

		ocpVersion, err := rantalmhelper.GetClusterVersion(rantalmhelper.Spoke2APIClient)
		if err != nil {
			return err
		}

		log.Printf("cluster %s has OCP version %s", rantalmhelper.Spoke2Name, ocpVersion)
	}

	return nil
}

// CreateTalmTestNamespace creates the TALM test namespace on each of the nodes.
func CreateTalmTestNamespace() error {
	// Hub may be optional depending on what tests are running
	if os.Getenv(rantalmparameters.HubKubeEnvKey) != "" {
		err = namespaces.Create(rantalmparameters.TalmTestNamespace, rantalmhelper.HubAPIClient)
		if err != nil {
			return err
		}
	}

	// Spoke1 is the default kubeconfig
	if os.Getenv(rantalmparameters.Spoke1KubeEnvKey) != "" {
		err = namespaces.Create(rantalmparameters.TalmTestNamespace, rantalmhelper.Spoke1APIClient)
		if err != nil {
			return err
		}
	}

	// Spoke2 may be optional depending on what tests are running
	if os.Getenv(rantalmparameters.Spoke2KubeEnvKey) != "" {
		err = namespaces.Create(rantalmparameters.TalmTestNamespace, rantalmhelper.Spoke2APIClient)
		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteTalmTestNamespace deletes the TALM test namespace on each of the nodes.
func DeleteTalmTestNamespace() error {
	// Hub may be optional depending on what tests are running
	if os.Getenv(rantalmparameters.HubKubeEnvKey) != "" {
		err = namespaces.DeleteAndWait(
			rantalmhelper.HubAPIClient,
			rantalmparameters.TalmTestNamespace,
			rantalmparameters.TalmTestPollInterval)
		if err != nil {
			return err
		}
	}

	// Spoke1 is the default kubeconfig
	if os.Getenv(rantalmparameters.Spoke1KubeEnvKey) != "" {
		err = namespaces.DeleteAndWait(
			rantalmhelper.Spoke1APIClient,
			rantalmparameters.TalmTestNamespace,
			rantalmparameters.TalmTestPollInterval)
		if err != nil {
			return err
		}
	}

	// Spoke2 may be optional depending on what tests are running
	if os.Getenv(rantalmparameters.Spoke2KubeEnvKey) != "" {
		err = namespaces.DeleteAndWait(
			rantalmhelper.Spoke2APIClient,
			rantalmparameters.TalmTestNamespace,
			rantalmparameters.TalmTestPollInterval)
		if err != nil {
			return err
		}
	}

	return nil
}
