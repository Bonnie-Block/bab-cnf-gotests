package protocols

// CommonTest keeps common vars from connectivity tests
type CommonTest struct {
	MTU             int
	ServerIP        string
	ProtocolVersion int
	Negative        bool
}
