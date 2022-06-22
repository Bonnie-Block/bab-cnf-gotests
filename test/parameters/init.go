package parameters

import (
	"os"
)

var (
	SriovOperatorNamespace            string
	PtpOperatorNamespace              string
	MachineConfigOperatorNamespace    string
	PerformanceAddonOperatorNamespace string
	HwEventProxyNamespace             string
)

func init() {
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
}
