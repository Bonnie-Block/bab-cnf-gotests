package ranptphelper

import ranptpparameters "gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters/metricparameters"

// metrics fields
type Metrics struct {
	CneAmqpConnectionReset                int                     `json:"cne_amqp_connection_reset"`
	CneAmqpEventsPublished                []MetricAddr            `json:"cne_amqp_events_published"`
	CneAmqpReceiver                       []MetricAddr            `json:"cne_amqp_receiver"`
	CneAmqpSender                         MetricAddr              `json:"cne_amqp_sender"`
	CneApiEventsPublished                 MetricAddr              `json:"cne_api_events_published"`
	CneApiPublishers                      metricDetails           `json:"cne_api_publishers"`
	CneEventsAck                          cneEventsAck            `json:"cne_events_ack"`
	OpenshiftPtpClockClass                ClockClass              `json:"openshift_ptp_clock_class"`
	OpenshiftPtpClockState                ClockState              `json:"openshift_ptp_clock_state"`
	OpenshiftPtpDelayNs                   struct5                 `json:"openshift_ptp_delay_ns"`
	OpenshiftPtpFrequencyAdjustmentNs     struct5                 `json:"openshift_ptp_frequency_adjustment_ns"`
	OpenshiftPtpInterfaceRole             InterfaceRole           `json:"openshift_ptp_interface_role"`
	OpenshiftPtpMaxOffsetNs               struct5                 `json:"openshift_ptp_max_offset_ns"`
	OpenshiftPtpOffsetNs                  struct5                 `json:"openshift_ptp_offset_ns"`
	OpenshiftPtpProcessRestartCount       ProcessRestartCount     `json:"openshift_ptp_process_restart_count"`
	OpenshiftPtpProcessStatus             ProcessStatus           `json:"openshift_ptp_process_status"`
	PromHttpMetricHandlerRequestsInFlight int                     `json:"prom_http_metric_handler_requests_in_flight"`
	PromHttpMetricHandlerRequestsTotal    promHttpHandlerRequests `json:"prom_http_metric_handler_requests_total"`
}

// cne_amqp_connection_reset int

// cne_api_publishers
type metricDetails struct {
	Status ranptpparameters.Status `json:"status"`
	Value  int                     `json:"value"`
}

// cne_amqp_events_published
// cne_amqp_receiver
// cne_amqp_sender
// cne_api_events_published
type MetricAddr struct {
	Address       string        `json:"address"`
	MetricDetails metricDetails `json:"metricDetails"`
}

// cne_events_ack
type cneEventsAck struct {
	Type          string        `json:"type"`
	MetricDetails metricDetails `json:"metricDetails"`
}

type metricsNodeDetails struct {
	Node    string                   `json:"node"`
	Process ranptpparameters.Process `json:"process"`
}

// openshift_ptp_clock_class
type ClockClass struct {
	MetricNodeDetails metricsNodeDetails `json:"metricNodeDetails"`
	Value             int                `json:"value"`
}

type struct4 struct {
	Interface         string             `json:"interface"`
	MetricNodeDetails metricsNodeDetails `json:"metricNodeDetails"`
	Value             int                `json:"value"`
}

// openshift_ptp_clock_state
type ClockState struct {
	Interface         string                      `json:"interface"`
	MetricNodeDetails metricsNodeDetails          `json:"metricNodeDetails"`
	Value             ranptpparameters.ClockState `json:"value"`
}

// openshift_ptp_interface_role
type InterfaceRole struct {
	Interface         string                         `json:"interface"`
	MetricNodeDetails metricsNodeDetails             `json:"metricNodeDetails"`
	Value             ranptpparameters.InterfaceRole `json:"value"`
}

// openshift_ptp_delay_ns
// openshift_ptp_frequency_adjustment_ns
// openshift_ptp_max_offset_ns
// openshift_ptp_offset_ns
type struct5 struct {
	From        string  `json:"from"`
	MoreDetails struct4 `json:"moreDetails"`
}

// openshift_ptp_process_restart_count
type ProcessRestartCount struct {
	Config      string             `json:"config"`
	MoreDetails metricsNodeDetails `json:"moreDetails"`
	Value       int                `json:"value"`
}

// openshift_ptp_process_status
type ProcessStatus struct {
	Config      string                         `json:"config"`
	MoreDetails metricsNodeDetails             `json:"moreDetails"`
	Value       ranptpparameters.ProcessStatus `json:"value"`
}

// prom_http_metric_handler_requests_in_flight int

// prom_http_metric_handler_requests_total
type promHttpHandlerRequests struct {
	Code  int `json:"code"`
	Value int `json:"value"`
}
