package protocols

import (
	"fmt"
	"log"
	"os"
	"strings"
)

const (
	// ProtocolTCP the name of the protocol
	ProtocolTCP                        = "tcp"
	packagesNumberTCP                  = 5
	nmapConnectionSuccessOutputPattern = "Successful connections"
	nmapTrafficSuccessOutputPattern    = "Rcvd"
	nmapTrafficFailedOutputPattern     = "Lost"
)

// TCPTest define, run and process return code of tcp test command
type TCPTest struct {
	CommonTest
	ServerPort int
	FrameSize  int
}

// NewTCPTest creates new instance of ConnectivityTestParameters
func NewTCPTest(mtu int, protocolVersion int, serverIP string, serverPort int, negative bool) *TCPTest {
	var frameSize int
	if mtu <= 1450 {
		frameSize = 1464
	} else if mtu <= 1500 {
		frameSize = 1512
	} else {
		frameSize = 9000
	}
	return &TCPTest{CommonTest{mtu, serverIP, protocolVersion, negative}, serverPort, frameSize}
}

func (test *TCPTest) defineUnicastBaseCommand() []string {
	return []string{"nping", fmt.Sprintf("-%d", test.ProtocolVersion), fmt.Sprintf("-c %d", packagesNumberTCP), "-df"}
}

func (test *TCPTest) runCommandAndCompareOutput(command string, output string) error {
	commandOutput, err := test.RunCommand(command)
	if err != nil {
		return err
	}
	if strings.Contains(string(commandOutput), output) {
		return nil
	}
	return fmt.Errorf("wrong command output - %s expected substring - %s", commandOutput, output)
}

func (test *TCPTest) testUnicastTCP() error {
	var expectedOutputConnection string
	var expectedOutputTraffic string
	var err error
	if test.Negative {
		expectedOutputTraffic = fmt.Sprintf("%s: %d", nmapTrafficFailedOutputPattern, packagesNumberTCP)
	} else {
		expectedOutputConnection = fmt.Sprintf("%s: %d", nmapConnectionSuccessOutputPattern, packagesNumberTCP)
		expectedOutputTraffic = fmt.Sprintf("%s: %d", nmapTrafficSuccessOutputPattern, packagesNumberTCP)
	}
	testCommand := append(test.defineUnicastBaseCommand(), fmt.Sprintf("-p %d", test.ServerPort), test.ServerIP)
	if !test.Negative {
		err = test.runCommandAndCompareOutput(strings.Join(append(testCommand, "--tcp-connect"), " "), expectedOutputConnection)
		if err != nil {
			return err
		}
	}
	err = test.runCommandAndCompareOutput(
		strings.Join(append(testCommand, fmt.Sprintf("--mtu %d", test.FrameSize), fmt.Sprintf("--data-length %d", test.MTU), "--tcp"), " "),
		expectedOutputTraffic)
	if err != nil {
		return err
	}
	return nil
}

// RunTest runs the test
func (test *TCPTest) RunTest() {
	err := test.testUnicastTCP()
	if err == nil {
		if test.Negative {
			log.Print("TCP test failed as expected")
		} else {
			log.Print("TCP test passed as expected")
		}
		os.Exit(0)
	}
	log.Print(err)
	log.Print("TCP test failed")
	os.Exit(1)
}
