package protocols

import (
	"fmt"
	"log"
	"os/exec"
)

// CommonTest keeps common vars from connectivity tests
type CommonTest struct {
	MTU             int
	ServerIP        string
	ProtocolVersion int
	Negative        bool
}

//RunCommand runs command and return output
func (ct *CommonTest) RunCommand(cmd string) (string, error) {
	log.Printf("run command: %s", cmd)
	commandOutput, err := exec.Command("/bin/sh", "-c", cmd).Output()
	if err != nil {
		return "", fmt.Errorf("command execution failed - %s due to the error - %s", cmd, err)
	}
	return string(commandOutput), nil
}
