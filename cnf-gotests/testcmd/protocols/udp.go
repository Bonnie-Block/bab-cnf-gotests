package protocols

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

const (
	// ProtocolUDP the name of the protocol
	ProtocolUDP       = "udp"
	packagesNumberUDP = 5
	timeout           = 5
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
)

// UDPTest define, run and process return code of udp test command
type UDPTest struct {
	CommonTest
	ServerPort    int
	Multicast     bool
	Broadcast     bool
	InterfaceName *net.Interface
}

//NewUDPTest creates new instance of ConnectivityTestParameters
func NewUDPTest(mtu int, protocolVersion int, serverIP string, serverPort int, negative bool, multicast bool, broadcast bool, interfaceName string) *UDPTest {
	if multicast {
		intFace, err := net.InterfaceByName(interfaceName)
		if err != nil {
			fmt.Print(err)
			os.Exit(1)
		}
		return &UDPTest{CommonTest{mtu, serverIP, protocolVersion, negative}, serverPort, multicast, broadcast, intFace}
	}
	return &UDPTest{CommonTest{mtu, serverIP, protocolVersion, negative}, serverPort, multicast, broadcast, nil}
}

func (test *UDPTest) resolveAddress() *net.UDPAddr {
	addr, err := net.ResolveUDPAddr(fmt.Sprintf("%s%d", ProtocolUDP, test.ProtocolVersion),
		fmt.Sprintf("[%s]:%d", test.ServerIP, test.ServerPort))
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
	deadline := time.Now().Add(timeout * time.Second)
	conn.SetDeadline(deadline)
	f, err := conn.File()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	timeVal := new(syscall.Timeval)
	timeVal.Sec = timeout
	err = syscall.SetsockoptTimeval(int(f.Fd()), syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, timeVal)
	if err != nil {
		fmt.Printf("Error define send timeout %s", err)
		os.Exit(1)
	}
	err = syscall.SetsockoptTimeval(int(f.Fd()), syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, timeVal)
	if err != nil {
		fmt.Printf("Error define DF receive timeout %s", err)
		os.Exit(1)
	}
	if test.ProtocolVersion == 4 {
		err = syscall.SetsockoptInt(int(f.Fd()), syscall.IPPROTO_IP, syscall.IP_MTU_DISCOVER, syscall.IP_PMTUDISC_DO)
	} else {
		err = syscall.SetsockoptInt(int(f.Fd()), syscall.IPPROTO_IPV6, syscall.IPV6_MTU_DISCOVER, syscall.IPV6_PMTUDISC_DO)
	}
	if err != nil {
		fmt.Printf("Error define MTU discovery flag %s", err)
		os.Exit(1)
	}
	_, err = conn.Write(byteTestString)
	elapsed := time.Since(startTime)
	if err != nil {
		fmt.Println(err)
		statTotalTime += elapsed.Microseconds()
		statPacketLost++
		exitCode = 1
		return
	}
	bnumber, addr, err := conn.ReadFromUDP(buffer)
	if err != nil {
		fmt.Printf("Package lost\n")
		statPacketLost++
		exitCode = 1
		return
	}
	receivedFromServerString := string(bytes.Trim(buffer, "\x00"))
	if receivedFromServerString == testString {
		statTotalTime += elapsed.Microseconds()
		fmt.Printf("%d bytes from %s: udp_seq=%d time=%dms\n", bnumber, addr, packetNumber, elapsed.Microseconds())
		statPacketReceived++
	}
}

func (test *UDPTest) testUnicastUDP() error {
	raddr := test.resolveAddress()
	conn, err := net.DialUDP(fmt.Sprintf("%s%d", ProtocolUDP, test.ProtocolVersion), nil, raddr)
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

func (test *UDPTest) receiveUDPTraffic(conn *net.UDPConn) error {
	buffer := make([]byte, test.MTU)
	for i := 0; i <= packagesNumberUDP; i++ {
		deadline := time.Now().Add(timeout * time.Second)
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
	pc, err := net.ListenMulticastUDP(fmt.Sprintf("%s%d", ProtocolUDP, test.ProtocolVersion), test.InterfaceName, addr)
	if err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
	defer pc.Close()
	pc.SetReadBuffer(test.MTU)
	err = test.receiveUDPTraffic(pc)
	if err != nil {
		return err
	}
	return nil
}

func (test *UDPTest) testBroadcastUDP() error {
	var err error
	addr := test.resolveAddress()
	pc, err := net.ListenUDP(ProtocolUDP, addr)
	if err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
	defer pc.Close()
	pc.SetReadBuffer(test.MTU)
	err = test.receiveUDPTraffic(pc)
	if err != nil {
		return err
	}
	return nil
}

// RunTest runs the test
func (test *UDPTest) RunTest() {
	var err error
	switch {
	case test.Multicast:
		err = test.testMulticastUDP()
	case test.Broadcast:
		err = test.testBroadcastUDP()
	default:
		err = test.testUnicastUDP()
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
