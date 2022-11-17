package tests

import (
	"fmt"
	"log"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmparameters"
	configurationPolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/cluster-group-upgrades-operator/api/v1alpha1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	policyNameCommonName       = "generated-policy"
	policySetNameCommonName    = "generated-policyset"
	placementRuleCommonName    = "generated-placementrule"
	placementBindingCommonName = "generated-placementbinding"
	cGUCommonName              = "generated-cgu"
	nsCommonName               = "generated-namespace"

	blockingA = "blocking-a"
	blockingB = "blocking-b"

	cguSuccessBlockingMsgA = "The ClusterGroupUpgrade CR has all clusters compliant with all the managed policies"
	cguSuccessBlockingMsgB = "The ClusterGroupUpgrade CR has all clusters " +
		"already compliant with the specified managed policies"
)

var _ = Describe("Talm Blocking CRs Tests", func() {

	var (
		cguA v1alpha1.ClusterGroupUpgrade
		cguB v1alpha1.ClusterGroupUpgrade
	)

	//  ocp-47948
	It("verifies CGU succeeded with blocking CR", func() {
		By("creating two sets of CRs where b will be blocked until a is done")
		// cguA
		cguA = getNewBlockingCGU(blockingA)
		// cguB
		cguB = getNewBlockingCGU(blockingB)
		cguB.Spec.BlockingCRs = []v1alpha1.BlockingCR{
			{
				Name:      fmt.Sprintf("%s-%s", cGUCommonName, blockingA),
				Namespace: rantalmparameters.TalmTestNamespace,
			},
		}

		// apply
		nsForBlockingA := rantalmhelper.GetNamespaceDefinition(fmt.Sprintf("%s-%s", nsCommonName, blockingB))
		err := rantalmhelper.CreatePolicyAndCgu(
			rantalmhelper.HubAPIClient,
			nsForBlockingA,
			configurationPolicyv1.MustHave,
			configurationPolicyv1.Inform,
			fmt.Sprintf("%s-%s", policyNameCommonName, blockingA),
			fmt.Sprintf("%s-%s", policySetNameCommonName, blockingA),
			fmt.Sprintf("%s-%s", placementBindingCommonName, blockingA),
			fmt.Sprintf("%s-%s", placementRuleCommonName, blockingA),
			rantalmparameters.TalmTestNamespace,
			metav1.LabelSelector{},
			cguA,
		)
		Expect(err).To(BeNil())

		nsForBlockingB := rantalmhelper.GetNamespaceDefinition(fmt.Sprintf("%s-%s", nsCommonName, blockingB))
		err = rantalmhelper.CreatePolicyAndCgu(
			rantalmhelper.HubAPIClient,
			nsForBlockingB,
			configurationPolicyv1.MustHave,
			configurationPolicyv1.Inform,
			fmt.Sprintf("%s-%s", policyNameCommonName, blockingB),
			fmt.Sprintf("%s-%s", policySetNameCommonName, blockingB),
			fmt.Sprintf("%s-%s", placementBindingCommonName, blockingB),
			fmt.Sprintf("%s-%s", placementRuleCommonName, blockingB),
			rantalmparameters.TalmTestNamespace,
			metav1.LabelSelector{},
			cguB,
		)
		Expect(err).To(BeNil())

		// enable cguA first
		err = rantalmhelper.EnableCgu(rantalmhelper.HubAPIClient, cguA)
		Expect(err).To(BeNil())
		err = rantalmhelper.EnableCgu(rantalmhelper.HubAPIClient, cguB)
		Expect(err).To(BeNil())

		By("waiting to verify if cgu B is blocked by A")
		err = rantalmhelper.WaitForCguInCondition(rantalmhelper.HubAPIClient,
			cguB.Name,
			cguB.Namespace,
			"Ready",
			fmt.Sprintf("The ClusterGroupUpgrade CR is blocked by other "+
				"CRs that have not yet completed: [%s]", fmt.Sprintf("%s-%s", cGUCommonName, blockingA)),
			metav1.ConditionFalse, "", 5*time.Minute)
		Expect(err).To(BeNil())

		By("waiting for cgu A to succeed")
		err = rantalmhelper.WaitForCguInCondition(rantalmhelper.HubAPIClient,
			cguA.Name,
			cguA.Namespace,
			"Ready",
			cguSuccessBlockingMsgA,
			metav1.ConditionTrue, "", 5*time.Minute)
		Expect(err).To(BeNil())

		By("waiting for cgu B to succeed")
		err = rantalmhelper.WaitForCguInCondition(rantalmhelper.HubAPIClient,
			cguB.Name,
			cguB.Namespace,
			"Ready",
			cguSuccessBlockingMsgB,
			metav1.ConditionTrue, "", 5*time.Minute)
		Expect(err).To(BeNil())
	})

})

// getNewBlockingCGU get new blocking CGU CR.
func getNewBlockingCGU(curName string) v1alpha1.ClusterGroupUpgrade {
	cgu := rantalmhelper.GetCguDefinition(
		fmt.Sprintf("%s-%s", cGUCommonName, curName),
		[]string{rantalmhelper.Spoke1Name},
		[]string{},
		[]string{fmt.Sprintf("%s-%s", policyNameCommonName, curName)},
		rantalmparameters.TalmTestNamespace, 1, 240)

	log.Println("setting cgu Enable to false")

	cgu.Spec.Enable = rantalmhelper.BoolAddr(false)

	return cgu
}
