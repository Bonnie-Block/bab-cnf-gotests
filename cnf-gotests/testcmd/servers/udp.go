package servers

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"syscall"
	"time"
)

func defineConnection(serverPort int) net.PacketConn {
	pc, err := net.ListenPacket("udp", fmt.Sprintf("0.0.0.0:%d", serverPort))
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	return pc
}

// RunBroadcastUDPServer starts multicast udp server
func RunBroadcastUDPServer(serverPort int, serverIP string, udpDatagramSize int, interfaceName string) {
	runGenericUDPServer("broadcast", serverPort, serverIP, udpDatagramSize, interfaceName)
}

// RunMulticastUDPServer starts multicast udp server
func RunMulticastUDPServer(serverPort int, serverIP string, udpDatagramSize int, interfaceName string) {
	runGenericUDPServer("multicast", serverPort, serverIP, udpDatagramSize, interfaceName)
}

func runGenericUDPServer(mode string, serverPort int, serverIP string, udpDatagramSize int, interfaceName string) {
	var testString string
	raddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", serverIP, serverPort))
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	intFace, err := net.InterfaceByName(interfaceName)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	intFaceAddreses, err := intFace.Addrs()
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	if len(intFaceAddreses) < 1 {
		log.Print(fmt.Sprintf("error: can not find ip address on interface %s", interfaceName))
		os.Exit(1)
	}
	intFaceAddr := strings.Split(intFaceAddreses[0].String(), "/")[0]
	laddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", intFaceAddr, serverPort))
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	conn, err := net.DialUDP("udp", laddr, raddr)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	defer conn.Close()
	//Set DF flage on socket
	f, err := conn.File()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	timeVal := new(syscall.Timeval)
	timeVal.Sec = 5
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
	err = syscall.SetsockoptInt(int(f.Fd()), syscall.IPPROTO_IP, syscall.IP_MTU_DISCOVER, syscall.IP_PMTUDISC_DO)
	if err != nil {
		fmt.Printf("Error define DF flag %s", err)
		os.Exit(1)
	}
	err = syscall.SetsockoptInt(int(f.Fd()), syscall.IPPROTO_IP, syscall.IP_MTU_DISCOVER, syscall.IP_PMTUDISC_DO)
	if err != nil {
		fmt.Printf("Error define DF flag %s", err)
		os.Exit(1)
	}
	for i := 1; i <= udpDatagramSize; i++ {
		testString += "a"
	}
	byteTestString := []byte(testString)
	log.Printf("Start UDP %s Server", mode)
	for {
		log.Printf("Transmit udp datagramm: size %d to %s address %s", udpDatagramSize, mode, serverIP)
		time.Sleep(2 * time.Second)
		byteTransmitted, err := conn.Write(byteTestString)
		if err != nil {
			log.Printf("udp datagramm size %d transmission to %s status error: %s", byteTransmitted, serverIP, err)
		}
		log.Printf("udp datagramm size %d transmission to %s status OK", byteTransmitted, serverIP)
	}
}

// RunUDPServer starts udp server
func RunUDPServer(serverPort int, bufferSize int) {
	pc := defineConnection(serverPort)
	defer pc.Close()
	doneChan := make(chan error, 1)
	buffer := make([]byte, bufferSize)
	log.Print("Start UDP Server")
	go func() {
		for {
			n, addr, err := pc.ReadFrom(buffer)
			if err != nil {
				doneChan <- err
				return
			}
			log.Printf("packet-received: bytes=%d from=%s\n",
				n, addr.String())
			deadline := time.Now().Add(20 * time.Second)
			err = pc.SetWriteDeadline(deadline)
			if err != nil {
				doneChan <- err
				return
			}
			n, err = pc.WriteTo(buffer[:n], addr)
			if err != nil {
				doneChan <- err
				return
			}
			log.Printf("packet-written: bytes=%d to=%s\n", n, addr.String())
		}
	}()
	var err error
	select {
	case err = <-doneChan:
		log.Printf("error occurred: %s", err)
	}
	return
}
