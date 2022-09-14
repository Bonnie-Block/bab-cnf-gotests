package netpolicyparameters

import (
	"time"

	multinetpolicyapiv1 "github.com/k8snetworkplumbingwg/multi-networkpolicy/pkg/apis/k8s.cni.cncf.io/v1beta1"
	v1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TestNamespace              = "test-policy"
	PodWaitingTime             = 1 * time.Minute
	RetryInterval              = 5 * time.Second
	WaitingTime                = 20 * time.Minute
	VRFName                    = "policyvrf"
	SriovNetworkName           = "test-sriov-network-with-static-ipam"
	SriovNetworkNameWithVRF    = "test-sriov-network-with-vrf"
	ResourceNamePolicy         = "testsriovnicnetworkpolicy"
	MultusNamespace            = "openshift-multus"
	NetworkPolicyDaemonsetName = "multus-networkpolicy"
	Pod1IPAddress              = "192.168.0.1"
	Pod2IPAddress              = "192.168.0.2"
	Pod3IPAddress              = "192.168.0.3"
	Port5001                   = "5001"
	Port5003                   = "5003"
	Container5003Name          = "test-5003"
	LabelType                  = "pod"
	LabelPod1Name              = "pod1"
	LabelPod2Name              = "pod2"
	LabelPod3Name              = "pod3"
)

var (
	// ReporterNamespacesToDump tells to reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		parameters.SriovOperatorNamespace: "sriov",
		TestNamespace:                     "other",
	}

	// ReporterCrds tells to reporter what resources to collect.
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &sriovv1.SriovNetworkNodePolicyList{}},
		{Cr: &sriovv1.SriovNetworkList{}},
		{Cr: &sriovv1.SriovNetworkNodeStateList{}},
		{Cr: &sriovv1.SriovOperatorConfigList{}},
		{Cr: &v1.NetworkAttachmentDefinitionList{}},
		{Cr: &multinetpolicyapiv1.MultiNetworkPolicyList{}},
	}

	LabelSelectorPod1 = metav1.LabelSelector{MatchLabels: map[string]string{LabelType: LabelPod1Name}}
	LabelSelectorPod2 = metav1.LabelSelector{MatchLabels: map[string]string{LabelType: LabelPod2Name}}
	LabelSelectorPod3 = metav1.LabelSelector{MatchLabels: map[string]string{LabelType: LabelPod3Name}}
)
