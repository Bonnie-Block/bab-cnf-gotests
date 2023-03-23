//nolint
package netsriovhelper

import (
	"encoding/json"
	"fmt"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/nethelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/switchcmd"
)

var InterfaceConfigs []string

// DeleteNonLACPLAGsOnJunos deletes given LACP Link Aggregated ports.
func DeleteNonLACPLAGsOnJunos(credentials *nethelper.SwitchCredentials,
	aggregatedInterfaceNames []string) error {
	jnpr, err := switchcmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	var commands []string
	for _, aggregatedInterface := range aggregatedInterfaceNames {
		commands = append(commands, fmt.Sprintf("delete interfaces %s", aggregatedInterface))
	}

	err = jnpr.Config(commands)

	return err
}

// RestoreSwitchInterfacesConfiguration restores the configuration of specified switch interfaces.
func RestoreSwitchInterfacesConfiguration(credentials *nethelper.SwitchCredentials, switchInterfaces []string) error {
	err := RemoveAllConfigurationFromInterfaces(credentials, switchInterfaces)
	if err != nil {
		return err
	}

	err = restoreInterfaceConfigs(credentials)
	if err != nil {
		return err
	}

	return nil
}

// RemoveAllConfigurationFromInterfaces removes  all configuration from given switch interfaces.
func RemoveAllConfigurationFromInterfaces(credentials *nethelper.SwitchCredentials, switchInterfaces []string) error {
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

func dumpInterfaceConfigs(credentials *nethelper.SwitchCredentials, switchInterfaces []string) error {
	for _, switchInterface := range switchInterfaces {
		config, err := getInterfaceConfig(credentials, switchInterface)
		if err != nil {
			return err
		}
		InterfaceConfigs = append(InterfaceConfigs, config)
	}

	return nil
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

func setNonLACPLAGOnJunos(credentials *nethelper.SwitchCredentials,
	slaveInterfaceNames []string, aggregatedInterfaceName string) error {
	jnpr, err := switchcmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	var commands []string

	if len(slaveInterfaceNames) > 0 {
		for _, slaveInterfaceName := range slaveInterfaceNames {
			commands = append(commands, fmt.Sprintf("set interfaces %s ether-options 802.3ad %s",
				slaveInterfaceName, aggregatedInterfaceName))
		}
	}

	commands = append(commands, fmt.Sprintf("set interfaces %s unit 0 family ethernet-switching",
		aggregatedInterfaceName))

	err = jnpr.Config(commands)

	return err
}

func restoreInterfaceConfigs(credentials *nethelper.SwitchCredentials) error {
	if len(InterfaceConfigs) > 0 {
		var err error
		for _, interfaceConfig := range InterfaceConfigs {
			err = applyInterfaceConfig(credentials, interfaceConfig)
			if err != nil {
				return err
			}
		}

		InterfaceConfigs = []string{}
	}

	return nil
}

func getInterfaceConfig(credentials *nethelper.SwitchCredentials, switchInterface string) (string, error) {
	jnpr, err := switchcmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return "", err
	}
	defer jnpr.Close()

	interfaceConfig, err := jnpr.GetInterfaceConfig(switchInterface)
	if err != nil {
		return "", err
	}

	return interfaceConfig, nil
}

func applyInterfaceConfig(credentials *nethelper.SwitchCredentials, interfaceConfig string) error {
	jnpr, err := switchcmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	err = jnpr.ApplyConfigInterface(interfaceConfig)

	return err
}
