package helper

import (
	"context"
	"fmt"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

var podWaitingTime time.Duration = 5 * time.Minute

// PullTestImage pulls test image on all relevant nodes
func PullTestImage(cnfNodeLabel string, image string) {
	nodesList, err := nodes.GetByLabel(Apiclient, cnfNodeLabel)
	Expect(err).ToNot(HaveOccurred())
	for _, node := range nodesList.Items {
		pullPodDefenition := pod.RedefineWithRestartPolicy(pod.RedefineWithCommand(pod.DefinePodOnNode("default", image, node.Name),
			[]string{"echo", "image pulled Successfully && exit 0"}, []string{}), k8sv1.RestartPolicyNever)
		pullPod, err := Apiclient.Pods("default").Create(context.Background(), pullPodDefenition, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() k8sv1.PodPhase {
			pullPod, _ = Apiclient.Pods("default").Get(context.Background(), pullPod.Name, metav1.GetOptions{})
			return pullPod.Status.Phase
		}, podWaitingTime, time.Second).Should(Equal(k8sv1.PodSucceeded), fmt.Sprint("Invalid pulling image"))
	}
	err = namespaces.CleanPods("default", Apiclient)
	Expect(err).ToNot(HaveOccurred())
}

