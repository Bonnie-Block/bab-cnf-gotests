package ranptpparameters

type ClockStateDef int8
type InterfaceRoleDef int8
type ProcessStatusDef int8
type Process int8
type Status int
type InterfaceState string
type RoleMap map[string]string

const (
<<<<<<< HEAD
	FreeRunState  ClockState = 0
	LockedState   ClockState = 1
	HoldOverState ClockState = 2

	PassiveRole InterfaceRole = 0
	SlaveRole   InterfaceRole = 1
	MasterRole  InterfaceRole = 2
	FaultyRole  InterfaceRole = 3
	UnknownRole InterfaceRole = 4
	Listening   InterfaceRole = 5

	Down ProcessStatus = 0
	Up   ProcessStatus = 1
=======
	FREERUN  ClockStateDef = 0
	LOCKED   ClockStateDef = 1
	HOLDOVER ClockStateDef = 2
)

const (
	PASSIVE InterfaceRoleDef = 0
	SLAVE   InterfaceRoleDef = 1
	MASTER  InterfaceRoleDef = 2
	FAULTY  InterfaceRoleDef = 3
	UNKNOWN InterfaceRoleDef = 4
)

const (
	DOWN ProcessStatusDef = 0
	UP   ProcessStatusDef = 1
)
>>>>>>> 02b3c769 (Infa tests)

	PTP4L   Process = 1
	PHC2SYS Process = 2

	Success Status = 0
	Failed  Status = 1
	Active  Status = 2

	Off InterfaceState = "down"
	On  InterfaceState = "up"

	OpenshiftPtpClockState    = "openshift_ptp_clock_state"
	OpenshiftPtpProcessStatus = "openshift_ptp_process_status"
	OpenshiftPtpInterfaceRole = "openshift_ptp_interface_role"
	OpenshiftPtpThreshold     = "openshift_ptp_threshold"

	HoldOverTimeout    = "HoldOverTimeout"
	MaxOffsetThreshold = "MaxOffsetThreshold"
	MinOffsetThreshold = "MinOffsetThreshold"
)

var (
	// StatusMap is used to compare the wanted status value with the one in the metrics that are read from the cluster.
	StatusMap = map[string]Status{
		"success": Success,
		"active":  Active,
		"failed":  Failed,
		"":        -1,
	}

	// ProcessMap is used to compare the wanted status value with the one in the metrics that are read from the cluster.
	ProcessMap = map[string]Process{
		"ptp4l":   PTP4L,
		"phc2sys": PHC2SYS,
		"":        -1,
	}

	// MetricMap contains all the metrics detail, the key is the profile name, and then the metric name.
	MetricMap map[string][]MetricDetails

	// InterfacesRoleMap maps Interface IDs to their role (master/slave).
	InterfacesRoleMap map[string]RoleMap
)

<<<<<<< HEAD
type MetricDetails struct {
	Status             Status        `json:"status"`
	Value              int64         `json:"value"`
	Address            string        `json:"address"`
	Type               string        `json:"type"`
	Node               string        `json:"node"`
	Process            Process       `json:"process"`
	Interface          string        `json:"interface"`
	ClockStateValue    ClockState    `json:"clock_state_value"`
	InterfaceRoleValue InterfaceRole `json:"interface_role_value"`
	ProcessStatusValue ProcessStatus `json:"process_status_value"`
	From               string        `json:"from"`
	Config             string        `json:"config"`
	Code               int           `json:"code"`
	Threshold          string        `json:"threshold"`
=======
var (
	MerticsTypes = []string{
		"cne_api_events_published",
		"cne_api_publishers",
		"cne_events_ack",
		"openshift_ptp_clock_class",
		"openshift_ptp_clock_state",
		"openshift_ptp_delay_ns",
		"openshift_ptp_frequency_adjustment_ns",
		"openshift_ptp_interface_role",
		"openshift_ptp_max_offset_ns",
		"openshift_ptp_offset_ns",
		"openshift_ptp_process_restart_count",
		"openshift_ptp_process_status",
		"openshift_ptp_threshold",
		"promhttp_metric_handler_requests_in_flight",
		"promhttp_metric_handler_requests_total",
	}
)

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
	Status Status `json:"status"`
	Value  int    `json:"value"`
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
	Node    string  `json:"node"`
	Process Process `json:"process"`
}

// openshift_ptp_clock_class
type ClockClass struct {
	MetricNodeDetails metricsNodeDetails `json:"metricNodeDetails"`
	Value             int                `json:"value"`
}

// todo change name
type struct4 struct {
	Interface         string             `json:"interface"`
	MetricNodeDetails metricsNodeDetails `json:"metricNodeDetails"`
	Value             int                `json:"value"`
}

// openshift_ptp_clock_state
type ClockState struct {
	Interface         string             `json:"interface"`
	MetricNodeDetails metricsNodeDetails `json:"metricNodeDetails"`
	Value             ClockStateDef      `json:"value"`
}

// openshift_ptp_interface_role
type InterfaceRole struct {
	Interface         string             `json:"interface"`
	MetricNodeDetails metricsNodeDetails `json:"metricNodeDetails"`
	Value             InterfaceRoleDef   `json:"value"`
}

// openshift_ptp_delay_ns
// openshift_ptp_frequency_adjustment_ns
// openshift_ptp_max_offset_ns
// openshift_ptp_offset_ns
// todo change name
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
	Config      string             `json:"config"`
	MoreDetails metricsNodeDetails `json:"moreDetails"`
	Value       ProcessStatusDef   `json:"value"`
}

// prom_http_metric_handler_requests_in_flight int

// prom_http_metric_handler_requests_total
type promHttpHandlerRequests struct {
	Code  int `json:"code"`
	Value int `json:"value"`
>>>>>>> 02b3c769 (Infa tests)
}
