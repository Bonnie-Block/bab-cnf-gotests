package ranparameters

import (
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
)

const (
	ConsumerContainerName  = "cloud-event-consumer"
	ConsumerPodLabel       = "app=consumer"
	ConsumerDeploymentName = "consumer"
	TransportHTTP          = "http"
	TransportAMQP          = "amqp"
)

var (
	RequiredImages = map[string][]string{
		"kube_rbac_proxy_image":   {"ose-kube-rbac-proxy", "kube_rbac_proxy_image"},
		"cloud_event_proxy_image": {"ose-cloud-event-proxy", "cloud_event_proxy_image"},
	}
	DebugTest            = false
	ConsumerManifestHTTP = "consumer_http_manifest.j2"
	ConsumerManifestAMQP = "consumer_amqp_manifest.j2"
	ConsumerImageName    = "cloud_event_consumer"
	// TransportType retrieved from application and to be used in consumer deployment.
	TransportType = TransportHTTP
)

func CsvDict() func(string) string {
	innerMap := map[string]string{
		parameters.BmerNamespace: "bare-metal-event-relay.",
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
