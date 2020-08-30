package servers

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

const maxBufferSize = 8972

func defineConnection(serverPort int) net.PacketConn {
	pc, err := net.ListenPacket("udp", fmt.Sprintf("0.0.0.0:%d", serverPort))
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	return pc
}

// RunMulticastUDPServer starts multicast udp server
func RunMulticastUDPServer(serverPort int, serverIP string, udpDatagramSize int) {
	var testString string
	raddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", serverIP, serverPort))
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	defer conn.Close()
	for i := 1; i <= udpDatagramSize; i++ {
		testString += "a"
	}
	byteTestString := []byte(testString)
	log.Print("Start UDP Mulicast Server")
	for {
		log.Printf("Transmit udp datagramm: size %d to multicast address %s", udpDatagramSize, serverIP)
		time.Sleep(2 * time.Second)
		byteTransmitted, err := conn.Write(byteTestString)
		if err != nil {
			log.Printf("udp datagramm size %d transmission to %s status error: %s", byteTransmitted, serverIP, err)
		}
		log.Printf("udp datagramm size %d transmission to %s status OK", byteTransmitted, serverIP)
	}
}

// RunUDPServer starts udp server
func RunUDPServer(serverPort int) {
	pc := defineConnection(serverPort)
	defer pc.Close()
	doneChan := make(chan error, 1)
	buffer := make([]byte, maxBufferSize)
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
