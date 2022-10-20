package talm

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
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
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

var _, currentFile, _, _ = runtime.Caller(0)

var (
	workerNodesList []corev1.Node
	err             error
	talmPods        *corev1.PodList
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
	rantalmhelper.TalmDynamicClient, err = GetAuthenticatedDynamicClient()
	Expect(err).ToNot(HaveOccurred())
	err = VerifyTalmIsInstalled()
	Expect(err).ToNot(HaveOccurred())
	PurgeYamlResources()

	log.Println("initializing spoke1 name")
	rantalmparameters.Spoke1Name = rantalmhelper.
		GetClusterName(os.Getenv("KUBECONFIG")) // assume SNO is from kubeconf
	Expect(rantalmparameters.Spoke1Name).ToNot(BeZero())

	log.Println("initializing hub clientset")
	rantalmparameters.HubClientset = rantalmhelper.InitHubClient(helper.Config.Ran.KubeconfigHub)
	Expect(rantalmparameters.HubClientset).ToNot(BeNil())
})

var _ = AfterSuite(func() {
	PurgeYamlResources()
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, rantalmparameters.TalmNamespaces, rantalmparameters.TalmCrds)
})

func VerifyTalmIsInstalled() error {
	// Get all worker nodes
	workerNodesList, err = nodes.GetByRole(helper.Apiclient, "worker")
	if err != nil {
		return err
	}

	// Check for talm pods
	talmPods, err = helper.Apiclient.Pods(rantalmparameters.TalmOperatorNamespace).List(context.Background(),
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

	// Check for presence of ClusterGroupUpgrade CRD
	// Depends on https://issues.redhat.com/browse/CNF-6462

	return nil
}

func PurgeYamlResources() {
	// Get the current directory
	pwd, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())

	if _, err := os.Stat(pwd + "/tests/resources"); os.IsNotExist(err) {
		return
	}

	// Open the yaml file
	files, err := ioutil.ReadDir(pwd + "/tests/resources")
	Expect(err).ToNot(HaveOccurred())

	for _, file := range files {
		if !file.IsDir() {
			// Open the yaml file
			raw, err := ioutil.ReadFile(pwd + "/tests/resources/" + file.Name())
			Expect(err).ToNot(HaveOccurred())

			// Delete the resource
			err = rantalmhelper.DeleteTalmResource(raw)
			Expect(err).ToNot(HaveOccurred())
		}
	}
}

// As far as I can tell there is no way to get an actual dynamic client as oppose to an interface
// Since the non-interface client is not exported by dynamic
// nolint:ireturn
func GetAuthenticatedDynamicClient() (dynamic.Interface, error) {
	// Authentication flow inspired by https://stackoverflow.com/a/73461820
	// Read the kubeconfig file
	kubeconfigFile := os.Getenv("KUBECONFIG")
	if kubeconfigFile == "" {
		return nil, fmt.Errorf("unable to find kubeconfig file")
	}

	// Read a config object
	authConfig, err := clientcmd.LoadFromFile(kubeconfigFile)
	if err != nil {
		return nil, err
	}

	// Convert the config object to a client config object
	clientConfig := clientcmd.NewDefaultClientConfig(*authConfig, &clientcmd.ConfigOverrides{})

	// Convert the client config to a rest config
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	// Create a dynamic client using the rest config
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	return dynamicClient, nil
}
