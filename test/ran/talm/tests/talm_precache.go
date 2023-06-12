package tests

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/cluster-group-upgrades-operator/api/v1alpha1"
	v1alpha12 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/pkg/apis/core"
	configurationPolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// precache const.
const (
	SpokeNS               = "openshift-talo-pre-cache"
	PreCacheContainerName = "pre-cache-container"
	PreCachePodLabel      = "job-name=pre-cache"
)

// use this struct to delete the deleted policies and its components.
type policyandco struct {
	policyName           string
	policySetName        string
	placementBindingName string
	placementRuleName    string
}

var _ = Describe("Talm precache one spoke", Label("talmprecache"), func() {

	Context("Precache operator", func() {
		var (
			listPolicy policiesv1.PolicyList
			// policies with at least one subscription cr. They are all non-compliant
			policyAndCoWithSub []policyandco
		)

		curName := "precache-operator"
		cguName := fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, curName)

		BeforeEach(func() {
			log.Println("verifying list of policies in config are already available in hub required Precache operator")

			err := rantalmhelper.HubAPIClient.List(context.Background(), &listPolicy, runtimeclient.InNamespace(""))
			if err != nil {
				log.Println(err)
				Skip("could not list all policies from all namespaces")
			}
			listPolicy, exists := rantalmhelper.AllPoliciesExist(listPolicy)
			if !exists {
				Skip("could not find all the policies specified in config or in TALM_PRECACHE_POLICIES env")
			}

			policyAndCoWithSub = findAllPoliciesWithSubAndCopyAndApply(listPolicy)
		})

		AfterEach(func() {
			if CurrentSpecReport().Failed() {
				// Best effort print pod log in case pod in error
				_ = checkPrecachePodLog(rantalmhelper.Spoke1APIClient)
			}

			// Best effort print CGU and precache pod log in case of test failure
			printCguAndPolicyOnFailure(rantalmparameters.TalmTestNamespace)

			// all generated policy and components
			for _, curPolAndCo := range policyAndCoWithSub {
				rantalmhelper.DeletePolicyAndItsComponents(
					rantalmhelper.HubAPIClient,
					curPolAndCo.policyName,
					rantalmparameters.TalmTestNamespace,
					curPolAndCo.placementBindingName,
					curPolAndCo.placementRuleName,
					curPolAndCo.policySetName,
				)
			}

			err := rantalmhelper.DeleteCguAndWait(
				rantalmhelper.HubAPIClient,
				cguName,
				rantalmparameters.TalmTestNamespace,
			)
			Expect(err).To(BeNil())

		})

		It("tests for precache operator with multiple sources", func() {
			By("creating CGU with created operator upgrade policy")
			// prep cgu with one spoke
			cgu := getNewPrecacheCGU(cguName, helper.Config.Ran.TalmPrecachePolicies, []string{rantalmhelper.Spoke1Name})

			// apply
			err := rantalmhelper.CreateCguAndWait(
				rantalmhelper.HubAPIClient,
				cgu,
			)
			Expect(err).To(BeNil())

			By("verifying spoke1 succeeded in CGU")
			assertPrecacheStatus(cgu.Name, rantalmhelper.Spoke1Name, "Succeeded")

			By("verifying image precache pod succeeded on spoke")
			err = checkPrecachePodLog(rantalmhelper.Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Precache OCP with version", func() {
		curName := "precache-ocp"
		cguName := fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, curName)
		policyName := fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName)

		AfterEach(func() {
			printCguAndPolicyOnFailure(rantalmparameters.TalmTestNamespace)

			// delete generated CRs
			rantalmhelper.CleanupTestResourcesOnClient(
				rantalmhelper.HubAPIClient,
				cguName,
				policyName,
				rantalmparameters.TalmTestNamespace,
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementBindingCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementRuleCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PolicySetNameCommonName, curName),
				"",
				false,
			)
		})

		It("tests for ocp cache with version", func() {
			By("creating and applying policy with clusterversion CR that defines the upgrade graph, channel, and version")
			// prep cgu
			cgu := getNewPrecacheCGU(cguName, []string{fmt.Sprintf("%s-%s",
				rantalmparameters.PolicyNameCommonName, curName)},
				[]string{rantalmhelper.Spoke1Name})

			// prep clusterVersion
			clusterVersion, err := rantalmhelper.GetClusterVersionDefinition("Version",
				rantalmhelper.Spoke1APIClient)
			Expect(err).To(BeNil())

			// apply
			err = rantalmhelper.CreatePolicyAndCgu(
				rantalmhelper.HubAPIClient,
				clusterVersion,
				configurationPolicyv1.MustHave,
				configurationPolicyv1.Inform,
				fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PolicySetNameCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementBindingCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementRuleCommonName, curName),
				rantalmparameters.TalmTestNamespace,
				metav1.LabelSelector{},
				cgu,
			)
			Expect(err).To(BeNil())

			By("waiting until CGU Succeeded")
			assertPrecacheStatus(cgu.Name, rantalmhelper.Spoke1Name, "Succeeded")

			By("waiting until new precache pod in spoke1 succeeded and log reports done")
			err = checkPrecachePodLog(rantalmhelper.Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Precache OCP with image", Ordered, func() {
		curName := "precache-ocp"
		cguName := fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, curName)
		policyName := fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName)
		excludedPrecacheImage := "openshift/ose-vsphere-problem-detector"
		// Command to generate a list of cached images on the spoke cluster
		spokeImageListCmd := fmt.Sprintf(`podman images  --noheading --filter "label=name=%s"`, excludedPrecacheImage)

		// Command to delete excludedPrecacheimage
		spokeImageDeleteCmd := fmt.Sprintf(`podman images --noheading  --filter "label=name=%s" --format {{.ID}}|`+
			`xargs podman rmi`, excludedPrecacheImage)

		var spoke1Master *k8sv1.Node

		BeforeEach(func() {
			By("Check spoke cluster precache images for excluded images")
			masterNodeList, err := nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
			Expect(err).To(BeNil())
			spoke1Master = &masterNodeList[0]

			By("wiping any existing images from the spoke cluster")
			status, _ := helper.ExecCommandOnNodeWithHostBinaries(spoke1Master,
				[]string{"bash", "-c", spokeImageDeleteCmd})
			log.Println("status:", status)
			// Expect(err).ToNot(HaveOccurred())

		})
		AfterEach(func() {
			printCguAndPolicyOnFailure(rantalmparameters.TalmTestNamespace)

			// delete generated CRs
			rantalmhelper.CleanupTestResourcesOnClient(
				rantalmhelper.HubAPIClient,
				cguName,
				policyName,
				rantalmparameters.TalmTestNamespace,
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementBindingCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementRuleCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PolicySetNameCommonName, curName),
				"",
				false,
			)
		})

		It("tests for ocp cache with image", func() {
			By("creating and applying policy with clusterversion " +
				"CR that defines the upgrade graph, channel, and version")
			// prep cgu
			cgu := getNewPrecacheCGU(cguName, []string{fmt.Sprintf("%s-%s",
				rantalmparameters.PolicyNameCommonName, curName)},
				[]string{rantalmhelper.Spoke1Name})

			// prep clusterVersion
			clusterVersion, err := rantalmhelper.GetClusterVersionDefinition("Image",
				rantalmhelper.Spoke1APIClient)
			Expect(err).To(BeNil())

			// apply
			err = rantalmhelper.CreatePolicyAndCgu(
				rantalmhelper.HubAPIClient,
				clusterVersion,
				configurationPolicyv1.MustHave,
				configurationPolicyv1.Inform,
				fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PolicySetNameCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementBindingCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementRuleCommonName, curName),
				rantalmparameters.TalmTestNamespace,
				metav1.LabelSelector{},
				cgu,
			)
			Expect(err).To(BeNil())

			By("waiting until CGU Succeeded")
			assertPrecacheStatus(cgu.Name, rantalmhelper.Spoke1Name, "Succeeded")

			By("waiting until new precache pod in spoke1 succeeded and log reports done")
			err = checkPrecachePodLog(rantalmhelper.Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred())

			By("generating list of pracached images on spoke cluster to ensure excluded image is present")
			precachedImages, err := helper.ExecCommandOnNodeWithHostBinaries(spoke1Master,
				[]string{"bash", "-c", spokeImageListCmd})
			Expect(err).ToNot(HaveOccurred())

			By("Ensure excludedPrecacheImage is present on spoke cluster")
			Expect(precachedImages).ToNot(BeEmpty())
		})

		// ocp-59948
		It("tests precache image filtering", func() {
			if !ranhelper.IsVersionStringInRange(
				rantalmhelper.TalmHubVersion,
				"4.13",
				"",
			) {
				Skip("Skiping Precache Filtering if TALM is older than 4.13")
			}

			By("defining a configmap to exclude images matching  {excludedPrecacheImages} from precaching")
			filterConfigMap := core.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ConfigMap",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-group-upgrade-overrides",
					Namespace: rantalmparameters.TalmTestNamespace,
				},
				Data: map[string]string{"excludePrecachePatterns": "vsphere"},
			}

			By("creating the configmap on hubcluster")
			_, err := rantalmhelper.HubAPIClient.ConfigMaps(rantalmparameters.TalmTestNamespace).Create(context.Background(),
				(*k8sv1.ConfigMap)(&filterConfigMap),
				metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Prepare to create second CGU, this one with image filtering enabled.
			By("Creating a CGU with an image filter")
			cgu := getNewPrecacheCGU(cguName, []string{fmt.Sprintf("%s-%s",
				rantalmparameters.PolicyNameCommonName, curName)},
				[]string{rantalmhelper.Spoke1Name})

			// prep clusterVersion
			clusterVersion, err := rantalmhelper.GetClusterVersionDefinition("Image",
				rantalmhelper.Spoke1APIClient)
			Expect(err).To(BeNil())

			// apply CGU
			err = rantalmhelper.CreatePolicyAndCgu(
				rantalmhelper.HubAPIClient,
				clusterVersion,
				configurationPolicyv1.MustHave,
				configurationPolicyv1.Inform,
				fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PolicySetNameCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementBindingCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementRuleCommonName, curName),
				rantalmparameters.TalmTestNamespace,
				metav1.LabelSelector{},
				cgu,
			)
			Expect(err).To(BeNil())

			By("waiting until CGU Succeeded")
			assertPrecacheStatus(cgu.Name, rantalmhelper.Spoke1Name, "Succeeded")

			By("Checking images list for excluded image")
			precachedImages, err := helper.ExecCommandOnNodeWithHostBinaries(spoke1Master,
				[]string{"bash", "-c", spokeImageListCmd})
			Expect(err).ToNot(HaveOccurred())
			Expect(precachedImages).Should(BeEmpty())
		})
	})
})

// findAllPoliciesWithSubAndCopyAndApply it finds policies with subscription,
// makes a copy and updates TalmPrecachePolicies variable if needed
// this func is made for precache operator test.
func findAllPoliciesWithSubAndCopyAndApply(listPolicy policiesv1.PolicyList) []policyandco {
	// set to easily modify TalmPrecachePolicies if needed
	policyMap := make(map[string]string)

	for _, s := range helper.Config.Ran.TalmPrecachePolicies {
		policyMap[s] = s
	}

	var (
		// policies with at least one subscription cr. They are all non-compliant
		policyAndCoWithSub []policyandco
	)

	for idx, curPolicy := range listPolicy.Items {
		// find that it cur policy contains an instance of subscritption
		curPTempl := curPolicy.Spec.PolicyTemplates[0]
		uConfigPolicy := &unstructured.Unstructured{}
		err := uConfigPolicy.UnmarshalJSON(curPTempl.ObjectDefinition.Raw)
		Expect(err).To(BeNil())

		tConfigPolicy := configurationPolicyv1.ConfigurationPolicy{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(uConfigPolicy.UnstructuredContent(), &tConfigPolicy)
		Expect(err).To(BeNil())

		// loop over the list of obj and look for Subscription
		for _, obj := range tConfigPolicy.Spec.ObjectTemplates {
			uCurObjTemp := &unstructured.Unstructured{}
			err = uCurObjTemp.UnmarshalJSON(obj.ObjectDefinition.Raw)
			Expect(err).To(BeNil())

			// only process if that the policy contains a Subscription
			if uCurObjTemp.GetObjectKind().GroupVersionKind().Kind == "Subscription" {
				curPolicyAndCo := policyandco{
					policyName:           rantalmparameters.PolicyNameCommonName + "-with-subscription-" + strconv.Itoa(idx),
					policySetName:        rantalmparameters.PolicySetNameCommonName + "-with-subscription-" + strconv.Itoa(idx),
					placementBindingName: rantalmparameters.PlacementBindingCommonName + "-with-subscription-" + strconv.Itoa(idx),
					placementRuleName:    rantalmparameters.PlacementRuleCommonName + "-with-subscription-" + strconv.Itoa(idx),
				}
				policyAndCoWithSub = append(policyAndCoWithSub, curPolicyAndCo)

				log.Printf("copying policy [%s] and generating a new one called [%s]\n", curPolicy.Name, curPolicyAndCo.policyName)

				// copying so ignore the original one
				_, exists := policyMap[curPolicy.Name]
				if exists {
					delete(policyMap, curPolicy.Name)
				}

				// make copy of the policy and extract
				cpPolicy := curPolicy.DeepCopy()

				pTempRef := cpPolicy.Spec.PolicyTemplates[0]

				// get the config policy and add a namespace to maybe it non-compliant
				uConfigPolicy := &unstructured.Unstructured{}
				err = uConfigPolicy.UnmarshalJSON(pTempRef.ObjectDefinition.Raw)
				Expect(err).To(BeNil())

				tConfigPolicy := configurationPolicyv1.ConfigurationPolicy{}
				err = runtime.DefaultUnstructuredConverter.FromUnstructured(uConfigPolicy.UnstructuredContent(), &tConfigPolicy)
				Expect(err).To(BeNil())

				for _, curConfig := range tConfigPolicy.Spec.ObjectTemplates {
					// covert raw to unstructured
					tempUnstructured := &unstructured.Unstructured{}
					err = tempUnstructured.UnmarshalJSON(curConfig.ObjectDefinition.Raw)
					Expect(err).To(BeNil())

					// covert unstructured to structured
					tempStructured := v1alpha12.Subscription{}
					err = runtime.DefaultUnstructuredConverter.
						FromUnstructured(tempUnstructured.UnstructuredContent(), &tempStructured)
					Expect(err).To(BeNil())

					curConfig.ObjectDefinition.Raw = nil
					curConfig.ObjectDefinition.Object = &tempStructured
				}

				o := configurationPolicyv1.ObjectTemplate{
					ObjectDefinition: runtime.RawExtension{Object: rantalmhelper.GetNamespaceDefinition("make-it-non-compliant")},
					ComplianceType:   configurationPolicyv1.MustHave,
				}
				tConfigPolicy.Spec.ObjectTemplates = append(tConfigPolicy.Spec.ObjectTemplates, &o)

				// create a new policy
				genP := rantalmhelper.GetPolicyDefinition(
					curPolicyAndCo.policyName,
					rantalmparameters.TalmTestNamespace,
					&tConfigPolicy, configurationPolicyv1.Inform)

				// apply new policy
				err := rantalmhelper.ApplyPolicyAndCreateAllComponents(rantalmhelper.HubAPIClient,
					genP,
					curPolicyAndCo.policySetName,
					curPolicyAndCo.placementBindingName,
					curPolicyAndCo.placementRuleName,
					rantalmparameters.TalmTestNamespace,
					[]string{rantalmhelper.Spoke1Name},
					metav1.LabelSelector{},
				)
				Expect(err).To(BeNil())

				// wait until newly generated is non-compliant
				waitUntilPolicyIsNonCompliant(genP)

				// no need to check further since one subscription is found. Move to next policy
				break
			}
		}
	}

	// repopulate helper.Config.Ran.TalmPrecachePolicies
	helper.Config.Ran.TalmPrecachePolicies = []string{}
	for pol := range policyMap {
		helper.Config.Ran.TalmPrecachePolicies = append(helper.Config.Ran.TalmPrecachePolicies, pol)
	}

	// append the copied CR name to TalmPrecachePolicies
	for _, policyNameWithSub := range policyAndCoWithSub {
		helper.Config.Ran.TalmPrecachePolicies = append(helper.Config.Ran.TalmPrecachePolicies,
			policyNameWithSub.policyName)
	}

	return policyAndCoWithSub
}

func waitUntilPolicyIsNonCompliant(p policiesv1.Policy) {
	Eventually(func() bool {
		curP, err := rantalmhelper.GetPolicy(rantalmhelper.HubAPIClient, p.Name, p.Namespace)
		Expect(err).To(BeNil())

		return curP.Status.ComplianceState == policiesv1.NonCompliant
	}, 5*time.Minute, 5*time.Second).Should(BeTrue())
}

var _ = Describe("TALM tests with multiple spokes where one turns off", Ordered, Label("talmprecache"), func() {
	curName := "multi-spokes-one-unavailable"
	cguName := fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, curName)
	policyName := fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName)
	var nodeToTurnOff *k8sv1.Node
	var talmCompleteLabel = "talmcomplete"

	BeforeAll(func() {
		// For 4.11- releases, if one spoke fails, then precache will fail for all spokes
		if !ranhelper.IsVersionStringInRange(
			rantalmhelper.TalmHubVersion,
			"4.12",
			"",
		) {
			Skip("Proceeding with precache if one spoke fails requires 4.12 or higher")
		}

		// tests below requires all clusters to be present. hub + spoke1 + spoke2
		clusterList := rantalmhelper.GetAllTestClients()
		err := ranhelper.IsClustersPresent(clusterList)
		if err != nil {
			Skip(fmt.Sprintf("error occurred validating required clusters are present: %s", err.Error()))
		}

		// keep a copy of the node before turning off
		nodeList, err := rantalmhelper.Spoke1APIClient.Nodes().List(context.Background(), metav1.ListOptions{})
		Expect(err).To(BeNil())
		nodeToTurnOff = &nodeList.Items[0]

		// If BMC_HOST is not defined then skip the test
		if os.Getenv("BMC_HOSTS") == "" {
			Skip("BMC_HOSTS not defined, unable to reboot spoke")
		}

		By("turning off spoke1 and waiting")
		errArr := ranhelper.PowerOffSnoWithIpmi()
		Expect(len(errArr)).To(BeNumerically("==", 0))
	})

	Context("precaching with one managed cluster powered off and unavailable", func() {
		AfterEach(func() {
			printCguAndPolicyOnFailure(rantalmparameters.TalmTestNamespace)

			// delete generated CRs
			rantalmhelper.CleanupTestResourcesOnClient(
				rantalmhelper.HubAPIClient,
				cguName,
				policyName,
				rantalmparameters.TalmTestNamespace,
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementBindingCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementRuleCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PolicySetNameCommonName, curName),
				"",
				false,
			)
		})

		It("verifies precaching fails for one spoke and succeeds for the other", func() {
			By("creating precache CGU with two spokes and OCP upgrade policy ")
			cgu := getNewPrecacheCGU(cguName,
				[]string{fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName)},
				[]string{rantalmhelper.Spoke1Name, rantalmhelper.Spoke2Name})

			// prep clusterVersion
			clusterVersion, _ := rantalmhelper.GetClusterVersionDefinition("Both", rantalmhelper.Spoke2APIClient)

			// apply
			err := rantalmhelper.CreatePolicyAndCgu(
				rantalmhelper.HubAPIClient,
				clusterVersion,
				configurationPolicyv1.MustHave,
				configurationPolicyv1.Inform,
				fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PolicySetNameCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementBindingCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementRuleCommonName, curName),
				rantalmparameters.TalmTestNamespace,
				metav1.LabelSelector{},
				cgu,
			)
			Expect(err).To(BeNil())

			message := "Precaching spec is valid and consistent"
			if !ranhelper.IsVersionStringInRange(
				rantalmhelper.TalmHubVersion,
				"4.11",
				"",
			) {
				message = "Pre-caching spec is valid and consistent"
			}

			log.Println("waiting for precache to confirm that it is valid")
			err = rantalmhelper.WaitForCguInCondition(rantalmhelper.HubAPIClient,
				cgu.Name,
				cgu.Namespace,
				"PrecacheSpecValid",
				message,
				metav1.ConditionTrue,
				"", 5*time.Minute)
			Expect(err).To(BeNil())

			By("verifying precache succeeded for spoke2")
			assertPrecacheStatus(cgu.Name, rantalmhelper.Spoke2Name, "Succeeded")

			By("enabling CGU")
			err = rantalmhelper.EnableCgu(rantalmhelper.HubAPIClient, cgu)
			Expect(err).To(BeNil())

			By("verifying CGU reports one spoke failed in PrecachingSucceeded condition")
			err = rantalmhelper.WaitForCguInCondition(rantalmhelper.HubAPIClient,
				cgu.Name,
				cgu.Namespace,
				"PrecachingSuceeded",
				"Precaching failed for 1 clusters",
				metav1.ConditionTrue,
				"PartiallyDone", 5*time.Minute)
			Expect(err).To(BeNil())

			By("verifying CGU reports spoke1 failed with UnrecoverableError in precache status")
			assertPrecacheStatus(cgu.Name, rantalmhelper.Spoke1Name, "UnrecoverableError")
		})
	})

	Context("batching with one managed cluster powered off and unavailable", Ordered, func() {
		BeforeAll(func() {
			By("creating CGU with two spokes, one of which is unavailable")

			cgu := rantalmhelper.GetCguDefinition(
				cguName,
				[]string{rantalmhelper.Spoke1Name, rantalmhelper.Spoke2Name},
				[]string{},
				[]string{fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName)},
				rantalmparameters.TalmTestNamespace, 1, 9)

			// Apply CGU.
			err := rantalmhelper.CreatePolicyAndCgu(
				rantalmhelper.HubAPIClient,
				rantalmhelper.GetNamespaceDefinition(rantalmhelper.TemporaryNamespaceName),
				configurationPolicyv1.MustHave,
				configurationPolicyv1.Inform,
				fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PolicySetNameCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementBindingCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementRuleCommonName, curName),
				rantalmparameters.TalmTestNamespace,
				metav1.LabelSelector{},
				cgu,
			)
			Expect(err).ToNot(HaveOccurred())

			// CGU Patch Payload creates an afterCompletion action to add a cluster label called talmcomplete
			payload := fmt.Sprintf(`{"spec":{"actions":{"afterCompletion":{"addClusterLabels":{"%s":""}}}}}`, talmCompleteLabel)

			// Patch CGU to add the afterCompletion action
			_, err = rantalmhelper.PatchCgu(
				rantalmhelper.HubAPIClient,
				payload,
				cgu,
				metav1.PatchOptions{},
			)
			Expect(err).ToNot(HaveOccurred())

		})
		// ocp-54854
		It("Verifies CGU fails on 'down' spoke in first batch and succeeds for the 'up' spoke in second batch", func() {
			By("creating CGU with two spokes, one of which is unavailable")
			err := rantalmhelper.WaitForClusterSuccessInCgu(
				rantalmhelper.HubAPIClient,
				cguName,
				rantalmhelper.Spoke2Name,
				rantalmparameters.TalmTestNamespace,
				15*time.Minute,
			)
			Expect(err).ToNot(HaveOccurred())

			By("waiting for the cgu to timeout")
			err = rantalmhelper.WaitForCguToTimeout(cguName, rantalmparameters.TalmTestNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred())

		})
		// ocp-59946
		It("Verifies that CGU afterCompletion action executes on spoke2 when spoke1 is offline", func() {

			By("waiting for the cgu to timeout")
			err := rantalmhelper.WaitForCguToTimeout(cguName, rantalmparameters.TalmTestNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred())

			// Verify that the online cluster has the 'talmcomplete' label
			By("Checking cluster for post-action label")
			labelPresent, err := rantalmhelper.IsClusterLabelExist(
				rantalmhelper.Spoke2Name,
				talmCompleteLabel,
			)

			Expect(err).ToNot(HaveOccurred())
			Expect(labelPresent).To(BeTrue())

			// Verify that the offline cluster does not have the 'talmcomplete' label
			By("Checking offline cluster for post-action label")
			labelPresent, err = rantalmhelper.IsClusterLabelExist(
				rantalmhelper.Spoke1Name,
				talmCompleteLabel,
			)
			Expect(labelPresent).To(BeFalse())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("is not found in managedcluster"))

		})

		AfterAll(func() {
			printCguAndPolicyOnFailure(rantalmparameters.TalmTestNamespace)

			// Delete temporary namespace on spoke cluster.
			spoke2ClusterList := []*testClient.ClientSet{rantalmhelper.Spoke2APIClient}
			spoke2CleanupErr := rantalmhelper.CleanupNamespace(spoke2ClusterList, rantalmhelper.TemporaryNamespaceName)

			// delete generated CRs
			crCleanupErr := rantalmhelper.CleanupTestResourcesOnClient(
				rantalmhelper.HubAPIClient,
				cguName,
				policyName,
				rantalmparameters.TalmTestNamespace,
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementBindingCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PlacementRuleCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PolicySetNameCommonName, curName),
				"",
				false,
			)

			// Delete label from managedcluster
			labelCleanupErr := rantalmhelper.DeleteClusterLabel(rantalmhelper.Spoke2Name, talmCompleteLabel)

			Expect(labelCleanupErr).ToNot(HaveOccurred())
			Expect(spoke2CleanupErr).ToNot(HaveOccurred())
			Expect(crCleanupErr).To(BeEmpty())

		})

	})

	AfterAll(func() {
		log.Println("turning on spoke1 and waiting")
		errArr := ranhelper.PowerOnSnoWithImpi()
		Expect(len(errArr)).To(BeNumerically("==", 0))

		By("waiting until all spoke1 pods are ready")
		err := ranhelper.WaitForClusterRecover(nodeToTurnOff, []string{})
		Expect(err).To(BeNil())
	})
})

// getNewPrecacheCGU get new precache CGU CR.
func getNewPrecacheCGU(cguName string, policyNames []string, spokes []string) v1alpha1.ClusterGroupUpgrade {
	cgu := rantalmhelper.GetCguDefinition(
		cguName,
		spokes,
		[]string{},
		policyNames,
		rantalmparameters.TalmTestNamespace, 2, 240)
	cgu.Spec.Enable = rantalmhelper.BoolAddr(false)
	cgu.Spec.PreCaching = true

	return cgu
}

// assertBackupPodLog asserts status of backup struct.
func assertPrecacheStatus(cguName, spokeName, expectation string) {
	Eventually(func() string {
		cgu, err := rantalmhelper.HubAPIClient.ClustergroupupgradesoperatorV1alpha1Interface.
			ClusterGroupUpgrades(rantalmparameters.TalmTestNamespace).
			Get(context.Background(), cguName, metav1.GetOptions{})
		Expect(err).To(BeNil())

		if cgu.Status.Precaching == nil {
			log.Println("precache struct not ready yet")

			return ""
		}

		_, ok := cgu.Status.Precaching.Status[spokeName]
		if !ok {
			log.Println("cluster name as key did not appear yet")

			return ""
		}

		log.Printf("[%s] %s precache status: %s\n", cgu.Name, spokeName, cgu.Status.Precaching.Status[spokeName])

		return cgu.Status.Precaching.Status[spokeName]
	}, 20*time.Minute, 15*time.Second).Should(Equal(expectation))
}

// assertBackupPodLog retrieves the backup pod generated by job and asserts on the log.
func checkPrecachePodLog(client *testClient.ClientSet) error {
	var plog string

	err := wait.PollImmediate(5*time.Second, 1*time.Minute, func() (bool, error) {
		podList, err := client.Pods(SpokeNS).List(context.Background(), metav1.ListOptions{
			LabelSelector: PreCachePodLabel,
		})
		if err != nil {
			return false, nil
		}

		if len(podList.Items) == 0 {
			log.Println("precache pod does not exist on spoke - skip pod log check.")

			return true, nil
		}

		p := podList.Items[0]
		plog, err = pod.GetLog(client, &p, 1*time.Hour, PreCacheContainerName)
		if err != nil {
			return false, nil
		}

		if strings.Contains(plog, "Image pre-cache done") {
			return true, nil
		}

		return false, nil
	})

	if err != nil && plog != "" {
		log.Println("generated pod logs: \n", plog)
	}

	return err
}

func printCguAndPolicyOnFailure(namespace string) {
	if CurrentSpecReport().Failed() {
		log.Println("Test failed - printing policy and CGU status with best effort")

		_ = rantalmhelper.PrintPolicyStatus(rantalmhelper.HubAPIClient, namespace)
		_ = rantalmhelper.PrintCguSpecAndStatus(rantalmhelper.HubAPIClient, namespace)
	}
}
