package helper

import (
	"os"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/parameters"
)

var (
	SriovOperatorNamespace string
)

func init() {
	SriovOperatorNamespace = os.Getenv("SRIOV_OPERATOR_NAMESPACE")
	if SriovOperatorNamespace == "" {
		SriovOperatorNamespace = parameters.OperatorNamespace
	}
}
