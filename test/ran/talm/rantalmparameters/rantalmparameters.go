package rantalmparameters

import (
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

const (
	HubKubeEnvKey            = "KUBECONFIG_HUB"
	Spoke1KubeEnvKey         = "KUBECONFIG"
	Spoke2KubeEnvKey         = "KUBECONFIG_SPOKE2"
	TalmContainerName        = "manager"
	TalmDefaultReconcileTime = 5 * time.Minute
	TalmOperatorNamespace    = "openshift-cluster-group-upgrades"
	TalmPodLabelSelector     = "pod-template-hash"
	TalmPodNameHub           = "cluster-group-upgrades-controller-manager"
	TalmTestNamespace        = "talm-test"
	TalmTestPollInterval     = 15 * time.Second
)

var (
	// TalmNamespaces contains a list of all the talm related namespaces.
	TalmNamespaces = map[string]string{
		TalmOperatorNamespace: "talm",
	}
	// TalmCrds contains a list of all the talm related CRDs.
	TalmCrds = []k8sreporter.CRData{
		// Depends on https://issues.redhat.com/browse/CNF-6462
		// {Cr: &talmv1alpha1.ClusterGroupUpgrade},
	}
)
