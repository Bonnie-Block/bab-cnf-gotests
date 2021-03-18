package tests

import (
	"os"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/ptp/parameters"
	testclient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
)

var (
	apiclient         *testclient.ClientSet
	operatorNamespace string
)

func init() {
	operatorNamespace = os.Getenv("PTP_OPERATOR_NAMESPACE")
	if operatorNamespace == "" {
		operatorNamespace = parameters.OperatorNamespace
	}
	apiclient = testclient.New("")
}
