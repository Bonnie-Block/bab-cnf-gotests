package parameters

import (
	"os"
)

var (
	AmqNamespace                      string
	SriovOperatorNamespace            string
	PtpOperatorNamespace              string
	MachineConfigOperatorNamespace    string
	PerformanceAddonOperatorNamespace string
	BmerNamespace                     string
	CloudEventNamespace               string
)

func init() {
	AmqNamespace = "amq-router"

	PtpOperatorNamespace = os.Getenv("PTP_OPERATOR_NAMESPACE")
	if PtpOperatorNamespace == "" {
		PtpOperatorNamespace = ptpOperatorNamespace
	}

	SriovOperatorNamespace = os.Getenv("SRIOV_OPERATOR_NAMESPACE")

	if SriovOperatorNamespace == "" {
		SriovOperatorNamespace = sriovOperatorNamespace
	}

	MachineConfigOperatorNamespace = os.Getenv("MACHINE_CONFIG_OPERATOR_NAMESPACE")

	if MachineConfigOperatorNamespace == "" {
		MachineConfigOperatorNamespace = machineConfigOperatorNamespace
	}

	PerformanceAddonOperatorNamespace = os.Getenv("PERFORMANCE_ADDON_OPERATOR_NAMESPACE")

	if MachineConfigOperatorNamespace == "" {
		MachineConfigOperatorNamespace = performanceAddonOperatorNamespace
	}

	BmerNamespace = os.Getenv("BMER_OPERATOR_NAMESPACE")
	if BmerNamespace == "" {
		BmerNamespace = bmerOperatorNamespace
	}

	CloudEventNamespace = os.Getenv("BMER_OPERATOR_NAMESPACE")
	if CloudEventNamespace == "" {
		CloudEventNamespace = cloudEventNamespace
	}
}
