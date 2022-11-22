package netsriovhelper

import (
	"encoding/json"
	"fmt"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/switchcmd"
)

// RollBackToOriginalConfig returns the switch configuration that was before the test.
func RollBackToOriginalConfig(credentials *nethelper.SwitchCredentials) error {
	jnpr, err := switchcmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	err = jnpr.RollbackConfig(switchcmd.CountChanges)
	if err != nil {
		return err
	}

	switchcmd.CountChanges = 0

	return nil
}

func setOrDeleteNonLACPLAGOnJunos(credentials *nethelper.SwitchCredentials,
	slaveInterfaceNames []string, aggregatedInterfaceName, action string) error {
	if action != "set" && action != "delete" {
		return fmt.Errorf("unknown action %s", action)
	}

	jnpr, err := switchcmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	var commands []string
	for _, slaveInterfaceName := range slaveInterfaceNames {
		commands = append(commands, fmt.Sprintf("%s interfaces %s ether-options 802.3ad %s", action,
			slaveInterfaceName, aggregatedInterfaceName))
	}
	commands = append(commands, fmt.Sprintf("%s interfaces %s unit 0 family ethernet-switching",
		action, aggregatedInterfaceName))

	err = jnpr.Config(commands)

	return err
}

func removeAllConfigurationFromInterfaces(credentials *nethelper.SwitchCredentials, switchInterfaces []string) error {
	jnpr, err := switchcmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	for _, switchInterface := range switchInterfaces {
		commands := []string{fmt.Sprintf("edit interfaces %s", switchInterface), switchcmd.DeleteAction}

		err = jnpr.Config(commands)
		if err != nil {
			return err
		}
	}

	return nil
}

func setSwitchInterfaceStatus(credentials *nethelper.SwitchCredentials, switchInterface, action string) error {
	if action != switchcmd.SetAction && action != switchcmd.DeleteAction {
		return fmt.Errorf("unknown action %s", action)
	}

	jnpr, err := switchcmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	err = jnpr.Config([]string{fmt.Sprintf("%s interfaces %s disable", action, switchInterface)})

	return err
}

func configureMTUOnSwitchInterfaces(credentials *nethelper.SwitchCredentials,
	switchIntFace []string, mtu string) error {
	jnpr, err := switchcmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	var commands []string
	for _, switchInterface := range switchIntFace {
		commands = append(commands, fmt.Sprintf("set interfaces %s mtu %s", switchInterface, mtu))
	}

	err = jnpr.Config(commands)

	return err
}

func isSwitchInterfaceUp(credentials *nethelper.SwitchCredentials, switchInterface string) (bool, error) {
	jnpr, err := switchcmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return false, err
	}
	defer jnpr.Close()

	jsonOutput, err := jnpr.RunCommand(fmt.Sprintf("show interfaces %s", switchInterface))
	if err != nil {
		return false, err
	}

	var interfaceStatus switchcmd.InterfaceStatus

	err = json.Unmarshal([]byte(jsonOutput), &interfaceStatus)
	if err != nil {
		return false, err
	}

	return interfaceStatus.InterfaceInformation[0].PhysicalInterface[0].OperStatus[0].Data == "up", nil
}
