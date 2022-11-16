package ranptphelper

import (
	"fmt"

	ptpv1api "github.com/openshift/ptp-operator/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"context"
	"strconv"
	"strings"
)

// GetInterfaces gets an interface role, "mode" and returns the interfaces' ID as a slice.
// the function returns only "master" or "slave" interfaces.
// an error is returned if any occurred.
func GetInterfaces(mode ptpv1api.PtpRole) ([]string, error) {
	var interfaces []string

	configList, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{})
	if nil != err {
		return interfaces, err
	}

	interfacesRoleMap := make(map[string]ranptpparameters.RoleMap)
	for _, config := range configList.Items {
		interfacesRoleMap, err = interfaceSectionParser(config, interfacesRoleMap)
		if err != nil {
			return interfaces, err
		}
	}

	ranptpparameters.InterfacesRoleMap = interfacesRoleMap
	for index, section := range ranptpparameters.InterfacesRoleMap {
		if section["masterOnly"] == strconv.Itoa(int(mode)) {
			interfaces = append(interfaces, index)
		}
	}

	return interfaces, nil
}

// SetInterfaceStatus sets a given interface, "ifaceID", to a given state, "newState",
// for a given pod, "clientPod" in a given container "containerName".
// an error return if any accord.
func SetInterfaceStatus(clientPod *corev1.Pod,
	containerName string,
	ifaceID string,
	newState ranptpparameters.InterfaceState) error {
	_, err := pod.ExecCommand(helper.Apiclient,
		*clientPod, []string{"ip", "link", "set", ifaceID, string(newState)},
		containerName)

	return err
}

// interfaceSectionParser parses the interface section of a given pointer to configuration file, "config".
// an error is returned if any accord.
func interfaceSectionParser(config ptpv1api.PtpConfig,
	interfacesRoleMap map[string]ranptpparameters.RoleMap) (map[string]ranptpparameters.RoleMap, error) {
	err := checkConfiguration(config)
	if nil != err {
		return nil, err
	}

	lines := strings.Split(*config.Spec.Profile[0].Ptp4lConf, "\n")

	for i, line := range lines {
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			ifaceID := line[1 : len(line)-1]
			role := lines[i+1]

			if strings.Contains(role, "master") || strings.Contains(role, "slave") {
				ifaceRoleSlice := strings.Split(role, " ")
				ifaceRole := make(map[string]string)
				ifaceRole[ifaceRoleSlice[0]] = ifaceRoleSlice[1]
				interfacesRoleMap[ifaceID] = ifaceRole
			}
		}
	}

	return interfacesRoleMap, nil
}

// checkConfiguration returns an error in the following cases:
// 1) the given configuration, "config", has more than one profile.
// 2) the PTP4l configuration is not exists in the profile.
// 3) the PTP4l configuration is not empty.
func checkConfiguration(config ptpv1api.PtpConfig) error {
	if len(config.Spec.Profile) != 1 {
		return fmt.Errorf("more than one or no profile detected for ptpconfig %s", config.ObjectMeta.Name)
	}

	if nil == config.Spec.Profile[0].Ptp4lConf || len(*config.Spec.Profile[0].Ptp4lConf) == 0 {
		return fmt.Errorf("configuration is not evaleble")
	}

	return nil
}
