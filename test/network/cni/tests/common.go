package tests

import (
	"context"
	"fmt"
	"time"

	k8sv1 "k8s.io/api/core/v1"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/gomega"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// PingIPViaInterface runs ping on the given pod and returns exit code.
func PingIPViaInterface(clientPod k8sv1.Pod, vrfName string, destIPAddr string, negative bool) error {
	command := []string{"testcmd", "-interface", vrfName, "-server", destIPAddr, "-protocol", "icmp", "-mtu", "100"}
	if negative {
		command = append(command, "--negative")
	}

	_, err := pod.ExecCommand(
		helper.Apiclient,
		clientPod,
		command)

	return err
}

// WaitUntilSriovBecomesStable waits until sr-iov operator become stable.
func WaitUntilSriovBecomesStable() {
	var snoTimeoutMultiplier time.Duration = 1

	isSingleNode, err := nodes.IsSingleNodeCluster(helper.Apiclient)
	Expect(err).ToNot(HaveOccurred(), "error to check if cluster if SNO")

	if isSingleNode {
		snoTimeoutMultiplier = 2
		disableDrainState := helper.GetNodeDrainState(parameters.SriovOperatorNamespace)

		if !disableDrainState {
			helper.SetDisableNodeDrainState(true, parameters.SriovOperatorNamespace)
			helper.ChangedNodeDrainState = true
		}
	}

	helper.WaitForSRIOVStable(parameters.SriovOperatorNamespace, netcniparameters.WaitingTime, snoTimeoutMultiplier)
}

// WaitUntilSriovNadListBecomeAvailable waits until sr-iov configures nads in the needed namespace.
func WaitUntilSriovNadListBecomeAvailable(nadListNames []string) {
	for _, sriovNetworkName := range nadListNames {
		Eventually(func() error {
			netAttDef := &netattdefv1.NetworkAttachmentDefinition{}

			return helper.Apiclient.Get(
				context.Background(),
				runtimeclient.ObjectKey{
					Name:      sriovNetworkName,
					Namespace: netcniparameters.TestNamespace},
				netAttDef)
		}, 60*time.Second, 1*time.Second).ShouldNot(HaveOccurred(),
			"error occurred while waiting for NetworkAttachmentDefinition to be up ")
	}
}

// CleanSriovNetworkSriovPolicyNadsAndPods removes all sr-iov network, policy, nads and pods.
func CleanSriovNetworkSriovPolicyNadsAndPods() {
	err := namespaces.Clean(
		parameters.SriovOperatorNamespace,
		netcniparameters.TestNamespace,
		helper.Apiclient,
		false)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error to clean namespace %s", netcniparameters.TestNamespace))
}
