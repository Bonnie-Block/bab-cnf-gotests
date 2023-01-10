package ztp

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztphelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/tests"
	testutils "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/utils"
)

var _, currentFile, _, _ = runtime.Caller(0)

var (
	ArgoGitRepo   string
	ArgoGitBranch string
	ArgoGitDir    string
)

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
})

var _ = AfterSuite(func() {
	// Restore the original Argocd configuration after the tests are completed
	err := ResetArgocdGitDetails()
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

	// Save them all to restore them later
	ArgoGitRepo = repo
	ArgoGitBranch = branch
	ArgoGitDir = dir

	log.Printf("Existing Argocd test repo '%s'\n", repo)
	log.Printf("Existing Argocd test branch '%s'\n", branch)
	log.Printf("Existing Argocd test dir '%s'\n", dir)

	ranztphelper.ZtpGitRepo = os.Getenv(ranztpparameters.ZtpGitRepoEnvKey)
	if ranztphelper.ZtpGitRepo == "" {
		if err == nil {
			ranztphelper.ZtpGitRepo = repo
		}
	}

	log.Printf("Configured test repo '%s'\n", ranztphelper.ZtpGitRepo)

	ranztphelper.ZtpGitBranch = os.Getenv(ranztpparameters.ZtpGitBranchEnvKey)
	if ranztphelper.ZtpGitBranch == "" {
		if err == nil {
			ranztphelper.ZtpGitBranch = branch
		}
	}

	log.Printf("Configured test branch '%s'\n", ranztphelper.ZtpGitBranch)

	ranztphelper.ZtpGitDir = os.Getenv(ranztpparameters.ZtpGitDirEnvKey)
	if ranztphelper.ZtpGitDir == "" {
		if err == nil {
			ranztphelper.ZtpGitDir = dir
		}
	}

	log.Printf("Configured test dir '%s'\n", ranztphelper.ZtpGitDir)

	log.Println("Updating Argocd app with test configuration")

	err = ranztphelper.SetGitDetailsInArcgocd(ranztphelper.ZtpGitRepo, ranztphelper.ZtpGitBranch, ranztphelper.ZtpGitDir)
	if err != nil {
		return err
	}

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

// ResetArgocdGitDetails is used to configure Argocd back to the values it had before the tests started.
func ResetArgocdGitDetails() error {
	log.Println("Resetting Argocd app back to initial values")

	err := ranztphelper.SetGitDetailsInArcgocd(ArgoGitRepo, ArgoGitBranch, ArgoGitDir)
	if err != nil {
		return err
	}

	return nil
}
