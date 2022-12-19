package tests

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	k8sv1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	configurationPolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/cluster-group-upgrades-operator/api/v1alpha1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

		BeforeEach(func() {
			log.Println("verifying list of policies in config are already available in hub required Precache operator")

			err := rantalmhelper.HubAPIClient.List(context.Background(), &listPolicy, runtimeclient.InNamespace(""))
			if err != nil {
				log.Println(err)
				Skip("could not list all policies from all namespaces")
			}
			if !rantalmhelper.AllPoliciesExist(listPolicy) {
				Skip("could not find all the policies specified in config or in TALM_PRECACHE_POLICIES env")
			}

			policyAndCoWithSub = findAllPoliciesWithSubAndCopyAndApply(listPolicy)

		})

		AfterEach(func() {
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
				fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, curName),
				rantalmparameters.TalmTestNamespace,
			)
			Expect(err).To(BeNil())

		})

		It("tests for precache operator with multiple sources", func() {
			By("creating CGU with created operator upgrade policy")
			// prep cgu with one spoke
			for _, policyNameWithSub := range policyAndCoWithSub {
				helper.Config.Ran.TalmPrecachePolicies = append(helper.Config.Ran.TalmPrecachePolicies,
					policyNameWithSub.policyName)
			}

			cgu := getNewPrecacheCGU(curName, helper.Config.Ran.TalmPrecachePolicies, []string{rantalmhelper.Spoke1Name})

			// apply
			err := rantalmhelper.CreateCguAndWait(
				rantalmhelper.HubAPIClient,
				cgu,
			)
			Expect(err).To(BeNil())

			By("verifying spoke1 succeeded in CGU")
			assertPrecacheStatus(cgu.Name, rantalmhelper.Spoke1Name, "Succeeded")

			By("verifying image precache pod succeeded on spoke")
			assertPrecachePodLog(rantalmhelper.Spoke1APIClient, "Image pre-cache done")
		})
	})

	Context("Precache OCP", func() {
		curName := "precache-ocp"

		AfterEach(func() {
			// delete generated CRs
			rantalmhelper.CleanupTestResourcesOnClient(
				rantalmhelper.HubAPIClient,
				fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, curName),
				fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName),
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
			cgu := getNewPrecacheCGU(curName, []string{fmt.Sprintf("%s-%s",
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
			assertPrecachePodLog(rantalmhelper.Spoke1APIClient, "Image pre-cache done")
		})

		It("tests for ocp cache with image", func() {
			By("creating and applying policy with clusterversion " +
				"CR that defines the upgrade graph, channel, and version")
			// prep cgu
			cgu := getNewPrecacheCGU(curName, []string{fmt.Sprintf("%s-%s",
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
			assertPrecachePodLog(rantalmhelper.Spoke1APIClient, "Image pre-cache done")
		})
	})

})

func findAllPoliciesWithSubAndCopyAndApply(listPolicy policiesv1.PolicyList) []policyandco {
	var (
		// policies with at least one subscription cr. They are all non-compliant
		policyAndCoWithSub []policyandco
	)

	for idx, curPolicy := range listPolicy.Items {
		// ignore policies that has root-policy in label
		var skipPolicy bool

		for key := range curPolicy.Labels {
			if strings.Contains(key, "root-policy") {
				skipPolicy = true

				break
			}
		}

		if skipPolicy {
			continue
		}

		// find that it cur policy contains an instance of subscritption
		curPTempl := curPolicy.Spec.PolicyTemplates[0]
		uConfigPolicy := &unstructured.Unstructured{}
		err := uConfigPolicy.UnmarshalJSON(curPTempl.ObjectDefinition.Raw)
		Expect(err).To(BeNil())

		tConfigPolicy := configurationPolicyv1.ConfigurationPolicy{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(uConfigPolicy.UnstructuredContent(), &tConfigPolicy)
		Expect(err).To(BeNil())

		// loop over the list of obj and look for Subscription
		for _, objs := range tConfigPolicy.Spec.ObjectTemplates {
			uCurObjTemp := &unstructured.Unstructured{}
			err = uCurObjTemp.UnmarshalJSON(objs.ObjectDefinition.Raw)
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
	var nodeToTurnOff *k8sv1.Node

	BeforeAll(func() {
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

	AfterEach(func() {
		// delete generated CRs
		rantalmhelper.CleanupTestResourcesOnClient(
			rantalmhelper.HubAPIClient,
			fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, curName),
			fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName),
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
		cgu := getNewPrecacheCGU(curName,
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

		log.Println("waiting for precache to confirm that it is valid")
		err = rantalmhelper.WaitForCguInCondition(rantalmhelper.HubAPIClient,
			cgu.Name,
			cgu.Namespace,
			"PrecacheSpecValid",
			"Precaching spec is valid and consistent",
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

	Context("with one managed cluster powered off and unavailable", func() {
		AfterEach(func() {

			// Delete temporary namespace on spoke cluster.
			spoke2ClusterList := []*testClient.ClientSet{rantalmhelper.Spoke2APIClient}
			cleanupErr := rantalmhelper.CleanupNamespace(spoke2ClusterList, rantalmhelper.TemporaryNamespaceName)
			Expect(cleanupErr).ToNot(HaveOccurred())
		})

		// ocp-54854
		It("Verifies CGU fails on 'down' spoke in first batch and succeeds for the 'up' spoke in second batch", func() {
			By("creating CGU with two spokes, one of which is unavailable")

			cgu := rantalmhelper.GetCguDefinition(
				fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, curName),
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

			By("Waiting for running spoke cluster to report success")
			err = rantalmhelper.WaitForClusterSuccessInCgu(
				rantalmhelper.HubAPIClient,
				cgu.Name,
				rantalmhelper.Spoke2Name,
				rantalmparameters.TalmTestNamespace,
				7*time.Minute,
			)
			Expect(err).ToNot(HaveOccurred())

			By("waiting for the cgu to timeout")
			err = rantalmhelper.WaitForCguToTimeout(cgu.Name, rantalmparameters.TalmTestNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred())

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
func getNewPrecacheCGU(curName string, policyNames []string, spokes []string) v1alpha1.ClusterGroupUpgrade {
	cgu := rantalmhelper.GetCguDefinition(
		fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, curName),
		spokes,
		[]string{},
		policyNames,
		rantalmparameters.TalmTestNamespace, 2, 250)
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
	}, 15*time.Minute, 5*time.Second).Should(Equal(expectation))
}

// assertBackupPodLog retrieves the backup pod generated by job and asserts on the log.
func assertPrecachePodLog(client *testClient.ClientSet, expectationSubString string) {
	// added Eventually since logs take longer to show up in console
	Eventually(func() string {
		podList, err := client.Pods(SpokeNS).List(context.Background(), metav1.ListOptions{
			LabelSelector: PreCachePodLabel,
		})
		Expect(err).To(BeNil())
		Expect(len(podList.Items)).To(BeNumerically("==", 1))
		p := podList.Items[0]
		plog, err := pod.GetLog(client, &p, -time.Until(p.CreationTimestamp.Time), PreCacheContainerName)
		Expect(err).To(BeNil())
		log.Println("generated pod logs: \n", plog)

		return plog
	}, 1*time.Minute, 5*time.Second).Should(ContainSubstring(expectationSubString))
}
