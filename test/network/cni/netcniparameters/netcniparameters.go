package netcniparameters

import (
	"time"

	v1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

const (
	TestNamespace             = "cni-tests"
	PodWaitingTime            = 2 * time.Minute
	WaitingTime               = 20 * time.Minute
	RetryInterval             = 5 * time.Second
	AnnotationNetStat         = "k8s.v1.cni.cncf.io/network-status"
	SriovPolicyName           = "test-sriov-policy-cni"
	MultusFirstInterfaceName  = "net1"
	MultusSecondInterfaceName = "net2"
)

var (
	// ReporterNamespacesToDump tells to reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		"openshift-performance-addon-operator": "performance",
		parameters.SriovOperatorNamespace:      "sriov",
		TestNamespace:                          "other",
	}

	// ReporterCrds tells to reporter what resources to collect.
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &sriovv1.SriovNetworkNodePolicyList{}},
		{Cr: &sriovv1.SriovNetworkList{}},
		{Cr: &sriovv1.SriovNetworkNodeStateList{}},
		{Cr: &sriovv1.SriovOperatorConfigList{}},
		{Cr: &v1.NetworkAttachmentDefinitionList{}},
	}
)
