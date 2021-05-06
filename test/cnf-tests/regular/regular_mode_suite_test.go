package regular

import (
	"fmt"
	"github.com/onsi/ginkgo/reporters"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	_ "gitlab.cee.redhat.com/cnf/cnf-gotests/test/cnf-tests/regular/tests"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/config"
)

const (
	waitingTime time.Duration = 20 * time.Minute
	timeout                   = 1800 * time.Second
)

func TestRegular(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	configSuite, err := config.NewConfig()
	if err != nil {
		fmt.Print(err)
		return
	}
	junitPath := configSuite.GetReportPath(currentFile)
	RegisterFailHandler(Fail)
	rr := append([]Reporter{}, reporters.NewJUnitReporter(junitPath))
	RunSpecsWithDefaultAndCustomReporters(t, "CNF containers regular mode", rr)
}

