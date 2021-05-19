package helper

import (
	"fmt"
	"github.com/opentracing/opentracing-go/log"
	testclient "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/client"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
)

var (
	Apiclient *testclient.ClientSet
)

func init() {
	var err error
	Apiclient, err = config.DefineClients()
	if err != nil {
		log.Error(fmt.Errorf("can not load api client. Please check KUBECONFIG env var"))
	}
}

