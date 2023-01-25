package tests

import (
	"fmt"
	"log"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"

	"k8s.io/apimachinery/pkg/runtime"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmparameters"
	configurationPolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/cluster-group-upgrades-operator/api/v1alpha1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/talm/rantalmhelper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	blockingA = "blocking-a"
	blockingB = "blocking-b"
)

var _ = Describe("Talm Blocking CRs Tests", Ordered, Label("talmblockingcr"), func() {

	var (
		cguA v1alpha1.ClusterGroupUpgrade
		cguB v1alpha1.ClusterGroupUpgrade
	)

	//  ocp-47948
	Context("when blocking CR passes", func() {
		blockingAPass := fmt.Sprintf("%s-pass", blockingA)
		blockingBPass := fmt.Sprintf("%s-pass", blockingB)

		AfterEach(func() {
			// delete generated blocking CRs
			cleanUpGeneratedBlockingCrs(blockingAPass)
			cleanUpGeneratedBlockingCrs(blockingBPass)
			// delete generated ns
			deleteGeneratedNs(blockingAPass)
			deleteGeneratedNs(blockingBPass)
		})

		It("verifies CGU succeeded with blocking CR", func() {
			By("creating two sets of CRs where b will be blocked until a is done")
			// cguA
			cguA = getNewBlockingCGU(blockingAPass, 10)
			// cguB
			cguB = getNewBlockingCGU(blockingBPass, 10)
			cguB.Spec.BlockingCRs = []v1alpha1.BlockingCR{
				{
					Name:      fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, blockingAPass),
					Namespace: rantalmparameters.TalmTestNamespace,
				},
			}

			// apply
			nsForBlockingA := rantalmhelper.
				GetNamespaceDefinition(fmt.Sprintf("%s-%s", rantalmparameters.NsCommonName, blockingAPass))
			err := applyBlockingCrs(cguA, nsForBlockingA, blockingAPass)
			Expect(err).To(BeNil())

			nsForBlockingB := rantalmhelper.
				GetNamespaceDefinition(fmt.Sprintf("%s-%s", rantalmparameters.NsCommonName, blockingBPass))
			err = applyBlockingCrs(cguB, nsForBlockingB, blockingBPass)
			Expect(err).To(BeNil())

			By("waiting for the system to settle", func() {
				time.Sleep(rantalmparameters.TalmSystemStablizationTime)
			})

			// enable cguA first
			err = rantalmhelper.EnableCgu(rantalmhelper.HubAPIClient, cguA)
			Expect(err).To(BeNil())
			err = rantalmhelper.EnableCgu(rantalmhelper.HubAPIClient, cguB)
			Expect(err).To(BeNil())

			By("waiting to verify if cgu B is blocked by A")
			blockErrMsg := fmt.Sprintf("Blocking CRs that are not completed: "+
				"[%s]", fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, blockingAPass))

			// TALM 4.11 and below had a different error message
			if !ranhelper.IsVersionStringInRange(
				rantalmhelper.TalmHubVersion,
				rantalmparameters.TalmUpdatedConditionsVersion,
				"",
			) {
				blockErrMsg = fmt.Sprintf("The ClusterGroupUpgrade "+
					"CR is blocked by other CRs that have not yet completed: "+
					"[%s]", fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, blockingAPass))
			}

			err = verifyCguBlocked(cguB, blockErrMsg)
			Expect(err).To(BeNil())

			// Validating the conditions depends on the TALM version
			completedType := rantalmhelper.SucceededType
			if !ranhelper.IsVersionStringInRange(
				rantalmhelper.TalmHubVersion,
				rantalmparameters.TalmUpdatedConditionsVersion,
				"",
			) {
				completedType = rantalmhelper.ReadyType
			}

			By("waiting for cgu A to succeed")
			err = rantalmhelper.WaitForCguInCondition(
				rantalmhelper.HubAPIClient,
				cguA.Name,
				cguA.Namespace,
				completedType,
				"",
				metav1.ConditionTrue,
				"",
				15*time.Minute)
			Expect(err).To(BeNil())

			By("waiting for cgu B to succeed")
			err = rantalmhelper.WaitForCguInCondition(
				rantalmhelper.HubAPIClient,
				cguB.Name,
				cguB.Namespace,
				completedType,
				"",
				metav1.ConditionTrue,
				"",
				15*time.Minute)
			Expect(err).To(BeNil())
		})
	})

	//  ocp-47948
	Context("when blocking CR fails", func() {
		blockingAFail := fmt.Sprintf("%s-fail", blockingA)
		blockingBFail := fmt.Sprintf("%s-fail", blockingB)

		AfterEach(func() {
			// delete generated blocking CRs
			cleanUpGeneratedBlockingCrs(blockingAFail)
			cleanUpGeneratedBlockingCrs(blockingBFail)
		})

		It("verifies CGU fails with blocking CR", func() {
			// cguA using a small timeout value to simulate A failed
			cguA = getNewBlockingCGU(blockingAFail, 2)
			// cguB
			cguB = getNewBlockingCGU(blockingBFail, 1)
			cguB.Spec.BlockingCRs = []v1alpha1.BlockingCR{
				{
					Name:      fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, blockingAFail),
					Namespace: rantalmparameters.TalmTestNamespace,
				},
			}

			// apply
			nsForBlockingA := rantalmhelper.
				GetNamespaceDefinition(fmt.Sprintf("%s-%s", rantalmparameters.NsCommonName, blockingAFail))
			nsForBlockingA.Kind = nsForBlockingA.APIVersion // faulty ns def for artificial timeout
			err := applyBlockingCrs(cguA, nsForBlockingA, blockingAFail)
			Expect(err).To(BeNil())

			nsForBlockingB := rantalmhelper.
				GetNamespaceDefinition(fmt.Sprintf("%s-%s", rantalmparameters.NsCommonName, blockingBFail))
			err = applyBlockingCrs(cguB, nsForBlockingB, blockingBFail)
			Expect(err).To(BeNil())

			By("waiting for the system to settle", func() {
				time.Sleep(rantalmparameters.TalmSystemStablizationTime)
			})

			// enable cguA first
			err = rantalmhelper.EnableCgu(rantalmhelper.HubAPIClient, cguA)
			Expect(err).To(BeNil())
			err = rantalmhelper.EnableCgu(rantalmhelper.HubAPIClient, cguB)
			Expect(err).To(BeNil())

			By("waiting to verify if cgu B is blocked by A")
			blockErrMsg := fmt.Sprintf(
				"Blocking CRs that are not completed: [%s]",
				fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, blockingAFail))

			// TALM 4.11 and below had a different error message
			if !ranhelper.IsVersionStringInRange(
				rantalmhelper.TalmHubVersion,
				rantalmparameters.TalmUpdatedConditionsVersion,
				"",
			) {
				blockErrMsg = fmt.Sprintf("The ClusterGroupUpgrade "+
					"CR is blocked by other CRs that have not yet "+
					"completed: [%s]", fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, blockingAFail))
			}

			err = verifyCguBlocked(cguB, blockErrMsg)
			Expect(err).To(BeNil())

			By("waiting for cgu A to fail because of timeout")
			// Validating the conditions depends on the TALM version
			completedType := rantalmhelper.SucceededType
			completedMessage := rantalmhelper.Talm412TimeoutMessage
			if !ranhelper.IsVersionStringInRange(
				rantalmhelper.TalmHubVersion,
				rantalmparameters.TalmUpdatedConditionsVersion,
				"",
			) {
				completedType = rantalmhelper.ReadyType
				completedMessage = rantalmhelper.Talm411TimeoutMessage
			}

			err = rantalmhelper.WaitForCguInCondition(
				rantalmhelper.HubAPIClient,
				cguA.Name,
				cguA.Namespace,
				completedType,
				completedMessage,
				metav1.ConditionFalse,
				"",
				7*time.Minute,
			)
			Expect(err).To(BeNil())

			By("verify cgu B is still blocked")
			err = verifyCguBlocked(cguB, blockErrMsg)
			Expect(err).To(BeNil())
		})
	})

	//  ocp-47948
	Context("when blocking CR missing", func() {
		blockingAMissing := fmt.Sprintf("%s-missing", blockingA)
		blockingBMissing := fmt.Sprintf("%s-missing", blockingB)

		AfterEach(func() {
			// delete generated blocking CRs
			cleanUpGeneratedBlockingCrs(blockingAMissing)
			cleanUpGeneratedBlockingCrs(blockingBMissing)
			// delete generated ns
			deleteGeneratedNs(blockingAMissing)
			deleteGeneratedNs(blockingBMissing)
		})

		It("verifies CGU is blocked until blocking CR created and succeeded", func() {
			// cguA using a small timeout value to simulate A failed
			cguA = getNewBlockingCGU(blockingAMissing, 5)
			// cguB
			cguB = getNewBlockingCGU(blockingBMissing, 5)
			cguB.Spec.BlockingCRs = []v1alpha1.BlockingCR{
				{
					Name:      fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, blockingAMissing),
					Namespace: rantalmparameters.TalmTestNamespace,
				},
			}
			// apply and enable B
			nsForBlockingB := rantalmhelper.
				GetNamespaceDefinition(fmt.Sprintf("%s-%s", rantalmparameters.NsCommonName, blockingBMissing))
			err := applyBlockingCrs(cguB, nsForBlockingB, blockingBMissing)
			Expect(err).To(BeNil())

			By("waiting for the system to settle", func() {
				time.Sleep(rantalmparameters.TalmSystemStablizationTime)
			})

			err = rantalmhelper.EnableCgu(rantalmhelper.HubAPIClient, cguB)
			Expect(err).To(BeNil())

			// check that B is reporting Missing
			By("waiting to verify if cgu B is blocked by A because it's missing")
			blockErrMsg := fmt.Sprintf("Missing blocking CRs: [%s]",
				fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, blockingAMissing))

			// TALM 4.11 and below had a different error message
			if !ranhelper.IsVersionStringInRange(
				rantalmhelper.TalmHubVersion,
				rantalmparameters.TalmUpdatedConditionsVersion,
				"",
			) {
				blockErrMsg = fmt.Sprintf("The ClusterGroupUpgrade CR has blocking CRs that are missing: [%s]",
					fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, blockingAMissing))
			}

			err = verifyCguBlocked(cguB, blockErrMsg)
			Expect(err).To(BeNil())

			// apply and enable A
			By("by apply all A crs")
			nsForBlockingA := rantalmhelper.
				GetNamespaceDefinition(fmt.Sprintf("%s-%s", rantalmparameters.NsCommonName,
					blockingAMissing))
			err = applyBlockingCrs(cguA, nsForBlockingA, blockingAMissing)
			Expect(err).To(BeNil())

			err = rantalmhelper.EnableCgu(rantalmhelper.HubAPIClient, cguA)
			Expect(err).To(BeNil())

			// Validating the conditions depends on the TALM version
			completedType := rantalmhelper.SucceededType
			if !ranhelper.IsVersionStringInRange(
				rantalmhelper.TalmHubVersion,
				rantalmparameters.TalmUpdatedConditionsVersion,
				"",
			) {
				completedType = rantalmhelper.ReadyType
			}

			By("waiting for cgu A to succeed")
			err = rantalmhelper.WaitForCguInCondition(
				rantalmhelper.HubAPIClient,
				cguA.Name,
				cguA.Namespace,
				completedType,
				"",
				metav1.ConditionTrue,
				"",
				6*time.Minute)
			Expect(err).To(BeNil())

			By("waiting for cgu B to succeed")
			err = rantalmhelper.WaitForCguInCondition(
				rantalmhelper.HubAPIClient,
				cguB.Name,
				cguB.Namespace,
				completedType,
				"",
				metav1.ConditionTrue,
				"",
				6*time.Minute)
			Expect(err).To(BeNil())
		})
	})
})

// getNewBlockingCGU get new blocking CGU CR.
func getNewBlockingCGU(curName string, timeout int) v1alpha1.ClusterGroupUpgrade {
	cgu := rantalmhelper.GetCguDefinition(
		fmt.Sprintf("%s-%s", rantalmparameters.CguCommonName, curName),
		[]string{rantalmhelper.Spoke1Name},
		[]string{},
		[]string{fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, curName)},
		rantalmparameters.TalmTestNamespace, 1, timeout)

	log.Println("setting cgu Enable to false")

	cgu.Spec.Enable = rantalmhelper.BoolAddr(false)

	return cgu
}

// applyBlockingCrs apply common blocking crs.
func applyBlockingCrs(cgu v1alpha1.ClusterGroupUpgrade, object runtime.Object, currName string) error {
	err := rantalmhelper.CreatePolicyAndCgu(
		rantalmhelper.HubAPIClient,
		object,
		configurationPolicyv1.MustHave,
		configurationPolicyv1.Inform,
		fmt.Sprintf("%s-%s", rantalmparameters.PolicyNameCommonName, currName),
		fmt.Sprintf("%s-%s", rantalmparameters.PolicySetNameCommonName, currName),
		fmt.Sprintf("%s-%s", rantalmparameters.PlacementBindingCommonName, currName),
		fmt.Sprintf("%s-%s", rantalmparameters.PlacementRuleCommonName, currName),
		rantalmparameters.TalmTestNamespace,
		metav1.LabelSelector{},
		cgu,
	)

	return err
}

// verifyCguBlocked verify cr is blocked.
func verifyCguBlocked(cgu v1alpha1.ClusterGroupUpgrade, blockedByMsg string) error {
	// Validating the conditions depends on the TALM version
	expectedType := rantalmhelper.ProgressingType

	if !ranhelper.IsVersionStringInRange(
		rantalmhelper.TalmHubVersion,
		rantalmparameters.TalmUpdatedConditionsVersion,
		"",
	) {
		expectedType = rantalmhelper.ReadyType
	}

	if !ranhelper.IsVersionStringInRange(
		rantalmhelper.TalmHubVersion,
		"4.11",
		"",
	) {
		blockedByMsg = ""
	}

	return rantalmhelper.WaitForCguInCondition(rantalmhelper.HubAPIClient,
		cgu.Name,
		cgu.Namespace,
		expectedType,
		blockedByMsg,
		metav1.ConditionFalse,
		"", 6*time.Minute)
}

// deleteGeneratedNs clean up ns created by blocking CRs.
func deleteGeneratedNs(curName string) {
	err := namespaces.DeleteAndWait(
		rantalmhelper.Spoke1APIClient,
		fmt.Sprintf("%s-%s", rantalmparameters.NsCommonName, curName),
		5*time.Minute)
	if err != nil {
		log.Println("error deleting ns: ", err)
	}
}

func cleanUpGeneratedBlockingCrs(curName string) {
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
}
