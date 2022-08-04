package nodevendor

import (
	"fmt"
	"strings"

	"github.com/stmcginnis/gofish"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventparameters"
)

// GetRedfishVendor queries the redfish root for the OEM field and returns the vendor.
func GetRedfishVendor(session *gofish.APIClient) (string, error) {
	serviceRoot, err := gofish.ServiceRoot(session)
	if err != nil {
		return "", fmt.Errorf("failed to GET redfish service root")
	}

	oem := string(serviceRoot.Oem)

	switch {
	case strings.Contains(oem, ranhweventparameters.DellRedfishOem):
		return ranhweventparameters.Dell, nil
	case strings.Contains(oem, ranhweventparameters.HpeRedfishOem):
		return ranhweventparameters.Hpe, nil
	case strings.Contains(oem, ranhweventparameters.ZTRedfishOem):
		return ranhweventparameters.ZT, nil
	default:
		return "", fmt.Errorf("failed to match vendor from output: %v", oem)
	}
}
