package servers

import (
	"fmt"
	"log"
	"net"
	"syscall"

	"github.com/ishidawataru/sctp"
)

// RunSCTP runs a sctp server
func RunSCTP(serverAddr string, port int, mtu int, interfaceName string, protocolVersion int) {
	address, err := net.ResolveIPAddr("ip", serverAddr)
	if err != nil {
		exitWithError(err)
	}

	listenAddr := &sctp.SCTPAddr{
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
						log.Fatalf("syscall.SetsockoptInt(SCTP_DISABLE_FRAGMENTS) error: %v", err)
					}
					if interfaceName != "" {
						err = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, interfaceName)
						if err != nil {
							log.Fatalf("syscall.SetsockoptInt(SO_BINDTODEVICE) error: %v", err)
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

	network := fmt.Sprintf("ipv%d", protocolVersion)

	listener, err := socketConfig.Listen(network, listenAddr)
	if err != nil {
		exitWithError(err)
	}

	conn, err := listener.Accept()
	if err != nil {
		exitWithError(err)
	}

	buf := make([]byte, mtu)
	_, err = conn.Read(buf)
	if err != nil {
		exitWithError(err)
	}

	err = conn.Close()
	if err != nil {
		exitWithError(err)
	}
}

func exitWithError(err error) {
	log.Fatalf("sctp server error: %v", err)
}
