package ranparameters

import (
	"strings"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
)

const (
	ConsumerContainerName      = "cloud-event-consumer"
	ConsumerPodLabel           = "app=consumer"
	TransportHTTP              = "http"
	TransportAMQP              = "amqp"
	BmerConsumerDeploymentName = "consumer"
	PtpConsumerDeploymentName  = "cloud-consumer-deployment"
)

var (
	RequiredImages = map[string][]string{
		"kube_rbac_proxy_image":   {"ose-kube-rbac-proxy", "kube_rbac_proxy_image"},
		"cloud_event_proxy_image": {"ose-cloud-event-proxy", "cloud_event_proxy_image"},
	}
	DebugTest            = strings.ToLower(helper.Config.Ran.RanEventTestDebug) == "true"
	ConsumerManifestHTTP = "consumer_http_manifest.j2"
	ConsumerManifestAMQP = "consumer_amqp_manifest.j2"
	ConsumerImageName    = "cloud_event_consumer"
	// TransportType retrieved from application and to be used in consumer deployment.
	TransportType = TransportHTTP
)

func CsvDict() func(string) string {
	innerMap := map[string]string{
		parameters.BmerNamespace:        "bare-metal-event-relay.",
		parameters.PtpOperatorNamespace: "ptp-operator.",
	}

	return func(key string) string {
		return innerMap[key]
	}
}

func TemplatePathDict() func(string) string {
	innerMap := map[string]string{
		parameters.BmerNamespace:       "resources/bmer-consumer",
		parameters.CloudEventNamespace: "resources/ptp-consumer",
	}

	return func(key string) string {
		return innerMap[key]
	}
}

func ConsumerDeploymentDict() func(string) string {
	innerMap := map[string]string{
		parameters.BmerNamespace:       BmerConsumerDeploymentName,
		parameters.CloudEventNamespace: PtpConsumerDeploymentName,
	}

	return func(key string) string {
		return innerMap[key]
	}
}

func ConfigDirDict() func(string) string {
	innerMap := map[string]string{
		parameters.BmerNamespace:       helper.Config.Ran.BmerConfigsDir,
		parameters.CloudEventNamespace: helper.Config.Ran.PtpConfigsDir,
	}

	return func(key string) string {
		return innerMap[key]
	}
}
