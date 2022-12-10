package tests

import (
	"context"
	"fmt"
	"log"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	k8sv1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/cluster-group-upgrades-operator/api/v1alpha1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmparameters"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	configurationPolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
)

// precache const.
const (
	SpokeNS               = "openshift-talo-pre-cache"
	PreCacheContainerName = "pre-cache-container"
	PreCachePodLabel      = "job-name=pre-cache"
)

var _ = Describe("Talm precache one spoke", func() {

	Context("Precache operator", func() {
		curName := "precache-operator"
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

		It("tests for precache operator with multiple sources", func() {
			By("creating CGU with created operator upgrade policy")
			// prep cgu with one spoke
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

var _ = Describe("Talm precache with multiple spokes where one turns off", Ordered, func() {
	curName := "precache-multiple-spoke"
	var nodeToTurnOff *k8sv1.Node

	BeforeEach(func() {
		By("turning off spoke1")
		ranhelper.PowerOffSnoWithIpmi()

		// keep a copy of the node before turning off
		nodeList, _ := rantalmhelper.Spoke1APIClient.Nodes().List(context.Background(), metav1.ListOptions{})
		nodeToTurnOff = &nodeList.Items[0]
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

	It("fails for one spoke and succeeds for the other", func() {
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

	AfterAll(func() {
		log.Println("turning on spoke1")
		ranhelper.PowerOnSnoWithImpi()

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
