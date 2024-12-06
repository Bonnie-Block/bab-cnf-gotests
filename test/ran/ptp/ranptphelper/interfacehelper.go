package ranptphelper

import (
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/nodes"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/schemes/ptp/ptpv1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/strings/slices"

	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// GetInterfaces returns ptp interfaces with specified role on given node.
// the function returns only "master" or "slave" interfaces.
func GetInterfaces(role ptpv1.PtpRole, node corev1.Node) ([]string, error) {
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
		nic := GetNic(iface)
		if !slices.Contains(ifaceGroupMap[nic], iface) {
			ifaceGroupMap[nic] = append(ifaceGroupMap[nic], iface)
		}
	}

	return ifaceGroupMap
}

func GetNic(iface string) string {
	return iface[:len(iface)-1] + "x"
}

// SetInterfaceStatus sets a given interface to a given state with option to retry.
// retries must be 0 or larger.
func SetInterfaceStatus(clientPod *corev1.Pod, containerName string, iface string,
	state ranptpparameters.InterfaceState, retries int) error {

	var err error
	for i := 0; i < retries+1; i++ {
		err = setInterfaceStatusAndCheck(clientPod, containerName, iface, state)
		if err == nil {
			log.Printf("%s is successfully set to %s\n", iface, state)

			return nil
		}
	}

	log.Printf("Failed to set %s to %s\n", iface, state)

	return err
}

// setInterfaceStatusAndCheck sets a given interface to a given state and checks the interface is in expected state.
func setInterfaceStatusAndCheck(clientPod *corev1.Pod, containerName string, ifaceID string,
	state ranptpparameters.InterfaceState) error {
	cmd := fmt.Sprintf("ip link set %s %s", ifaceID, string(state))
	_, err := pod.ExecCommand(helper.Apiclient, *clientPod, []string{"bash", "-c", cmd}, containerName)
	if err != nil {
		return err
	}

	cmdCheck := fmt.Sprintf("ip link show %s | grep \" state %s \"", ifaceID, strings.ToTitle(string(state)))

	return wait.PollImmediate(3*time.Second, 15*time.Second, func() (bool, error) {
		time.Sleep(1 * time.Second)
		_, err = pod.ExecCommand(helper.Apiclient, *clientPod, []string{"bash", "-c", cmdCheck}, containerName)

		return err == nil, nil
	})
}

// interfaceParser parses the interface section of a given pointer to configuration file, "config".
// an error is returned if any accord.
func interfaceParser(config ptpv1.PtpConfig, node corev1.Node,
	interfacesRoleMap map[string]ranptpparameters.RoleMap) (map[string]ranptpparameters.RoleMap, error) {
	err := checkConfiguration(config)
	if nil != err {
		return nil, err
	}

	nodeProfileMap, err := GetPtpProfilesPerNode(config)
	if err != nil {
		return nil, err
	}

	interfacesRoleMap = getIfaceRoleMap(nodeProfileMap[node.Name], interfacesRoleMap)

	return interfacesRoleMap, nil
}

// GetPtpProfilesPerNode returns a map of ptp profile list for each node.
func GetPtpProfilesPerNode(config ptpv1.PtpConfig) (map[string][]ptpv1.PtpProfile, error) {
	var nodeProfileMap = map[string][]ptpv1.PtpProfile{}

	nodeList, err := helper.Apiclient.Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nodeProfileMap, err
	}

	for _, node := range nodeList.Items {
		nodeProfileMap[node.Name] = []ptpv1.PtpProfile{}
	}

	var profileMap = map[string]ptpv1.PtpProfile{}
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

func bcPtpProfileParser(profile ptpv1.PtpProfile) map[string]ranptpparameters.RoleMap {
	ifaceRoleMap := make(map[string]ranptpparameters.RoleMap)

	lines := strings.Split(*profile.Ptp4lConf, "\n")

	for i, line := range lines {
		if strings.HasPrefix(line, "[en") && strings.HasSuffix(line, "]") {
			ifaceID := line[1 : len(line)-1]
			role := lines[i+1]

			if strings.Contains(role, "master") || strings.Contains(role, "slave") {
				ifaceRoleSlice := strings.Split(role, " ")
				ifaceRole := make(map[string]string)
				ifaceRole[ifaceRoleSlice[0]] = ifaceRoleSlice[1]
				ifaceRoleMap[ifaceID] = ifaceRole
			}
		}
	}

	return ifaceRoleMap
}

// checkConfiguration returns an error in the following cases:
// 1) the given configuration, "config", has more than one profile.
// 2) the PTP4l configuration is not exists in the profile.
// 3) the PTP4l configuration is empty.
func checkConfiguration(config ptpv1.PtpConfig) error {
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
	log.Println("primary interface:", ranptpparameters.OcpInterface)

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

// BuildProfileSlaveInterfaceMap returns a slave interface by getting a profile name ptpProfileName.
func BuildProfileSlaveInterfaceMap(node corev1.Node) (map[string]string, error) {
	profileSlaveIface := make(map[string]string)
	// getting all slaves interfaces.
	slaveIfaces, err := GetInterfaces(ptpv1.Slave, node)
	if err != nil {
		return nil, err
	}

	// creates a profile interface map the match slave interface to wanted profile.
	ptpProfileIfaces, err := BuildPtpProfileIfacesMap()
	if err != nil {
		return nil, err
	}

	// match the slave interface connected to the ptpProfileName profile.
	for _, slaveIfaceFromInterface := range slaveIfaces {
		for profileName, ifaces := range ptpProfileIfaces {
			for _, iface := range ifaces {
				if iface == slaveIfaceFromInterface {
					profileSlaveIface[profileName] = iface
				}
			}
		}
	}

	return profileSlaveIface, nil
}

// BuildPtpProfileIfacesMap creates a ptp-profile name to interface name map.
func BuildPtpProfileIfacesMap() (map[string][]string, error) {
	ptpConfigsList, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	ptpProfileIfaces := make(map[string][]string)

	for _, ptpConfig := range ptpConfigsList.Items {
		for _, ptpProfile := range ptpConfig.Spec.Profile {
			interfacesRoleMap := make(map[string]ranptpparameters.RoleMap)
			for iface := range getIfaceRoleMap(ptpConfig.Spec.Profile, interfacesRoleMap) {
				ptpProfileIfaces[*ptpProfile.Name] = append(ptpProfileIfaces[*ptpProfile.Name], iface)
			}
		}
	}

	return ptpProfileIfaces, nil
}

// getIfaceRoleMap greats a profile name to interface role map.
func getIfaceRoleMap(ptpProfileList []ptpv1.PtpProfile,
	interfacesRoleMap map[string]ranptpparameters.RoleMap) map[string]ranptpparameters.RoleMap {
	for _, ptpProfile := range ptpProfileList {
		if IsOrdinaryClockProfile(ptpProfile) {
			if _, ok := interfacesRoleMap[*ptpProfile.Interface]; ok {
				log.Printf("Warning: slave interface %s is configured more than once", *ptpProfile.Interface)
			}

			interfacesRoleMap[*ptpProfile.Interface] = map[string]string{"masterOnly": "0"}

			continue
		}

		if IsHaProfile(ptpProfile) {
			continue
		}

		ifaceRoleMap := bcPtpProfileParser(ptpProfile)

		for iface, role := range ifaceRoleMap {
			if _, ok := interfacesRoleMap[iface]; ok {
				log.Printf("Warning: interface %s is configured more than once", iface)
			}

			interfacesRoleMap[iface] = role
		}
	}

	return interfacesRoleMap
}

// BuildProfileNameConfigMap creates a ptpProfile name to ptpConfig map.
func BuildProfileNameConfigMap() (map[string]ptpv1.PtpConfig, error) {
	profileNameConfigMap := make(map[string]ptpv1.PtpConfig)

	ptpConfigList, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, ptpConfig := range ptpConfigList.Items {
		for _, ptpProfile := range ptpConfig.Spec.Profile {
			profileNameConfigMap[*ptpProfile.Name] = ptpConfig
		}
	}

	return profileNameConfigMap, nil
}
