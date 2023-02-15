package ranztphelper

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	argocdoperatorv1alpha1 "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
	argocdappv1alpha "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	imageregistryv1 "github.com/openshift/api/imageregistry/v1"
	kacv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	"github.com/tidwall/gjson"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ztp/ranztpparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	HubAPIClient   *testClient.ClientSet
	HubName        string
	SpokeAPIClient *testClient.ClientSet
	SpokeName      string
	ArgocdApps     = map[string]ranztpparameters.ArgocdGitDetails{}
	ZtpVersion     string
	AcmVersion     string
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
		15*time.Second,
		timeout,
		func() (bool, error) {
			// If the policy doesn't exist yet then this will return an error
			_, err := GetPolicy(policyName, namespace)

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
		15*time.Second,
		timeout,
		func() (bool, error) {
			// Get the policy
			policy, err := GetPolicy(policyName, namespace)
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

// GetPolicy is used to get a policy from the hub.
func GetPolicy(policyName, namespace string) (policiesv1.Policy, error) {
	// Create a typed namespace object
	typedNamespace := types.NamespacedName{}
	typedNamespace.Name = policyName
	typedNamespace.Namespace = namespace

	// Create a policy object
	policy := policiesv1.Policy{}

	// Get the policy from the hub
	err := HubAPIClient.Client.Get(GetZtpContext(), typedNamespace, &policy)

	// Return the results
	return policy, err
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
		err = WaitForArgocdChangeToComplete(syncMustBeValid, ranztpparameters.ArgocdChangeTimeout)
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
func WaitForArgocdChangeToComplete(syncMustBeValid bool, timeout time.Duration) error {
	log.Println("Waiting for Argocd change to finish syncing")

	err := wait.PollImmediate(ranztpparameters.ArgocdChangeInterval, timeout, func() (bool, error) {
		app, err := GetArgocdApp(ranztpparameters.ArgocdPoliciesAppName, ranztpparameters.OpenshiftGitops)

		if err != nil {
			return false, err
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

		// If there are any conditions then it probably means theres a problem
		// By printing them here we can make diagnosing a failing test easier
		if len(app.Status.Conditions) > 0 {
			log.Printf("Current conditions: '%s'\n", app.Status.Conditions)
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
	policy, err := GetPolicy(policyName, namespace)
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

// CleanupImageRegistryConfig is used to cleanup all the configuration related to an image registry configuration.
func CleanupImageRegistryConfig(
	storageClassName, storageClassNamespace,
	persistentVolumeName,
	persistentVolumeClaimName, persistentVolumeClaimNamespace,
	registryConfig string, client *testClient.ClientSet) error {
	log.Println("Cleaning up image registry configuration")

	// Check if the client is defined
	if client == nil {
		return fmt.Errorf("provided nil client")
	}

	// We must delete the pieces in order or else we will get an error
	// 1. Registry configuration
	// 2. Persistent volume claim
	// 3. Persistent volume
	// 4. Storage class

	// Check if we were provided with a registry config
	if registryConfig != "" {
		// Delete the config if it exists
		err := DeleteAndWaitImageRegistryConfig(
			registryConfig,
			5*time.Minute,
			client,
		)
		if err != nil {
			return err
		}
	}

	// Check if we were given a persistent volume claim
	if persistentVolumeClaimName != "" {
		// Delete the config if it exists
		err := DeleteAndWaitPersistentVolumeClaim(
			persistentVolumeClaimName,
			persistentVolumeClaimNamespace,
			5*time.Minute,
			client,
		)
		if err != nil {
			return err
		}
	}

	// Check if we were given a persistent volume
	if persistentVolumeName != "" {
		// Delete the pv if it exists
		err := DeleteAndWaitPersistentVolume(
			persistentVolumeName,
			5*time.Minute,
			client,
		)
		if err != nil {
			return err
		}
	}

	// Check if we were given a storage class
	if storageClassName != "" {
		// Delete the sc if it exists
		err := DeleteAndWaitStorageClass(
			storageClassName,
			storageClassNamespace,
			5*time.Minute,
			client,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

//----------------------------
// ImageRegistryconfig helpers
//----------------------------

// GetImageRegistryConfig is used to get the specified image registry config.
func GetImageRegistryConfig(registryConfigName string, client *testClient.ClientSet) (*imageregistryv1.Config, error) {
	// Check if test client is defined
	if client == nil {
		return &imageregistryv1.Config{}, fmt.Errorf("provided nil client")
	}

	// Get the registry config from the client
	imageRegistryConfig, err := client.
		ImageregistryV1Interface.
		Configs().
		Get(
			GetZtpContext(),
			registryConfigName,
			metav1.GetOptions{},
		)

	// Regardless of whether an error occurred or not we want to return both of these
	return imageRegistryConfig, err
}

// DoesImageRegistryConfigExist is used to check whether a specified image registry config exists.
func DoesImageRegistryConfigExist(registryConfigName string, client *testClient.ClientSet) (bool, error) {
	// Check if test client is defined
	if client == nil {
		return false, fmt.Errorf("provided nil client")
	}

	// Get the image registry config
	imageRegistryConfig, err := GetImageRegistryConfig(registryConfigName, client)

	// If it wasn't found then specifically return nil here
	if err != nil && strings.Contains(err.Error(), "not found") {
		return false, nil
	}

	// If there was an error then the name would be empty on the returned config instead of matching
	return imageRegistryConfig.Name == registryConfigName, err
}

// DeleteAndWaitImageRegistryConfig is used to delete an image registry config and wait for it to be gone.
func DeleteAndWaitImageRegistryConfig(
	registryConfigName string,
	timeout time.Duration,
	client *testClient.ClientSet) error {
	// Check if test client is defined
	if client == nil {
		return fmt.Errorf("provided nil client")
	}

	// Check if the image registry exists
	exists, err := DoesImageRegistryConfigExist(registryConfigName, client)
	if err != nil {
		return err
	}

	if exists {
		// Delete the config
		log.Printf("Deleting image registry config '%s'\n", registryConfigName)

		err = client.
			ImageregistryV1Interface.
			Configs().
			Delete(
				GetZtpContext(),
				registryConfigName,
				metav1.DeleteOptions{},
			)
		if err != nil {
			return err
		}

		// Wait until the image registry is gone
		log.Printf("Waiting until image registry config '%s' is gone\n", registryConfigName)

		err = wait.PollImmediate(
			15*time.Second,
			5*time.Minute,
			func() (done bool, err error) {
				exists, err := DoesImageRegistryConfigExist(registryConfigName, client)
				if err != nil {
					return false, err
				}

				return !exists, nil
			},
		)

		// Whether or not this is nil we want to return here to avoid the additional logging below for the non existent case
		return err
	}

	log.Printf("Image registry config '%s' does not exist\n", registryConfigName)

	return nil
}

//----------------------------
// PersistentVolumeClaim helpers
//----------------------------

// GetPersistentVolumeClaim is used to get the specified persistent volume claim.
func GetPersistentVolumeClaim(
	persistentVolumeClaimName, persistentVolumeClaimNamespace string,
	client *testClient.ClientSet) (*corev1.PersistentVolumeClaim, error) {
	// Check if test client is defined
	if client == nil {
		return &corev1.PersistentVolumeClaim{}, fmt.Errorf("provided nil client")
	}

	// Check if the persistentVolumeClaim exists
	persistentVolumeClaim, err := client.PersistentVolumeClaims(persistentVolumeClaimNamespace).Get(
		GetZtpContext(),
		persistentVolumeClaimName,
		metav1.GetOptions{},
	)

	// Regardless of whether an error occurred or not we want to return both of these
	return persistentVolumeClaim, err
}

// DoesPersistentVolumeClaimExist is used to check whether a specified persistent volume claim exists.
func DoesPersistentVolumeClaimExist(
	persistentVolumeClaimName, persistentVolumeClaimNamespace string,
	client *testClient.ClientSet) (bool, error) {
	// Check if test client is defined
	if client == nil {
		return false, fmt.Errorf("provided nil client")
	}

	// Get the image registry config
	persistentVolumeClaim, err := GetPersistentVolumeClaim(
		persistentVolumeClaimName,
		persistentVolumeClaimNamespace,
		client,
	)

	// If it wasn't found then specifically return nil here
	if err != nil && strings.Contains(err.Error(), "not found") {
		return false, nil
	}

	// If there was an error then the name would be empty on the returned config instead of matching
	return persistentVolumeClaim.Name == persistentVolumeClaimName, err
}

// DeleteAndWaitPersistentVolume is used to delete a persistent volume and wait for it to be gone.
func DeleteAndWaitPersistentVolumeClaim(
	persistentVolumeClaimName, persistentVolumeClaimNamespace string,
	timeout time.Duration,
	client *testClient.ClientSet) error {
	// Check if test client is defined
	if client == nil {
		return fmt.Errorf("provided nil client")
	}

	// Check if the image registry exists
	exists, err := DoesPersistentVolumeClaimExist(persistentVolumeClaimName, persistentVolumeClaimNamespace, client)
	if err != nil {
		return err
	}

	if exists {
		// Delete the config
		log.Printf("Deleting persistent volume claim '%s'\n", persistentVolumeClaimName)

		err = client.PersistentVolumeClaims(persistentVolumeClaimNamespace).Delete(
			GetZtpContext(),
			persistentVolumeClaimName,
			metav1.DeleteOptions{},
		)
		if err != nil {
			return err
		}

		// Wait until the image registry is gone
		log.Printf("Waiting until persistent volume claim '%s' is gone\n", persistentVolumeClaimName)

		err = wait.PollImmediate(
			15*time.Second,
			5*time.Minute,
			func() (done bool, err error) {
				exists, err := DoesPersistentVolumeClaimExist(persistentVolumeClaimName, persistentVolumeClaimNamespace, client)
				if err != nil {
					return false, err
				}

				return !exists, nil
			},
		)

		// Whether or not this is nil we want to return here to avoid the additional logging below for the non existent case
		return err
	}

	log.Printf("Persistent volume claim '%s' does not exist\n", persistentVolumeClaimName)

	return nil
}

//----------------------------
// PersistentVolume helpers
//----------------------------

// GetPersistentVolume is used to get the specified persistent volume.
func GetPersistentVolume(persistentVolumeName string, client *testClient.ClientSet) (*corev1.PersistentVolume, error) {
	// Check if test client is defined
	if client == nil {
		return &corev1.PersistentVolume{}, fmt.Errorf("provided nil client")
	}

	// Check if the pvc exists
	persistentVolume, err := client.PersistentVolumes().Get(
		GetZtpContext(),
		persistentVolumeName,
		metav1.GetOptions{},
	)

	// Regardless of whether an error occurred or not we want to return both of these
	return persistentVolume, err
}

// DoesPersistentVolumeExist is used to check whether a specified persistent volume exists.
func DoesPersistentVolumeExist(persistentVolumeName string, client *testClient.ClientSet) (bool, error) {
	// Check if test client is defined
	if client == nil {
		return false, fmt.Errorf("provided nil client")
	}

	// Get the image registry config
	persistentVolume, err := GetPersistentVolume(persistentVolumeName, client)

	// If it wasn't found then specifically return nil here
	if err != nil && strings.Contains(err.Error(), "not found") {
		return false, nil
	}

	// If there was an error then the name would be empty on the returned config instead of matching
	return persistentVolume.Name == persistentVolumeName, err
}

// DeleteAndWaitPersistentVolume is used to delete a persistent volume and wait for it to be gone.
func DeleteAndWaitPersistentVolume(
	persistentVolumeName string,
	timeout time.Duration,
	client *testClient.ClientSet) error {
	// Check if test client is defined
	if client == nil {
		return fmt.Errorf("provided nil client")
	}

	// Check if the image registry exists
	exists, err := DoesPersistentVolumeExist(persistentVolumeName, client)
	if err != nil {
		return err
	}

	if exists {
		// Delete the config
		log.Printf("Deleting persistent volume '%s'\n", persistentVolumeName)

		err = client.PersistentVolumes().Delete(
			GetZtpContext(),
			persistentVolumeName,
			metav1.DeleteOptions{},
		)
		if err != nil {
			return err
		}

		// Wait until the image registry is gone
		log.Printf("Waiting until persistent volume '%s' is gone\n", persistentVolumeName)

		err = wait.PollImmediate(
			15*time.Second,
			5*time.Minute,
			func() (done bool, err error) {
				exists, err := DoesPersistentVolumeExist(persistentVolumeName, client)
				if err != nil {
					return false, err
				}

				return !exists, nil
			},
		)

		// Whether or not this is nil we want to return here to avoid the additional logging below for the non existent case
		return err
	}

	log.Printf("Persistent volume '%s' does not exist\n", persistentVolumeName)

	return nil
}

//----------------------------
// StorageClass helpers
//----------------------------

// GetStorageClass is used to get the specified storage class.
func GetStorageClass(
	storageClassName, storageClassNamespace string,
	client *testClient.ClientSet) (storagev1.StorageClass, error) {
	// Check if test client is defined
	if client == nil {
		return storagev1.StorageClass{}, fmt.Errorf("provided nil client")
	}

	// We need to use a typed namespace to get a storage class
	scType := types.NamespacedName{}
	scType.Name = storageClassName
	scType.Namespace = storageClassNamespace

	storageClass := storagev1.StorageClass{}
	err := client.Get(GetZtpContext(), scType, &storageClass)

	// Regardless of whether an error occurred or not we want to return both of these
	return storageClass, err
}

// DoesStorageClassExist is used to check whether a specified storage class exists.
func DoesStorageClassExist(storageClassName, storageClassNamespace string, client *testClient.ClientSet) (bool, error) {
	// Check if test client is defined
	if client == nil {
		return false, fmt.Errorf("provided nil client")
	}

	// Get the image registry config
	storageClass, err := GetStorageClass(storageClassName, storageClassNamespace, client)

	// If it wasn't found then specifically return nil here
	if err != nil && strings.Contains(err.Error(), "not found") {
		return false, nil
	}

	// If there was an error then the name would be empty on the returned config instead of matching
	return storageClass.Name == storageClassName, err
}

// DeleteAndWaitStorageClass is used to delete a persistent volume and wait for it to be gone.
func DeleteAndWaitStorageClass(
	storageClassName, storageClassNamespace string,
	timeout time.Duration,
	client *testClient.ClientSet) error {
	// Check if test client is defined
	if client == nil {
		return fmt.Errorf("provided nil client")
	}

	// Check if the image registry exists
	exists, err := DoesStorageClassExist(storageClassName, storageClassNamespace, client)
	if err != nil {
		return err
	}

	if exists {
		// Delete the config
		log.Printf("Deleting storage class '%s'\n", storageClassName)

		// Get the storage class
		storageClass, err := GetStorageClass(storageClassName, storageClassNamespace, client)
		if err != nil {
			return err
		}

		// Delete the storage class
		err = client.Delete(GetZtpContext(), &storageClass, &runtimeClient.DeleteOptions{})
		if err != nil {
			return err
		}

		// Wait until the image registry is gone
		log.Printf("Waiting until storage class '%s' is gone\n", storageClassName)

		err = wait.PollImmediate(
			15*time.Second,
			5*time.Minute,
			func() (done bool, err error) {
				exists, err := DoesStorageClassExist(storageClassName, storageClassNamespace, client)
				if err != nil {
					return false, err
				}

				return !exists, nil
			},
		)

		// Whether or not this is nil we want to return here to avoid the additional logging below for the non existent case
		return err
	}

	log.Printf("Storage class '%s' does not exist\n", storageClassName)

	return nil
}
