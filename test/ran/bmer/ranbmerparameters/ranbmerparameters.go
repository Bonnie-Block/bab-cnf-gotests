package ranbmerparameters

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
	AppName                      = "hw-event-proxy"
	ConsumerContainerName        = "cloud-event-consumer"
	AppPodLabel                  = "app=" + AppName
	Dell                  string = "dell"
	Hpe                   string = "hpe"
	ZT                    string = "zt"
	SecretName                   = "redfish-basic-auth"
	DellRedfishOem               = "Dell"
	HpeRedfishOem                = "Hpe"
	ZTRedfishOem                 = "Ami"
	AppRouteName                 = AppName
)

var (
	Redfish = RedfishConfig{
		helper.Config.Ran.BmcHosts,
		helper.Config.Ran.BmcUser,
		helper.Config.Ran.BmcPassword,
		"https://" + helper.Config.Ran.BmcHosts,
		"",
		map[string]bool{},
		&gofish.APIClient{},
	}
	// ReporterNamespacesToDump tells to reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		parameters.BmerNamespace: "bmer",
		ran.NamespaceTesting:     "other",
	}
	// ReporterCrds tells to reporter what resources to collect.
	ReporterCrds = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
	}
	EventRxTimeout = time.Duration(30) * time.Second

	DellPowerOffEvents = []string{"PSU0800", "PSU0003", "RDU0012"}
	DellPowerOnEvents  = []string{"PSU0800", "PSU0019", "RDU0011"}
	HPEPowerOnEvents   = []string{"iLOEvents.2.1.PowerSupplyOK", "iLOEvents.2.1.PowerRedundancyRestored"}
	HPEPowerOffEvents  = []string{"iLOEvents.2.1.PowerSupplyACPowerLoss", "iLOEvents.2.1.PowerRedundancyLost"}

	ZtEvents = []string{
		"EventLog.1.0.StatusChange",
		"EventLog.1.0.ResourceAdded",
		"EventLog.1.0.ResourceUpdated",
		"EventLog.1.0.ResourceRemoved",
		"EventLog.1.0.Alert",
	}
	ZTSendEventInterval = 17 * time.Second
	ZTSendEventTimeout  = (17 * 6) * time.Second

	// HP EventService.SubmitTestEvent API accepts any fake message IDs.
	HpEvents = []string{
		"iLOEvents.2.1.TempNotif1",
		"iLOEvents.2.1.FanNotif1",
		"iLOEvents.2.1.DiskNotif1",
		"iLOEvents.2.1.PowerNotif1",
		"iLOEvents.2.1.MemoeryNotif1",
		"iLOEvents.2.1.TempNotif2",
		"iLOEvents.2.1.FanNotif2",
		"iLOEvents.2.1.DiskNotif2",
		"iLOEvents.2.1.PowerNotif2",
		"iLOEvents.2.1.MemoeryNotif2",
	}

	// use only critical alarms.
	IDRACEvents = []string{
		"TMP0101", "TMP0103", "TMP0104", "TMP0107", "TMP0109",
		"TMP0110", "TMP0113", "TMP0115", "TMP0116", "TMP0119",
	}

	CustomResourceDefinition = "openshift-bare-metal-events"
)

func GetPDU() bool {
	return helper.Config.Ran.PduAddr != ""
}
