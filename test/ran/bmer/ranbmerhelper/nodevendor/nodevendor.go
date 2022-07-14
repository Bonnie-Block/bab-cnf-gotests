package nodevendor

import (
	"fmt"
	"strings"

	"github.com/stmcginnis/gofish"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/bmer/ranbmerparameters"
)

// GetRedfishVendor queries the redfish root for the OEM field and returns the vendor.
func GetRedfishVendor(session *gofish.APIClient) (string, error) {
	serviceRoot, err := gofish.ServiceRoot(session)
	if err != nil {
		return "", fmt.Errorf("failed to GET redfish service root")
	}

	oem := string(serviceRoot.Oem)

	switch {
	case strings.Contains(oem, ranbmerparameters.DellRedfishOem):
		return ranbmerparameters.Dell, nil
	case strings.Contains(oem, ranbmerparameters.HpeRedfishOem):
		return ranbmerparameters.Hpe, nil
	case strings.Contains(oem, ranbmerparameters.ZTRedfishOem):
		return ranbmerparameters.ZT, nil
	default:
		return "", fmt.Errorf("failed to match vendor from output: %v", oem)
	}
}
