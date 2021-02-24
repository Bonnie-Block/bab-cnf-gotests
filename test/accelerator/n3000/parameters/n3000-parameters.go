package parameters

import (
	fpgav1 "github.com/open-ness/openshift-operator/N3000/api/v1"
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

const (
	OperatorNamespace        = "n3000-operators-resources"
	DaemonsetDriverName      = "fpga-driver-daemonset"
	DaemonsetTelemetryName   = "fpgainfo-exporter"
	DaemonsetN3000DaemonName = "n3000-daemonset"
	TestNamespace            = "n3000-operator-test"
	ImageBitstreamFlush      = "20ww14.5-1x2x25G-5GLDPC-v1.5.7-3.0.0-unsigned.bin"
	// Todo Webserver  pod with N3000 images should be changed
	ImageBitstreamImages   = "docker-registry.upshift.redhat.com/n3000/images"
	ImageDefault           = "sr_vista_rot_2x2x25-v1.3.16.bin"
	ChecksumBitstreamImage = "fd4b6a9a69480e4f5ad39aa8cae85163"
	ChecksumDefaultImage   = "1c2010bb71e85bfa4902d8fc959e6eaf"
)

var (
	TestLabel = map[string]string{"testKey": "testValue"}
	// ReporterNamespacesToDump tells to reporter from where to collect logs
	ReporterNamespacesToDump = map[string]string{
		"openshift-performance-addon-operator": "performance",
		OperatorNamespace:                      "other",
		TestNamespace:                          "other",
	}
	// ReporterCrds tells to reporter what resources to collect
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &fpgav1.N3000ClusterList{}},
		{Cr: &fpgav1.N3000NodeList{}},
	}
)
