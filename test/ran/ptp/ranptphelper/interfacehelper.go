package ranptphelper

import (
	ptpv1api "github.com/openshift/ptp-operator/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/strings/slices"

	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
)

// GetInterfaces returns ptp interfaces with specified role on given node.
// the function returns only "master" or "slave" interfaces.
func GetInterfaces(role ptpv1api.PtpRole, node corev1.Node) ([]string, error) {
	var interfaces []string

	configList, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{})
	if nil != err {
		return interfaces, err
	}

	interfacesRoleMap := make(map[string]ranptpparameters.RoleMap)
	for _, config := range configList.Items {
		interfacesRoleMap, err = interfaceParser(config, node, interfacesRoleMap)
		if err != nil {
			return interfaces, err
		}
	}

	ranptpparameters.InterfacesRoleMap = interfacesRoleMap
	for index, section := range ranptpparameters.InterfacesRoleMap {
		if section["masterOnly"] == strconv.Itoa(int(role)) {
			interfaces = append(interfaces, index)
		}
	}

	return interfaces, nil
}

// GetInterfaceGroups returns a map with interface group as key, and interface names as value.
// e.g., {"ens4fx": ["ensf40", "ens4f1"]}.
func GetInterfaceGroups(interfaces []string) map[string][]string {
	ifaceGroupMap := make(map[string][]string)

	for _, iface := range interfaces {
		nic := getNic(iface)
		if !slices.Contains(ifaceGroupMap[nic], iface) {
			ifaceGroupMap[nic] = append(ifaceGroupMap[nic], iface)
		}
	}

	return ifaceGroupMap
}

func getNic(iface string) string {
	return iface[:len(iface)-1] + "x"
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

// interfaceParser parses the interface section of a given pointer to configuration file, "config".
// an error is returned if any accord.
func interfaceParser(config ptpv1api.PtpConfig, node corev1.Node,
	interfacesRoleMap map[string]ranptpparameters.RoleMap) (map[string]ranptpparameters.RoleMap, error) {
	err := checkConfiguration(config)
	if nil != err {
		return nil, err
	}

	nodeProfileMap, err := GetPtpProfilesPerNode(config)
	if err != nil {
		return nil, err
	}

	for _, profile := range nodeProfileMap[node.Name] {
		if IsOrdinaryClockProfile(profile) {
			if _, ok := interfacesRoleMap[*profile.Interface]; ok {
				log.Printf("Warning: slave interface %s is configured more than once", *profile.Interface)
			}

			interfacesRoleMap[*profile.Interface] = map[string]string{"masterOnly": "0"}

			continue
		}

		if IsHaProfile(profile) {
			continue
		}

		ifceRoleMap := bcPtpProfileParser(profile)

		for ifce, role := range ifceRoleMap {
			if _, ok := interfacesRoleMap[ifce]; ok {
				log.Printf("Warning: interface %s is configured more than once", ifce)
			}

			interfacesRoleMap[ifce] = role
		}
	}

	return interfacesRoleMap, nil
}

// GetPtpProfilesPerNode returns a map of ptp profile list for each node.
func GetPtpProfilesPerNode(config ptpv1api.PtpConfig) (map[string][]ptpv1api.PtpProfile, error) {
	var nodeProfileMap = map[string][]ptpv1api.PtpProfile{}

	nodeList, err := helper.Apiclient.Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nodeProfileMap, err
	}

	for _, node := range nodeList.Items {
		nodeProfileMap[node.Name] = []ptpv1api.PtpProfile{}
	}

	var profileMap = map[string]ptpv1api.PtpProfile{}
	for _, profile := range config.Spec.Profile {
		profileMap[*profile.Name] = profile
	}

	for _, recommend := range config.Spec.Recommend {
		recommedProfile := profileMap[*recommend.Profile]

		for _, match := range recommend.Match {
			if match.NodeName != nil {
				nodeProfileMap[*match.NodeName] = append(nodeProfileMap[*match.NodeName], recommedProfile)
			} else if match.NodeLabel != nil {
				nodeList, err = nodes.GetByLabel(helper.Apiclient, *match.NodeLabel)
				if err != nil {
					return nodeProfileMap, err
				}
				for _, node := range nodeList.Items {
					nodeProfileMap[node.Name] = append(nodeProfileMap[node.Name], recommedProfile)
				}
			}
		}
	}

	return nodeProfileMap, nil
}

func bcPtpProfileParser(profile ptpv1api.PtpProfile) map[string]ranptpparameters.RoleMap {
	ifceRoleMap := make(map[string]ranptpparameters.RoleMap)

	lines := strings.Split(*profile.Ptp4lConf, "\n")

	for i, line := range lines {
		if strings.HasPrefix(line, "[en") && strings.HasSuffix(line, "]") {
			ifaceID := line[1 : len(line)-1]
			role := lines[i+1]

			if strings.Contains(role, "master") || strings.Contains(role, "slave") {
				ifaceRoleSlice := strings.Split(role, " ")
				ifaceRole := make(map[string]string)
				ifaceRole[ifaceRoleSlice[0]] = ifaceRoleSlice[1]
				ifceRoleMap[ifaceID] = ifaceRole
			}
		}
	}

	return ifceRoleMap
}

// checkConfiguration returns an error in the following cases:
// 1) the given configuration, "config", has more than one profile.
// 2) the PTP4l configuration is not exists in the profile.
// 3) the PTP4l configuration is empty.
func checkConfiguration(config ptpv1api.PtpConfig) error {
	if len(config.Spec.Profile) != 1 {
		return fmt.Errorf("more than one or no profile detected for ptpconfig %s", config.ObjectMeta.Name)
	}

	if !IsHaProfile(config.Spec.Profile[0]) && (nil == config.Spec.Profile[0].Ptp4lConf ||
		len(*config.Spec.Profile[0].Ptp4lConf) == 0) {
		return fmt.Errorf("configuration is not available")
	}

	return nil
}

// GetOcpInterface returns the interface whose physical port is also used by br-ex virtual interface.
// PTP test will avoid bringing down this interface.
func GetOcpInterface(privPod corev1.Pod, containerName string) (string, error) {
	cmd := []string{"bash", "-c", "MAC=`cat /sys/class/net/br-ex/address`; ip addr | grep -B 1 ${MAC} | " +
		"grep \" UP \" | grep -v br-ex | awk '{print $2}' | tr -d [:]"}

	ocpInterfaceBytes, err := pod.ExecCommand(helper.Apiclient, privPod, cmd, containerName)
	if err != nil {
		return "", err
	}

	ranptpparameters.OcpInterface = strings.TrimSpace(ocpInterfaceBytes.String())
	log.Println("Interface used by OCP:", ranptpparameters.OcpInterface)

	return ranptpparameters.OcpInterface, nil
}

// ContainsOcpInterface returns true if port for any given interfaces is also used by br-ex virtual interface.
func ContainsOcpInterface(interfaces []string) bool {
	for _, iface := range interfaces {
		if iface == ranptpparameters.OcpInterface {
			return true
		}
	}

	return false
}
