package parameters

import (
	"os"
)

var (
	SriovOperatorNamespace string
	PtpOperatorNamespace   string
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
}
