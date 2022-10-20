package tests

import (
	"context"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/cluster-group-upgrades-operator/api/v1alpha1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	testClient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
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
	CGUName               = "generated-precache-operator-multi-catsrc"
	SpokeNS               = "openshift-talo-pre-cache"
	PreCacheContainerName = "pre-cache-container"
	PreCachePodLabel      = "job-name=pre-cache"
	PreCacheJobName       = "pre-cache"
)

var _ = Describe("Talm precache", func() {
	var (
		hub                  *testClient.ClientSet
		spoke1Name           string
		hubNs                = ran.NamespaceTesting                   // ns used by generated CGU
		spoke1               = helper.Apiclient                       // initialized automatically with KUBECONFIG
		talmPrecachePolicies = helper.Config.Ran.TalmPrecachePolicies // override with env TALM_PRECACHE_POLICIES
	)

	Describe("Precache operator with multiple sources", func() {

		BeforeEach(func() {
			hub = rantalmhelper.GetHubclient()
			spoke1Name = rantalmhelper.GetSpoke1Name()

			By("verifying list of policies in config are already available in hub. Required Precache multi src")
			var listPolicy policiesv1.PolicyList
			err := hub.List(context.Background(), &listPolicy, runtimeclient.InNamespace(""))
			if err != nil {
				log.Println(err)
				Skip("couldn't list all policies from all namespaces")
			}
			if !rantalmhelper.AllPoliciesExist(listPolicy) {
				Skip("couldn't not find all the policies specified in config")
			}

			By("deleting existing CGU of the same name if exists")
			err = hub.ClustergroupupgradesoperatorV1alpha1Interface.
				ClusterGroupUpgrades(hubNs).Delete(context.Background(), CGUName, metav1.DeleteOptions{})
			if err != nil && !errors.IsNotFound(err) {
				log.Println(err)
				Skip("could not delete the existing cgu")
			}
		})

		It("tests for precache operator with multiple sources", func() {
			By("generating new CGU with precache enabled")
			cguToTest := v1alpha1.ClusterGroupUpgrade{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ClusterGroupUpgrade",
					APIVersion: v1alpha1.SchemeGroupVersion.Version,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      CGUName,
					Namespace: hubNs,
				},
				Spec: v1alpha1.ClusterGroupUpgradeSpec{
					PreCaching:      true,
					Enable:          BoolAddr(false),
					Clusters:        []string{spoke1Name},
					ManagedPolicies: talmPrecachePolicies,
					RemediationStrategy: &v1alpha1.RemediationStrategySpec{
						MaxConcurrency: 1,
					},
				},
			}

			clusterGroupUpgrade, err := hub.ClustergroupupgradesoperatorV1alpha1Interface.
				ClusterGroupUpgrades(hubNs).Create(context.Background(), &cguToTest, metav1.CreateOptions{})

			Expect(err).To(BeNil())
			Expect(clusterGroupUpgrade.Name).To(Equal(CGUName))

			y, err := yaml.Marshal(cguToTest)
			Expect(err).To(BeNil())
			log.Printf("--- generated CGU dump:\n%s\n\n", string(y))

			By("waiting until CGU starts precaching")
			Eventually(func() string {
				cgu, err := hub.ClustergroupupgradesoperatorV1alpha1Interface.
					ClusterGroupUpgrades(hubNs).Get(context.Background(), CGUName, metav1.GetOptions{})
				Expect(err).To(BeNil())

				if cgu.Status.Precaching == nil {
					log.Println("precaching struct not ready yet")

					return ""
				}

				_, ok := cgu.Status.Precaching.Status[spoke1Name]
				if !ok {
					log.Println("cluster name as key did not appear yet")

					return ""
				}

				log.Println("current pre-cache status: " + cgu.Status.Precaching.Status[spoke1Name])

				return cgu.Status.Precaching.Status[spoke1Name]
			}, 5*time.Minute, 10*time.Second).Should(Equal("Starting"))

			By("waiting until new precache pod in spoke1 succeeded")
			Eventually(func() k8sv1.PodPhase {
				newlygenpods, _ := spoke1.Pods(SpokeNS).List(context.Background(), metav1.ListOptions{
					LabelSelector: PreCachePodLabel,
				})
				for _, p := range newlygenpods.Items {
					log.Println("current precache pod status: ", p.Status.Phase)

					return p.Status.Phase
				}
				log.Println("no pods yet")

				return ""
			}, 5*time.Minute, 10*time.Second).Should(Equal(k8sv1.PodSucceeded))

			By("verifying precache pod's logs in spoke1")
			Eventually(func() string {
				newlygenpods, _ := spoke1.Pods(SpokeNS).List(context.Background(), metav1.ListOptions{
					LabelSelector: PreCachePodLabel,
				})
				p := newlygenpods.Items[0]
				plog, err := pod.GetLog(spoke1, &p, -time.Until(p.CreationTimestamp.Time), PreCacheContainerName)
				Expect(err).To(BeNil())
				log.Println("generated pod logs: \n", plog)

				return plog
			}, 3*time.Minute, 5*time.Second).Should(ContainSubstring("Image pre-cache done"))

			By("waiting until CGU finishes with Succeeded in hub")
			Eventually(func() string {
				cgu, err := hub.ClustergroupupgradesoperatorV1alpha1Interface.
					ClusterGroupUpgrades(hubNs).Get(context.Background(), CGUName, metav1.GetOptions{})
				if err != nil {
					return err.Error()
				}
				log.Println("current pre-cache status: " + cgu.Status.Precaching.Status[spoke1Name])

				return cgu.Status.Precaching.Status[spoke1Name]
			}, 10*time.Minute, 10*time.Second).Should(Equal("Succeeded"))
		})

		AfterEach(func() {
			By("deleting generated CGU")
			err := hub.ClustergroupupgradesoperatorV1alpha1Interface.
				ClusterGroupUpgrades(hubNs).Delete(context.Background(), CGUName, metav1.DeleteOptions{})
			if err != nil {
				log.Println("could not delete generated CGU in hub:", err)
			}

			By("deleting generated spoke job")
			err = spoke1.BatchV1Interface.Jobs(SpokeNS).Delete(context.Background(), PreCacheJobName, metav1.DeleteOptions{})
			if err != nil {
				log.Println("could delete job spoke:", err)
			}
		})
	})
})
