package ranptpparameters

import (
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	ptpv1 "github.com/openshift/ptp-operator/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

var (
	// ReporterNamespacesToDump tells to reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		parameters.PtpOperatorNamespace: "openshift-ptp",
	}
	// ReporterCrds tells to reporter what resources to collect.
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &ptpv1.PtpConfigList{}},
		{Cr: &ptpv1.NodePtpDeviceList{}},
		{Cr: &ptpv1.PtpOperatorConfigList{}},
	}

	PtpVersion   string
	OcpInterface string

	PtpProfileIfaces map[string][]string
)

const (
	PtpOperatorName     = "ptp-operator"
	CloudEventContainer = "cloud-event-proxy"
	ConsumerContainer   = "cloud-event-consumer"
	EventFreeRun        = "FREERUN"
	EventLocked         = "LOCKED"
	EventHoldOver       = "HOLDOVER"

	Master = "master"
)

type Log struct {
	Time  string `json:"time"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

type EventMsg struct {
	ID              string    `json:"id"`
	EventType       string    `json:"type"`
	Source          string    `json:"source"`
	DataContentType string    `json:"dataContentType"`
	Time            string    `json:"time"`
	Data            EventData `json:"data"`
}

type EventData struct {
	Version string   `json:"version"`
	Values  []Values `json:"values"`
}

type Values struct {
	Resource  string `json:"resource"`
	DataType  string `json:"dataType"`
	ValueType string `json:"valueType"`
	Value     string `json:"value"`
}
