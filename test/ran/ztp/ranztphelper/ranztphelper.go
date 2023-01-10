package ranztphelper

import (
	"context"
	"log"
	"time"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
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

// GetZtpContext is used to get the context for the Ztp test client interactions.
func GetZtpContext() context.Context {
	return context.Background()
}

// GetAllTestClients is used to quickly obtain a list of all the test clients.
func GetAllTestClients() []*testClient.ClientSet {
	return []*testClient.ClientSet{
		HubAPIClient,
		SpokeAPIClient,
	}
}

// GetArgocdApp is used to fetch the Argocd application that is being used by Ztp.
func GetArgocdApp() (*argocdv1alpha1.Application, error) {
	app, err := HubAPIClient.
		ArgoprojV1alpha1Interface.
		Applications(ranztpparameters.OpenshiftGitops).
		Get(GetZtpContext(), ranztpparameters.Policies, v1.GetOptions{})

	return app, err
}

// SetGitDetailsInArgocd is used to update the git repo, branch, and path in the Argocd app.
func SetGitDetailsInArcgocd(gitRepo string, gitBranch string, gitPath string) error {
	app, err := GetArgocdApp()
	if err != nil {
		return err
	}

	if app.Spec.Source.RepoURL == gitRepo &&
		app.Spec.Source.TargetRevision == gitBranch &&
		app.Spec.Source.Path == gitPath {
		log.Println("Provided git details are the already configured details in Argocd. No change required.")

		return nil
	}

	app.Spec.Source.RepoURL = gitRepo
	app.Spec.Source.TargetRevision = gitBranch
	app.Spec.Source.Path = gitPath

	log.Println("Updating existing argocd app")
	log.Printf("Configuring RepoURL '%s'\n", gitRepo)
	log.Printf("Configuring TargetRevision '%s'\n", gitBranch)
	log.Printf("Configuring Path '%s'\n", gitPath)

	_, err = HubAPIClient.
		ArgoprojV1alpha1Interface.
		Applications(ranztpparameters.OpenshiftGitops).
		Update(GetZtpContext(), app, v1.UpdateOptions{})
	if err != nil {
		return err
	}

	err = WaitForArgocdChangeToComplete(ranztpparameters.ArgocdChangeTimeout)
	if err != nil {
		return err
	}

	return nil
}

// SetGitDetailsInArgocd is used to get the current git repo, branch, and path in the Argocd app.
func GetGitDetailsFromArgocd() (string, string, string, error) {
	app, err := GetArgocdApp()
	if err != nil {
		return "", "", "", err
	}

	return app.Spec.Source.RepoURL, app.Spec.Source.TargetRevision, app.Spec.Source.Path, nil
}

// WaitForArgocdChangeToComplete is used to wait until Argocd has updated its configuration.
func WaitForArgocdChangeToComplete(timeout time.Duration) error {
	log.Println("Waiting for Argocd change to finish syncing")

	err := wait.PollImmediate(ranztpparameters.ArgocdChangeInterval, timeout, func() (bool, error) {
		app, err := GetArgocdApp()

		if err != nil {
			return false, err
		}

		// The status should be 'Synced' and we expect the status sync fields to match the configured spec
		if app.Status.Sync.Status == "Synced" &&
			app.Status.Sync.ComparedTo.Source.RepoURL == app.Spec.Source.RepoURL &&
			app.Status.Sync.ComparedTo.Source.TargetRevision == app.Spec.Source.TargetRevision &&
			app.Status.Sync.ComparedTo.Source.Path == app.Spec.Source.Path {
			return true, nil
		}

		// If there are any conditions then it probably means theres a problem
		// By printing them here we can make diagnosing a failing test easier
		if len(app.Status.Conditions) > 0 {
			log.Printf("Current conditions: '%s'\n", app.Status.Conditions)
		}

		return false, nil
	})

	return err
}
