package ranztphelper

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	argocdappv1alpha "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/tidwall/gjson"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

var (
	HubAPIClient   *testClient.ClientSet
	HubName        string
	SpokeAPIClient *testClient.ClientSet
	SpokeName      string
	ZtpGitRepo     string
	ZtpGitBranch   string
	ZtpGitDir      string
	ZtpVersion     string
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

// GetNode is used to get a node object from a test client.
func GetNode(client *testClient.ClientSet) (corev1.Node, error) {
	nodeList, err := client.CoreV1Interface.Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return corev1.Node{}, nil
	}

	return nodeList.Items[0], nil
}

// GetArgocdApp is used to fetch the Argocd application that is being used by Ztp.
func GetArgocdApp(appName string, namespace string) (*argocdappv1alpha.Application, error) {
	argoApp, err := HubAPIClient.
		ArgoprojV1alpha1Interface.
		Applications(namespace).
		Get(GetZtpContext(), appName, metav1.GetOptions{})

	return argoApp, err
}

// SetGitDetailsInArgocd is used to update the git repo, branch, and path in the Argocd app.
func SetGitDetailsInArcgocd(gitRepo string, gitBranch string, gitPath string, waitForSync bool) error {
	app, err := GetArgocdApp(ranztpparameters.Policies, ranztpparameters.ZtpDeployedNamespace)
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
		Applications(ranztpparameters.ZtpDeployedNamespace).
		Update(GetZtpContext(), app, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	if waitForSync {
		err = WaitForArgocdChangeToComplete(ranztpparameters.ArgocdChangeTimeout)
		if err != nil {
			return err
		}
	}

	return nil
}

// SetGitDetailsInArgocd is used to get the current git repo, branch, and path in the Argocd app.
func GetGitDetailsFromArgocd() (string, string, string, error) {
	app, err := GetArgocdApp(ranztpparameters.Policies, ranztpparameters.ZtpDeployedNamespace)
	if err != nil {
		return "", "", "", err
	}

	return app.Spec.Source.RepoURL, app.Spec.Source.TargetRevision, app.Spec.Source.Path, nil
}

// GetZtpVersionFromArgocd is used to fetch the version of the ztp-site-generator init container.
func GetZtpVersionFromArgocd(name string, namespace string) (string, error) {
	deployment, err := HubAPIClient.Deployments(namespace).Get(GetZtpContext(), name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	for _, container := range deployment.Spec.Template.Spec.InitContainers {
		// Legacy 4.11 uses the image name as `ztp-site-generator`
		// While 4.12+ uses the image name as `ztp-site-generate`
		// So just check for `ztp-site-gen` to cover both
		if strings.Contains(container.Image, "ztp-site-gen") {
			ztpVersion := strings.Split(container.Image, ":")[1]

			if ztpVersion == "latest" {
				log.Println("Site generator version tag was 'latest', so returning empty version")

				return "", nil
			}

			// The format here will be like vX.Y.Z so we need to remove the v at the start
			return ztpVersion[1:], nil
		}
	}

	return "", fmt.Errorf("unable to identify ztp version")
}

// WaitForArgocdChangeToComplete is used to wait until Argocd has updated its configuration.
func WaitForArgocdChangeToComplete(timeout time.Duration) error {
	log.Println("Waiting for Argocd change to finish syncing")

	err := wait.PollImmediate(ranztpparameters.ArgocdChangeInterval, timeout, func() (bool, error) {
		app, err := GetArgocdApp(ranztpparameters.Policies, ranztpparameters.ZtpDeployedNamespace)

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

func GetEvaluationIntervals(policyName string, namespace string) (string, string, error) {
	log.Printf("Checking policy '%s' in namespace '%s' to fetch evaluation intervals\n", policyName, namespace)

	// Create a typed namespace object
	typedNamespace := types.NamespacedName{}
	typedNamespace.Name = policyName
	typedNamespace.Namespace = namespace

	// Create a policy object
	policy := policiesv1.Policy{}

	// Get the policy from the hub
	err := HubAPIClient.Client.Get(GetZtpContext(), typedNamespace, &policy)
	if err != nil {
		return "", "", err
	}

	// The configured policy evaluation intervals are a bit buried down in the spec
	// e.g.
	// spec:
	// 	  disabled: false
	// 	  policy-templates:
	// 	  - objectDefinition:
	// 		  apiVersion: policy.open-cluster-management.io/v1
	// 		  kind: ConfigurationPolicy
	// 		  metadata:
	// 			  name: example-policy
	// 		  spec:
	// 			  evaluationInterval:
	// 				  compliant: 2m
	// 				  noncompliant: 2m

	// First convert the runtime object to json
	jsonBytes, err := policy.Spec.PolicyTemplates[0].ObjectDefinition.Marshal()
	if err != nil {
		return "", "", err
	}

	// Next convert the byte array to an actual string
	jsonString := bytes.NewBuffer(jsonBytes).String()

	// Then use gjson to get the nested values out from the json
	complianceInterval := gjson.Get(jsonString, "spec.evaluationInterval.compliant").String()
	nonComplianceInterval := gjson.Get(jsonString, "spec.evaluationInterval.noncompliant").String()

	// Get the intervals from the policy
	return complianceInterval, nonComplianceInterval, nil
}

// WaitForCguConditionToMatchExpectedMessage waits until a specified condition type
// matches the provided message and/or status.
func WaitForConditionInArgocdApp(
	client *testClient.ClientSet,
	application string,
	namespace string,
	expectedMessage string,
	timeout time.Duration) error {
	log.Printf(
		"Checking application '%s' in namespace' %s' for condition with message '%s'\n",
		application,
		namespace,
		expectedMessage,
	)

	// Use a poll to check the argocd app condition
	err := wait.PollImmediate(
		ranztpparameters.ArgocdChangeInterval,
		timeout,
		func() (done bool, err error) {
			// Get the application
			app, err := client.
				ArgoprojV1alpha1Interface.
				Applications(namespace).
				Get(GetZtpContext(), application, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			// Loop over all the conditions
			for _, condition := range app.Status.Conditions {

				// If we found a matching condition then return immediately
				if strings.Contains(condition.Message, expectedMessage) {
					println("Found matching condition")

					return true, nil
				}

				log.Printf("Condition message '%s' did not match\n", condition.Message)
			}

			// If we didn't find a matching condition then we'll try again on the next loop
			return false, nil

		},
	)

	return err
}
