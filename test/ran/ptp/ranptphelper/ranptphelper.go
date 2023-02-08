package ranptphelper

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	ptpv1 "github.com/openshift/ptp-operator/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
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
	timeout time.Duration, stableDuration time.Duration) error {
	var err error

	if iface != "" && !strings.HasSuffix(iface, "x") {
		iface = iface[:len(iface)-1] + "x"
	}

	startTime := time.Now()
	interval := 15 * time.Second

	successMsg := fmt.Sprintf("Reached PTP clock state %v for %s", state, iface)
	if iface == "" {
		successMsg += "all interfaces"
	}

	if stableDuration > 0 {
		successMsg += fmt.Sprintf(" for %v seconds", stableDuration)
	}

	errTimeout := wait.PollImmediate(interval, timeout, func() (bool, error) {
		err = GetPTPMetrics(ptpDaemonPod)
		if err != nil {
			startTime = time.Now()

			return false, nil
		}

		for _, actualVal := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpClockState] {
			if actualVal.Interface == ranptpparameters.Master {
				continue
			}
			if iface == "" || actualVal.Interface == iface {
				if actualVal.ClockStateValue != state {
					err = fmt.Errorf("%s has value %v for %s, that is different than expected: %v",
						ranptpparameters.OpenshiftPtpClockState, actualVal.ClockStateValue, actualVal.Interface, state)
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

		log.Println(successMsg)

		return true, nil
	})

	if errTimeout != nil {
		return err
	}

	return nil
}

// IsOrdinaryClockProfile checks if given profile has slave only config.
func IsOrdinaryClockProfile(profile ptpv1.PtpProfile) bool {
	return profile.Interface != nil && profile.Ptp4lOpts != nil && strings.Contains(*profile.Ptp4lOpts, " -s ")
}

// IsBoundaryClockProfile checks if given profile has boundary clock config.
func IsBoundaryClockProfile(profile ptpv1.PtpProfile) bool {
	if profile.Interface != nil || (profile.Ptp4lOpts != nil && strings.Contains(*profile.Ptp4lOpts, " -s ")) {
		return false
	}

	ptp4lconf := *profile.Ptp4lConf

	return strings.Contains(ptp4lconf, "[en") && strings.Contains(ptp4lconf, "masterOnly 1")
}
