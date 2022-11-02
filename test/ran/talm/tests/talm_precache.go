package tests

import (
	"context"
	"fmt"
	"log"
	"time"

	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"

	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/runtime"
	configurationPolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/cluster-group-upgrades-operator/api/v1alpha1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// BoolAddr TODO move this global helper.
func BoolAddr(b bool) *bool {
	boolVar := b

	return &boolVar
}

// precache const.
const (
	SpokeNS               = "openshift-talo-pre-cache"
	PreCacheContainerName = "pre-cache-container"
	PreCachePodLabel      = "job-name=pre-cache"
	PreCacheJobName       = "pre-cache"

	// CGUNameOperator const precache operator.
	CGUNameOperator = "generated-precache-operator"

	// const precache ocp.
	policyName              = "generated-policy-precache-ocp"
	placementRuleName       = "generated-placementrule-precache-ocp"
	placementBindingName    = "generated-placementbinding-precache-ocp"
	configurationPolicyName = "generated-config-policy-precache-ocp"
	CGUNameOCP              = "generated-precache-ocp"
)

var _ = Describe("Talm precache", func() {

	Context("Precache operator", func() {
		var cgu v1alpha1.ClusterGroupUpgrade
		BeforeEach(func() {
			log.Println("verifying list of policies in config are already available in hub required Precache operator")
			var listPolicy policiesv1.PolicyList
			err := rantalmhelper.HubAPIClient.List(context.Background(), &listPolicy, runtimeclient.InNamespace(""))
			if err != nil {
				log.Println(err)
				Skip("could not list all policies from all namespaces")
			}
			if !rantalmhelper.AllPoliciesExist(listPolicy) {
				Skip("could not find all the policies specified in config or in TALM_PRECACHE_POLICIES env")
			}

			log.Println("deleting existing CGU of the same name if exists")
			err = DeleteGeneratedCGU(CGUNameOperator, ran.NamespaceTesting)
			if err != nil {
				Skip(fmt.Sprintf("could not delete cgu: %s", err))
			}
		})

		It("tests for precache operator with multiple sources", func() {
			By("creating CGU with created operator upgrade policy")
			spokeClusters := []string{rantalmhelper.Spoke1Name}
			policyNames := helper.Config.Ran.TalmPrecachePolicies
			cgu = GetAndApplyNewCGU(CGUNameOperator, ran.NamespaceTesting, spokeClusters, policyNames)

			err := CommonPreCacheVerificationSteps(&cgu)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			CommonPreCacheTeardownSteps(&cgu)
		})
	})

	Context("Precache OCP", func() {
		var policy policiesv1.Policy
		var placementRule placementrulev1.PlacementRule
		var placementBinding policiesv1.PlacementBinding
		var cgu v1alpha1.ClusterGroupUpgrade

		BeforeEach(func() {
			if !namespaces.Exists(ran.NamespaceTesting, rantalmhelper.HubAPIClient) {
				Skip(fmt.Sprintf("missing required namespace '%s'", ran.NamespaceTesting))
			}

			err := DeleteGeneratedCGU(CGUNameOCP, ran.NamespaceTesting)
			if err != nil {
				Skip(fmt.Sprintf("could not delete cgu: %s", err))
			}
		})

		It("tests for ocp cache with version", func() {
			By("creating and applying policy with clusterversion CR that defines the upgrade graph, channel, and version")
			policy, placementRule, placementBinding =
				CommonPrecacheOCPStepsAndGetNewPolicyPlacementRulePlacementBinding("Version", helper.Apiclient)

			By("creating CGU with created clusterversion policy")
			spokeClusters := []string{rantalmhelper.Spoke1Name}
			policyNames := []string{policy.Name}
			cgu = GetAndApplyNewCGU(CGUNameOCP, ran.NamespaceTesting, spokeClusters, policyNames)

			err := CommonPreCacheVerificationSteps(&cgu)
			Expect(err).To(BeNil())
		})

		It("tests for ocp cache with image", func() {
			By("creating and applying policy with clusterversion " +
				"CR that defines the upgrade graph, channel, and version")
			policy, placementRule, placementBinding =
				CommonPrecacheOCPStepsAndGetNewPolicyPlacementRulePlacementBinding("Image", helper.Apiclient)

			By("creating CGU with created clusterversion policy")
			spokeClusters := []string{rantalmhelper.Spoke1Name}
			policyNames := []string{policy.Name}
			cgu = GetAndApplyNewCGU(CGUNameOCP, ran.NamespaceTesting, spokeClusters, policyNames)

			err := CommonPreCacheVerificationSteps(&cgu)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			CommonPreCacheOCPTeardownSteps(&policy, &placementRule, &placementBinding, &cgu)
		})
	})

})

// --- funcs common to precache operator and ocp  ---

func CommonPreCacheOCPTeardownSteps(
	policy *policiesv1.Policy,
	placementRule *placementrulev1.PlacementRule,
	placementBinding *policiesv1.PlacementBinding,
	cgu *v1alpha1.ClusterGroupUpgrade) {
	log.Println("deleting generated Policy")
	DeleteGeneratedCR(policy)

	log.Println("deleting generated PlacementRule")
	DeleteGeneratedCR(placementRule)

	log.Println("deleting generated PlacementBinding")
	DeleteGeneratedCR(placementBinding)

	log.Println("performing common precache teardown steps")
	CommonPreCacheTeardownSteps(cgu)
}

func CommonPrecacheOCPStepsAndGetNewPolicyPlacementRulePlacementBinding(
	config string,
	spokeClient *testClient.ClientSet) (policiesv1.Policy, placementrulev1.PlacementRule, policiesv1.PlacementBinding) {
	log.Println("generating clusterversion, configurationPolicy, Policy, PlacementRule and PlacementBinding")

	clusterVersion, err := GetNewConfiguredClusterVersion(config, spokeClient)
	Expect(err).To(BeNil())

	configurationPolicy := GetNewConfigurationPolicyWithOneObj(configurationPolicyName, &clusterVersion)

	policy := GetNewPolicyWithOneObj(policyName, ran.NamespaceTesting, &configurationPolicy)

	placementRule := GetNewPlacementRule(placementRuleName, ran.NamespaceTesting)

	placementBinding := GetNewPlacementBinding(placementBindingName, ran.NamespaceTesting, placementRule.Name, policy.Name)

	log.Println("applying Policy, PlacementRule and PlacementBinding")

	err = ApplyAndWaitPolicyPlacementRulePlacementBinding(&policy, &placementRule, &placementBinding)
	Expect(err).To(BeNil())

	return policy, placementRule, placementBinding
}

func CommonPreCacheTeardownSteps(cgu *v1alpha1.ClusterGroupUpgrade) {
	log.Println("deleting generated cgu")

	_ = DeleteGeneratedCGU(cgu.Name, cgu.Namespace)

	log.Println("deleting generated spoke job")
	DeleteJob(PreCacheJobName, SpokeNS)
}

func CommonPreCacheVerificationSteps(cguToTest *v1alpha1.ClusterGroupUpgrade) error {
	By("waiting until CGU Succeeded")
	Eventually(func() string {
		cgu, err := rantalmhelper.HubAPIClient.ClustergroupupgradesoperatorV1alpha1Interface.
			ClusterGroupUpgrades(ran.NamespaceTesting).Get(context.Background(), cguToTest.Name, metav1.GetOptions{})
		Expect(err).To(BeNil())

		if cgu.Status.Precaching == nil {
			log.Println("precaching struct not ready yet")

			return ""
		}

		_, ok := cgu.Status.Precaching.Status[rantalmhelper.Spoke1Name]
		if !ok {
			log.Println("cluster name as key did not appear yet")

			return ""
		}

		log.Printf("%s pre-cache status: %s\n", cgu.Name, cgu.Status.Precaching.Status[rantalmhelper.Spoke1Name])

		return cgu.Status.Precaching.Status[rantalmhelper.Spoke1Name]
	}, 10*time.Minute, 10*time.Second).Should(Equal("Succeeded"))

	By("waiting until new precache pod in spoke1 succeeded and log reports done")

	podList, err := helper.Apiclient.Pods(SpokeNS).List(context.Background(), metav1.ListOptions{
		LabelSelector: PreCachePodLabel,
	})
	Expect(err).To(BeNil())
	Expect(len(podList.Items)).To(BeNumerically("==", 1))
	p := podList.Items[0]
	Expect(p.Status.Phase).To(Equal(k8sv1.PodSucceeded))
	plog, err := pod.GetLog(helper.Apiclient, &p, -time.Until(p.CreationTimestamp.Time), PreCacheContainerName)
	Expect(err).To(BeNil())
	log.Println("generated pod logs: \n", plog)
	Expect(plog).To(ContainSubstring("Image pre-cache done"))

	return nil
}

// --- funcs common to TALM  ---

func GetAndApplyNewCGU(name string, namespace string, spokeClusterNames []string,
	policyNames []string) v1alpha1.ClusterGroupUpgrade {
	cgu := v1alpha1.ClusterGroupUpgrade{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterGroupUpgrade",
			APIVersion: v1alpha1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.ClusterGroupUpgradeSpec{
			PreCaching:      true,
			Enable:          BoolAddr(false),
			Clusters:        spokeClusterNames,
			ManagedPolicies: policyNames,
			RemediationStrategy: &v1alpha1.RemediationStrategySpec{
				MaxConcurrency: 1,
			},
		},
	}

	PrintGeneratedCR(cgu)

	_, err := rantalmhelper.HubAPIClient.ClustergroupupgradesoperatorV1alpha1Interface.
		ClusterGroupUpgrades(namespace).Create(context.Background(), &cgu, metav1.CreateOptions{})

	Expect(err).To(BeNil())

	return cgu
}

func DeleteGeneratedCGU(name string, namespace string) error {
	get, err := rantalmhelper.HubAPIClient.ClustergroupupgradesoperatorV1alpha1Interface.
		ClusterGroupUpgrades(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		log.Printf("could not get %s in hub before performing delete: %s\n", name, err)
	} else {
		PrintGeneratedCR(get)
	}

	err = rantalmhelper.HubAPIClient.ClustergroupupgradesoperatorV1alpha1Interface.
		ClusterGroupUpgrades(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("could not delete %s in hub: %w", name, err)
	}

	return nil
}

func PrintGeneratedCR(b interface{}) {
	y, err := yaml.Marshal(b)
	Expect(err).To(BeNil())
	log.Printf("--- generated CR dump:\n%s\n", string(y))
}

func DeleteGeneratedCR(obj runtimeclient.Object) {
	err := rantalmhelper.HubAPIClient.Delete(context.Background(), obj)
	if err != nil && !errors.IsNotFound(err) {
		log.Printf("could not delete generated CR: %s\n", err)
	}
}

func DeleteJob(jobName string, jobNS string) {
	err := helper.Apiclient.BatchV1Interface.Jobs(jobNS).Delete(context.Background(), jobName, metav1.DeleteOptions{})
	if err != nil {
		log.Println("could delete job:", err)
	}
}

// --- funcs to handle Policy CRs for Precache OCP but maybe useful for other TALM tests  ---

func GetNewPlacementBinding(
	placementBindingName string,
	namespace string,
	placementRuleName string,
	policyName string) policiesv1.PlacementBinding {
	placementBinding := policiesv1.PlacementBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "policy.open-cluster-management.io/v1",
			APIVersion: "PlacementBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      placementBindingName,
			Namespace: namespace,
		},
		PlacementRef: policiesv1.PlacementSubject{
			APIGroup: "apps.open-cluster-management.io",
			Kind:     "PlacementRule",
			Name:     placementRuleName,
		},
		Subjects: []policiesv1.Subject{
			{
				APIGroup: "policy.open-cluster-management.io",
				Kind:     "Policy",
				Name:     policyName,
			},
		},
	}

	PrintGeneratedCR(placementBinding)

	return placementBinding
}

func GetNewPlacementRule(placementRuleName string, namespace string) placementrulev1.PlacementRule {
	placementRule := placementrulev1.PlacementRule{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PlacementRule",
			APIVersion: "apps.open-cluster-management.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      placementRuleName,
			Namespace: namespace,
		},
		Spec: placementrulev1.PlacementRuleSpec{
			GenericPlacementFields: placementrulev1.GenericPlacementFields{
				Clusters: nil,
				ClusterSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "group-du-sno",
							Operator: "In",
							Values:   []string{""},
						},
					},
				},
			},
		},
	}

	PrintGeneratedCR(placementRule)

	return placementRule
}

func GetNewPolicyWithOneObj(policyName string, namespace string, obj runtimeclient.Object) policiesv1.Policy {
	policy := policiesv1.Policy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Policy",
			APIVersion: "policy.open-cluster-management.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: namespace,
		},
		Spec: policiesv1.PolicySpec{
			Disabled:          false,
			RemediationAction: "inform",
			PolicyTemplates: []*policiesv1.PolicyTemplate{
				{
					ObjectDefinition: runtime.RawExtension{
						Object: obj,
					},
				},
			},
		},
	}

	PrintGeneratedCR(policy)

	return policy
}

func GetNewConfigurationPolicyWithOneObj(
	configurationPolicyName string, obj runtimeclient.Object) configurationPolicyv1.ConfigurationPolicy {
	configurationPolicy := configurationPolicyv1.ConfigurationPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigurationPolicy",
			APIVersion: "policy.open-cluster-management.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: configurationPolicyName,
		},
		Spec: configurationPolicyv1.ConfigurationPolicySpec{
			Severity:          "low",
			RemediationAction: "inform",
			NamespaceSelector: configurationPolicyv1.Target{
				Include: []configurationPolicyv1.NonEmptyString{"kube-*"},
				Exclude: []configurationPolicyv1.NonEmptyString{"*"},
			},
			ObjectTemplates: []*configurationPolicyv1.ObjectTemplate{
				{
					ComplianceType: "musthave",
					ObjectDefinition: runtime.RawExtension{
						Object: obj,
					},
				},
			},
			EvaluationInterval: configurationPolicyv1.EvaluationInterval{
				Compliant:    "10m",
				NonCompliant: "10s",
			},
		},
	}

	PrintGeneratedCR(configurationPolicy)

	return configurationPolicy
}

func GetNewConfiguredClusterVersion(config string, spokeclient *testClient.ClientSet) (configv1.ClusterVersion, error) {
	var (
		image   string
		version string
	)

	switch config {
	case "Image":
		image = GetClusterDesiredUpdateImage(spokeclient)
	case "Version":
		version = GetClusterVersion(spokeclient)
	default:
		return configv1.ClusterVersion{}, fmt.Errorf("config value must be either Image or Version")
	}

	clusterVersion := configv1.ClusterVersion{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterVersion",
			APIVersion: "config.openshift.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "version",
		},
		Spec: configv1.ClusterVersionSpec{
			DesiredUpdate: &configv1.Update{
				Version: version,
				Force:   false,
				Image:   image,
			},
			Upstream: configv1.URL(helper.Config.Ran.OcpUpgradeUpstreamURL),
			Channel:  GetClusterChannel(spokeclient),
		},
	}

	PrintGeneratedCR(clusterVersion)

	return clusterVersion, nil
}

func GetClusterDesiredUpdateImage(apiclient *testClient.ClientSet) string {
	get, _ := apiclient.ConfigV1Interface.ClusterVersions().Get(context.Background(),
		"version", metav1.GetOptions{})

	return get.Status.Desired.Image
}

func GetClusterVersion(apiclient *testClient.ClientSet) string {
	get, _ := apiclient.ConfigV1Interface.ClusterVersions().Get(context.Background(),
		"version", metav1.GetOptions{})

	return get.Status.Desired.Version
}

func GetClusterChannel(apiclient *testClient.ClientSet) string {
	get, _ := apiclient.ConfigV1Interface.ClusterVersions().Get(context.Background(),
		"version", metav1.GetOptions{})

	return get.Spec.Channel
}

func ApplyAndWaitPolicyPlacementRulePlacementBinding(
	policy *policiesv1.Policy,
	placementRule *placementrulev1.PlacementRule,
	placementBinding *policiesv1.PlacementBinding) error {
	err := rantalmhelper.HubAPIClient.Create(context.Background(), policy)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("could not apply generated policy: %w", err)
	}

	err = rantalmhelper.HubAPIClient.Create(context.Background(), placementRule)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("could not apply generated placementRule: %w", err)
	}

	err = rantalmhelper.HubAPIClient.Create(context.Background(), placementBinding)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("could not apply generated placementBinding: %w", err)
	}

	Eventually(func() policiesv1.ComplianceState {
		var curPolicy policiesv1.Policy
		c := runtimeclient.ObjectKey{
			Namespace: policy.Namespace,
			Name:      policy.Name,
		}
		err := rantalmhelper.HubAPIClient.Get(context.Background(), c, &curPolicy)
		Expect(err).To(BeNil())

		return curPolicy.Status.ComplianceState
	}, 3*time.Minute, 5*time.Second).Should(Not(BeEmpty()))

	return nil
}
