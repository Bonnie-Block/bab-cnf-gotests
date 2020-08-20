package tests

import (
	"os"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/parameters"
	testclient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
)

var (
	clients           *testclient.ClientSet
	operatorNamespace string
)

func init() {
	operatorNamespace = os.Getenv("OPERATOR_NAMESPACE")
	if operatorNamespace == "" {
		operatorNamespace = parameters.OperatorNamespace
	}

	clients = testclient.New("")

}
