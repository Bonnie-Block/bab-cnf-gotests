package tests

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/bfd/netbfdhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/bfd/netbfdparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/netparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/execute"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"

	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	workerNodesAddresses []string
	masterNodePod        *k8sv1.Pod
)

var _ = Describe("BFD", func() {
	execute.BeforeAll(func() {

		By("Getting ip addresses of worker and master nodes")
		masterNodes, err := nodes.GetByRole(helper.Apiclient, parameters.RoleMaster)
		Expect(err).ToNot(HaveOccurred())
		masterNodesAddresses := nethelper.NodeIPsForFamily(masterNodes, netparameters.IPV4Family)

		workerNodes, err := nodes.GetByRole(helper.Apiclient, parameters.RoleWorker)
		Expect(err).ToNot(HaveOccurred())
		workerNodesAddresses = nethelper.NodeIPsForFamily(workerNodes, netparameters.IPV4Family)
		Expect(err).ToNot(HaveOccurred())

		By("Creating configmaps")
		workerConfigMap := netbfdhelper.DefineBFDConfigMap(masterNodesAddresses, netbfdparameters.WorkerConfigMapName)
		_, err = helper.Apiclient.ConfigMaps(netbfdparameters.TestNamespace).Create(
			context.TODO(),
			workerConfigMap,
			metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		masterConfigMap := netbfdhelper.DefineBFDConfigMap(workerNodesAddresses, netparameters.MasterConfigMapName)
		_, err = helper.Apiclient.ConfigMaps(netbfdparameters.TestNamespace).Create(
			context.TODO(),
			masterConfigMap,
			metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		By("Creating a role")
		role := netbfdhelper.DefineBFDRole()
		_, err = helper.Apiclient.Roles(netbfdparameters.TestNamespace).Create(
			context.TODO(),
			role,
			metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		By("Creating a role binding")
		roleBinding := netbfdhelper.DefineBFDRoleBinding()
		_, err = helper.Apiclient.RoleBindings(netbfdparameters.TestNamespace).Create(
			context.TODO(),
			roleBinding,
			metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		By("Creating a daemonset")
		daemonset := netbfdhelper.DefineBFDDaemonset()
		_, err = helper.Apiclient.DaemonSets(netbfdparameters.TestNamespace).Create(
			context.TODO(),
			daemonset,
			metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		By("Checking the daemonset is in running state")
		Eventually(func() error {
			return helper.IsDaemonsetReady(helper.Apiclient,
				netbfdparameters.TestNamespace, netbfdparameters.AppName)
		}, netbfdparameters.WaitingTime, netbfdparameters.Interval).Should(BeNil(),
			"daemonset failed to get into running state")

		By("Creating FRR container on a Master node")
		frrPod := nethelper.DefineFRRPod(masterNodes[0].Name, netbfdparameters.TestNamespace, true)
		masterNodePod = helper.WaitUntilPodCreatedAndRunning(frrPod, netbfdparameters.WaitingTime)
	})

	It("Should have BFD status up", func() {
		Eventually(func() error {
			return netbfdhelper.IsBFDStatusUp(masterNodePod, workerNodesAddresses)
		}, netbfdparameters.WaitingTime, netbfdparameters.Interval).ShouldNot(HaveOccurred())
	})
})
