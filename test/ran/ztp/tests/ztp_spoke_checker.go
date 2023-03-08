package tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
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
			It("should disable the chronyd service on spoke", func() {
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
		})
})
