package ranztphelper

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	argocdoperatorv1alpha1 "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
	argocdappv1alpha "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	mcv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	kacv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	"github.com/tidwall/gjson"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	mcp "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/machineconfigpool"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	HubAPIClient      *testClient.ClientSet
	HubName           string
	SpokeAPIClient    *testClient.ClientSet
	SpokeName         string
	ArgocdApps        = map[string]ranztpparameters.ArgocdGitDetails{}
	ZtpVersion        string
	AcmVersion        string
	TalmVersion       string
	pidAndAffinityExp = regexp.MustCompile(`pid (\d+)'s current affinity list: (.*)$`)
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

// WaitForPolicyToExist is used to wait until the specified policy exists on the hub.
func WaitForPolicyToExist(policyName, namespace string, timeout time.Duration) error {
	err := wait.PollImmediate(
		ranztpparameters.ArgocdChangeInterval,
		timeout,
		func() (bool, error) {
			// If the policy doesn't exist yet then this will return an error
			_, err := rantalmhelper.GetPolicy(HubAPIClient, policyName, namespace)

			if err != nil {
				log.Println(err)

				if strings.Contains(err.Error(), "not found") {
					return false, nil
				}

				return true, err
			}

			// If err was nil then we are done
			return true, nil
		},
	)

	return err
}

// WaitForPolicyToHaveComplianceState is used to wait until the specified policy is in the specified compliance state.
func WaitForPolicyToHaveComplianceState(
	policyName string,
	namespace string,
	complianceState policiesv1.ComplianceState,
	timeout time.Duration) error {
	err := wait.PollImmediate(
		ranztpparameters.ArgocdChangeInterval,
		timeout,
		func() (bool, error) {
			// Get the policy
			policy, err := rantalmhelper.GetPolicy(HubAPIClient, policyName, namespace)
			if err != nil {
				return true, err
			}

			// Check its compliance state
			if policy.Status.ComplianceState == complianceState {
				return true, nil
			}

			// If it is not in the matching state then continue to wait
			return false, nil
		},
	)

	return err
}

// WaitForConfigPolicyMessageToContainSubstring is used to check a policy's most recent message
// and see if it contains a the provided substring.
func WaitForConfigPolicyMessageToContainSubstring(policyName, namespace, expectedMessage string) error {
	log.Printf("Checking policy '%s' in namespace '%s'\n", policyName, namespace)

	return wait.PollImmediate(
		ranztpparameters.ArgocdChangeInterval,
		ranztpparameters.ArgocdChangeTimeout,
		func() (bool, error) {
			message, err := GetLastPolicyMessage(policyName, namespace)

			fmt.Printf("Checking if actual message '%s' contains substring '%s'\n", message, expectedMessage)

			if err != nil {
				return false, err
			}

			return strings.Contains(message, expectedMessage), nil
		},
	)
}

// GetLastPolicyMessage is used to get the most recent message from a policy.
func GetLastPolicyMessage(policyName, namespace string) (string, error) {
	// Get the policy
	policy, err := rantalmhelper.GetPolicy(HubAPIClient, policyName, namespace)

	if err != nil {
		return "", err
	}

	if len(policy.Status.Details) > 0 {
		if len(policy.Status.Details[0].History) > 0 {
			return policy.Status.Details[0].History[0].Message, nil
		}
	}

	return "", nil
}

// JoinGitPaths is used to join any combination of git strings but also avoiding double slashes.
func JoinGitPaths(inputs []string) string {
	// We want to preserve any existing double slashes but we don't want to add any between the input elements
	// To work around this we will use a special join character and a couple replacements
	special := "<<join>>"

	// Combine the inputs with the special join character
	result := strings.Join(
		inputs,
		special,
	)

	// Replace any special joins that have a slash prefix
	result = strings.ReplaceAll(result, "/"+special, "/")

	// Replace any special joins that have a slash suffix
	result = strings.ReplaceAll(result, special+"/", "/")

	// Finally replace any remaining special joins
	result = strings.ReplaceAll(result, special, "/")

	// The final result should never have double slashes between the joined elements
	// However if they already had any double slashes, e.g. "http://", they will be preserved
	return result
}

// DoesGitPathExist checks if the specified git url exists.
func DoesGitPathExist(gitURL, gitBranch, gitPath string) bool {
	// Combine the separate pieces to get the url
	// We also need to remove the ".git" from the end of the url
	url := JoinGitPaths(
		[]string{
			strings.Replace(gitURL, ".git", "", 1),
			"raw",
			gitBranch,
			gitPath,
		},
	)

	// Log the url we are trying
	log.Printf("Checking if git url '%s' exists\n", url)

	// Create a custom http.Transport with insecure TLS configuration
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	// Create a custom http.Client using the insecure transport
	client := &http.Client{Transport: transport}

	// Make a request using the custom client
	resp, err := client.Get(url)

	// Check if we got a valid response
	if err == nil && resp.StatusCode == 200 {
		log.Printf("found valid git url for '%s'\n", gitPath)

		return true
	}

	// If we got here then none of the urls could be found.
	log.Printf("could not find valid url for '%s'\n", gitPath)

	return false
}

// GetArgocdInstance is used to fetch the Argocd gitops instance.
func GetArgocdInstance(name, namespace string) (argocdoperatorv1alpha1.ArgoCD, error) {
	// Create a typed namespace object
	typedNamespace := types.NamespacedName{}
	typedNamespace.Name = ranztpparameters.OpenshiftGitops
	typedNamespace.Namespace = ranztpparameters.OpenshiftGitops

	// Create a argocd object
	argocd := argocdoperatorv1alpha1.ArgoCD{}

	// Get the configuration from the hub
	err := HubAPIClient.Client.Get(GetZtpContext(), typedNamespace, &argocd)

	return argocd, err
}

// UpdateArgocdInstance is used to update the provided Argocd on the hub.
func UpdateArgocdInstance(argocd argocdoperatorv1alpha1.ArgoCD) error {
	err := HubAPIClient.Client.Update(GetZtpContext(), &argocd, &runtimeClient.UpdateOptions{})

	return err
}

// GetArgocdApp is used to fetch the Argocd application that is being used by Ztp.
func GetArgocdApp(appName, namespace string) (*argocdappv1alpha.Application, error) {
	argoApp, err := HubAPIClient.
		ArgoprojV1alpha1Interface.
		Applications(namespace).
		Get(GetZtpContext(), appName, metav1.GetOptions{})

	return argoApp, err
}

// SetGitDetailsInArgocd is used to update the git repo, branch, and path in the Argocd app.
func SetGitDetailsInArcgocd(gitRepo, gitBranch, gitPath, argocdApp string, waitForSync, syncMustBeValid bool) error {
	app, err := GetArgocdApp(argocdApp, ranztpparameters.OpenshiftGitops)
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

	log.Printf("Updating existing argocd app '%s'\n", argocdApp)
	log.Printf("Configuring RepoURL '%s'\n", gitRepo)
	log.Printf("Configuring TargetRevision '%s'\n", gitBranch)
	log.Printf("Configuring Path '%s'\n", gitPath)

	_, err = HubAPIClient.
		ArgoprojV1alpha1Interface.
		Applications(ranztpparameters.OpenshiftGitops).
		Update(GetZtpContext(), app, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	if waitForSync {
		err = WaitForArgocdChangeToComplete(argocdApp, syncMustBeValid, ranztpparameters.ArgocdChangeTimeout)
		if err != nil {
			return err
		}
	}

	return nil
}

// SetGitDetailsInArgocd is used to get the current git repo, branch, and path in the Argocd app.
func GetGitDetailsFromArgocd(appName, namespace string) (string, string, string, error) {
	app, err := GetArgocdApp(appName, namespace)
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
			arr := strings.Split(container.Image, ":")
			// Get the image tag which is the last element
			ztpVersion := arr[len(arr)-1]

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
func WaitForArgocdChangeToComplete(appName string, syncMustBeValid bool, timeout time.Duration) error {
	log.Println("Waiting for Argocd change to finish syncing")

	err := wait.PollImmediate(ranztpparameters.ArgocdChangeInterval, timeout, func() (bool, error) {
		log.Println("Checking if argo change is complete...")

		app, err := GetArgocdApp(appName, ranztpparameters.OpenshiftGitops)

		if err != nil {
			return false, err
		}

		if app != nil {
			// If there are any conditions then it probably means theres a problem
			// By printing them here we can make diagnosing a failing test easier
			for index, condition := range app.Status.Conditions {
				log.Printf("Condition #%d: '%s'\n", index, condition)
			}

			// The sync result may also have helpful information in the event of an error
			if app.Status.OperationState != nil && app.Status.OperationState.SyncResult != nil {
				for index, resource := range app.Status.OperationState.SyncResult.Resources {
					if resource != nil {
						log.Printf("Sync resource #%d: '%s\n", index, resource)
					}
				}
			}

			// Check if all the git details match
			if app.Status.Sync.ComparedTo.Source.RepoURL == app.Spec.Source.RepoURL &&
				app.Status.Sync.ComparedTo.Source.TargetRevision == app.Spec.Source.TargetRevision &&
				app.Status.Sync.ComparedTo.Source.Path == app.Spec.Source.Path {
				// If we expect the sync to be successful then we also need to check that the status is 'Synced'
				if syncMustBeValid {
					return app.Status.Sync.Status == "Synced", nil
				}

				return true, nil
			}
		}

		return false, nil
	})

	return err
}

// GetKlusterletConfiguration is used to get the klusterlet addon configuration for a specified cluster.
func GetKlusterletConfiguration(clusterName string, namespace string) (kacv1.KlusterletAddonConfig, error) {
	// Create a typed namespace object
	typedNamespace := types.NamespacedName{}
	typedNamespace.Name = clusterName
	typedNamespace.Namespace = namespace

	// Create a kac object
	kac := kacv1.KlusterletAddonConfig{}

	// Get the kac from the hub
	err := HubAPIClient.Client.Get(GetZtpContext(), typedNamespace, &kac)
	if err != nil {
		return kacv1.KlusterletAddonConfig{}, err
	}

	return kac, nil
}

// GetEvaluationIntervals is used to get the configured evaluation intervals for a specified policy.
func GetEvaluationIntervals(policyName string, namespace string) (string, string, error) {
	log.Printf("Checking policy '%s' in namespace '%s' to fetch evaluation intervals\n", policyName, namespace)

	// Get the policy from the hub
	policy, err := rantalmhelper.GetPolicy(HubAPIClient, policyName, namespace)
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

// CheckAffinitiesByProcessMatch checks CPU affinities for an array of processes against an specfied
// reserved CPU set in a given OCP node.
func CheckAffinitiesByProcessMatch(node *corev1.Node, processNames []string, reservedCPUSet cpuset.CPUSet) error {
	if node == nil {
		return fmt.Errorf("node was not specified")
	}

	if len(processNames) == 0 {
		return fmt.Errorf("processes names were undefined")
	}

	if len(strings.TrimSpace(reservedCPUSet.String())) == 0 {
		return fmt.Errorf("reservedCPUSet was undefined")
	}

	for _, processName := range processNames {
		if len(strings.TrimSpace(processName)) == 0 {
			return fmt.Errorf("processName was undefined")
		}

		cmd := fmt.Sprintf("pgrep %s | while read i; do taskset -cp $i; done", processName)
		output, err := helper.ExecCommandOnNode(node,
			[]string{"bash", "-c", cmd})

		if err != nil {
			return err
		}

		for _, line := range strings.Split(output, "\r\n") {
			line = strings.TrimSpace(line)

			// if process does not exist, return error
			if len(line) == 0 {
				return fmt.Errorf("process name: %s is not matched", processName)
			}

			match := pidAndAffinityExp.FindAllStringSubmatch(line, -1)
			if match == nil {
				return fmt.Errorf("unmatched pid and affinity for process name: %s", processName)
			}

			pid, affinity := match[0][1], strings.TrimSpace(match[0][2])

			pidCpuset := cpuset.MustParse(affinity)
			if !pidCpuset.IsSubsetOf(reservedCPUSet) {
				return fmt.Errorf("process: %s pid: %s with actual affinity: %s but expected: %s",
					processName, pid, affinity, reservedCPUSet.String())
			}
		}
	}

	return nil
}

type WaitForMcpUpdateFunc func(clientSet *testClient.ClientSet, machineConfigPoolName string) error

// CheckNodeIsFunctionalAfterMCchanges checks whether an OCP node is functional after a reboot
// caused by configuration (machine config) changes.Internally, it waits for three  conditions
// to  be successfully  evaluated in this order: specific  mcp transitioned  from updating  to
// updated, all mcps stable in that updated  state and finally, cluster nodes ready. The first
// condition is fully customizable and must be defined (non nil) with a callback.
func CheckNodeIsFunctionalAfterMCchanges(clientSet *testClient.ClientSet, mcpName string) error {
	if len(strings.TrimSpace(mcpName)) == 0 {
		return fmt.Errorf("machine config name is undefined")
	}

	// waits for mcp updating transition
	log.Printf("Waiting for mcp %s updating transition", mcpName)

	err := mcp.WaitForCondition(
		clientSet,
		&mcv1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Name: mcpName}},
		mcv1.MachineConfigPoolUpdating,
		10*time.Minute)

	if err != nil {
		return err
	}

	// check mcps are updated for an stable interval
	// sometimes it may take ~5mins for mcp to transit from updated to updating status
	interval := 5 * time.Second
	stableInterval := 5 * time.Minute

	log.Printf("Waiting for all mcps are updated for at least %s", stableInterval.String())
	err = mcp.WaitForClusterStable(helper.Apiclient, 60*time.Minute, interval, stableInterval)

	if err != nil {
		return err
	}

	// check all nodes are Ready
	err = nodes.WaitForNodesReady(helper.Apiclient, 10*time.Minute, interval)

	if err != nil {
		return err
	}

	return nil
}
