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
	err = GetArgocdAppGitDetails()
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

// GetArgocdAppGitDetails is used to check the environment variables for any ztp test configuration.
// If any are undefined then the default values are used instead.
func GetArgocdAppGitDetails() error {
	// Check if the hub is defined
	if os.Getenv(ranztpparameters.HubKubeEnvKey) != "" {
		// Loop over the apps and save the git details
		for _, app := range ranztpparameters.ArgocdApps {
			repo, branch, dir, err := ranztphelper.GetGitDetailsFromArgocd(app, ranztpparameters.OpenshiftGitops)
			if err != nil {
				return err
			}

			// Save the git details to the map
			ranztphelper.ArgocdApps[app] = ranztpparameters.ArgocdGitDetails{
				Repo:   repo,
				Branch: branch,
				Path:   dir,
			}
		}
	}

	return nil
}

// InitializeClients is used to create the API clients for the spoke and hub.
func InitializeClients() error {
	var err error

	if os.Getenv(ranztpparameters.HubKubeEnvKey) != "" {
		// Define all the hub information
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

		ranztphelper.ZtpVersion, err = ranztphelper.GetZtpVersionFromArgocd(
			ranztpparameters.OpenshiftGitopsRepoServer,
			ranztpparameters.OpenshiftGitops,
		)

		if err != nil {
			return err
		}

		log.Printf("cluster '%s' has ZTP version '%s'\n", ranztphelper.HubName, ranztphelper.ZtpVersion)
	}

	// Spoke is the default kubeconfig
	if os.Getenv(ranztpparameters.SpokeKubeEnvKey) != "" {
		ranztphelper.SpokeAPIClient, err = ranhelper.DefineAPIClient(ranztpparameters.SpokeKubeEnvKey)
		if err != nil {
			return err
		}

		ranztphelper.SpokeName, err = ranhelper.GetClusterName(ranztpparameters.SpokeKubeEnvKey)
		if err != nil {
			return err
		}

		ocpVersion, err := ranhelper.GetClusterVersion(ranztphelper.SpokeAPIClient)
		if err != nil {
			return err
		}

		log.Printf("cluster '%s' has OCP version '%s'\n", ranztphelper.SpokeName, ocpVersion)

		ranztphelper.AcmVersion, err = ranhelper.GetOperatorVersionFromCSV(
			ranztphelper.HubAPIClient,
			ranztpparameters.AcmOperatorName,
			ranztpparameters.AcmOperatorNamespace,
		)
		if err != nil {
			return err
		}

		log.Printf("cluster '%s' has ACM version '%s'\n", ranztphelper.HubName, ranztphelper.AcmVersion)
	}

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
	if os.Getenv(ranztpparameters.HubKubeEnvKey) != "" {
		// Loop over the apps and restore the git details
		for _, app := range ranztpparameters.ArgocdApps {
			// Restore the app's git details
			err := ranztphelper.SetGitDetailsInArcgocd(
				ranztphelper.ArgocdApps[app].Repo,
				ranztphelper.ArgocdApps[app].Branch,
				ranztphelper.ArgocdApps[app].Path,
				app,
				false,
			)

			if err != nil {
				return err
			}
		}
	}

	return nil
}
