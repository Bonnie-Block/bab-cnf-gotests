package ranptpparameters

type ClockState int8
type InterfaceRole int8
type ProcessStatus int8
type Process int8
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

	PTP4L       Process = 1
	PHC2SYS     Process = 2
	GRANDMASTER Process = 3
	DPLL        Process = 4
	GNSS        Process = 5

	Success Status = 0
	Failed  Status = 1
	Active  Status = 2

	Off InterfaceState = "down"
	On  InterfaceState = "up"

	OpenshiftPtpClockState    = "openshift_ptp_clock_state"
	OpenshiftPtpProcessStatus = "openshift_ptp_process_status"
	OpenshiftPtpInterfaceRole = "openshift_ptp_interface_role"
	OpenshiftPtpThreshold     = "openshift_ptp_threshold"

	ClockClassFreerun  ClockClass = 248
	ClockClassHoldOver ClockClass = 7
	ClockClassLocked   ClockClass = 6

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
		"GM":      GRANDMASTER,
		"dpll":    DPLL,
		"gnss":    GNSS,
		"":        -1,
	}

	// MetricMap contains all the metrics detail, the key is the profile name, and then the metric name.
	MetricMap map[string][]MetricDetails

	// InterfacesRoleMap maps Interface IDs to their role (master/slave).
	InterfacesRoleMap map[string]RoleMap
)

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
}
