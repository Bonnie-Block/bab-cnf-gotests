package tests

import (
	testclient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
)

var (
	apiclient *testclient.ClientSet
)

func init() {
	apiclient = testclient.New("")

}