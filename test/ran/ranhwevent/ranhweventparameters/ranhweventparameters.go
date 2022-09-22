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
	AppName                       = "hw-event-proxy"
	ConsumerContainerName         = "cloud-event-consumer"
	NamespaceConsumer             = "openshift-bare-metal-events"
	AppPodLabel                   = "app=" + AppName
	ConsumerPodLabel              = "app=consumer"
	Dell                   string = "dell"
	Hpe                    string = "hpe"
	ZT                     string = "zt"
	SecretName                    = "redfish-basic-auth"
	DellRedfishOem                = "Dell"
	HpeRedfishOem                 = "Hpe"
	ZTRedfishOem                  = "Ami"
	AppRouteName                  = AppName
	ConsumerDeploymentName        = "consumer"
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
	ZTSendEventInterval = 17 * time.Second
	ZTSendEventTimeout  = (17 * 6) * time.Second

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

	HwEventCsv = "bare-metal-event-relay."
	// RequiredImages in 4_11 "kube_rbac_proxy_image", "cloud_event_proxy_image" .
	// RequiredImages in 4_10 "ose-kube-rbac-proxy", "ose-cloud-event-proxy" .
	RequiredImages = map[string][]string{
		"kube_rbac_proxy_image":   {"ose-kube-rbac-proxy", "kube_rbac_proxy_image"},
		"cloud_event_proxy_image": {"ose-cloud-event-proxy", "cloud_event_proxy_image"},
	}
	ConsumerManifestTemplate = "resources/ranhwevent-consumer/consumer_manifest.j2"
	CustomResourceDefinition = "openshift-bare-metal-events"
	ConsumerImageName        = "cloud_event_consumer"
)

func GetPDU() bool {
	return helper.Config.Ran.PduAddr != ""
}
