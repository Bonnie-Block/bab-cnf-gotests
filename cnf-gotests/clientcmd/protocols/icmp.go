package protocols

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

const (
	// ProtocolICMP the name of the protocol
	ProtocolICMP   = "icmp"
	packagesNumber = 5
)

// ICMPTest define, run and process return code of icmp test command
type ICMPTest struct {
	common CommonTest
}

// NewICMPTest creates new instance of ConnectivityTestParameters
func NewICMPTest(mtu int, protocolVersion int, serverIP string, negative bool) *ICMPTest {
	return &ICMPTest{
		common: CommonTest{
			MTU:             mtu,
			ServerIP:        serverIP,
			ProtocolVersion: protocolVersion,
			Negative:        negative,
		}}
}

func (test *ICMPTest) defineCommand() string {
	command := []string{"ping", fmt.Sprintf("-%d", test.common.ProtocolVersion),
		test.common.ServerIP, "-c", fmt.Sprintf("%d", packagesNumber), "-s", fmt.Sprintf("%d", test.common.MTU), "-M", "do"}
	return strings.Join(command, " ")
}

// RunTest runs the test
func (test *ICMPTest) RunTest() {
	command := test.defineCommand()
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Println(cmd.String())
	log.Print(command)
	err := cmd.Run()
	if test.common.Negative {
		if err != nil {
			log.Print("ICMP test failed as expected")
			os.Exit(0)
		} else {
			log.Fatalf("negative test failed to return code 1")
			os.Exit(1)
		}
	}
	if err != nil {
		log.Fatalf("cmd.Run() failed with %s\n", err)
		os.Exit(1)
	}
}
