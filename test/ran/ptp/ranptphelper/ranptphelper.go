package ranptphelper

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	openmetrics "github.com/prometheus/common/model"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/schemes/ptp/ptpv1"
	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/strings/slices"
)

func CheckPtpLockState(timeout time.Duration, stableDuration time.Duration) error {
	// Validate that PTP event container is running1
	ptpDaemonPods, err := helper.Apiclient.Pods(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{LabelSelector: parameters.PtpDaemonsetLabelSelector})
	if err != nil {
		log.Println("Failed to get PTP pod list")

		return err
	}

	for _, ptpDaemonPod := range ptpDaemonPods.Items {
		helper.WaitForPodsHealthy([]*corev1.Pod{&ptpDaemonPod}, 3*time.Minute)
		err = WaitForPtpClockStateMetric(ptpDaemonPod, ranptpparameters.LockedState, "",
			timeout, stableDuration)

		if err != nil {
			return err
		}
	}

	return nil
}

// WaitForDesiredMetricVector waits for the metrics to match the desired state given.
func WaitForDesiredMetricVector(
	ptpDaemonPod corev1.Pod,
	timeout time.Duration,
	filter openmetrics.Vector) error {

	interval := 5 * time.Second

	err := wait.PollImmediate(interval, timeout, func() (bool, error) {
		metrics, err := GetPtpMetrics(ptpDaemonPod)
		if err != nil {
			log.Println(err)

			return false, nil
		}

		if _, err = FilterMetricsVectorByVector(metrics, filter); err != nil {
			log.Println(err)

			return false, nil
		}

		return true, nil
	})

	if err != nil {
		return err
	}

	return nil
}

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

	if iface != "" && iface != "CLOCK_REALTIME" && !strings.HasSuffix(iface, "x") {
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

// getPtpConfigCounts counts ptpconfig types.
// arguments:		"ptpConfigsList"-	a list of ptpconfigs.
// return value:	PtpConfigTypeCounter struct.
func GetPtpConfigCounts(ptpConfigsList ptpv1.PtpConfigList) ranptpparameters.PtpConfigTypeCounter {
	var counter ranptpparameters.PtpConfigTypeCounter

	for _, ptpconfig := range ptpConfigsList.Items {
		for _, profile := range ptpconfig.Spec.Profile {
			counter.Total++
			switch {
			case IsOrdinaryClockProfile(profile):
				counter.OCOnePort++
			case IsOrdinaryClock2PortProfile(profile):
				counter.OCTwoPort++
			case IsBoundaryClockProfile(profile):
				counter.BC++
			case IsHaProfile(profile):
				counter.HA++
			case IsGmOneCardProfile(profile):
				counter.GMOneNIC++
			case IsGmMultiCardProfile(profile):
				counter.GMMultiNIC++
			default:
				log.Println("Warning: unrecognized PTP profile type: ", *profile.Name)
			}
		}
	}

	return counter
}

// IsOrdinaryClockProfile checks if given profile has slave only config.
func IsOrdinaryClockProfile(profile ptpv1.PtpProfile) bool {
	return profile.Interface != nil && profile.Ptp4lOpts != nil && strings.Contains(*profile.Ptp4lOpts, " -s")
}

// IsOrdinaryClock2PortProfile checks if given profile has slave only config.
func IsOrdinaryClock2PortProfile(profile ptpv1.PtpProfile) bool {
	if profile.Ptp4lConf == nil {
		return false
	}
	ptp4lconf := *profile.Ptp4lConf

	ifaces := make([]string, 0)
	ifaceRegex := regexp.MustCompile(`(?i)^\[(en[a-z0-9]+)\]$`)
	scanner := bufio.NewScanner(strings.NewReader(ptp4lconf))
	for scanner.Scan() {
		line := scanner.Text()
		if match := ifaceRegex.FindString(line); match != "" {
			iface := strings.Trim(line, "[]")
			ifaces = append(ifaces, iface)
		}
	}

	twoIfaces := profile.Interface == nil &&
		len(ifaces) == 2

	slaveOnly := strings.Contains(*profile.Ptp4lOpts, " -s") ||
		strings.Contains(ptp4lconf, "slaveOnly 1")

	return twoIfaces && slaveOnly
}

func IsHaProfile(profile ptpv1.PtpProfile) bool {
	return profile.Interface == nil && profile.Ptp4lConf == nil
}

// IsBoundaryClockProfile checks if given profile has boundary clock config.
func IsBoundaryClockProfile(profile ptpv1.PtpProfile) bool {
	if profile.Interface != nil || profile.Ts2PhcConf != nil || (profile.Ptp4lOpts != nil &&
		strings.Contains(*profile.Ptp4lOpts, " -s")) {
		return false
	}

	if profile.Ptp4lConf == nil {
		return false
	}
	ptp4lconf := *profile.Ptp4lConf

	return strings.Contains(ptp4lconf, "[en") && strings.Contains(ptp4lconf, "masterOnly 1")
}

// IsGmOneCardProfile checks if given profile has grandmaster configuration with one card.
func IsGmOneCardProfile(profile ptpv1.PtpProfile) bool {
	if profile.Ts2PhcConf != nil {
		return strings.Contains(*profile.Ts2PhcConf, "ts2phc.master 1") &&
			!strings.Contains(*profile.Ts2PhcConf, "ts2phc.master 0")
	}

	return false
}

// IsGmMultiCardProfile checks if given profile has multiple WPC cards that configure as GM.
func IsGmMultiCardProfile(profile ptpv1.PtpProfile) bool {
	if profile.Ts2PhcConf != nil {
		return strings.Contains(*profile.Ts2PhcConf, "ts2phc.master 1") &&
			strings.Contains(*profile.Ts2PhcConf, "ts2phc.master 0")
	}

	return false
}

// GetGmPtpConfig return a GM ptp configuration.
func GetGmPtpConfig() (*ptpv1.PtpConfig, error) {
	listPtpConfig, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).List(context.Background(),
		metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, ptpConfig := range listPtpConfig.Items {
		for _, ptpProfile := range ptpConfig.Spec.Profile {
			if IsGmOneCardProfile(ptpProfile) || IsGmMultiCardProfile(ptpProfile) {
				log.Println("found GM ptp configuration")

				return &ptpConfig, nil
			}
		}
	}

	return nil, fmt.Errorf("no GM configuraion found")
}

// GetGmPtpConfigWithProfileIndex returns the PTP configuration which has a GM profile & the index
// of the profile within the configuration. Returns an error if no Grandmaster profile is found.
func GetGmPtpConfigWithProfileIndex(listPtpConfig ptpv1.PtpConfigList) (*ptpv1.PtpConfig, int, error) {
	for _, ptpConfig := range listPtpConfig.Items {
		for i, ptpProfile := range ptpConfig.Spec.Profile {
			if IsGmOneCardProfile(ptpProfile) || IsGmMultiCardProfile(ptpProfile) {
				log.Println("found GM ptp configuration")

				return &ptpConfig, i, nil
			}
		}
	}

	return nil, 0, fmt.Errorf("no GM configuraion found")
}

// GetOc2PortPtpConfigs returns the PTP configuration which has a OC 2 port profile.
// of the profile within the configuration. Returns an error if no OC 2 port profile is found.
func GetOc2PortPtpConfigs(listPtpConfig ptpv1.PtpConfigList) (
	[]*ptpv1.PtpConfig, error) {
	configs := make([]*ptpv1.PtpConfig, 0)
	for _, ptpConfig := range listPtpConfig.Items {
		for _, ptpProfile := range ptpConfig.Spec.Profile {
			if IsOrdinaryClock2PortProfile(ptpProfile) {
				log.Println("found oc 2 port ptp configuration")

				configs = append(configs, &ptpConfig)
			}
		}
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no oc 2 port configuration found")
	}

	return configs, nil
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
	cmd := "ubxtool -p COLDBOOT; sleep 0.1"
	_, err := pod.ExecCommand(helper.Apiclient, *ptpPod, []string{"/bin/bash", "-c", cmd}, parameters.PtpContainerName)

	return err
}

// WaitForLog waits for specified string to appear in ptp log.
func WaitForLog(ptpPod *corev1.Pod, container string, wantedLog string, startTime time.Time,
	timeout time.Duration) error {

	interval, extraTime := 5*time.Second, 1*time.Second

	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		newStartTime := time.Now()
		logs, err := pod.GetLog(helper.Apiclient, ptpPod, time.Since(startTime)+extraTime,
			container)
		if err != nil {
			return false, nil
		}

		if strings.Contains(logs, wantedLog) {
			return true, nil
		}

		startTime = time.Now()
		extraTime = time.Since(newStartTime) + 1*time.Second
		time.Sleep(interval)

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

func getGmIface(ptpConfig ptpv1.PtpConfig, smaString string) ([]string, error) {
	ifaces, ifaceJSON, err := getJSONInterface(ptpConfig)
	if err != nil {
		return nil, err
	}
	var wantedIfaces []string
	for _, iface := range ifaces {
		ifaceStr, err := json.Marshal(ifaceJSON[iface])
		if err != nil {
			return nil, err
		}

		var sma1Json map[string]interface{}
		err = json.Unmarshal(ifaceStr, &sma1Json)
		if err != nil {
			return nil, err
		}

		sma1Str, err := json.Marshal(sma1Json["SMA1"])
		if err != nil {
			return nil, err
		}

		if string(sma1Str) == fmt.Sprintf("\"%s\"", smaString) {
			log.Printf("found interface %s with value %s", iface, smaString)
			wantedIfaces = append(wantedIfaces, iface)
		}
	}

	return wantedIfaces, nil
}

// GetRxIface gets the RX interface.
func GetRxIface(ptpConfig ptpv1.PtpConfig) ([]string, error) {
	return getGmIface(ptpConfig, ranptpparameters.RxConfiguration)
}

// GetTxIface gets the TX interface.
func GetTxIface(ptpConfig ptpv1.PtpConfig) (string, error) {
	txIfaces, err := getGmIface(ptpConfig, ranptpparameters.TxConfiguration)
	if err != nil {
		return "", err
	}

	// only one interface can be as TX.
	if len(txIfaces) > 1 {
		return "", fmt.Errorf("found more than 1 interface with value %s", ranptpparameters.TxConfiguration)
	}

	return txIfaces[0], nil
}

// GetGmInterfaceToGPS	returns a GM interface that is connected to GPS via GNSS module.
func GetGmInterfaceToGPS(gmPtpConfig ptpv1.PtpConfig) (string, error) {
	ifaces, _, err := getJSONInterface(gmPtpConfig)
	if err != nil {
		return "", err
	}

	if len(ifaces) == 0 {
		log.Printf("No interface found from PTP config %s - spec: \n%v\n", gmPtpConfig.Name, gmPtpConfig.Spec)

		return "", fmt.Errorf("no interface found from %s", gmPtpConfig.Name)
	} else if len(ifaces) == 1 {
		return ifaces[0], nil
	}

	// Returns Tx interface when more than 1 GM is configured
	return GetTxIface(gmPtpConfig)
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

// GetHaProfile returns a profile name with the wanted state, active (1) or inactive (0).
func GetHaProfile(status int64) ([]string, error) {
	var haProfile []string

	for _, HaMetrics := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpHaProfileStatus] {
		if HaMetrics.Value == status {
			log.Printf("found profile %s with status %d\n", HaMetrics.Profile, status)
			haProfile = append(haProfile, HaMetrics.Profile)
		}
	}

	if len(haProfile) == 0 {
		return nil, fmt.Errorf("no ha profile with status %v found", status)
	}

	return haProfile, nil
}

// BuildStatusProfileNamesMap creates a status to profile names map.
func BuildStatusProfileNamesMap(ptpDaemonPod corev1.Pod) (map[int64][]string, error) {
	statusProfilesMap := make(map[int64][]string)

	var (
		HaMetrics []ranptpparameters.MetricDetails
		errMsg    string
		err       error
	)

	errTimeout := wait.PollImmediate(10*time.Second, 3*time.Minute, func() (bool, error) {

		err = GetPTPMetrics(ptpDaemonPod)
		HaMetrics = ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpHaProfileStatus]
		if HaMetrics == nil {
			errMsg = "no openshift_ptp_ha_profile_status metrics found"

			return false, nil
		}

		return true, nil
	})

	if errTimeout != nil {
		return nil, fmt.Errorf(errMsg)
	}

	for _, HaMetric := range HaMetrics {
		if HaMetric.Value == ranptpparameters.Inactive {
			log.Printf("found profile %s with status inactive\n", HaMetric.Profile)

			statusProfilesMap[ranptpparameters.Inactive] = append(statusProfilesMap[ranptpparameters.Inactive],
				HaMetric.Profile)
		}

		if HaMetric.Value == ranptpparameters.Active {
			log.Printf("found profile %s with status active\n", HaMetric.Profile)

			statusProfilesMap[ranptpparameters.Active] = append(statusProfilesMap[ranptpparameters.Active],
				HaMetric.Profile)
		}
	}

	return statusProfilesMap, err
}

// WaitForMHaMetricsUpdate waits for ha metric to update after removing one bc configuration.
func WaitForMHaMetricsUpdate(ptpDaemonPod corev1.Pod, profileName string, timeout time.Duration) error {
	var (
		err    error
		errMsg string
	)

	interval := 5 * time.Second

	successMsg := "profile " + profileName + " deleted from metrics"

	errTimeout := wait.PollImmediate(interval, timeout, func() (bool, error) {
		errMsg = ""

		err = GetPTPMetrics(ptpDaemonPod)
		HaMetrics := ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpHaProfileStatus]

		if len(HaMetrics) == 0 || (len(HaMetrics) == 1 && HaMetrics[0].Value == ranptpparameters.Inactive) {
			errMsg = "No active profile found"

			return false, nil
		}

		for _, HaMetric := range HaMetrics {
			if HaMetric.Profile == profileName {
				errMsg = "Profile " + profileName + " still appears in metrics"

				return false, nil
			}
		}

		return true, nil
	})

	if errTimeout != nil {
		return fmt.Errorf(errMsg)
	}

	if err != nil {
		return err
	}

	log.Println(successMsg)

	return nil
}

func getPtpConfigProfileByIndex(
	ptpConfig *ptpv1.PtpConfig,
	ptpProfileIndex int) (
	*ptpv1.PtpProfile, error) {
	l := len(ptpConfig.Spec.Profile)
	if ptpProfileIndex >= l {
		return nil, fmt.Errorf("PTP Profile Index %d out of range, there are only %d PTP Profiles in the configuration",
			ptpProfileIndex, l)
	}

	return &ptpConfig.Spec.Profile[ptpProfileIndex], nil
}

// GetPtpSettings gets the e810 plugin settings from the ptp configuration.
// return value: PtpSettings map[string]string, err.
func GetPtpConfigProfilePluginE810Settings(
	ptpConfig *ptpv1.PtpConfig,
	ptpProfileIndex int) (
	*ranptpparameters.E810PluginSettings, error) {

	ptpProfile, err := getPtpConfigProfileByIndex(ptpConfig, ptpProfileIndex)
	if err != nil {
		return nil, err
	}

	pluginJSONStr, err := json.Marshal(ptpProfile.Plugins["e810"])
	if err != nil {
		return nil, err
	}

	var plugin map[string]interface{}
	err = json.Unmarshal(pluginJSONStr, &plugin)
	if err != nil {
		return nil, err
	}

	pluginSettings, okSettingsKey := plugin["settings"]
	if !okSettingsKey {
		return nil, fmt.Errorf("'settings' key not found within plugin map: %s", plugin)
	}

	isettings, okSettingsTypeAssert := pluginSettings.(map[string]interface{})
	if !okSettingsTypeAssert {
		return nil, fmt.Errorf("failed asserting pluginSettings to map[string]interface: %v of type %t",
			pluginSettings, pluginSettings)
	}

	ilocalHoldoverTimeout, okTimeoutKey := isettings["LocalHoldoverTimeout"]
	if !okTimeoutKey {
		return nil, fmt.Errorf("'LocalHoldoverTimeout' key not found within plugin settings map: %s", pluginSettings)
	}

	localHoldoverTimeout, okTimeoutTypeAssert := ilocalHoldoverTimeout.(float64)
	if !okTimeoutTypeAssert {
		return nil, fmt.Errorf("failed asserting 'localHoldoverTimeout' to float64: %v of type %t",
			ilocalHoldoverTimeout, ilocalHoldoverTimeout)
	}

	ilocalMaxHoldoverOffset, okHoldoverOffsetKey := isettings["LocalMaxHoldoverOffSet"]
	if !okHoldoverOffsetKey {
		return nil, fmt.Errorf("'LocalMaxHoldoverOffSet' key not found within plugin settings map: %s", pluginSettings)
	}

	localMaxHoldoverOffset, okHoldoverOffsetTypeAssert := ilocalMaxHoldoverOffset.(float64)
	if !okHoldoverOffsetTypeAssert {
		return nil, fmt.Errorf("failed asserting 'LocalMaxHoldoverOffset' to float64: %v of type %t",
			ilocalMaxHoldoverOffset, ilocalMaxHoldoverOffset)
	}

	imaxInSpecOffset, okInSepcOffset := isettings["MaxInSpecOffset"]
	if !okInSepcOffset {
		return nil, fmt.Errorf("'MaxInSpecOffset' key not found within settings map: %s", pluginSettings)
	}

	maxInSpecOffset, okOffset := imaxInSpecOffset.(float64)
	if !okOffset {
		return nil, fmt.Errorf("failed asserting 'LocalMaxHoldoverOffset' to float64: %v of type %t",
			imaxInSpecOffset, imaxInSpecOffset)
	}

	settings := &ranptpparameters.E810PluginSettings{}
	settings.LocalHoldoverTimeout = uint(localHoldoverTimeout)
	settings.LocalMaxHoldoverOffset = uint(localMaxHoldoverOffset)
	settings.MaxInSpecOffset = uint(maxInSpecOffset)

	return settings, nil
}

// SetPtpSettings updates the e810 plugin settings from ptp configuration.
// return value:  error.
func SetPtpConfigProfilePluginE810Settings(
	ptpConfig *ptpv1.PtpConfig, ptpProfileIndex int,
	newSettings ranptpparameters.E810PluginSettings) error {
	ptpProfile, err := getPtpConfigProfileByIndex(ptpConfig, ptpProfileIndex)
	if err != nil {
		return err
	}

	pluginJSONStr, err := json.Marshal(ptpProfile.Plugins["e810"])
	if err != nil {
		return err
	}

	var plugin map[string]interface{}
	err = json.Unmarshal(pluginJSONStr, &plugin)
	if err != nil {
		return err
	}

	settings, ok := plugin["settings"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("failed to casting to map[string]interface")
	}

	settings["LocalHoldoverTimeout"] = newSettings.LocalHoldoverTimeout
	settings["LocalMaxHoldoverOffSet"] = newSettings.LocalMaxHoldoverOffset
	settings["MaxInSpecOffset"] = newSettings.MaxInSpecOffset

	raw, err := json.Marshal(plugin)
	if err != nil {
		return err
	}

	apiJSON := apiextv1.JSON{
		Raw: raw,
	}

	ptpConfig.Spec.Profile[0].Plugins["e810"] = &apiJSON
	_, err = helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).Update(context.Background(),
		ptpConfig, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

// GetPodInterfaceRoles() return the interface role associated with the pod as reported by the metrics.
// return value: map of interface names and interface role value, error if failed.
func GetPodInterfaceRoles(ptpDaemonPod corev1.Pod, ifaces []string) (map[string]ranptpparameters.InterfaceRole, error) {
	vector, err := GetPtpMetrics(ptpDaemonPod)
	if err != nil {
		return nil, err
	}

	ifaceMetricFilter := NewMetrics()
	for _, iface := range ifaces {
		ifaceMetricFilter = append(
			ifaceMetricFilter,
			NewMetric(
				ranptpparameters.OpenshiftPtpInterfaceRole,
				map[string]string{
					"iface": iface,
				}),
		)
	}

	ifaceVector, err := FilterMetricsVectorByMetrics(vector, ifaceMetricFilter)
	if err != nil {
		return nil, err
	}

	ifaceRoles := make(map[string]ranptpparameters.InterfaceRole)
	for _, metric := range ifaceVector {
		ifaceName := metric.Metric["iface"]
		ifaceRoles[string(ifaceName)] = ranptpparameters.InterfaceRole(metric.Value)
	}

	return ifaceRoles, nil
}
