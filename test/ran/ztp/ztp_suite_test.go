package ztp

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestZtp(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)

	RegisterFailHandler(Fail)
	// Stop ginkgo complaining about slow tests

	RunSpecs(t, "RAN ZTP tests", reporterConfig)
}

var _ = BeforeSuite(func() {
	// API clients must be initialized first since they will be used later
	err := InitializeClients()
	Expect(err).ToNot(HaveOccurred())

	// This will get the current data from Argocd and save it for later
	err = InitializeZtpGitEnvironment()
	Expect(err).ToNot(HaveOccurred())

	// Delete and re-create the namespace to start with a clean state
	err = DeleteNamespace(true)
	Expect(err).ToNot(HaveOccurred())
	err = CreateNamespace(false)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	// Restore the original Argocd configuration after the tests are completed
	err := ResetArgocdGitDetails()
	Expect(err).ToNot(HaveOccurred())

	// Delete the namespace
	err = DeleteNamespace(true)
	Expect(err).ToNot(HaveOccurred())
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, ranztpparameters.ZtpNamespaces, ranztpparameters.ZtpCrds)
})

// InitializeZtpGitEnvironment is used to check the environment variables for any ztp test configuration.
// If any are undefined then the default values are used instead.
func InitializeZtpGitEnvironment() error {
	// Get all the details from Argocd
	repo, branch, dir, err := ranztphelper.GetGitDetailsFromArgocd()

	if err != nil {
		return err
	}

	// Save them all to restore them later
	ranztphelper.ZtpGitRepo = repo
	ranztphelper.ZtpGitBranch = branch
	ranztphelper.ZtpGitDir = dir

	log.Printf("Existing Argocd test repo '%s'\n", repo)
	log.Printf("Existing Argocd test branch '%s'\n", branch)
	log.Printf("Existing Argocd test dir '%s'\n", dir)

	return nil
}

// InitializeClients is used to create the API clients for the spoke and hub.
func InitializeClients() error {
	// Hub is required as that's where argocd is running
	if os.Getenv(ranztpparameters.HubKubeEnvKey) == "" {
		return fmt.Errorf("required environment key %s was not defined", ranztpparameters.HubKubeEnvKey)
	}

	var err error

	ranztphelper.HubAPIClient, err = ranhelper.DefineAPIClient(ranztpparameters.HubKubeEnvKey)
	if err != nil {
		return err
	}

	ranztphelper.HubName, err = ranhelper.GetClusterName(ranztpparameters.HubKubeEnvKey)
	if err != nil {
		return err
	}

	ocpVersion, err := ranhelper.GetClusterVersion(ranztphelper.HubAPIClient)
	if err != nil {
		return err
	}

	log.Printf("cluster '%s' has OCP version '%s'\n", ranztphelper.HubName, ocpVersion)

	// Spoke is the default kubeconfig
	if os.Getenv(ranztpparameters.SpokeKubeEnvKey) == "" {
		return fmt.Errorf("required environment key %s was not defined", ranztpparameters.SpokeKubeEnvKey)
	}

	ranztphelper.SpokeAPIClient, err = ranhelper.DefineAPIClient(ranztpparameters.SpokeKubeEnvKey)
	if err != nil {
		return err
	}

	ranztphelper.SpokeName, err = ranhelper.GetClusterName(ranztpparameters.SpokeKubeEnvKey)
	if err != nil {
		return err
	}

	ocpVersion, err = ranhelper.GetClusterVersion(ranztphelper.SpokeAPIClient)
	if err != nil {
		return err
	}

	log.Printf("cluster '%s' has OCP version '%s'\n", ranztphelper.SpokeName, ocpVersion)

	return nil
}

// CreateNamespace is used to create the test namespace `ztp-test` on the hub node.
func CreateNamespace(allowExists bool) error {
	if os.Getenv(ranztpparameters.HubKubeEnvKey) != "" {
		// If the namespace already exists but we weren't expecting it to then return an error
		if namespaces.Exists(ranztpparameters.ZtpTestNamespace, ranztphelper.HubAPIClient) {
			if !allowExists {
				return fmt.Errorf(
					"Namespace '%s' exists when it should not on node '%s'",
					ranztpparameters.ZtpTestNamespace,
					ranztphelper.HubName,
				)
			}

			log.Printf("Namespace '%s' already exists on node '%s'\n", ranztpparameters.ZtpTestNamespace, ranztphelper.HubName)

			return nil
		}

		log.Printf("Creating namespace '%s' on node '%s'\n", ranztpparameters.ZtpTestNamespace, ranztphelper.HubName)

		// Otherwise create the namespace
		err := namespaces.Create(ranztpparameters.ZtpTestNamespace, ranztphelper.HubAPIClient)
		if err != nil {
			return fmt.Errorf(
				"Failed to create namespace '%s' on node '%s' due to error '%w'",
				ranztpparameters.ZtpTestNamespace,
				ranztphelper.HubName,
				err,
			)
		}
	}

	return nil
}

// DeleteNamespace is used to delete the test namespace `ztp-test` if it exists on the hub node.
func DeleteNamespace(allowNotExists bool) error {
	if os.Getenv(ranztpparameters.HubKubeEnvKey) != "" {
		// If the namespace already exists then delete it
		if namespaces.Exists(ranztpparameters.ZtpTestNamespace, ranztphelper.HubAPIClient) {
			log.Printf("Deleting namespace '%s' on node '%s'\n", ranztpparameters.ZtpTestNamespace, ranztphelper.HubName)

			err := namespaces.DeleteAndWait(ranztphelper.HubAPIClient, ranztpparameters.ZtpTestNamespace, 5*time.Minute)
			if err != nil {
				return fmt.Errorf(
					"Failed to delete namespace '%s' on node '%s' due to error '%w'",
					ranztpparameters.ZtpTestNamespace,
					ranztphelper.HubName,
					err,
				)
			}
		} else if !allowNotExists {
			// If we expected the namespace to exist but it wasn't then return an error

			return fmt.Errorf(
				"Namespace '%s' does not exist when it should on node '%s'",
				ranztpparameters.ZtpTestNamespace,
				ranztphelper.HubName,
			)
		}
	}

	return nil
}

// ResetArgocdGitDetails is used to configure Argocd back to the values it had before the tests started.
func ResetArgocdGitDetails() error {
	log.Println("Resetting Argocd app back to initial values")

	err := ranztphelper.SetGitDetailsInArcgocd(ranztphelper.ZtpGitRepo, ranztphelper.ZtpGitBranch, ranztphelper.ZtpGitDir, false)
	if err != nil {
		return err
	}

	return nil
}
