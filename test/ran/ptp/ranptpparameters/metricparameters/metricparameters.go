package ranptpparameters

type ClockState int8
type InterfaceRole int8
type ProcessStatus int8
type Process int8
type Status int

const (
	FREERUN  ClockState = 0
	LOCKED   ClockState = 1
	HOLDOVER ClockState = 2
)

const (
	PASSIVE InterfaceRole = 0
	SLAVE   InterfaceRole = 1
	MASTER  InterfaceRole = 2
	FAULTY  InterfaceRole = 3
	UNKNOWN InterfaceRole = 4
)

const (
	DOWN ProcessStatus = 0
	UP   ProcessStatus = 1
)

const (
	PHC2SYS Process = 2
	PTP4L   Process = 1
)

const (
	SUCCESS Status = 0
	FAILED  Status = 1
	ACTIVE  Status = 2
)
