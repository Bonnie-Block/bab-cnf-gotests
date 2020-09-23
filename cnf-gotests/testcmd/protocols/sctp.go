package protocols

import (
	"errors"
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
func NewSCTPTest(mtu int, serverIP string, protocolVersion int, serverPort int) *SCTPTest {
	return &SCTPTest{CommonTest{mtu, serverIP, protocolVersion, false}, serverPort}
}

// RunTest runs the sctp test
func (sctpTest *SCTPTest) RunTest() {
	err := runClient(sctpTest.ServerIP, sctpTest.ServerPort, sctpTest.MTU, "")
	if err != nil {
		if sctpTest.Negative == true {
			log.Println("SCTP test failed as expected")
			return
		}
		log.Fatalf("SCTP test failed with error: %v\n", err)
	}
	log.Println("SCTP test passed as expected")
}

func runClient(serverAddr string, port int, mtu int, interfaceName string) error {
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
		IPAddrs: []net.IPAddr{{IP: net.IPv4zero}},
		Port:    0,
	}

	conn, err := socketConfig.Dial("ipv4", laddr, server)
	if err != nil {
		return err
	}

	buff := make([]byte, mtu)
	info := &sctp.SndRcvInfo{}
	n, err := conn.SCTPWrite(buff, info)
	if err != nil {
		return err
	} else if n != mtu {
		return errors.New("SCTPWrite() failed to write all of the buffer")
	}

	return conn.Close()
}
