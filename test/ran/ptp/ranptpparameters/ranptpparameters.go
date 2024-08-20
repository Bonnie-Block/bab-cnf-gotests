package ranptpparameters

import (
	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/schemes/ptp/ptpv1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

var (
	// ReporterNamespacesToDump tells to reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		parameters.PtpOperatorNamespace: "",
		parameters.CloudEventNamespace:  "",
		parameters.PrivPodNamespace:     "",
	}
	// ReporterCrds tells to reporter what resources to collect.
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &ptpv1.PtpConfigList{}},
		{Cr: &ptpv1.NodePtpDeviceList{}},
		{Cr: &ptpv1.PtpOperatorConfigList{}},
		{Cr: &appsv1.DaemonSetList{}},
		{Cr: &v1.ConfigMapList{}},
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

	EventTypeClockClassChange   = "event.sync.ptp-status.ptp-clock-class-change"
	EventTypePtpStateChange     = "event.sync.ptp-status.ptp-state-change"
	EventTypeGnssStateChange    = "event.sync.gnss-status.gnss-state-change"
	EventTypeOsClockStateChange = "event.sync.sync-status.os-clock-sync-state-change"

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
	Resource      string `json:"resource"`
	ResourceORan  string `json:"ResourceAddress"`
	DataType      string `json:"dataType"`
	DataTypeORan  string `json:"data_type"`
	ValueType     string `json:"valueType"`
	ValueTypeORan string `json:"value_type"`
	Value         string `json:"value"`
}
