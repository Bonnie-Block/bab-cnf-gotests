package rantalmhelper

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/openshift-kni/cluster-group-upgrades-operator/api/v1alpha1"
	configv1 "github.com/openshift/api/config/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	configurationPolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	policiesv1beta1 "open-cluster-management.io/governance-policy-propagator/api/v1beta1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	HubAPIClient    *testClient.ClientSet
	HubName         string
	Spoke1APIClient *testClient.ClientSet
	Spoke1Name      string
	Spoke2APIClient *testClient.ClientSet
	Spoke2Name      string
	TalmHubVersion  string
)

const (
	CguName                         string = "talm-cgu"
	Namespace                       string = "talm-namespace"
	PlacementBindingName            string = "talm-placement-binding"
	PlacementRule                   string = "talm-placement-rule"
	PolicyName                      string = "talm-policy"
	PolicySetName                   string = "talm-policyset"
	CatalogSourceName               string = "talm-catsrc"
	Talm411TimeoutMessage           string = "The ClusterGroupUpgrade CR policies are taking too long to complete"
	Talm412TimeoutMessage           string = "Policy remediation took too long"
	TemporaryNamespaceName          string = Namespace + "-temp"
	ProgressingType                 string = "Progressing"
	ReadyType                       string = "Ready"
	SucceededType                   string = "Succeeded"
	ValidatedType                   string = "Validated"
	ConditionReasonCompleted        string = "Completed"
	ConditionReasonUpgradeCompleted string = "UpgradeCompleted"
)

// GetTestContext fetches a k8s context object for the talm tests.
func GetTestContext() context.Context {
	// Not really sure whether this should be background or todo but this seems to be working fine
	return context.Background()
}

// BoolAddr is used to convert a boolean to a boolean pointer.
func BoolAddr(b bool) *bool {
	boolVar := b

	return &boolVar
}

// GetAllTestClients is used to quickly obtain a list of all the test clients.
func GetAllTestClients() []*testClient.ClientSet {
	return []*testClient.ClientSet{
		HubAPIClient,
		Spoke1APIClient,
		Spoke2APIClient,
	}
}

// GetNamespaceDefinition gets a namespace object with the provided name.
func GetNamespaceDefinition(namespaceName string) *corev1.Namespace {
	customResource := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: corev1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}

	return customResource
}

// CreateSimplePolicyAndCgu is used to create a simplified CGU to cover the most common use case.
// This will automatically create a single policy enforcing the compliance type on the provided object.
// If you require multiple policies to be managed then consider CreateCgu() instead.
func CreatePolicyAndCgu(
	client *testClient.ClientSet,
	object runtime.Object,
	complianceType configurationPolicyv1.ComplianceType,
	remediationAction configurationPolicyv1.RemediationAction,
	policyName string,
	policySetName string,
	placementBindingName string,
	placementRule string,
	namespace string,
	clusterSelector metav1.LabelSelector,
	cgu v1alpha1.ClusterGroupUpgrade) error {
	// Step 1 - Create simple policy with all required components
	err := CreatePolicyWithAllComponents(
		client,
		object,
		complianceType,
		remediationAction,
		policyName,
		policySetName,
		placementBindingName,
		placementRule,
		namespace,
		cgu.Spec.Clusters,
		clusterSelector,
	)
	if err != nil {
		return err
	}

	// Step 2 - Create the cgu
	err = CreateCguAndWait(
		client,
		cgu,
	)
	if err != nil {
		return err
	}

	return nil
}

/*
	CGU Helpers
*/

// GetCguDefinition is used to get a CGU with simplified parameters.
func GetCguDefinition(
	cguName string,
	clusterList []string,
	canaryList []string,
	managedPolicies []string,
	namespace string,
	maxConcurrency int,
	timeout int) v1alpha1.ClusterGroupUpgrade {
	customResource := v1alpha1.ClusterGroupUpgrade{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterGroupUpgrade",
			APIVersion: v1alpha1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cguName,
			Namespace: namespace,
		},
		Spec: v1alpha1.ClusterGroupUpgradeSpec{
			Backup:                false,
			PreCaching:            false,
			Enable:                BoolAddr(true),
			Clusters:              clusterList,
			ClusterLabelSelectors: nil,
			ManagedPolicies:       managedPolicies,
			RemediationStrategy: &v1alpha1.RemediationStrategySpec{
				MaxConcurrency: maxConcurrency,
				Timeout:        timeout,
				Canaries:       canaryList,
			},
		},
	}

	return customResource
}

// GetCgu is used to get the specified Cgu object from the cluster.
func GetCgu(client *testClient.ClientSet, cguName string, namespace string) (v1alpha1.ClusterGroupUpgrade, error) {
	// Validate inputs first
	if cguName == "" {
		return v1alpha1.ClusterGroupUpgrade{}, errors.New("provided empty cguName")
	}

	if namespace == "" {
		return v1alpha1.ClusterGroupUpgrade{}, errors.New("provided empty namespace")
	}

	if client == nil {
		return v1alpha1.ClusterGroupUpgrade{}, errors.New("provided nil client")
	}

	// TALM only runs on the hub so consider non hub API clients to be not existing
	cgu, err := client.ClustergroupupgradesoperatorV1alpha1Interface.
		ClusterGroupUpgrades(namespace).
		Get(GetTestContext(), cguName, metav1.GetOptions{})

	// Filter errors that don't matter
	err = FilterMissingResourceErrors(err)
	if err != nil {
		return v1alpha1.ClusterGroupUpgrade{}, err
	}

	// Check if it matched
	if cgu.Name == cguName {
		return *cgu, nil
	}

	return v1alpha1.ClusterGroupUpgrade{}, errors.New("resource not found")
}

// IsCguExist can be used to check if a specific cgu exists.
func IsCguExist(client *testClient.ClientSet, cguName string, namespace string) (bool, error) {
	log.Printf("Checking for existence of cgu '%s' in namespace '%s'\n", cguName, namespace)

	_, err := GetCgu(client, cguName, namespace)

	// Filter errors that don't matter
	filtered := FilterMissingResourceErrors(err)

	// If err was defined but filtered is nil, then the resource no longer exists
	if err != nil && filtered == nil {
		return false, nil
	}

	// If filtered it not nil then an actual error occurred
	if filtered != nil {
		return false, filtered
	}

	// Else assume it exists
	return true, nil
}

// DeleteCguAndWait can be used to delete a CGU if it exists.
func DeleteCguAndWait(client *testClient.ClientSet, cguName string, namespace string) error {
	// Check if it exists first
	exists, err := IsCguExist(client, cguName, namespace)
	if err != nil {
		return err
	}

	// If it exists then attempt to delete it
	if exists {
		// Delete it
		err = client.ClustergroupupgradesoperatorV1alpha1Interface.
			ClusterGroupUpgrades(namespace).
			Delete(GetTestContext(), cguName, metav1.DeleteOptions{})
		if err != nil {
			return err
		}

		// Wait until its gone
		err = WaitUntilObjectDoesNotExist(
			client,
			cguName,
			namespace,
			IsCguExist,
		)
		if err != nil {
			return err
		}
	}

	return err
}

// CreateCguAndWait is used to create a CGU with the provided clusters list and managed policies.
// No policies, placements, bindings, policysets, etc will be created.
func CreateCguAndWait(
	client *testClient.ClientSet,
	cgu v1alpha1.ClusterGroupUpgrade) error {
	if client == nil {
		return errors.New("provided nil client")
	}

	if len(cgu.Spec.Clusters) > 0 {
		for _, cluster := range cgu.Spec.Clusters {
			if cluster == "" {
				return errors.New("provided empty cluster in clustersList")
			}
		}
	}

	if len(cgu.Spec.ManagedPolicies) > 0 {
		for _, policy := range cgu.Spec.ManagedPolicies {
			if policy == "" {
				return errors.New("provided empty policy in managedPolicies")
			}
			// If the fully generated name of the talm enforce policy is > 63 characters then they will just not work.
			// There is some wiggle room here since there is an additional identifier on the end of the policy.
			// So instead of hard erroring just print a warning if the length is possibly an issue.
			if len(policy)+len(cgu.Name) > 50 {
				log.Println("Warning: Length of generated TALM policies may exceed character limit and not work")
			}
		}
	}

	if cgu.Name == "" {
		return errors.New("provided empty cguName")
	}

	if err := ranhelper.PrintCr(cgu); err != nil {
		log.Println("error printing cr: ", err)
	}

	_, err := client.ClusterGroupUpgrades(cgu.Namespace).
		Create(GetTestContext(), &cgu, metav1.CreateOptions{})

	if err != nil {
		return err
	}

	err = WaitUntilObjectExists(
		client,
		cgu.Name,
		cgu.Namespace,
		IsCguExist,
	)

	return err
}

// WaitForCguConditionToMatchExpectedMessage waits until a specified condition type
// matches the provided message and/or status.
func WaitForCguInCondition(
	client *testClient.ClientSet,
	cguName string,
	namespace string,
	conditionType string,
	expectedMessage string,
	expectedStatus metav1.ConditionStatus,
	expectedReason string,
	timeout time.Duration) error {
	// This will be used inside the wait to keep track of the current status
	// Since we can't return an error outside the poll we need to save it outside the loop
	// and then return it after timeout occurs.
	var lastStatus error

	msg := fmt.Sprintf("Waiting for cgu %s in namespace %s to be in condition: %s=%s",
		cguName, namespace, conditionType, expectedStatus)
	if expectedReason != "" {
		msg = msg + ", with reason: " + expectedReason
	}

	log.Println(msg)
	// Use a poll to check the cgu condition
	_ = wait.PollImmediate(
		rantalmparameters.TalmTestPollInterval,
		timeout,
		func() (done bool, err error) {
			// Get the CGU
			clusterGroupUpgrade, err := client.
				ClusterGroupUpgrades(namespace).
				Get(GetTestContext(), cguName, metav1.GetOptions{})
			if err != nil {
				lastStatus = err

				return false, err
			}

			// Get the condition
			condition := meta.FindStatusCondition(clusterGroupUpgrade.Status.Conditions, conditionType)

			// If the condition does not exist that is not considered a match
			if condition == nil {
				lastStatus = fmt.Errorf("condition for type '%s' was nil", conditionType)

				return false, nil
			}

			// Check the status if it was defined
			if expectedStatus != "" {
				if condition.Status != expectedStatus {
					lastStatus = fmt.Errorf(
						"actual status '%s' did not match expected status '%s'",
						condition.Status,
						expectedStatus,
					)

					return false, nil
				}
			}

			// Check the message if it was defined
			if expectedMessage != "" {
				if !strings.Contains(condition.Message, expectedMessage) {
					lastStatus = fmt.Errorf(
						"actual message '%s' did not contain expected message '%s'",
						condition.Message,
						expectedMessage,
					)

					return false, nil
				}
			}

			// Check the reason if it was defined
			if expectedReason != "" {
				if condition.Reason != expectedReason {
					lastStatus = fmt.Errorf(
						"actual reason '%s' did not match expected reason '%s'",
						condition.Reason,
						expectedReason,
					)

					return false, nil
				}
			}

			// If it did match we will return nil here to exit the eventually
			lastStatus = nil

			return true, nil
		},
	)

	return lastStatus
}

// WaitForCguToStartProgressing waits until the provided CGU reaches the progressings tate
// and the remediating non-compliant policies message.
func WaitForCguToStartProgressing(cguName string, namespace string, timeout time.Duration) error {
	// Wait for the cgu to start
	log.Println("waiting for CGU to start progressing")

	// TALM uses different conditions starting in 4.12
	conditionType := ProgressingType
	conditionMessage := "Remediating non-compliant policies"
	conditionReason := "InProgress"

	if !ranhelper.IsVersionStringInRange(
		TalmHubVersion,
		rantalmparameters.TalmUpdatedConditionsVersion,
		"",
	) {
		conditionType = ReadyType
		conditionMessage = "The ClusterGroupUpgrade CR has upgrade policies that are still non compliant"
		conditionReason = "UpgradeNotCompleted"
	}

	return WaitForCguInCondition(
		HubAPIClient,
		cguName,
		namespace,
		conditionType,
		conditionMessage,
		metav1.ConditionTrue,
		conditionReason,
		timeout,
	)
}

// WaitForCguToFinishSuccessfully waits until the provided CGU reaches the succeeded state
// and all clusters were successful message.
func WaitForCguToFinishSuccessfully(cguName string, namespace string, timeout time.Duration) error {
	// Wait for the cgu to finish
	log.Println("waiting for CGU to finish successfully")

	// TALM uses different conditions starting in 4.12
	conditionType := SucceededType
	conditionReason := ConditionReasonCompleted

	if !ranhelper.IsVersionStringInRange(
		TalmHubVersion,
		rantalmparameters.TalmUpdatedConditionsVersion,
		"",
	) {
		conditionType = ReadyType
		conditionReason = ConditionReasonUpgradeCompleted
	}

	return WaitForCguInCondition(
		HubAPIClient,
		cguName,
		namespace,
		conditionType,
		"",
		metav1.ConditionTrue,
		conditionReason,
		timeout,
	)
}

// WaitForCguToFinishSuccessfully waits until the provided CGU reaches the succeeded state
// and all clusters were successful message.
func WaitForCguToTimeout(cguName string, namespace string, timeout time.Duration) error {
	// Wait for the cgu to timeout
	log.Println("waiting for CGU to timeout")

	// TALM uses different conditions starting in 4.12
	conditionType := SucceededType
	conditionReason := "TimedOut"

	if !ranhelper.IsVersionStringInRange(
		TalmHubVersion,
		rantalmparameters.TalmUpdatedConditionsVersion,
		"",
	) {
		conditionType = ReadyType
		conditionReason = "UpgradeTimedOut"
	}

	return WaitForCguInCondition(
		HubAPIClient,
		cguName,
		namespace,
		conditionType,
		"",
		"",
		conditionReason,
		timeout,
	)
}

/*
	Policy helpers
*/

// GetConfigurationPolicyDefinition is used to get a configuration policy that contains the provided object.
func GetConfigurationPolicyDefinition(
	policyName string,
	complianceType configurationPolicyv1.ComplianceType,
	remediationAction configurationPolicyv1.RemediationAction,
	object runtime.Object) configurationPolicyv1.ConfigurationPolicy {
	customResource := configurationPolicyv1.ConfigurationPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigurationPolicy",
			APIVersion: "policy.open-cluster-management.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-config", policyName),
		},
		Spec: configurationPolicyv1.ConfigurationPolicySpec{
			Severity:          "low",
			RemediationAction: remediationAction,
			NamespaceSelector: configurationPolicyv1.Target{
				Include: []configurationPolicyv1.NonEmptyString{"kube-*"},
				Exclude: []configurationPolicyv1.NonEmptyString{"*"},
			},
			ObjectTemplates: []*configurationPolicyv1.ObjectTemplate{
				{
					ComplianceType: complianceType,
					ObjectDefinition: runtime.RawExtension{
						Object: object,
					},
				},
			},
			EvaluationInterval: configurationPolicyv1.EvaluationInterval{
				Compliant:    "10s",
				NonCompliant: "10s",
			},
		},
	}

	return customResource
}

// GetPolicyDefinition is used to get a policy that can be used with a CGU.
func GetPolicyDefinition(
	policyName string,
	namespace string,
	object runtime.Object,
	remediationAction configurationPolicyv1.RemediationAction) policiesv1.Policy {
	customResource := policiesv1.Policy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Policy",
			APIVersion: policiesv1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: namespace,
		},
		Spec: policiesv1.PolicySpec{
			Disabled: false,
			PolicyTemplates: []*policiesv1.PolicyTemplate{
				{
					ObjectDefinition: runtime.RawExtension{
						Object: object,
					},
				},
			},
			RemediationAction: policiesv1.RemediationAction(remediationAction),
		},
	}

	return customResource
}

// GetPolicy is used to get the specified policy object from the cluster.
func GetPolicy(client *testClient.ClientSet, policyName string, namespace string) (policiesv1.Policy, error) {
	// Validate inputs first
	if policyName == "" {
		return policiesv1.Policy{}, errors.New("provided empty policyName")
	}

	if namespace == "" {
		return policiesv1.Policy{}, errors.New("provided empty namespace")
	}

	var policyList policiesv1.PolicyList

	// Get a list of policies from the cluster
	err := client.Client.List(
		GetTestContext(),
		&policyList,
		&runtimeclient.ListOptions{
			Namespace: namespace,
		})

	// Filter errors that don't matter
	err = FilterMissingResourceErrors(err)
	if err != nil {
		return policiesv1.Policy{}, err
	}

	// Check the returned policy list for the specific policy
	for _, policy := range policyList.Items {
		if policy.Name == policyName {
			return policy, nil
		}
	}

	return policiesv1.Policy{}, errors.New("resource not found")
}

// GetPolicyNameWithPrefix finds a policy starting with policyPrefix and returns
// the name of first policy that matches or returns a blank string if not found.
func GetPolicyNameWithPrefix(
	client *testClient.ClientSet,
	policyPrefix string,
	namespace string) (string, error) {
	var policyList policiesv1.PolicyList

	generatedPolicyName := ""

	// Get a list of policies from the cluster
	err := client.Client.List(
		GetTestContext(),
		&policyList,
		&runtimeclient.ListOptions{
			Namespace: namespace,
		})

	// Filter errors that don't matter
	err = FilterMissingResourceErrors(err)
	if err != nil {
		return generatedPolicyName, err
	}

	// Check the returned policy list for the specific policy
	for _, policy := range policyList.Items {
		if strings.HasPrefix(policy.Name, policyPrefix) {
			generatedPolicyName = policy.Name

			return generatedPolicyName, nil
		}
	}

	return generatedPolicyName, nil
}

// IsPolicyExist can be used to check if a specific policy exists.
func IsPolicyExist(client *testClient.ClientSet, policyName string, namespace string) (bool, error) {
	log.Printf("Checking for existence of policy '%s' in namespace '%s'\n", policyName, namespace)

	// We can use another helper to get the object
	policy, err := GetPolicy(client, policyName, namespace)

	// Filter any missing resource errors before checking the result
	err = FilterMissingResourceErrors(err)
	if err != nil {
		return false, err
	}

	return policyName == policy.Name, nil
}

// DeletePolicyAndWait can be used to delete a policy if it exists.
func DeletePolicyAndWait(client *testClient.ClientSet, policyName string, namespace string) error {
	// Check if it exists first
	exists, err := IsPolicyExist(client, policyName, namespace)
	if err != nil {
		return err
	}

	// If it exists then attempt to delete it
	if exists {
		// Get the specific object to be deleted
		policy, err := GetPolicy(client, policyName, namespace)
		if err != nil {
			return err
		}

		// Delete the object
		err = client.Client.Delete(
			GetTestContext(),
			&policy,
			&runtimeclient.DeleteOptions{},
		)
		if err != nil {
			return err
		}

		// Wait until its gone
		err = WaitUntilObjectDoesNotExist(
			client,
			policyName,
			namespace,
			IsPolicyExist,
		)
		if err != nil {
			return err
		}
	}

	return err
}

// CreatePolicyAndWait is used to create a policy and wait for it to exist.
func CreatePolicyAndWait(
	client *testClient.ClientSet,
	policy policiesv1.Policy) error {
	// Create the policy
	_ = ranhelper.PrintCr(policy)

	err := client.Client.Create(GetTestContext(), &policy)
	if err != nil {
		return err
	}

	// Wait for it to exist
	err = WaitUntilObjectExists(
		client,
		policy.Name,
		policy.Namespace,
		IsPolicyExist,
	)

	return err
}

// AllPoliciesExist checks if polices, named in config, is already deployed.
func AllPoliciesExist(listPolicy policiesv1.PolicyList) (policiesv1.PolicyList, bool) {
	var (
		count     int
		tempPlist policiesv1.PolicyList
	)

	for _, curPolicy := range helper.Config.Ran.TalmPrecachePolicies {
		for _, deployedPolicy := range listPolicy.Items {
			if curPolicy == deployedPolicy.Name {
				tempPlist.Items = append(tempPlist.Items, deployedPolicy)
				count++

				log.Printf("policy found:'%s' in ns:'%s' createTS:'%s'",
					curPolicy, deployedPolicy.Namespace, deployedPolicy.CreationTimestamp)
			}
		}
	}

	return tempPlist, count == len(helper.Config.Ran.TalmPrecachePolicies)
}

// CreatePolicyWithAllComponents is used to create a policy and all the requireed components for
// applying that policy such as policyset, placementrule, placement binding, etc.
func CreatePolicyWithAllComponents(
	client *testClient.ClientSet,
	object runtime.Object,
	complianceType configurationPolicyv1.ComplianceType,
	remediationAction configurationPolicyv1.RemediationAction,
	policyName string,
	policySetName string,
	placementBindingName string,
	placementRule string,
	namespace string,
	clusters []string,
	clusterSelector metav1.LabelSelector,
) error {
	// Step 0 - Validate inputs
	if client == nil {
		return errors.New("provided nil client")
	}

	if object == nil {
		return errors.New("provided nil object")
	}

	if policyName == "" {
		return errors.New("provided empty policyName")
	}

	if policySetName == "" {
		return errors.New("provided empty policySetName")
	}

	if placementBindingName == "" {
		return errors.New("provided empty placementBindingName")
	}

	if placementRule == "" {
		return errors.New("provided empty placementRule")
	}

	if namespace == "" {
		return errors.New("provided empty namespace")
	}

	// Step 1 - Create the policy
	log.Println("create the policy that the cgu will apply")

	configurationPolicy := GetConfigurationPolicyDefinition(policyName, complianceType, remediationAction, object)

	cguPolicy := GetPolicyDefinition(policyName, namespace, &configurationPolicy, remediationAction)

	return ApplyPolicyAndCreateAllComponents(client,
		cguPolicy,
		policySetName,
		placementBindingName,
		placementRule,
		namespace,
		clusters,
		clusterSelector)
}

// ApplyPolicyAndCreateAllComponents is used only apply already created policy and all the required components for
// applying that policy such as policyset, placementrule, placement binding, etc.
func ApplyPolicyAndCreateAllComponents(
	client *testClient.ClientSet,
	policy policiesv1.Policy,
	policySetName string,
	placementBindingName string,
	placementRule string,
	namespace string,
	clusters []string,
	clusterSelector metav1.LabelSelector,
) error {
	err := CreatePolicyAndWait(client, policy)

	if err != nil {
		return err
	}

	// Step 2 - Create the policy set
	log.Println("creating the policyset")

	var nonEmptyStringList []policiesv1beta1.NonEmptyString
	nonEmptyStringList = append(nonEmptyStringList, policiesv1beta1.NonEmptyString(policy.Name))

	policySet := GetPolicySetDefinition(policySetName, nonEmptyStringList, namespace)

	err = CreatePolicySetAndWait(client, policySet)

	if err != nil {
		return err
	}

	// Step 3 - Get a placement field
	log.Println("creating the generic placement fields")

	fields := GetPlacementFieldDefinition(clusters, clusterSelector)

	// Step 4 - Create the placement rule
	log.Println("creating the placementrule")

	placement := GetPlacementRuleDefinition(placementRule, namespace, fields)

	err = CreatePlacementRuleAndWait(client, placement)

	if err != nil {
		return err
	}

	// Step 5 - Create the placement binding
	log.Println("creating the placementbinding")

	placementBinding := GetPlacementBindingDefinition(
		placementBindingName,
		policySetName,
		placementRule,
		namespace,
	)

	err = CreatePlacementBindingAndWait(client, placementBinding)

	if err != nil {
		return err
	}

	return nil
}

/*
	Placement Binding helpers
*/

// GetPlacementBindingDefinition is used to get a placement binding to use with a cgu.
func GetPlacementBindingDefinition(
	placementBindingName string,
	policySetName string,
	placementRuleName string,
	namespace string) policiesv1.PlacementBinding {
	customResource := policiesv1.PlacementBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PlacementBinding",
			APIVersion: policiesv1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      placementBindingName,
			Namespace: namespace,
		},
		PlacementRef: policiesv1.PlacementSubject{
			Name:     placementRuleName,
			APIGroup: "apps.open-cluster-management.io",
			Kind:     "PlacementRule",
		},
		Subjects: []policiesv1.Subject{
			{
				Name:     policySetName,
				APIGroup: "policy.open-cluster-management.io",
				Kind:     "PolicySet",
			},
		},
	}

	return customResource
}

// GetPlacementBinding can be used to get a specific placement binding object from the cluster.
func GetPlacementBinding(
	client *testClient.ClientSet,
	placementBindingName string,
	namespace string) (policiesv1.PlacementBinding, error) {
	// Validate inputs first
	if placementBindingName == "" {
		return policiesv1.PlacementBinding{}, errors.New("provided empty placementBindingName")
	}

	if namespace == "" {
		return policiesv1.PlacementBinding{}, errors.New("provided empty namespace")
	}

	var placementBindingList policiesv1.PlacementBindingList

	// Get a list of placement bindings from the cluster
	err := client.Client.List(
		GetTestContext(),
		&placementBindingList,
		&runtimeclient.ListOptions{
			Namespace: namespace,
		},
	)

	// Filter errors that don't matter
	err = FilterMissingResourceErrors(err)
	if err != nil {
		return policiesv1.PlacementBinding{}, err
	}

	// Check the returned placement binding list for the specific binding
	for _, placementBinding := range placementBindingList.Items {
		if placementBinding.Name == placementBindingName {
			return placementBinding, nil
		}
	}

	return policiesv1.PlacementBinding{}, errors.New("resource not found")
}

// IsPlacementBindingExist can be used to check if a specific placement binding exists.
func IsPlacementBindingExist(
	client *testClient.ClientSet,
	placementBindingName string,
	namespace string) (bool, error) {
	log.Printf("Checking for existence of placement binding '%s' in namespace '%s'\n", placementBindingName, namespace)

	// We can use another helper to get the object
	placementBinding, err := GetPlacementBinding(client, placementBindingName, namespace)

	// Filter any missing resource errors before checking the result
	err = FilterMissingResourceErrors(err)
	if err != nil {
		return false, err
	}

	return placementBinding.Name == placementBindingName, nil
}

// DeletePlacementBindingAndWait can be used to delete a placement binding if it exists.
func DeletePlacementBindingAndWait(client *testClient.ClientSet, placementBindingName string, namespace string) error {
	// Check if it exists first
	exists, err := IsPlacementBindingExist(client, placementBindingName, namespace)
	if err != nil {
		return err
	}

	// If it exists then attempt to delete it
	if exists {
		// Get the specific object to be deleted
		placementBinding, err := GetPlacementBinding(client, placementBindingName, namespace)
		if err != nil {
			return err
		}

		// Delete the object
		err = client.Client.Delete(
			GetTestContext(),
			&placementBinding,
			&runtimeclient.DeleteOptions{},
		)
		if err != nil {
			return err
		}

		// Wait until its gone
		err = WaitUntilObjectDoesNotExist(
			client,
			placementBindingName,
			namespace,
			IsPlacementBindingExist,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// CreatePlacementBindingAndWait is used to create a placement binding and wait for it to exist.
func CreatePlacementBindingAndWait(
	client *testClient.ClientSet,
	placementBinding policiesv1.PlacementBinding) error {
	// Create the policy
	_ = ranhelper.PrintCr(placementBinding)

	err := client.Client.Create(GetTestContext(), &placementBinding)
	if err != nil {
		return err
	}

	// Wait for it to exist
	err = WaitUntilObjectExists(
		client,
		placementBinding.Name,
		placementBinding.Namespace,
		IsPlacementBindingExist,
	)

	return err
}

/*
	Placement Rule helpers
*/

// GetPlacementFieldDefinition is used to get a generic placement field for use with a placement rule.
func GetPlacementFieldDefinition(
	clusters []string,
	clusterSelector metav1.LabelSelector) placementrulev1.GenericPlacementFields {
	// Build the placement object we need in lieu of a flat string list
	clustersPlacementField := []placementrulev1.GenericClusterReference{}
	for _, cluster := range clusters {
		clustersPlacementField = append(clustersPlacementField, placementrulev1.GenericClusterReference{Name: cluster})
	}

	customResource := placementrulev1.GenericPlacementFields{
		Clusters:        clustersPlacementField,
		ClusterSelector: &clusterSelector,
	}

	return customResource
}

// GetPlacementRuleDefinition is used to get a placement rule to use with a cgu.
func GetPlacementRuleDefinition(
	placementRuleName string,
	namespace string,
	placementFields placementrulev1.GenericPlacementFields) placementrulev1.PlacementRule {
	customResource := placementrulev1.PlacementRule{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PlacementRule",
			APIVersion: placementrulev1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      placementRuleName,
			Namespace: namespace,
		},
		Spec: placementrulev1.PlacementRuleSpec{
			GenericPlacementFields: placementFields,
		},
	}

	return customResource
}

// GetPlacementRule can be used to get a specific placement rule object from the cluster.
func GetPlacementRule(
	client *testClient.ClientSet,
	placementRuleName string,
	namespace string) (placementrulev1.PlacementRule, error) {
	// Validate inputs first
	if placementRuleName == "" {
		return placementrulev1.PlacementRule{}, errors.New("provided empty placementRuleName")
	}

	if namespace == "" {
		return placementrulev1.PlacementRule{}, errors.New("provided empty namespace")
	}

	var placementRuleList placementrulev1.PlacementRuleList

	// Get a list of placement rules from the cluster
	err := client.Client.List(
		GetTestContext(),
		&placementRuleList,
		&runtimeclient.ListOptions{
			Namespace: namespace,
		},
	)

	// Filter errors that don't matter
	err = FilterMissingResourceErrors(err)
	if err != nil {
		return placementrulev1.PlacementRule{}, err
	}

	// Check the returned placement rule list for the specific placement rule
	for _, placementRule := range placementRuleList.Items {
		if placementRule.Name == placementRuleName {
			return placementRule, nil
		}
	}

	return placementrulev1.PlacementRule{}, errors.New("resource not found")
}

// IsPlacementRuleExist can be used to check if a specific placement rule exists.
func IsPlacementRuleExist(client *testClient.ClientSet, placementRuleName string, namespace string) (bool, error) {
	log.Printf("Checking for existence of placement rule '%s' in namespace '%s'\n", placementRuleName, namespace)

	// We can use another helper to get the object
	placementRule, err := GetPlacementRule(client, placementRuleName, namespace)

	// Filter any missing resource errors before checking the result
	err = FilterMissingResourceErrors(err)
	if err != nil {
		return false, err
	}

	return placementRule.Name == placementRuleName, nil
}

// DeletePlacementRuleAndWait can be used to delete a placement rule if it exists.
func DeletePlacementRuleAndWait(client *testClient.ClientSet, placementRuleName string, namespace string) error {
	// Check if it exists first
	exists, err := IsPlacementRuleExist(client, placementRuleName, namespace)
	if err != nil {
		return err
	}

	// If it exists then attempt to delete it
	if exists {
		// Get the specific object to be deleted
		placementRule, err := GetPlacementRule(client, placementRuleName, namespace)
		if err != nil {
			return err
		}

		// Delete the object
		err = client.Client.Delete(
			GetTestContext(),
			&placementRule,
			&runtimeclient.DeleteOptions{},
		)
		if err != nil {
			return err
		}

		// Wait until its gone
		err = WaitUntilObjectDoesNotExist(
			client,
			placementRuleName,
			namespace,
			IsPlacementRuleExist,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// CreatePlacementRuleAndWait is used to create a placement rule and wait for it to exist.
func CreatePlacementRuleAndWait(
	client *testClient.ClientSet,
	placementRule placementrulev1.PlacementRule) error {
	// Create the policy
	_ = ranhelper.PrintCr(placementRule)

	err := client.Client.Create(GetTestContext(), &placementRule)
	if err != nil {
		return err
	}

	// Wait for it to exist
	err = WaitUntilObjectExists(
		client,
		placementRule.Name,
		placementRule.Namespace,
		IsPlacementRuleExist,
	)

	return err
}

/*
	Policy Set helpers
*/

// GetPolicySetDefinition is used to get a policy set for the provided policies.
func GetPolicySetDefinition(
	policySetName string,
	policyList []policiesv1beta1.NonEmptyString,
	namespace string) policiesv1beta1.PolicySet {
	customResource := policiesv1beta1.PolicySet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PolicySet",
			APIVersion: policiesv1beta1.GroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      policySetName,
			Namespace: namespace,
		},
		Spec: policiesv1beta1.PolicySetSpec{
			Policies: policyList,
		},
	}

	return customResource
}

// GetPolicySet can be used to check if a specific policy set exists.
func GetPolicySet(
	client *testClient.ClientSet,
	policySetName string,
	namespace string) (policiesv1beta1.PolicySet, error) {
	// Validate inputs first
	if policySetName == "" {
		return policiesv1beta1.PolicySet{}, errors.New("provided empty policySetName")
	}

	if namespace == "" {
		return policiesv1beta1.PolicySet{}, errors.New("provided empty namespace")
	}

	var policySetList policiesv1beta1.PolicySetList

	// Get a list of policy sets from the cluster
	err := client.Client.List(
		GetTestContext(),
		&policySetList,
		&runtimeclient.ListOptions{
			Namespace: namespace,
		},
	)

	// Filter errors that don't matter
	err = FilterMissingResourceErrors(err)
	if err != nil {
		return policiesv1beta1.PolicySet{}, err
	}

	// Check the returned policy list for the specific policy
	for _, policySet := range policySetList.Items {
		if policySet.Name == policySetName {
			return policySet, nil
		}
	}

	return policiesv1beta1.PolicySet{}, errors.New("resource not found")
}

// IsPolicySetExist can be used to check if a specific policy set exists.
func IsPolicySetExist(client *testClient.ClientSet, policySetName string, namespace string) (bool, error) {
	log.Printf("Checking for existence of policy set '%s' in namespace '%s'\n", policySetName, namespace)

	// We can use another helper to get the object
	policySet, err := GetPolicySet(client, policySetName, namespace)

	// Filter any missing resource errors before checking the result
	err = FilterMissingResourceErrors(err)
	if err != nil {
		return false, err
	}

	return policySet.Name == policySetName, nil
}

// DeletePolicySetAndWait can be used to delete a policy set if it exists.
func DeletePolicySetAndWait(client *testClient.ClientSet, policySetName string, namespace string) error {
	// Check if it exists first
	exists, err := IsPolicySetExist(client, policySetName, namespace)
	if err != nil {
		return err
	}

	// If it exists then attempt to delete it
	if exists {
		// Get the specific object to be deleted
		policySet, err := GetPolicySet(client, policySetName, namespace)
		if err != nil {
			return err
		}

		// Delete the object
		err = client.Client.Delete(
			GetTestContext(),
			&policySet,
			&runtimeclient.DeleteOptions{},
		)
		if err != nil {
			return err
		}

		// Wait until its gone
		err = WaitUntilObjectDoesNotExist(
			client,
			policySetName,
			namespace,
			IsPolicySetExist,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// CreatePolicySetAndWait is used to create a policy set and wait for it to exist.
func CreatePolicySetAndWait(
	client *testClient.ClientSet,
	policySet policiesv1beta1.PolicySet) error {
	// Create the policy
	_ = ranhelper.PrintCr(policySet)

	err := client.Client.Create(GetTestContext(), &policySet)
	if err != nil {
		return err
	}

	// Wait for it to exist
	err = WaitUntilObjectExists(
		client,
		policySet.Name,
		policySet.Namespace,
		IsPolicySetExist,
	)

	return err
}

/*
	Catsrc helpers
*/

// GetCatsrcDefinition is used to get a catalog source definition for use in a policy.
func GetCatsrcDefinition(
	name string,
	namespace string,
	sourceType operatorsv1alpha1.SourceType,
	priority int,
	configMap string,
	address string,
	image string,
	displayName string) operatorsv1alpha1.CatalogSource {
	return operatorsv1alpha1.CatalogSource{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CatalogSource",
			APIVersion: operatorsv1alpha1.CatalogSourceCRDAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: operatorsv1alpha1.CatalogSourceSpec{
			SourceType:  sourceType,
			Priority:    priority,
			ConfigMap:   configMap,
			Address:     address,
			Image:       image,
			DisplayName: displayName,
			Description: "a catalog source created by the talm tests",
			Publisher:   "cnf-gotests/test/ran/talm",
		},
	}
}

// GetCatsrc is used to get the specified catalog source object from the specified cluster.
func GetCatsrc(client *testClient.ClientSet, name string, namespace string) (operatorsv1alpha1.CatalogSource, error) {
	// Validate inputs first
	if name == "" {
		return operatorsv1alpha1.CatalogSource{}, errors.New("provided empty catsrc name")
	}

	if namespace == "" {
		return operatorsv1alpha1.CatalogSource{}, errors.New("provided empty catsrc name")
	}

	catsrc, err := client.OperatorsV1alpha1Interface.CatalogSources(namespace).
		Get(GetTestContext(), name, metav1.GetOptions{})

	// Filter errors that don't matter
	err = FilterMissingResourceErrors(err)

	if err == nil {
		return *catsrc, nil
	}

	return operatorsv1alpha1.CatalogSource{}, errors.New("resource not found")
}

// IsCatsrcExist is used to check if the specified catalog source object exists on the cluster.
func IsCatsrcExist(client *testClient.ClientSet, name string, namespace string) (bool, error) {
	log.Printf("Checking for existence of catalog source '%s' in namespace '%s'\n", name, namespace)

	// We can use another helper to get the object
	catsrc, err := GetCatsrc(client, name, namespace)
	err = FilterMissingResourceErrors(err)

	// Filter any missing resource errors before checking the result
	if err != nil {
		return false, err
	}

	return catsrc.Name == name, nil
}

// DeleteCatsrcAndWait is used to delete the specified catalog source object and wait for it to no longer exist.
func DeleteCatsrcAndWait(client *testClient.ClientSet, name string, namespace string) error {
	// Check if it exists first
	exists, err := IsCatsrcExist(client, name, namespace)
	if err != nil {
		return err
	}

	// If it exists then attempt to delete it
	if exists {
		// Delete the object
		err := client.OperatorsV1alpha1Interface.CatalogSources(namespace).
			Delete(GetTestContext(), name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}

		// Wait until its gone
		err = WaitUntilObjectDoesNotExist(client, name, namespace, IsCatsrcExist)
		if err != nil {
			return err
		}
	}

	return nil
}

// CreateCatsrcAndWait is used to create the specified catalog source object and wait for it to exist.
func CreateCatsrcAndWait(client *testClient.ClientSet, catsrc operatorsv1alpha1.CatalogSource) error {
	// Create the catalog source
	_, err := client.OperatorsV1alpha1Interface.CatalogSources(catsrc.Namespace).
		Create(GetTestContext(), &catsrc, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	// Wait for it to exist
	err = WaitUntilObjectExists(client, catsrc.Name, catsrc.Namespace, IsCatsrcExist)
	if err != nil {
		return err
	}

	return nil
}

/*
	Cluster helpers
*/

// GetClusterVersionDefinition returns a unstructured ClusterVersion definition based on the apiClient.
// Use "Image" to include only DesiredUpdate.Image retrieved from the provided apiClient
// Use "Version" to include only DesiredUpdate.Version retrieved from the provided apiClient
// Use "Both" to include both DesiredUpdate.Image and DesiredUpdate.Image retrieved from the provided apiClient.
func GetClusterVersionDefinition(config string, apiClient *testClient.ClientSet) (*unstructured.Unstructured, error) {
	clusterVersionSpec := make(map[string]interface{})
	desiredUpdate := make(map[string]interface{})

	// channel and upstream specs are required when desiredUpdate.version is used
	if config != "Image" {
		version, err := ranhelper.GetClusterVersion(apiClient)
		if err != nil {
			return nil, err
		}

		desiredUpdate["version"] = version
		clusterVersionSpec["upstream"] = configv1.URL(helper.Config.Ran.OcpUpgradeUpstreamURL)
		clusterVersionSpec["channel"] = GetClusterChannel(apiClient)
	}

	// no other specs are needed when desiredUpdate.image is used, but usually force upgrade is used in combination
	// when upgrade path is unavailable in upgrade graph.
	if config != "Version" {
		desiredUpdate["image"] = GetClusterDesiredUpdateImage(apiClient)
		clusterVersionSpec["force"] = true
	}

	// Add composed desiredUpdate to cluster version spec.
	clusterVersionSpec["desiredUpdate"] = desiredUpdate

	// compose clusterversion definition using unstructured type due to clusterversion api always generates
	// unspecified spec and status, which will not work with ACM policy in general.
	clusterVersion := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "config.openshift.io/v1",
		"kind":       "ClusterVersion",
		"metadata":   map[string]interface{}{"name": "version"},
		"spec":       clusterVersionSpec,
	}}

	return clusterVersion, nil
}

// GetClusterDesiredUpdateImage get apiClient's Desired.Image from ClusterVersions cr.
func GetClusterDesiredUpdateImage(apiClient *testClient.ClientSet) string {
	get, _ := apiClient.ConfigV1Interface.ClusterVersions().Get(context.Background(),
		"version", metav1.GetOptions{})

	return get.Status.Desired.Image
}

// GetClusterDesiredUpdateImage get apiClient's Channel from ClusterVersions cr.
func GetClusterChannel(apiClient *testClient.ClientSet) string {
	get, _ := apiClient.ConfigV1Interface.ClusterVersions().Get(context.Background(),
		"version", metav1.GetOptions{})

	return get.Spec.Channel
}

// IsClusterStartedInCgu can be used to check if a particular cluster has started
// being remediated in the provided cgu and namespace.
func IsClusterStartedInCgu(
	client *testClient.ClientSet,
	cguName string,
	clusterName string,
	namespace string) (bool, error) {
	cgu, err := GetCgu(client, cguName, namespace)

	if err != nil {
		return false, err
	}

	clusterStatus := cgu.Status.Status.CurrentBatchRemediationProgress[clusterName]
	if clusterStatus != nil {
		if clusterStatus.State != "NotStarted" {
			return true, nil
		}
	}

	return false, nil
}

// IsClusterInProgressInCgu can be used to check if a particular cluster is actively
// being remediated in the provided cgu and namespace.
func IsClusterInProgressInCgu(
	client *testClient.ClientSet,
	cguName string,
	clusterName string,
	namespace string) (bool, error) {
	cgu, err := GetCgu(client, cguName, namespace)
	if err != nil {
		return false, err
	}

	clusterStatus := cgu.Status.Status.CurrentBatchRemediationProgress[clusterName]
	if clusterStatus != nil {
		if clusterStatus.State == "InProgress" {
			return true, nil
		}
	}

	return false, nil
}

// WaitForClusterInProgressInCgu can be used to wait until the provided cluster is actively
// being remediated in the provided cgu and namespace.
func WaitForClusterInProgressInCgu(
	client *testClient.ClientSet,
	cguName string,
	clusterName string,
	namespace string,
	timeout time.Duration) error {
	// Print the current check
	log.Printf("Waiting until cluster '%s' in progress in cgu '%s'",
		clusterName,
		cguName,
	)

	err := wait.PollImmediate(
		15*time.Second,
		timeout,
		func() (bool, error) {
			ok, err := IsClusterInProgressInCgu(client, cguName, clusterName, namespace)
			if err != nil {
				return true, err
			}

			return ok, nil
		},
	)

	return err
}

// WaitForBackupStart Waits for CGU to report backup started.
func WaitForBackupStart(client *testClient.ClientSet, cguName string, namespace string, timeout time.Duration) error {
	// Print the current check
	log.Println("Waiting for backup to begin")

	// get cgu status
	err := wait.PollImmediate(10*time.Second, timeout, func() (bool, error) {
		cgu, err := GetCgu(client, cguName, namespace)
		if err != nil {
			return false, err
		}

		return cgu.Status.Backup != nil, nil
	})

	return err
}

// IsClusterCompletedSuccessfullyInCgu can be used to check if a particular cluster
// has been successfully remediated in the provided cgu and namespace.
func IsClusterCompletedSuccessfullyInCgu(
	client *testClient.ClientSet,
	cguName string,
	clusterName string,
	namespace string) (bool, error) {
	cgu, err := GetCgu(client, cguName, namespace)
	if err != nil {
		return false, err
	}

	clusterStatus := cgu.Status.Status.CurrentBatchRemediationProgress[clusterName]
	if clusterStatus != nil {
		if clusterStatus.State == "Completed" {
			return true, nil
		}
	}

	return false, nil
}

// WaitForClusterProgressInCgu can be used to wait until the provided cluster is
// successfully remediated in the provided cgu and namespace.
func WaitForClusterSuccessInCgu(
	client *testClient.ClientSet,
	cguName string,
	clusterName string,
	namespace string,
	timeout time.Duration) error {
	// Print the current check
	log.Printf("Waiting until cluster '%s' in successful in cgu '%s'",
		clusterName,
		cguName,
	)

	err := wait.PollImmediate(
		15*time.Second,
		timeout,
		func() (bool, error) {
			ok, err := IsClusterCompletedSuccessfullyInCgu(client, cguName, clusterName, namespace)
			if err != nil {
				return true, err
			}

			return ok, nil
		},
	)

	return err
}

/*
	Cleanup helpers
*/

// CleanupNamespace is used to cleanup a namespace on multiple clients.
func CleanupNamespace(clients []*testClient.ClientSet, namespace string) error {
	for _, client := range clients {
		if namespaces.Exists(namespace, client) {
			err := namespaces.DeleteAndWait(client, namespace, 5*time.Minute)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// CleanupTestResourcesOnClient is used to delete everything on a specific cluster.
func CleanupTestResourcesOnClient(
	client *testClient.ClientSet,
	cguName string,
	policyName string,
	namespace string,
	placementBinding string,
	placementRule string,
	policySet string,
	catsrcName string,
	deleteNs bool,
) []error {
	// Create a list of errorList
	var errorList []error

	// Check for nil client first
	if client == nil {
		errorList = append(errorList, errors.New("provided nil client"))

		return errorList
	}

	// Attempt to delete cgu
	log.Printf("Deleting cgu '%s'", cguName)

	err := DeleteCguAndWait(client, cguName, namespace)
	if err != nil {
		errorList = append(errorList, err)
	}

	curErrors := DeletePolicyAndItsComponents(client, policyName, namespace, placementBinding, placementRule, policySet)

	errorList = append(errorList, curErrors...)

	// Attempt to delete catsrc
	if catsrcName != "" {
		log.Printf("Deleting catsrc '%s'", catsrcName)

		err = DeleteCatsrcAndWait(client, catsrcName, namespace)
		if err != nil {
			errorList = append(errorList, err)
		}
	}

	if namespace != "" && deleteNs {
		// Attempt to delete namespace
		log.Printf("Deleting namespace '%s'", namespace)

		if namespaces.Exists(namespace, client) {
			err := namespaces.DeleteAndWait(client, namespace, 5*time.Minute)
			if err != nil {
				errorList = append(errorList, err)
			}
		}
	}

	return errorList
}

func DeletePolicyAndItsComponents(
	client *testClient.ClientSet,
	policyName string,
	namespace string,
	placementBinding string,
	placementRule string,
	policySet string) []error {
	var errorList []error
	// Attempt to delete policy
	log.Printf("Deleting policy '%s'", policyName)

	err := DeletePolicyAndWait(client, policyName, namespace)
	if err != nil {
		errorList = append(errorList, err)
	}

	// Attempt to delete placement bindings
	log.Printf("Deleting placement binding '%s'", placementBinding)

	err = DeletePlacementBindingAndWait(client, placementBinding, namespace)
	if err != nil {
		errorList = append(errorList, err)
	}

	// Attempt to delete placement rules
	log.Printf("Deleting placement rule '%s'", placementRule)

	err = DeletePlacementRuleAndWait(client, placementRule, namespace)
	if err != nil {
		errorList = append(errorList, err)
	}

	// Attempt to delete policy set
	log.Printf("Deleting policy set '%s'", policySet)

	err = DeletePolicySetAndWait(client, policySet, namespace)
	if err != nil {
		errorList = append(errorList, err)
	}

	return errorList
}

// CleanupTestResourcesOnClients is used to delete all references to specified cgu,
// policy, and namespace on all clusters specified in the client list.
func CleanupTestResourcesOnClients(
	clients []*testClient.ClientSet,
	cguName string,
	policyName string,
	createdNamespace string,
	placementBinding string,
	placementRule string,
	policySet string,
	catsrcName string) []error {
	// Create a list of errors
	var errors []error

	// Loop over all clients for cleanup
	for _, client := range clients {
		// Perform the deletes
		log.Printf("cleaning up resources on client '%s'", client.Config.Host)

		cleanupErr := CleanupTestResourcesOnClient(
			client,
			cguName,
			policyName,
			createdNamespace,
			placementBinding,
			placementRule,
			policySet,
			catsrcName,
			true,
		)
		if len(cleanupErr) != 0 {
			errors = append(errors, cleanupErr...)
		}
	}

	return errors
}

// FilterMissingResourceErrors takes an input error and checks it for a few specific types of errors.
// If it matches any of the errors that are considered to be a resource not found, then it returns nil.
// Otherwise it returns the original error.
func FilterMissingResourceErrors(err error) error {
	// If the error was nil just immediately return
	if err == nil {
		return nil
	}

	// Reduce logging until we can have different log levels in future project
	// log.Printf("Checking error '%s'", err.Error())

	if strings.HasPrefix(err.Error(), "server could not find the requested resource") {
		return nil
	}

	if strings.HasPrefix(err.Error(), "no matches for kind") {
		return nil
	}

	if strings.HasSuffix(err.Error(), "not found") {
		return nil
	}

	return err
}

// WaitUntilObjectExists can be called to wait until a specified resource is present.
// This is called by all of the CreateXAndWait functions in this file.
func WaitUntilObjectExists(
	client *testClient.ClientSet,
	objectName string,
	namespace string,
	getStatus func(client *testClient.ClientSet, objectName string, namespace string) (bool, error)) error {
	// Wait for it to exist
	err := wait.PollImmediate(
		15*time.Second,
		5*time.Minute,
		func() (bool, error) {
			status, err := getStatus(client, objectName, namespace)

			// Wait until it definitely exists
			if err == nil && status {
				return true, nil
			}

			// Assume it does not exist otherwise
			return false, nil
		},
	)

	return err
}

// WaitUntilObjectDoesNotExist can be called to wait until a specified resource is deleted.
// This is called by all of the DeleteXAndWait functions in this file.
func WaitUntilObjectDoesNotExist(
	client *testClient.ClientSet,
	objectName string,
	namespace string,
	getStatus func(client *testClient.ClientSet, objectName string, namespace string) (bool, error)) error {
	// Wait for it to exist
	err := wait.PollImmediate(
		15*time.Second,
		5*time.Minute,
		func() (bool, error) {
			status, err := getStatus(client, objectName, namespace)

			// May or may not exist
			err = FilterMissingResourceErrors(err)
			if err == nil && !status {
				// Did exist, but is gone now
				return true, nil
			}

			// Assume it still exists
			return false, nil

		},
	)

	return err
}

// EnableCgu enable Cgu.
func EnableCgu(client *testClient.ClientSet, cgu v1alpha1.ClusterGroupUpgrade) error {
	payload := `{"spec":{"enable":true}}`

	_, err := PatchCgu(client, payload, cgu, metav1.PatchOptions{})

	return err
}

// PatchCgu patch CGU CR.
func PatchCgu(client *testClient.ClientSet,
	payload string,
	cgu v1alpha1.ClusterGroupUpgrade,
	options metav1.PatchOptions) (*v1alpha1.ClusterGroupUpgrade, error) {
	return client.
		ClustergroupupgradesoperatorV1alpha1Interface.
		ClusterGroupUpgrades(cgu.Namespace).
		Patch(context.Background(), cgu.Name, types.MergePatchType, []byte(payload), options)
}
