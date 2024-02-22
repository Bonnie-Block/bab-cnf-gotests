package tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/polarion"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Test configurations applied via ZTP", Ordered, Label("ztp-spoke-checker"), func() {
	var (
		snoNode corev1.Node
		snoName string
	)

	BeforeAll(func() {
		// get SNO
		nodeList, err := helper.Apiclient.Nodes().List(context.Background(), metav1.ListOptions{})
		Expect(err).ToNot(HaveOccurred())
		snoNode = nodeList.Items[0]
		snoName = snoNode.Name
	})

	Context("when a TunedPerformancePatch.yaml PGT disables the chronyd service",
		Label("ztp-chronyd-disable"), func() {
			// https://issues.redhat.com/browse/CNF-6298
			// 54237
			It("should disable the chronyd service on spoke", polarion.ID("54237"), func() {
				By("Checking chronyd status")
				status, err := helper.ExecCommandOnNode(&snoNode,
					[]string{"systemctl", "is-active", "chronyd.service"})
				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error on executing systemctl on %s", snoName))
				Expect(status == "inactive")
				status, err = helper.ExecCommandOnNode(&snoNode,
					[]string{"systemctl", "is-enabled", "chronyd.service"})
				Expect(err).To(HaveOccurred(), fmt.Sprintf("Error on executing systemctl on %s", snoName))
				Expect(status == "disabled")
			})
			// 60904
			It("verifies list of pods in "+ran.NamespaceNetdiag+" namespace on spoke", polarion.ID("60904"), func() {
				By("checking pods do not exist in " + ran.NamespaceNetdiag)
				networkDiagPods, err := helper.Apiclient.Pods(ran.NamespaceNetdiag).List(context.Background(), metav1.ListOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(networkDiagPods.Items).To(BeEmpty(), "Pods exist in ", ran.NamespaceNetdiag)
			})
			// 60905
			It("verifies list of pods in "+ran.NamespaceConsole+" namespace on spoke", polarion.ID("60905"), func() {
				By("checking pods do not exist in " + ran.NamespaceConsole)
				consolePods, err := helper.Apiclient.Pods(ran.NamespaceConsole).List(context.Background(), metav1.ListOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(consolePods.Items).To(BeEmpty(), "Pods exist in ", ran.NamespaceConsole)
			})
		})
})
