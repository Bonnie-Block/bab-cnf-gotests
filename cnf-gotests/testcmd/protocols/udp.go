package protocols

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"time"
)

const (
	// ProtocolUDP the name of the protocol
	ProtocolUDP       = "udp"
	packagesNumberUDP = 5
)

func totalPackageLoss(total int, loss int) int {
	if loss != 0 {
		return 100 / (total / loss)
	}
	return 0
}

var (
	statTotalTime      int64
	exitCode           int
	statPacketLost     int
	statPacketReceived int
	testString         string
	timeout            = 10 * time.Second
)

// UDPTest define, run and process return code of udp test command
type UDPTest struct {
	CommonTest
	ServerPort    int
	Multicast     bool
	InterfaceName *net.Interface
}

//NewUDPTest creates new instance of ConnectivityTestParameters
func NewUDPTest(mtu int, protocolVersion int, serverIP string, serverPort int, negative bool, multicast bool, interfaceName string) *UDPTest {
	if multicast {
		intFace, err := net.InterfaceByName(interfaceName)
		if err != nil {
			fmt.Print(err)
			os.Exit(1)
		}
		return &UDPTest{CommonTest{mtu, serverIP, protocolVersion, negative}, serverPort, multicast, intFace}
	}
	return &UDPTest{CommonTest{mtu, serverIP, protocolVersion, negative}, serverPort, multicast, nil}
}

func (test *UDPTest) resolveAddress() *net.UDPAddr {
	addr, err := net.ResolveUDPAddr(ProtocolUDP, fmt.Sprintf("%s:%d", test.ServerIP, test.ServerPort))
	if err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
	return addr
}

func (test *UDPTest) runUDPPing(conn *net.UDPConn, packetNumber int, byteTestString []byte) {
	time.Sleep(1 * time.Second)
	buffer := make([]byte, test.MTU)
	startTime := time.Now()
	deadline := time.Now().Add(timeout)
	conn.SetDeadline(deadline)
	conn.Write(byteTestString)
	bnumber, addr, err := conn.ReadFromUDP(buffer)
	if err != nil {
		fmt.Printf("Package lost\n")
		statPacketLost++
		exitCode = 1
	}
	receivedFromServerString := string(bytes.Trim(buffer, "\x00"))
	elapsed := time.Since(startTime)
	if receivedFromServerString == testString {
		statTotalTime += elapsed.Microseconds()
		fmt.Printf("%d bytes from %s: udp_seq=%d time=%dms\n", bnumber, addr, packetNumber, elapsed.Microseconds())
		statPacketReceived++
	}
}

func (test *UDPTest) testUnicastUDP() error {
	raddr := test.resolveAddress()
	conn, err := net.DialUDP(ProtocolUDP, nil, raddr)
	if err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
	defer conn.Close()
	for i := 1; i <= test.MTU; i++ {
		testString += "a"
	}
	byteTestString := []byte(testString)
	fmt.Printf(fmt.Sprintf("UDP PING %s %d(%d) bytes of data.\n", test.ServerIP, test.MTU, test.MTU+28))
	for i := 1; i <= packagesNumberUDP; i++ {
		test.runUDPPing(conn, i, byteTestString)
	}
	fmt.Printf(fmt.Sprintf("--- %s UDP statistics ---\n", test.ServerIP))
	fmt.Printf(fmt.Sprintf("%d packets transmitted, %d received, %d packet loss, time %dms\n",
		packagesNumberUDP, statPacketReceived, totalPackageLoss(packagesNumberUDP, statPacketLost), statTotalTime))
	if exitCode != 0 {
		return fmt.Errorf("connectivity test failed")
	}
	return nil
}

func (test *UDPTest) receiveUDPMulticast(conn *net.UDPConn) error {
	buffer := make([]byte, test.MTU)
	for i := 0; i <= packagesNumberUDP; i++ {
		deadline := time.Now().Add(timeout)
		conn.SetDeadline(deadline)
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			return err
		}
		fmt.Printf("packet-received: bytes=%d from=%s\n",
			n, addr.String())
	}
	return nil
}

func (test *UDPTest) testMulticastUDP() error {
	var err error
	addr := test.resolveAddress()
	pc, err := net.ListenMulticastUDP(ProtocolUDP, test.InterfaceName, addr)
	if err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
	defer pc.Close()
	pc.SetReadBuffer(test.MTU)
	err = test.receiveUDPMulticast(pc)
	if err != nil {
		return err
	}
	return nil
}

// RunTest runs the test
func (test *UDPTest) RunTest() {
	var err error
	if !test.Multicast {
		err = test.testUnicastUDP()
	} else {
		err = test.testMulticastUDP()
	}
	if err == nil {
		if test.Negative {
			fmt.Print("UDP Negative test failed")
			os.Exit(1)
		} else {
			fmt.Print("UDP test passed as expected")
		}
		os.Exit(0)
	} else {
		if test.Negative {
			fmt.Print("UDP Negative test failed as expected")
			os.Exit(0)
		}
		fmt.Print(err)
		fmt.Print("UDP test failed")
		os.Exit(1)
	}
}
