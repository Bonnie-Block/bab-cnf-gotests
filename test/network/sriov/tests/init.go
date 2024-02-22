package tests

import (
	"context"
	"log"
	"os"

	corev1 "k8s.io/api/core/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/netsriovparameters"

	v1 "github.com/operator-framework/api/pkg/operators/v1"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	sriovSmokeTestMode bool
	operatorGroup      v1.OperatorGroup
	sriovSubscription  *v1alpha1.Subscription
	namespace          *corev1.Namespace
)

func init() {
	sriovSmokeTestModeEnvVar := os.Getenv("CNF_GOTESTS_SRIOV_SMOKE")
	if sriovSmokeTestModeEnvVar == "true" {
		log.Print("Run sriov tests in smoke mode")

		sriovSmokeTestMode = true
	}

	if os.Getenv("KUBECONFIG") == "" {
		return
	}

	log.Print("Collect info about installed sriov operator")

	err := helper.Apiclient.Get(context.Background(),
		goclient.ObjectKey{Name: netsriovparameters.SriovOperatorGroupName, Namespace: parameters.SriovOperatorNamespace},
		&operatorGroup)

	if err != nil {
		log.Fatalf("error to collect info about sriov operatorGroup resource %s", err)
	}

	sriovSubscription, err = helper.Apiclient.Subscriptions(parameters.SriovOperatorNamespace).Get(
		context.Background(), netsriovparameters.SriovOperatorSubscriptionName, metav1.GetOptions{})

	if err != nil {
		log.Fatalf("error to collect sriov Subscription resource %s", err)
	}

	namespace, err = helper.Apiclient.Namespaces().Get(
		context.Background(), parameters.SriovOperatorNamespace, metav1.GetOptions{})

	if err != nil {
		log.Fatalf("error to collect sriov namespace resource %s", err)
	}
}
