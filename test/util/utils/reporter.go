package utils

import (
	"os"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
	"k8s.io/apimachinery/pkg/runtime"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	ptpv1 "github.com/openshift/ptp-operator/pkg/apis/ptp/v1"
)

// NewReporter creates a specific reporter for CNF tests.
func NewReporter(
	reportPath string,
	namespacesToDump map[string]string,
	crds []k8sreporter.CRData) (*k8sreporter.KubernetesReporter, error) {
	addToScheme := func(scheme *runtime.Scheme) {
		err := ptpv1.AddToScheme(scheme)
		if err != nil {
			panic(err)
		}

		err = mcfgv1.AddToScheme(scheme)

		if err != nil {
			panic(err)
		}

		err = sriovv1.AddToScheme(scheme)

		if err != nil {
			panic(err)
		}
	}

	skipByNamespace := func(ns string) bool {
		_, found := namespacesToDump[ns]

		return !found
	}
	err := os.Mkdir(reportPath, 0755)

	if err != nil {
		return nil, err
	}

	res, err := k8sreporter.New("", addToScheme, skipByNamespace, reportPath, crds...)

	if err != nil {
		return nil, err
	}

	return res, nil
}
