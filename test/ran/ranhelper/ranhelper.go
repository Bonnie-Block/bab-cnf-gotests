package ranhelper

import (
	"context"
	"fmt"

	"github.com/k8snetworkplumbingwg/sriov-network-operator/pkg/render"
	"github.com/pkg/errors"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

const (
	createMode = "create"
	deleteMode = "delete"
	updateMode = "update"
)

// ApplyObjects adds manifests from given directory to cluster.
func ApplyObjects(resDir string) error {
	return modifyObjects(createMode, resDir)
}

// DeleteObjects removes manifests from given directory from cluster.
func DeleteObjects(resDir string) error {
	return modifyObjects(deleteMode, resDir)
}

// UpdateObjects updates existing resurces based on manifests from given directory.
func UpdateObjects(resDir string) error {
	return modifyObjects(updateMode, resDir)
}

func modifyObjects(mode string, resDir string) error {
	data := render.MakeRenderData()
	objs, err := render.RenderDir(resDir, &data)

	if err != nil {
		return err
	}

	var errorList []error

	for _, obj := range objs {
		switch mode {
		case createMode:
			err = helper.Apiclient.Client.Create(context.TODO(), obj)
		case deleteMode:
			err = helper.Apiclient.Client.Delete(context.TODO(), obj)
		case updateMode:
			err = updateObject(obj)
		}

		if err != nil {
			errorList = append(errorList, err)
		}
	}

	if len(errorList) > 0 {
		return fmt.Errorf(
			"one or more errors occurred while processing resources from dir %s \n%v errors",
			resDir, errorList)
	}

	return nil
}

func updateObject(obj *unstructured.Unstructured) error {
	if obj.GetName() == "" {
		return errors.Errorf("Object %s has no name", obj.GroupVersionKind().String())
	}

	gvk := obj.GroupVersionKind()
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(gvk)
	err := helper.Apiclient.Client.Get(
		context.TODO(),
		types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()},
		existing)

	if err != nil {
		return err
	}

	obj.SetCreationTimestamp(existing.GetCreationTimestamp())
	obj.SetResourceVersion(existing.GetResourceVersion())
	obj.SetUID(existing.GetUID())
	obj.SetGeneration(existing.GetGeneration())
	obj.SetManagedFields(existing.GetManagedFields())
	obj.SetFinalizers(existing.GetFinalizers())

	return helper.Apiclient.Client.Update(context.TODO(), obj)
}
