package rantalmparameters

import (
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

const (
	TalmOperatorNamespace    = "openshift-cluster-group-upgrades"
	TalmPodName              = "cluster-group-upgrades-controller-manager"
	TalmContainerName        = "manager"
	TalmPodLabelSelector     = "pod-template-hash"
	TalmTestPollInterval     = 15 * time.Second
	TalmDefaultReconcileTime = 5 * time.Minute
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
	HubClientset *client.ClientSet // initialized in BeforeSuite
	Spoke1Name   string            // initialized in BeforeSuite
)
