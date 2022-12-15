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

func TestZtp(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = helper.Config.GetReportPath(currentFile)

	RegisterFailHandler(Fail)
	// Stop ginkgo complaining about slow tests

	RunSpecs(t, "RAN ZTP tests", reporterConfig)
}

var _ = BeforeSuite(func() {
	InitializeZtpGitEnvironment()
	err := InitializeClients()
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
})

var _ = ReportAfterEach(func(report types.SpecReport) {
	testutils.ReportIfFailed(report, currentFile, ranztpparameters.ZtpNamespaces, ranztpparameters.ZtpCrds)
})

// InitializeZtpGitEnvironment is used to check the environment variables for any ztp test configuration.
// If any are undefined then the default values are used instead.
func InitializeZtpGitEnvironment() {
	ranztphelper.ZtpGitRepo = os.Getenv(ranztpparameters.ZtpGitRepoEnvKey)
	if ranztphelper.ZtpGitRepo == "" {
		repo, _, _, err := ranztphelper.GetGitDetailsFromArgocd()
		if err == nil {
			ranztphelper.ZtpGitRepo = repo
		}
	}

	ranztphelper.ZtpGitBranch = os.Getenv(ranztpparameters.ZtpGitBranchEnvKey)
	if ranztphelper.ZtpGitBranch == "" {
		_, branch, _, err := ranztphelper.GetGitDetailsFromArgocd()
		if err == nil {
			ranztphelper.ZtpGitBranch = branch
		}
	}

	ranztphelper.ZtpGitDir = os.Getenv(ranztpparameters.ZtpGitDirEnvKey)
	if ranztphelper.ZtpGitDir == "" {
		_, _, dir, err := ranztphelper.GetGitDetailsFromArgocd()
		if err == nil {
			ranztphelper.ZtpGitDir = dir
		}
	}
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

	log.Printf("cluster '%s' has OCP version '%s'", ranztphelper.HubName, ocpVersion)

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

	log.Printf("cluster '%s' has OCP version '%s'", ranztphelper.SpokeName, ocpVersion)

	return nil
}
