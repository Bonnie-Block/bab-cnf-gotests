package namespaces

import (
	"context"
	"fmt"
	"strings"
	"time"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	k8sv1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	testclient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
)

// WaitForDeletion waits until the namespace will be removed from the cluster.
func WaitForDeletion(cs *testclient.ClientSet, nsName string, timeout time.Duration) error {
	return wait.PollImmediate(time.Second, timeout, func() (bool, error) {
		_, err := cs.Namespaces().Get(context.Background(), nsName, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			return true, nil
		}

		return false, nil
	})
}

// Create creates a new namespace with the given name.
// If the namespace exists, it returns.
func Create(namespace string, cs *testclient.ClientSet) error {
	_, err := cs.Namespaces().Create(context.Background(), &k8sv1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				"pod-security.kubernetes.io/audit":               "privileged",
				"pod-security.kubernetes.io/enforce":             "privileged",
				"pod-security.kubernetes.io/warn":                "privileged",
				"security.openshift.io/scc.podSecurityLabelSync": "false",
			},
		}}, metav1.CreateOptions{})

	if k8serrors.IsAlreadyExists(err) {
		return nil
	}

	return err
}

// DeleteAndWait deletes a namespace and waits until delete.
func DeleteAndWait(clientSet *testclient.ClientSet, namespace string, timeout time.Duration) error {
	err := clientSet.Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	return WaitForDeletion(clientSet, namespace, timeout)
}

// Exists tells whether the given namespace exists.
func Exists(namespace string, cs *testclient.ClientSet) bool {
	_, err := cs.Namespaces().Get(context.Background(), namespace, metav1.GetOptions{})

	return err == nil || !k8serrors.IsNotFound(err)
}

// CleanPods deletes all pods in namespace.
func CleanPods(namespace string, cs *testclient.ClientSet) error {
	if !Exists(namespace, cs) {
		return nil
	}

	err := cs.Pods(namespace).DeleteCollection(context.Background(), metav1.DeleteOptions{
		GracePeriodSeconds: pointer.Int64Ptr(0),
	}, metav1.ListOptions{})

	if err != nil {
		return fmt.Errorf("failed to delete pods %w", err)
	}

	return err
}

// CleanPolicies deletes all SriovNetworkNodePolicies in operatorNamespace.
func CleanPolicies(operatorNamespace string, clientSet *testclient.ClientSet) error {
	policies := sriovv1.SriovNetworkNodePolicyList{}
	err := clientSet.List(context.Background(),
		&policies,
		runtimeclient.InNamespace(operatorNamespace),
	)

	if err != nil {
		return err
	}

	for _, p := range policies.Items {
		if p.Name != "default" && strings.HasPrefix(p.Name, "test-") {
			err := clientSet.Delete(context.Background(), &p)
			if err != nil {
				return fmt.Errorf("failed to delete policy %w", err)
			}
		}
	}

	return err
}

// CleanNetworks deletes all network in operatorNamespace.
func CleanNetworks(operatorNamespace string, clientSet *testclient.ClientSet) error {
	networks := sriovv1.SriovNetworkList{}
	err := clientSet.List(context.Background(),
		&networks,
		runtimeclient.InNamespace(operatorNamespace))

	if err != nil {
		return err
	}

	for _, n := range networks.Items {
		if strings.HasPrefix(n.Name, "test-") {
			err := clientSet.Delete(context.Background(), &n)
			if err != nil {
				return fmt.Errorf("failed to delete network %w", err)
			}
		}
	}

	return waitForSriovNetworkDeletion(operatorNamespace, clientSet, 15*time.Second)
}

func waitForSriovNetworkDeletion(operatorNamespace string, cs *testclient.ClientSet, timeout time.Duration) error {
	return wait.PollImmediate(time.Second, timeout, func() (bool, error) {
		networks := sriovv1.SriovNetworkList{}
		err := cs.List(context.Background(),
			&networks,
			runtimeclient.InNamespace(operatorNamespace))
		if err != nil {
			return false, err
		}
		for _, network := range networks.Items {
			if strings.HasPrefix(network.Name, "test-") {
				return false, nil
			}
		}

		return true, nil
	})
}

// Clean cleans all dangling objects from the given namespace.
func Clean(operatorNamespace, namespace string, clientSet *testclient.ClientSet, discoveryEnabled bool) error {
	err := CleanPods(namespace, clientSet)
	if err != nil {
		return err
	}

	if !discoveryEnabled {
		err = CleanPolicies(operatorNamespace, clientSet)
		if err != nil {
			return err
		}
	}

	err = CleanNetworks(operatorNamespace, clientSet)

	if err != nil {
		return err
	}

	err = CleanNetworkAttachmentDefinitions(namespace, clientSet)

	return err
}

// LabelNamespace set label (key & value) to a namespace.
func LabelNamespace(clientSet *testclient.ClientSet, namespaceName, key, value string) (*k8sv1.Namespace, error) {
	namespace, err := clientSet.Namespaces().Get(context.Background(), namespaceName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	namespace.Labels[key] = value
	namespace, err = clientSet.Namespaces().Update(context.Background(), namespace, metav1.UpdateOptions{})

	if err != nil {
		return nil, err
	}

	return namespace, nil
}

// CleanEventsInNamespace removes all events from the given namespace.
func CleanEventsInNamespace(namespace string, clientSet *testclient.ClientSet) error {
	nsExist := Exists(namespace, clientSet)

	if !nsExist {
		return nil
	}

	err := clientSet.Events(namespace).DeleteCollection(context.Background(),
		metav1.DeleteOptions{GracePeriodSeconds: pointer.Int64Ptr(0)},
		metav1.ListOptions{})

	return err
}

// CleanNetworkAttachmentDefinitions removes all network-attachment-definition from the given namespace.
func CleanNetworkAttachmentDefinitions(namespace string, clientSet *testclient.ClientSet) error {
	nsExist := Exists(namespace, clientSet)

	if !nsExist {
		return nil
	}

	nadList := &nadv1.NetworkAttachmentDefinitionList{}
	err := clientSet.List(context.Background(), nadList)

	if err != nil {
		return err
	}

	err = clientSet.NetworkAttachmentDefinitions(namespace).DeleteCollection(
		context.Background(), metav1.DeleteOptions{}, metav1.ListOptions{
			FieldSelector: "metadata.name!=dummy-dhcp-network",
		})

	if err != nil {
		return err
	}

	return waitForNetworkAttachmentDefinitionDeletion(clientSet, namespace, 3*time.Minute)
}

func waitForNetworkAttachmentDefinitionDeletion(
	cs *testclient.ClientSet, namespace string, timeout time.Duration) error {
	return wait.PollImmediate(time.Second, timeout, func() (bool, error) {
		nadList := &nadv1.NetworkAttachmentDefinitionList{}
		err := cs.List(context.Background(), nadList)
		if err != nil {
			return false, err
		}
		for _, nad := range nadList.Items {
			if nad.Name != "dummy-dhcp-network" && nad.Namespace == namespace {
				return false, nil
			}
		}

		return true, nil
	})
}
