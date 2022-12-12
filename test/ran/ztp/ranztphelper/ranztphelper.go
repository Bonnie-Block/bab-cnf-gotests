package ranztphelper

import (
	"errors"

	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
)

var (
	HubAPIClient   *testClient.ClientSet
	HubName        string
	SpokeAPIClient *testClient.ClientSet
	SpokeName      string
	ZtpGitRepo     string
	ZtpGitBranch   string
	ZtpGitDir      string
	ZtpFullGitPath string
)

// GetAllTestClients is used to quickly obtain a list of all the test clients.
func GetAllTestClients() []*testClient.ClientSet {
	return []*testClient.ClientSet{
		HubAPIClient,
		SpokeAPIClient,
	}
}

func SetGitDetailsInArcgocd(gitRepo string, gitBranch string, gitPath string) error {
	return errors.New("not implemented")
}

func GetGitDetailsFromArgocd() (string, string, string, error) {
	return "", "", "", errors.New("not implemented")
}

func WaitForArgocdChangeToTakeEffect() error {
	return errors.New("not implemented")
}
