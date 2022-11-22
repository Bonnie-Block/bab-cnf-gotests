package netcnihelper

import (
	"context"
	"fmt"
	"strings"

	multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/cni/netcniparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"

	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CopyMap returns the new copy of given map.
func CopyMap(originalMap map[string]string) map[string]string {
	newMap := make(map[string]string)
	for key, value := range originalMap {
		newMap[key] = value
	}

	return newMap
}

// GetPodStatus returns pods status.
func GetPodStatus(podDefinition *k8sv1.Pod) k8sv1.PodPhase {
	tempPod, _ := helper.Apiclient.Pods(podDefinition.Namespace).Get(
		context.Background(),
		podDefinition.Name,
		metav1.GetOptions{})

	return tempPod.Status.Phase
}

// DefinePodWithInitContainers returns function that adds init container to pod manifest.
func DefinePodWithInitContainers(initContainers []*k8sv1.Container) func(podManifest *k8sv1.Pod) {
	return func(podManifest *k8sv1.Pod) {
		for _, initContainer := range initContainers {
			podManifest.Spec.InitContainers = append(podManifest.Spec.InitContainers,
				*initContainer)
		}
	}
}

// IsNamespacedEventListContainsMessage returns true/false in case if event is found/not found.
func IsNamespacedEventListContainsMessage(namespace string, message string) (bool, error) {
	eventsList, err := PullFailedCreatePodSandBoxEvents(netcniparameters.TestNamespace)

	if err != nil {
		return false, fmt.Errorf("error to collect events from the given namespace %s due to %w", namespace, err)
	}

	for _, event := range eventsList.Items {
		if strings.Contains(event.Message, message) {
			return true, nil
		}
	}

	return false, nil
}

// DefineServerNetCfg returns network configuration for server pod.
func DefineServerNetCfg(multipleIP bool, useBond bool) []multus.NetworkSelectionElement {
	serverIPs := []string{"10.100.100.200/24"}
	if multipleIP {
		serverIPs = append(serverIPs, "10.100.200.200/24")
	}

	return defineNetCfg(netcniparameters.NetworkWithoutSysctlMutation, serverIPs, useBond)
}

// DefineRedirectNetCfg returns network configuration for redirect pod.
func DefineRedirectNetCfg(multipleIP bool, useBond bool) []multus.NetworkSelectionElement {
	redirectIPs := []string{"10.100.100.1/24"}
	if multipleIP {
		redirectIPs = append(redirectIPs, "10.100.200.1/24")
	}

	return defineNetCfg(netcniparameters.NetworkWithoutSysctlMutation, redirectIPs, useBond)
}

// DefineClientNetCfg returns network configuration for client pod.
func DefineClientNetCfg(multipleIP bool, useBond bool) []multus.NetworkSelectionElement {
	clientNetCfg := defineNetCfg(netcniparameters.NetworkWithoutSysctlMutation, []string{"10.100.100.210/24"}, useBond)
	if multipleIP && useBond {
		clientNetCfg = append(
			clientNetCfg, defineNetCfg(netcniparameters.NetworkWithSysctlMutation, []string{"10.100.200.210/24"}, useBond)...)
		clientNetCfg[len(clientNetCfg)-1].InterfaceRequest = netcniparameters.BondInterfaceNameSecond

		return clientNetCfg
	}

	if multipleIP {
		clientNetCfg = append(
			clientNetCfg, defineNetCfg(netcniparameters.NetworkWithSysctlMutation, []string{"10.100.200.210/24"}, useBond)...)
	}

	return clientNetCfg
}

// PullFailedCreatePodSandBoxEvents returns list of event messages based on FailedCreatePodSandBox filer.
func PullFailedCreatePodSandBoxEvents(namespace string) (*k8sv1.EventList, error) {
	return pullEventsForFieldSelector(namespace, "reason=FailedCreatePodSandBox")
}

func pullEventsForFieldSelector(namespace string, fieldSelector string) (*k8sv1.EventList, error) {
	return helper.Apiclient.Events(namespace).List(context.Background(),
		metav1.ListOptions{FieldSelector: fieldSelector})
}

func defineNetCfg(netName string, ipAddr []string, useBond bool) []multus.NetworkSelectionElement {
	netParam := []multus.NetworkSelectionElement{
		{Name: netName, IPRequest: ipAddr}}
	if useBond {
		netParam[0].InterfaceRequest = netcniparameters.BondInterfaceName
		bondLinkConfig := multus.NetworkSelectionElement{Name: parameters.SriovPolicyName}
		netParam = append([]multus.NetworkSelectionElement{bondLinkConfig, bondLinkConfig}, netParam...)
	}

	return netParam
}
