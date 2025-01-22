package ranptpparameters

type ClockState int8
type InterfaceRole int8
type ProcessStatus int8
type Status int
type InterfaceState string
type RoleMap map[string]string
type ClockClass int64

const (
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

	ProcessPTP4L   = "ptp4l"
	ProcessPHC2SYS = "phc2sys"
	ProcessDPLL    = "dpll"
	ProcessGNSS    = "gnss"
	ProcessTS2PHC  = "ts2phc"
	ProcessGM      = "GM"

	Available   = 1
	Unavailable = 0

	Inactive = int64(0)
	Active   = int64(1)

	Off InterfaceState = "down"
	On  InterfaceState = "up"

	HoldOverTimeout    = "HoldOverTimeout"
	MaxOffsetThreshold = "MaxOffsetThreshold"
	MinOffsetThreshold = "MinOffsetThreshold"

	OpenshiftPtpClockState      = "openshift_ptp_clock_state"
	OpenshiftPtpProcessStatus   = "openshift_ptp_process_status"
	OpenshiftPtpInterfaceRole   = "openshift_ptp_interface_role"
	OpenshiftPtpThreshold       = "openshift_ptp_threshold"
	OpenshiftPtpNmeaStatus      = "openshift_ptp_nmea_status"
	OpenshiftPtpHaProfileStatus = "openshift_ptp_ha_profile_status"
	OpenshiftPtpPpsStatus       = "openshift_ptp_pps_status"
	OpenshiftPtpClockClass      = "openshift_ptp_clock_class"

	// PtpMetricsCmd cmd to collect ptp Metrics via ptp pod.
	// sleep 0.1 is needed to workaround buffer/stream handling issue, which often returns incomplete stdout.
	PtpMetricsCmd = "curl -s localhost:9091/metrics; sleep 0.1"
)

var (
	// MetricMap contains all the metrics detail, the key is the profile name, and then the metric name.
	MetricMap map[string][]MetricDetails

	// InterfacesRoleMap maps Interface IDs to their role (master/slave).
	InterfacesRoleMap map[string]RoleMap
)

type MetricDetails struct {
	Status             string        `json:"status"`
	Value              int64         `json:"value"`
	Address            string        `json:"address"`
	Type               string        `json:"type"`
	Node               string        `json:"node"`
	Process            string        `json:"process"`
	Interface          string        `json:"interface"`
	ClockStateValue    ClockState    `json:"clock_state_value"`
	InterfaceRoleValue InterfaceRole `json:"interface_role_value"`
	ProcessStatusValue ProcessStatus `json:"process_status_value"`
	From               string        `json:"from"`
	Config             string        `json:"config"`
	Code               int           `json:"code"`
	Threshold          string        `json:"threshold"`
	Profile            string        `json:"profile"`
}
