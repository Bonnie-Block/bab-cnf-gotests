package ranptphelper

import (
	"bytes"
	"fmt"
	"math/big"
	"strconv"

	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"

	"strings"
)

// GetPTPMetrics returns a buffer with metrics data for a given pod ptpPod.
// this function also returns an error if any accords.
func GetPTPMetrics(ptpPod corev1.Pod) (bytes.Buffer, error) {
	return pod.ExecCommand(helper.Apiclient, ptpPod, []string{"curl", "localhost:9091/metrics"})
}

// removeHashSigns gets a buffer ptpMetricsBuff removes the all lines that starts with '#' sign.
// the function returns an array of string, each line as an element.
func removeHashSigns(ptpMetricsBuff bytes.Buffer) []string {
	var ptpMetricsNoHash []string

	for _, line := range metricsBytesToStrings(ptpMetricsBuff) {
		if !strings.HasPrefix(line, "#") {
			ptpMetricsNoHash = append(ptpMetricsNoHash, line)
		}
	}

	return ptpMetricsNoHash
}

// MetricParser get all metrics details and the store them in the ranptpparameters.MetricMap map.
// the function gets the metrics as a buffer, returns an error if any accord.
func MetricParser(ptpMetricsBuff bytes.Buffer) error {
	if ptpMetricsBuff.Len() == 0 {
		return fmt.Errorf("buffer is empty, nothing to pasing")
	}

	ranptpparameters.MetricMap = make(map[string][]ranptpparameters.MetricDetails)

	for _, singleMetric := range removeHashSigns(ptpMetricsBuff) {
		var metric ranptpparameters.MetricDetails

		if singleMetric != "" {
			name := getMetricName(singleMetric)
			singleMetric, err := getDetails(singleMetric, metric)

			if nil != err {
				return err
			}
			ranptpparameters.MetricMap[name] = append(ranptpparameters.MetricMap[name], singleMetric)
		}
	}

	// get more special details for specific metrics keys.
	err := getClockState()
	if nil != err {
		return err
	}

	err = getInterfaceRoleValue()
	if nil != err {
		return err
	}

	err = getProcessStatusValue()

	return err
}

// getDetails inserts the details for a given metric string, singleMetric, in ranptpparameters.MetricDetails map.
// the function returns the ranptpparameters.MetricDetails and an error if any accord.
func getDetails(singleMetric string, metric ranptpparameters.MetricDetails) (ranptpparameters.MetricDetails, error) {
	var err error

	metric.Status = getStatus(singleMetric)
	metric.Value, err = getValue(singleMetric)

	if nil != err {
		return metric, err
	}

	metric.Address = getSpecificDetail(singleMetric, "address")
	metric.Type = getSpecificDetail(singleMetric, "type")
	metric.Node = getSpecificDetail(singleMetric, "node")
	metric.Process = getProcess(singleMetric)
	metric.Interface = getSpecificDetail(singleMetric, "iface")
	metric.From = getSpecificDetail(singleMetric, "from")
	metric.Config = getSpecificDetail(singleMetric, "config")
	metric.Code, err = getCode(singleMetric)

	if nil != err {
		return metric, err
	}

	metric.Threshold = getSpecificDetail(singleMetric, "threshold")

	return metric, nil
}

// getMetricName returns the metric name from a given metric string, metricDetails.
func getMetricName(metricDetails string) string {
	endName := strings.Index(metricDetails, "{")

	if endName == -1 {
		return strings.Split(metricDetails, " ")[0]
	}

	return metricDetails[:endName]
}

// getStatus returns the metric status from a given metric string, metricDetails.
func getStatus(metricDetails string) ranptpparameters.Status {
	const statusStr = "status="
	if strings.Contains(metricDetails, statusStr) {
		statusStrPos := strings.Index(metricDetails, statusStr)
		statusStart := metricDetails[statusStrPos+len(statusStr)+1:]

		return ranptpparameters.StatusMap[strings.Split(statusStart, "\"")[0]]
	}

	return ranptpparameters.StatusMap[""]
}

// getValue returns the metric value from a given metric string, metricDetails.
func getValue(metricDetails string) (int64, error) {
	if metricDetails == "" {
		return 0, nil
	}

	var value int64

	metricDetailsArr := strings.Split(metricDetails, " ")
	valueStr := metricDetailsArr[len(metricDetailsArr)-1]
	valueStr = valueStr[:len(valueStr)-1]
	bigFloat, _, err := big.ParseFloat(valueStr, 10, 0, big.ToNearestEven)

	if nil != err {
		return value, err
	}

	value, _ = bigFloat.Int64()

	return value, nil
}

// getProcess returns the metric process from a given metric string, metricDetails.
func getProcess(metricDetails string) ranptpparameters.Process {
	const processStr = "process="
	if strings.Contains(metricDetails, processStr) {
		procStrPos := strings.Index(metricDetails, processStr)
		processStart := metricDetails[procStrPos+len(processStr)+1:]

		return ranptpparameters.ProcessMap[strings.Split(processStart, "\"")[0]]
	}

	return ranptpparameters.ProcessMap[""]
}

// getCode returns the metric code from a given metric string, metricDetails.
// the function also returns an error if any accord.
func getCode(metricDetails string) (int, error) {
	const codeStr = "code="
	if strings.Contains(metricDetails, codeStr) {
		codeStrPos := strings.Index(metricDetails, codeStr)
		codeStart := metricDetails[codeStrPos+len(codeStr)+1:]
		code := strings.Split(codeStart, "\"")[0]
		codeInt, err := strconv.Atoi(code)

		return codeInt, err
	}

	return -1, nil
}

// getSpecificDetail returns the metric detail that is given to the function from a given metric string, metricDetails.
// If the given detail is not in the metricDetails string, an empty string is return.
func getSpecificDetail(metricDetails string, detail string) string {
	var retDetail string

	detail += "="

	if strings.Contains(metricDetails, detail) {
		detailStrPos := strings.Index(metricDetails, detail)
		typeStart := metricDetails[detailStrPos+len(detail)+1:]
		retDetail = strings.Split(typeStart, "\"")[0]

		return retDetail
	}

	return ""
}

// metricsBytesToStrings converts the metrics format from a buffer to an array of strings.
func metricsBytesToStrings(metrics bytes.Buffer) []string {
	var metricsStrs []string
	metricsStrs = append(metricsStrs, strings.Split(metrics.String(), "\n")...)

	return metricsStrs
}

// getClockState inserts the value of the ranptpparameters.MetricDetails.ClockStateValue field according to the
// ranptpparameters.MetricDetails.Value.
// this value if for 'clock_state_value' metric key only.
func getClockState() error {
	for i := range ranptpparameters.MetricMap["openshift_ptp_clock_state"] {
		details := &ranptpparameters.MetricMap["openshift_ptp_clock_state"][i]
		value := ranptpparameters.MetricMap["openshift_ptp_clock_state"][i].Value

		switch value {
		case int64(0):
			details.ClockStateValue = ranptpparameters.FreeRunState
		case int64(1):
			details.ClockStateValue = ranptpparameters.LockedState
		case int64(2):
			details.ClockStateValue = ranptpparameters.HoldOverState
		default:
			return fmt.Errorf("an unexpected value returned, returned value: %d", value)
		}
	}

	return nil
}

// getInterfaceRoleValue inserts the value of the ranptpparameters.MetricDetails.InterfaceRoleValue field according to
// the ranptpparameters.MetricDetails.Value.
// this value if for 'interface_role_value' metric key only.
func getInterfaceRoleValue() error {
	for i := range ranptpparameters.MetricMap["openshift_ptp_interface_role"] {
		details := &ranptpparameters.MetricMap["openshift_ptp_interface_role"][i]
		value := ranptpparameters.MetricMap["openshift_ptp_interface_role"][i].Value

		switch value {
		case int64(0):
			details.InterfaceRoleValue = ranptpparameters.Passive
		case int64(1):
			details.InterfaceRoleValue = ranptpparameters.Slave
		case int64(2):
			details.InterfaceRoleValue = ranptpparameters.Master
		case int64(3):
			details.InterfaceRoleValue = ranptpparameters.Faulty
		case int64(4):
			details.InterfaceRoleValue = ranptpparameters.Unknown
		default:
			return fmt.Errorf("an unexpected value returned, returned value: %d", value)
		}
	}

	return nil
}

// getProcessStatusValue inserts the value of the ranptpparameters.MetricDetails.ProcessStatusValue field according to
// the ranptpparameters.MetricDetails.Value.
// this value if for 'process_status_value' metric key only.
func getProcessStatusValue() error {
	for i := range ranptpparameters.MetricMap["openshift_ptp_process_status"] {
		details := &ranptpparameters.MetricMap["openshift_ptp_process_status"][i]
		value := ranptpparameters.MetricMap["openshift_ptp_process_status"][i].Value

		switch value {
		case int64(0):
			details.ProcessStatusValue = ranptpparameters.Down
		case int64(1):
			details.ProcessStatusValue = ranptpparameters.Up
		default:
			return fmt.Errorf("an unexpected value returned, returned value: %d", value)
		}
	}

	return nil
}
