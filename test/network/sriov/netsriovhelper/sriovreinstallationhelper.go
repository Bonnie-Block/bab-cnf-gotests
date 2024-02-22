package netsriovhelper

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo/v2"

	olmv1 "github.com/operator-framework/api/pkg/operators/v1"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/namespaces"
	admregv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// DeploySriovOperator deploys SR-IOV operator on given cluster.
func DeploySriovOperator(namespace *corev1.Namespace, operatorGroup *olmv1.OperatorGroup,
	sriovSubscription *v1alpha1.Subscription) error {
	err := helper.Apiclient.Create(context.Background(),
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:        namespace.Name,
				Annotations: namespace.Annotations,
				Labels:      namespace.Labels,
			},
		})

	if err != nil {
		return fmt.Errorf("can not deploy operator namespace %w", err)
	}

	err = helper.Apiclient.Create(context.Background(),
		&olmv1.OperatorGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:        operatorGroup.Name,
				Namespace:   operatorGroup.Namespace,
				Annotations: operatorGroup.Annotations},
			Spec: olmv1.OperatorGroupSpec{
				TargetNamespaces: operatorGroup.Spec.TargetNamespaces},
		},
	)

	if err != nil {
		return fmt.Errorf("can not deploy operatorGroup %w", err)
	}

	err = helper.Apiclient.Create(context.Background(),
		&v1alpha1.Subscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:        sriovSubscription.Name,
				Namespace:   sriovSubscription.Namespace,
				Annotations: sriovSubscription.Annotations,
			},
			Spec: &v1alpha1.SubscriptionSpec{
				Channel:                sriovSubscription.Spec.Channel,
				Package:                sriovSubscription.Spec.Package,
				CatalogSource:          sriovSubscription.Spec.CatalogSource,
				CatalogSourceNamespace: sriovSubscription.Spec.CatalogSourceNamespace,
				InstallPlanApproval:    v1alpha1.ApprovalAutomatic,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("can not install Subscription %w", err)
	}

	return nil
}

// IsSriovOperatorInstalled validates if SR-IOV operator deployed on the given cluster.
func IsSriovOperatorInstalled() error {
	By(fmt.Sprintf("Validate that operator namespace: %s exists", parameters.SriovOperatorNamespace))

	if !namespaces.Exists(parameters.SriovOperatorNamespace, helper.Apiclient) {
		return fmt.Errorf("sriov operator namespace doesn't exist")
	}

	By(fmt.Sprintf("Validate that operator's deployment %s exists", netsriovparameters.SriovOperatorDeploymentName))
	sriovOperatorInstalled, err := helper.IsDeploymentInstalled(
		helper.Apiclient, parameters.SriovOperatorNamespace, netsriovparameters.SriovOperatorDeploymentName)

	if err != nil {
		return err
	}

	if !sriovOperatorInstalled {
		return fmt.Errorf("sriov operator's deployment is not installed")
	}

	By("Validate that operator's daemonsets are up and running")

	for _, dsName := range netsriovparameters.SriovOperatorDaemonSets {
		err = helper.IsDaemonsetReady(helper.Apiclient, parameters.SriovOperatorNamespace, dsName)
		if err != nil {
			return fmt.Errorf("daemonset %s is not ready: %w", dsName, err)
		}
	}

	By("Validate that operator's CRDs are installed")

	for _, crdName := range netsriovparameters.SriovCrds {
		crd := &v1.CustomResourceDefinition{}
		err = helper.Apiclient.Get(context.Background(), goclient.ObjectKey{Name: crdName}, crd)

		if err != nil {
			return err
		}
	}

	webhook := &admregv1.ValidatingWebhookConfiguration{}
	err = helper.Apiclient.Get(context.Background(),
		goclient.ObjectKey{Name: netsriovparameters.SriovValidationWebhook,
			Namespace: parameters.SriovOperatorNamespace}, webhook)

	if err != nil {
		return err
	}

	return nil
}
