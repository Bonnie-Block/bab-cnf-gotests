package nodevendor

import (
	"fmt"
<<<<<<< HEAD
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
=======
	"log"
	"strings"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhwevent/ranhweventparameters"
	corev1 "k8s.io/api/core/v1"
)

// GetVendor get the server vendor as Dell, HPE or ZTsystems.
func GetVendor(node *corev1.Node) (string, error) {
	cmd := []string{
		"cat",
		"/sys/class/dmi/id/board_vendor",
	}
	output, err := helper.ExecCommandOnNode(node, cmd)

	if err != nil {
		log.Printf("GetVendor() failed to execute %v due to: %v\n", cmd, err)

		return "", err
	}

	log.Printf("Issuing %v on %v output: %v\n", cmd, node.Name, output)

	switch strings.TrimSpace(output) {
	case "Dell Inc.":
		return ranhweventparameters.Dell, nil
	case "HPE":
		return ranhweventparameters.Hpe, nil
	case "ZTSYSTEMS":
		return ranhweventparameters.ZT, nil
	default:
		return "", fmt.Errorf("failed to match vendor from output: %v", strings.TrimSpace(output))
>>>>>>> 6db12c57 (Add PTP events for boundary clock)
	}
}
