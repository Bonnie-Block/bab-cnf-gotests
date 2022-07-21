package ranhweventhelper

import (
	"fmt"
	"log"
	"time"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"

	"github.com/gosnmp/gosnmp"
)

const (
	ModeOn  int = 1
	ModeOff int = 2
)

var (
	Name      string
	socketOid string
	Client    *gosnmp.GoSNMP
)

// Init initiate a connection to PDU and keep it
// in a global snmp.Client.
func Init() error {
	if helper.Config.Ran.PduAddr != "" {
		Name = "socket " + helper.Config.Ran.PduSocket[len(helper.Config.Ran.PduSocket)-2:]
		socketOid = helper.Config.Ran.PduSocket // "1.3.6.1.4.1.318.1.1.4.4.2.1.3.15"
		Client = &gosnmp.GoSNMP{
			Target:    helper.Config.Ran.PduAddr, // "10.46.61.226",
			Port:      161,
			Community: "private",
			Version:   gosnmp.Version2c,
			Timeout:   time.Duration(3) * time.Second,
		}
		err := Client.Connect()

		if err != nil {
			return fmt.Errorf("SNMP connect() due to err: %w", err)
		}

		return nil
	}

	return fmt.Errorf("missing PduAddr in config")
}

// Close close the snmp client connection.
func Close() {
	Client.Conn.Close()
}

// GetSocket returns true if socket is on or error if it exist.
func GetSocket() (bool, error) {
	result, err := Client.Get([]string{socketOid})
	if err != nil {
		log.Printf("SNMP GetSocket() err: %v\n", err)

		return false, err
	}

	return result.Variables[0].Value == ModeOn, nil
}

// SetSocket returns true if socket is on or error if exist.
func SetSocket(value int) (bool, error) {
	var snmpPDU = []gosnmp.SnmpPDU{{
		Name:  socketOid,
		Type:  gosnmp.Integer,
		Value: value,
	}}

	setResult, setErr := Client.Set(snmpPDU)

	if setErr != nil {
		err := fmt.Errorf("socket failed to set to %v due to err: %w", value, setErr)

		return false, err
	}

	return setResult.Variables[0].Value == ModeOn, nil
}
