package ranptphelper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	ptpv1 "github.com/openshift/ptp-operator/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/strings/slices"
)

// NodesToPtpDaemonPods gets a list of nodes "nodesList" and a list of ptp daemon pods "podsList".
// and returns a map which the 'key' is a node and the 'value' is the ptp daemon pod of that specific node.
func NodesToPtpDaemonPods() (map[string]corev1.Pod, error) {
	ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{LabelSelector: parameters.PtpDaemonsetLabelSelector})
	if err != nil {
		return nil, err
	}

	podMap := make(map[string]corev1.Pod)
	for _, podInList := range ptpDaemonPods.Items {
		podMap[podInList.Spec.NodeName] = podInList
	}

	return podMap, nil
}

// GetPtpDaemonPodFromNode returns a ptp daemon pod from a given node "node".
// returns an error if any occurred, or if a ptp daemon pod is not exists in the node.
func GetPtpDaemonPodFromNode(node *corev1.Node) (*corev1.Pod, error) {
	ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{
			LabelSelector: parameters.PtpDaemonsetLabelSelector})
	if nil != err {
		return nil, err
	}

	for _, daemonPod := range ptpDaemonPods.Items {
		if daemonPod.Spec.NodeName == node.Name {
			return &daemonPod, nil
		}
	}

	return nil, fmt.Errorf("no new ptp daemon pod were created in %s node", node.Name)
}

// GetHoldOverTimeout gets the timeout value in seconds from the ptp configuration.
// return value:    the holdover timeout duration and an error if any occurred.
func GetHoldOverTimeout() (time.Duration, error) {
	configList, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{})
	if nil != err {
		return 0, err
	}

	if configList.Items[0].Spec.Profile[0].PtpClockThreshold != nil {
		return time.Duration(configList.Items[0].Spec.Profile[0].PtpClockThreshold.HoldOverTimeout) * time.Second, nil
	}

	// Default holdover timeout is 5 when unspecified
	return 5 * time.Second, nil
}

// BytesToStrings converts bytes.buffer to an array of strings.
// arguments:		"buff"-	a bytes buffer.
// return value:	an array of strings for each line in the bytes buffer.
func BytesToStrings(buff bytes.Buffer) []string {
	var strs []string
	strs = append(strs, strings.Split(buff.String(), "\n")...)

	return strs
}

// GetPtpConfigs gets all ptp configuration files and store them inside the map which the key is the name of the
// configuration.
// return value:	the map and an error if any occurred.
func GetPtpConfigs() (map[string]ptpv1.PtpConfig, error) {
	ptpConfigList, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{})

	if nil != err {
		return nil, err
	}

	ptpConfigMap := make(map[string]ptpv1.PtpConfig)

	for _, ptpConfig := range ptpConfigList.Items {
		ptpConfigMap[ptpConfig.Name] = ptpConfig
	}

	return ptpConfigMap, nil
}

// UpdatePtpConfigSpecs updates existing ptpconfigs with given specs. Do nothing if matching ptpconfig does not exist.
func UpdatePtpConfigSpecs(ptpConfigSpecs map[string]ptpv1.PtpConfigSpec) error {
	var errs []error

	ptpConfigList, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{})

	if err != nil {
		return err
	}

	for _, ptpConfig := range ptpConfigList.Items {
		// log.Println("Restore ptp config spec for ptpconfig: ", ptpConfig.Name)
		if val, ok := ptpConfigSpecs[ptpConfig.Name]; ok {
			ptpConfig.Spec = val
			_, err = helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).Update(context.Background(),
				&ptpConfig, metav1.UpdateOptions{})
		}
		// Do best to update all the configs before returning.
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("error encountered when updating ptp config: %v", errs)
	}

	return nil
}

// WaitForPtpClockStateMetric waits for given ptp clock state to reach expected value for a period of time.
func WaitForPtpClockStateMetric(ptpDaemonPod corev1.Pod, state ranptpparameters.ClockState, iface string,
	timeout time.Duration, stableDuration time.Duration, excludedProcess ...string) error {
	var (
		err           error
		clockStateMsg string
	)

	if iface != "" && !strings.HasSuffix(iface, "x") {
		iface = iface[:len(iface)-1] + "x"
	}

	startTime := time.Now()
	interval := 5 * time.Second

	successMsg := fmt.Sprintf("Reached PTP clock state %v for %s", state, iface)
	if iface == "" {
		successMsg += "all interfaces"
	}

	if stableDuration > 0 {
		successMsg += fmt.Sprintf(" for %v seconds", stableDuration)
	}

	errTimeout := wait.PollImmediate(interval, timeout, func() (bool, error) {
		clockStateMsg = ""

		err = GetPTPMetrics(ptpDaemonPod)
		if err != nil {
			startTime = time.Now()

			return false, nil
		}

		for _, actualVal := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpClockState] {
			if actualVal.Interface == ranptpparameters.Master {
				continue
			}

			if slices.Contains(excludedProcess, actualVal.Process) {
				continue
			}

			clockStateMsg += fmt.Sprintf("ptp_clock_state is %v for iface %s and process %v\n",
				actualVal.ClockStateValue, actualVal.Interface, actualVal.Process)

			if iface == "" || actualVal.Interface == iface {
				if actualVal.ClockStateValue != state {
					err = fmt.Errorf("%s has value %v for %s %s, that is different than expected: %v",
						ranptpparameters.OpenshiftPtpClockState, actualVal.ClockStateValue, actualVal.Interface,
						actualVal.Process, state)
					startTime = time.Now()

					return false, nil
				}
			}
		}

		// Metric state are in expected state. Check for stable duration if it's larger than zero.
		if stableDuration > 0 {
			actualStableDuration := time.Since(startTime)
			// Add an interval because the timer started before mcp became updated
			if actualStableDuration < stableDuration+interval {
				return false, nil
			}
		}

		return true, nil
	})

	if errTimeout != nil {
		log.Println("PTP clock states metrics:\n" + clockStateMsg)

		return err
	}

	log.Println(successMsg)

	return nil
}

// WaitForMetricValueStatus waits for given metrics type state to reach expected value for a period of time.
func WaitForMetricValueStatus(ptpDaemonPod corev1.Pod, metricsName string, state interface{},
	timeout time.Duration, stableDuration time.Duration) error {
	var (
		err      error
		stateMsg string
	)

	startTime := time.Now()
	interval := 5 * time.Second

	successMsg := fmt.Sprintf("Reached %s %v", metricsName, state)

	if stableDuration > 0 {
		successMsg += fmt.Sprintf(" for %v seconds", stableDuration)
	}

	errTimeout := wait.PollImmediate(interval, timeout, func() (bool, error) {
		stateMsg = ""

		err = GetPTPMetrics(ptpDaemonPod)
		if err != nil {
			startTime = time.Now()

			return false, nil
		}

		for _, actualVal := range ranptpparameters.MetricMap[metricsName] {
			stateMsg = stateMessage(stateMsg, metricsName, actualVal)

			if !stateCheck(actualVal, state) {
				err = errMessage(metricsName, actualVal, state)
				startTime = time.Now()

				return false, nil
			}
		}

		// Metric state are in expected state. Check for stable duration if it's larger than zero.
		if stableDuration > 0 {
			actualStableDuration := time.Since(startTime)
			// Add an interval because the timer started before mcp became updated
			if actualStableDuration < stableDuration+interval {
				return false, nil
			}
		}

		return true, nil
	})

	if errTimeout != nil {
		log.Printf("%s metrics:\n%s\n", metricsName, stateMsg)

		return err
	}

	log.Println(successMsg)

	return nil
}

func errMessage(metricsName string, actualVal ranptpparameters.MetricDetails, expectedState interface{}) error {
	state, ok := expectedState.(int)
	if !ok {
		return fmt.Errorf("%s has value %v for %s %s, that is different than expected: %v",
			metricsName, actualVal.Value, actualVal.Interface,
			actualVal.Process, int64(state))
	}

	return fmt.Errorf("%s has value %v for %s %s, that is different than expected: %v",
		metricsName, actualVal.Value, actualVal.Interface,
		actualVal.Process, state)
}

func stateCheck(actualVal ranptpparameters.MetricDetails, expectedState interface{}) bool {
	state, _ := expectedState.(int)

	return actualVal.Value == int64(state)
}

func stateMessage(stateMsg string, metricsName string, actualVal ranptpparameters.MetricDetails) string {
	stateMsg += fmt.Sprintf("%s is %v for iface %s and process %v\n", metricsName, actualVal.Value,
		actualVal.Interface, actualVal.Process)

	return stateMsg
}

// IsOrdinaryClockProfile checks if given profile has slave only config.
func IsOrdinaryClockProfile(profile ptpv1.PtpProfile) bool {
	return profile.Interface != nil && profile.Ptp4lOpts != nil && strings.Contains(*profile.Ptp4lOpts, " -s")
}

// IsBoundaryClockProfile checks if given profile has boundary clock config.
func IsBoundaryClockProfile(profile ptpv1.PtpProfile) bool {
	if profile.Interface != nil || (profile.Ptp4lOpts != nil && strings.Contains(*profile.Ptp4lOpts, " -s")) {
		return false
	}

	ptp4lconf := *profile.Ptp4lConf

	return strings.Contains(ptp4lconf, "[en") && strings.Contains(ptp4lconf, "masterOnly 1")
}

// IsGmOneCardProfile checks if given profile has boundary clock config.
func IsGmOneCardProfile(profile ptpv1.PtpProfile) bool {
	if profile.Ts2PhcConf != nil {
		return strings.Contains(*profile.Ts2PhcConf, "ts2phc.master 1") &&
			!strings.Contains(*profile.Ts2PhcConf, "ts2phc.master 0")
	}

	return false
}

// IsGmTwoCardProfile checks if given profile has boundary clock config.
func IsGmTwoCardProfile(profile ptpv1.PtpProfile) bool {
	if profile.Ts2PhcConf != nil {
		return strings.Contains(*profile.Ts2PhcConf, "ts2phc.master 1") &&
			strings.Contains(*profile.Ts2PhcConf, "ts2phc.master 0")
	}

	return false
}

// IncreaseMaxOffsetThresholdMlx sets OffsetThresholds to 200 for Mellanox NICs to workaround performance issue.
func IncreaseMaxOffsetThresholdMlx(ptpPod corev1.Pod) error {
	ptpConfigsList, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).
		List(context.Background(), metav1.ListOptions{})

	if err != nil {
		return err
	}

	for _, ptpConfig := range ptpConfigsList.Items {
		ptpProfilePerNode, err := GetPtpProfilesPerNode(ptpConfig)
		if err != nil {
			return err
		}

		// Get list of ptp profiles that applied to the node where the ptpPod is running on. This is necessary to
		// get the desired interface driver on the right node in the case of multi-node cluster.
		ptpProfilesOnNode := ptpProfilePerNode[ptpPod.Spec.NodeName]

		updated := false

		for index, profile := range ptpConfig.Spec.Profile {
			// Only change offsetThresholds if ptp profile is applied to target node and if it is configured as
			// ordinary clock using Mellanox NIC. As of OCP 4.15, only ordinary clock config is supported using
			// Mellanox cards. BC and GM are not.
			if isProfileInList(profile, ptpProfilesOnNode) && IsOrdinaryClockProfile(profile) {
				driver, err := GetNicDriver(ptpPod, *profile.Interface)
				if err != nil {
					return err
				}

				if strings.HasPrefix(driver, "mlx") {
					log.Println("Set maxOffsetThreshold in ptp profile to 200 for MLX interface " + *profile.Interface)
					profile.PtpClockThreshold = &ranptpparameters.MlxThresholdsValues
					updated = true
				}
			}

			// Compose the profile field for ptp configs to prepare for ptpConfig update
			ptpConfig.Spec.Profile[index] = profile
		}

		if updated {
			_, err = helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).Update(context.Background(),
				&ptpConfig, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// isProfileInList checks if a PTP profile is included in give list.
func isProfileInList(profile ptpv1.PtpProfile, profileList []ptpv1.PtpProfile) bool {
	for _, item := range profileList {
		if profile.Name == item.Name {
			return true
		}
	}

	return false
}

// GetNicDriver gets the driver for a given interface by running cmd via a PTP pod.
func GetNicDriver(ptpPod corev1.Pod, ifName string) (string, error) {
	cmd := fmt.Sprintf("ethtool -i %s | grep --color=no driver | awk '{print $2}'", ifName)
	out, err := pod.ExecCommand(helper.Apiclient, ptpPod, []string{"/bin/bash", "-c", cmd}, parameters.PtpContainerName)

	if nil != err {
		return "", err
	}

	return strings.Trim(out.String(), "\n"), nil
}

// GpsColdReboot reboots the gps using ubxtool, if the reboot failed, an error returns.
func GpsColdReboot(ptpPod *corev1.Pod) error {
	cmd := "ubxtool -p COLDBOOT" /* -v 3 29.20*/
	_, err := pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"/bin/bash", "-c", cmd}, parameters.PtpContainerName)

	return err
}

// WaitForLog waits for specified string to appear in ptp log.
func WaitForLog(ptpPod *corev1.Pod, container string, wantedLog string, since time.Duration,
	timeout time.Duration) error {
	if since < 1*time.Second {
		since = 1 * time.Second
	}

	startTime := time.Now()

	logs, err := pod.GetLog(helper.Apiclient, ptpPod, since, container)
	if err != nil {
		return err
	}

	interval := 5 * time.Second

	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		time.Sleep(interval)

		logs, err = pod.GetLog(helper.Apiclient, ptpPod, time.Since(startTime)+time.Second,
			container)
		if err != nil {
			return false, nil
		}

		if strings.Contains(logs, wantedLog) {
			return true, nil
		}

		startTime = time.Now()

		return false, nil
	})
}

// SetSma sets the SMA1 values.
// smaVal should be on format 'x y'. e.g. '1 2'.
func SetSma(ptpPod *corev1.Pod, iface string, smaVal string) error {
	cmd := fmt.Sprintf("echo %s > /sys/class/net/%s/device/ptp/*/pins/SMA1", smaVal, iface)
	_, err := pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"/bin/bash", "-c", cmd}, parameters.PtpContainerName)

	return err
}

// GetSma gets the SMA1 values.
// smaVal should be on format 'x y'. e.g. '1 2'.
func GetSma(ptpPod *corev1.Pod, iface string) (string, error) {
	cmd := fmt.Sprintf("cat /sys/class/net/%s/device/ptp/*/pins/SMA1", iface)
	smaVal, err := pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"/bin/bash", "-c", cmd},
		parameters.PtpContainerName)

	return smaVal.String(), err
}

// GetRxIface gets the RX interface.
func GetRxIface(ptpConfig ptpv1.PtpConfig) (string, error) {
	ifaces, ifaceJSON, err := getJSONInterface(ptpConfig)
	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		ifaceStr, err := json.Marshal(ifaceJSON[iface])
		if err != nil {
			return "", err
		}

		var sma1Json map[string]interface{}
		err = json.Unmarshal(ifaceStr, &sma1Json)

		if err != nil {
			return "", err
		}

		sma1Str, err := json.Marshal(sma1Json["SMA1"])

		if err != nil {
			return "", err
		}

		if string(sma1Str) == "\"1 1\"" {
			log.Printf("found interface %s with value 1 1", iface)

			return iface, nil
		}
	}

	return "", nil
}

func GetTxIface(ptpConfig ptpv1.PtpConfig) (string, error) {
	ifaces, ifaceJSON, err := getJSONInterface(ptpConfig)
	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		ifaceStr, err := json.Marshal(ifaceJSON[iface])

		if err != nil {
			return "", err
		}

		var sma1Json map[string]interface{}
		err = json.Unmarshal(ifaceStr, &sma1Json)

		if err != nil {
			return "", err
		}

		sma1Str, err := json.Marshal(sma1Json["SMA1"])
		if err != nil {
			return "", err
		}

		if string(sma1Str) == "\"2 1\"" {
			log.Printf("found interface %s with value 2 1", iface)

			return iface, nil
		}
	}

	return "", nil
}

func getJSONInterface(ptpConfig ptpv1.PtpConfig) ([]string, map[string]interface{}, error) {
	pluginJSONStr, err := json.Marshal(ptpConfig.Spec.Profile[0].Plugins["e810"])
	if err != nil {
		return nil, nil, err
	}

	var gmPlugin map[string]interface{}
	err = json.Unmarshal(pluginJSONStr, &gmPlugin)

	if err != nil {
		return nil, nil, err
	}

	// find interfaces in JSON
	pinsIfaces, ok := gmPlugin["pins"].(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("failed to casting to map[string]interface{}")
	}

	var ifaces []string
	for i := range pinsIfaces {
		ifaces = append(ifaces, i)
	}

	pinsJSONStr, err := json.Marshal(gmPlugin["pins"])
	if err != nil {
		return nil, nil, err
	}

	var ifaceJSON map[string]interface{}
	err = json.Unmarshal(pinsJSONStr, &ifaceJSON)

	if err != nil {
		return nil, nil, err
	}

	return ifaces, ifaceJSON, nil
}
