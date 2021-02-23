package protocols

import (
	"errors"
	"fmt"
	"log"
	"net"
	"syscall"

	"github.com/ishidawataru/sctp"
)

const (
	// ProtocolSCTP is sctp's protocol name
	ProtocolSCTP = "sctp"
)

// SCTPTest is a struct with information for sctp test
type SCTPTest struct {
	CommonTest
	ServerPort int
}

// NewSCTPTest returns a new SCTP test
func NewSCTPTest(mtu int, serverIP string, protocolVersion int, serverPort int, negative bool) *SCTPTest {
	return &SCTPTest{CommonTest{mtu, serverIP, protocolVersion, negative}, serverPort}
}

// RunTest runs the sctp test
func (sctpTest *SCTPTest) RunTest() {
	err := runClient(sctpTest.ServerIP, sctpTest.ServerPort, sctpTest.MTU, "", sctpTest.ProtocolVersion)
	if sctpTest.Negative == true {
		if err != nil {
			log.Printf("SCTP test failed as expected with error: %v\n", err)
			return
		}
		log.Fatalln("SCTP Negative test failed.")
	}
	if err != nil {
		log.Fatalf("SCTP test failed with error: %v\n", err)
	}
	log.Println("SCTP test passed as expected")
}

func runClient(serverAddr string, port int, mtu int, interfaceName string, protocolVersion int) error {
	address, err := net.ResolveIPAddr("ip", serverAddr)
	server := &sctp.SCTPAddr{
		IPAddrs: []net.IPAddr{*address},
		Port:    port,
	}

	socketConfig := &sctp.SocketConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			err := c.Control(
				func(fd uintptr) {
					// value is 1 to set SCTP_DISABLE_FRAGMENTS to true
					err := syscall.SetsockoptInt(int(fd), syscall.IPPROTO_SCTP, sctp.SCTP_DISABLE_FRAGMENTS, 1)
					if err != nil {
						log.Fatalf("runClient, syscall.SetsockoptInt(SCTP_DISABLE_FRAGMENTS) error: %v", err)
					}
					if interfaceName != "" {
						err = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, interfaceName)
						if err != nil {
							log.Fatalf("runClient, syscall.SetsockoptInt(SO_BINDTODEVICE) error: %v", err)
						}
					}
				},
			)
			return err
		},
		InitMsg: sctp.InitMsg{
			NumOstreams:  5,
			MaxInstreams: 5,
			MaxAttempts:  4,
		},
	}

	laddr := &sctp.SCTPAddr{
		IPAddrs: nil,
		Port:    0,
	}

	network := fmt.Sprintf("ipv%d", protocolVersion)

	conn, err := socketConfig.Dial(network, laddr, server)
	if err != nil {
		return fmt.Errorf("socketConfig.Dial() failed with error: %v", err)
	}

	buff := make([]byte, mtu)
	info := &sctp.SndRcvInfo{}
	n, err := conn.SCTPWrite(buff, info)
	if err != nil {
		return fmt.Errorf("conn.SCTPWrite failed with error: %v", err)
	} else if n != mtu {
		return errors.New("SCTPWrite() failed to write all of the buffer")
	}

	return conn.Close()
}
