package ranztphelper

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakeDynamic "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func newUnstructured(apiVersion, kind, namespace, name string,
	labels map[string]string, annotations map[string]string) *unstructured.Unstructured {
	unstructuredObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"namespace": namespace,
				"name":      name,
			},
		},
	}

	if labels != nil {
		unstructuredObj.SetLabels(labels)
	}

	if annotations != nil {
		unstructuredObj.SetAnnotations(annotations)
	}

	return unstructuredObj
}

func TestBareMetalHostExists(t *testing.T) {
	generateBMHObject := func(name string, namespace string) runtime.Object {
		return newUnstructured("metal3.io/v1alpha1", "BareMetalHost", namespace, name, nil, nil)
	}

	// Create fake dynamic client
	fakeClient := fakeDynamic.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			{
				Group:    "metal3.io",
				Version:  "v1alpha1",
				Resource: "baremetalhosts"}: "BareMetalHostList",
		}, generateBMHObject("bmh1", "default"))

	// Sanity check that the fakeClient has the BMH object
	listFirst, err := fakeClient.Resource(schema.GroupVersionResource{
		Group:    "metal3.io",
		Version:  "v1alpha1",
		Resource: "baremetalhosts",
	}).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}

	assert.NotNil(t, listFirst)

	// Test the function
	exists, err := bareMetalHostExists(fakeClient, "bmh1", "default")
	assert.Nil(t, err)
	assert.True(t, exists)

	exists, err = bareMetalHostExists(fakeClient, "bmh2", "default")
	assert.Nil(t, err)
	assert.False(t, exists)
}

func TestAgentExists(t *testing.T) {
	generateAgentObject := func(name string, namespace string) runtime.Object {
		return newUnstructured("agent-install.openshift.io/v1beta1", "Agent", namespace, name,
			map[string]string{
				"agent-install.openshift.io/bmh": name,
			}, nil)
	}

	// Create fake dynamic client
	fakeClient := fakeDynamic.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			{
				Group:    "agent-install.openshift.io",
				Version:  "v1beta1",
				Resource: "agents",
			}: "AgentList",
		}, generateAgentObject("agent1", "default"))

	// Sanity check that the fakeClient has the Agent object
	listFirst, err := fakeClient.Resource(schema.GroupVersionResource{
		Group:    "agent-install.openshift.io",
		Version:  "v1beta1",
		Resource: "agents",
	}).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}

	assert.NotNil(t, listFirst)

	// Test the function
	exists, err := agentExists(fakeClient, "agent1", "default")
	assert.Nil(t, err)
	assert.True(t, exists)

	exists, err = agentExists(fakeClient, "agent2", "default")
	assert.Nil(t, err)
	assert.False(t, exists)
}

func TestGetBareMetalHostAnnotation(t *testing.T) {
	generateBMHObject := func(name string, namespace string,
		annotations map[string]string) runtime.Object {
		return newUnstructured("metal3.io/v1alpha1", "BareMetalHost", namespace, name, nil, annotations)
	}

	// Create fake dynamic client
	fakeClient := fakeDynamic.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			{Group: "metal3.io",
				Version:  "v1alpha1",
				Resource: "baremetalhosts",
			}: "BareMetalHostList",
		}, generateBMHObject("bmh1", "default", map[string]string{"test-annotation": "test-value"}))

	// Sanity check that the fakeClient has the BMH object
	listFirst, err := fakeClient.Resource(schema.GroupVersionResource{
		Group:    "metal3.io",
		Version:  "v1alpha1",
		Resource: "baremetalhosts",
	}).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}

	assert.NotNil(t, listFirst)

	// Test the function
	annotations, err := getBareMetalHostAnnotations(fakeClient, "bmh1", "default")
	assert.Nil(t, err)
	assert.Equal(t, map[string]string{"test-annotation": "test-value"}, annotations)
}

func TestGetWorkerAnnotations(t *testing.T) {
	generateWorkerObject := func(name string, annotations map[string]string) runtime.Object {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Annotations: annotations,
			},
		}
	}

	fakeClient := k8sfake.NewSimpleClientset(
		generateWorkerObject("worker1", map[string]string{"test-annotation": "test-value"}))

	// Test the function
	annotations, err := getWorkerAnnotations(fakeClient.CoreV1(), "worker1")
	assert.Nil(t, err)
	assert.Equal(t, map[string]string{"test-annotation": "test-value"}, annotations)
}
