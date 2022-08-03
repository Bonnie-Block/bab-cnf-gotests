package ranhweventparameters

import (
	"time"

	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	"github.com/stmcginnis/gofish"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/k8sreporter"
)

type RedfishConfig struct {
	Hostname, Username, Password, RedfishURL, EventReceiver string
	SkippedEvents                                           map[string]bool
	Session                                                 *gofish.APIClient
}

const (
	Dell string = "dell"
	Hpe  string = "hpe"
	ZT   string = "zt"
)

var (
	ConsumerContainerName = "cloud-native-event-consumer"
	NamespaceConsumer     = "openshift-hw-events"
	AppPodLabel           = "app=hw-event-proxy"
	ConsumerPodLabel      = "app=consumer"
	Redfish               = RedfishConfig{
		helper.Config.Ran.BmcHosts,
		helper.Config.Ran.BmcUser,
		helper.Config.Ran.BmcPassword,
		"https://" + helper.Config.Ran.BmcHosts,
		"https://" + helper.Config.Ran.EventReceiver + "/webhook",
		map[string]bool{},
		&gofish.APIClient{},
	}
	// ReporterNamespacesToDump tells to reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		parameters.HwEventProxyNamespace: NamespaceConsumer,
		ran.NamespaceTesting:             "other",
	}
	// ReporterCrds tells to reporter what resources to collect.
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
	}
	EventRxTimeout = time.Duration(30) * time.Second
	DebugTest      = false

	DellPowerOffEvents = []string{"PSU0800", "PSU0003", "RDU0012"}
	DellPowerOnEvents  = []string{"PSU0800", "PSU0019", "RDU0011"}
	HPEPowerOnEvents   = []string{"iLOEvents.2.1.PowerSupplyOK", "iLOEvents.2.1.PowerRedundancyRestored"}
	HPEPowerOffEvents  = []string{"iLOEvents.2.1.PowerSupplyACPowerLoss", "iLOEvents.2.1.PowerRedundancyLost"}

	ZtEvents = []string{
		"EventLog.1.0.ResourceUpdated",
		"EventLog.1.0.ResourceUpdated",
		"EventLog.1.0.ResourceUpdated",
		"EventLog.1.0.ResourceUpdated",
		"EventLog.1.0.ResourceUpdated",
		"EventLog.1.0.ResourceUpdated",
		"EventLog.1.0.ResourceUpdated",
		"EventLog.1.0.ResourceUpdated",
		"EventLog.1.0.ResourceUpdated",
		"EventLog.1.0.ResourceUpdated",
	}
	ZTSendEventInterval = 5 * time.Second
	ZTSendEventTimeout  = 30 * time.Second

	HpEvents = []string{
		"iLOEvents.2.1.ServerPoweredOff",
		"iLOEvents.2.1.ServerPoweredOff",
		"iLOEvents.2.1.ServerPoweredOff",
		"iLOEvents.2.1.ServerPoweredOff",
		"iLOEvents.2.1.ServerPoweredOff",
		"iLOEvents.2.1.ServerPoweredOff",
		"iLOEvents.2.1.ServerPoweredOff",
		"iLOEvents.2.1.ServerPoweredOff",
		"iLOEvents.2.1.ServerPoweredOff",
		"iLOEvents.2.1.ServerPoweredOff",
	}

	IDRACEvents = []string{
		"AMP0300", "AMP0301", "AMP0302", "AMP0303", "AMP0304", "AMP0306", "AMP0307", "AMP0308",
		"AMP0309",
		"AMP0310",
	}
)

func GetPDU() bool {
	return helper.Config.Ran.PduAddr != ""
}
