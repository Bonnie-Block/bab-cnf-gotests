package tests

import (
	"os"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/network/sriov/parameters"
	testclient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
)

var (
	clients            *testclient.ClientSet
	operatorNamespace  string
	sriovSmokeTestMode bool
)

func init() {
	operatorNamespace = os.Getenv("SRIOV_OPERATOR_NAMESPACE")
	if operatorNamespace == "" {
		operatorNamespace = parameters.OperatorNamespace
	}
	sriovSmokeTestModeEnvVar := os.Getenv("CNF_GOTESTS_SRIOV_SMOKE")
	if sriovSmokeTestModeEnvVar == "true" {
		sriovSmokeTestMode = true
	}

	clients = testclient.New("")
}
