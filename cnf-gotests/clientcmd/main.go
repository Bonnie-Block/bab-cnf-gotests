package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/cnf-gotests/clientcmd/protocols"
)

var (
	supportedProtocols = []string{protocols.ProtocolICMP, "udp", "tcp", "sctp"}
)

func validateIP(host string) error {
	if net.ParseIP(host) != nil {
		return nil
	}
	return fmt.Errorf("Unsupported parameter server ip=%s", host)
}

func ipProtocolVersion(host string) int {
	if strings.Contains(host, ":") {
		return 6
	}
	return 4
}

func validateMtu(mtuSize int) error {
	if mtuSize > 50 && mtuSize < 9000 {
		return nil
	}
	return fmt.Errorf("Unsupported parameter mtu=%d", mtuSize)
}

func validateProtocol(protocolName string) error {
	for _, item := range supportedProtocols {
		if protocolName == item {
			return nil
		}
	}
	return fmt.Errorf("Unsupported parameter protocol=%s", protocolName)
}

func main() {
	protocol := flag.String("protocol", "icmp", "Protocol name. Options: tcp/udp/icmp/sctp")
	mtu := flag.Int("mtu", 1450, "Protocol name. Options: Any int in range 50-9000")
	dstAddress := flag.String("server", "", "Destination ip address IPv4/IPv6")
	negative := flag.Bool("negative", false, "Insert this flag if no connectivity expected")
	flag.Parse()
	err := validateProtocol(*protocol)

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	err = validateMtu(*mtu)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	err = validateIP(*dstAddress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	protocolVersion := ipProtocolVersion(*dstAddress)

	switch *protocol {
	case protocols.ProtocolICMP:
		test := protocols.NewICMPTest(*mtu, protocolVersion, *dstAddress, *negative)
		test.RunTest()
	}
}
