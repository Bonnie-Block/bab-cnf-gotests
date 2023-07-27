package ranptphelper

import (
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// GetPTPMetrics gets the metrics and checks if the all the metrics got correctly if not it will try again
// up to 15 minutes.
// when the metrics are correctly collected, the function will call to a parser function.
// arguments:		"ptpPod"-	a given ptp pod for getting the metrics from.
// return value:	an error if any occurred.
func GetPTPMetrics(ptpPod corev1.Pod) error {
	buff, err := pod.ExecCommand(helper.Apiclient, ptpPod, []string{"curl", "-s", "localhost:9091/metrics"},
		parameters.PtpContainerName)
	if nil != err {
		return err
	}

	var errFromParser error

	err = wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		errFromParser = metricParser(buff)
		if errFromParser != nil {
			log.Printf("parsing failed with error %s, try again\n", errFromParser.Error())
			buff, err = pod.ExecCommand(helper.Apiclient, ptpPod, []string{"curl", "-s", "localhost:9091/metrics"})
			if err != nil {
				return false, nil
			}

			return false, nil
		}

		return true, nil
	})

	if errFromParser != nil {
		return errFromParser
	}

	return err
}

// metricParser get all metrics details and the store them in the ranptpparameters.MetricMap map.
// arguments:		"ptpMetricsBuff"-	a metrics buffer.
// return value:	an error if any occurred.
func metricParser(ptpMetricsBuff bytes.Buffer) error {
	if ptpMetricsBuff.Len() == 0 {
		return fmt.Errorf("buffer is empty, nothing to pasing")
	}

	ranptpparameters.MetricMap = make(map[string][]ranptpparameters.MetricDetails)

	for _, singleMetric := range removeHashSigns(ptpMetricsBuff) {
		var metric ranptpparameters.MetricDetails

		if singleMetric != "" {
			name, err := getMetricName(singleMetric)
			if nil != err {
				return err
			}

			metric, err = getDetails(singleMetric, metric)

			if nil != err {
				return err
			}
			ranptpparameters.MetricMap[name] = append(ranptpparameters.MetricMap[name], metric)
		}
	}

	// get more special details for specific metrics keys.
	err := getClockState()
	if nil != err {
		return err
	}

	err = getProcessStatusValue()
	if nil != err {
		return err
	}

	// openshift_ptp_process_status is added in 4.11
	err = getInterfaceRoleValue()

	return err
}

// removeHashSigns removes all lines that start with '#' sign.
// arguments:		"ptpMetricsBuff"-	a metrics buffer.
// return value:	an array of strings, each line as an element.
func removeHashSigns(ptpMetricsBuff bytes.Buffer) []string {
	var ptpMetricsNoHash []string

	for _, line := range BytesToStrings(ptpMetricsBuff) {
		if !strings.HasPrefix(line, "#") {
			ptpMetricsNoHash = append(ptpMetricsNoHash, line)
		}
	}

	return ptpMetricsNoHash
}

// getDetails inserts the details of a single metric to ranptpparameters.MetricDetails map.
// arguments:		"singleMetric"-	a single metric string.
//
//	"metric"- the ranptpparameters.MetricDetails map.
//
// return values:	the ranptpparameters.MetricDetails after adding the details of a single metric.
//
//	an error if any occurred.
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

// getMetricName gets the metric name of a single metric.
// arguments:		"metricDetails"-	a single metrics string.
// return values:	the metric name as a string.
//
//	an error if any occurred.
func getMetricName(metricDetails string) (string, error) {
	if strings.Contains(metricDetails, "promhttp_metric_handler_requests_in_flight") ||
		strings.Contains(metricDetails, "cne_amqp_connection_reset") {
		return strings.Split(metricDetails, " ")[1], nil
	}

	if !strings.Contains(metricDetails, "{") {
		return "", fmt.Errorf("metrics didn't get correctly")
	}

	endName := strings.Index(metricDetails, "{")

	if endName == -1 {
		return strings.Split(metricDetails, " ")[0], nil
	}

	return metricDetails[:endName], nil
}

// getStatus gets the status of a single metric.
// arguments:		"metricDetails"-	a single metrics string.
// return value:	the status of the single metric if exists as a ranptpparameters.Status type.
func getStatus(metricDetails string) ranptpparameters.Status {
	const statusStr = "status="
	if strings.Contains(metricDetails, statusStr) {
		statusStrPos := strings.Index(metricDetails, statusStr)
		statusStart := metricDetails[statusStrPos+len(statusStr)+1:]

		return ranptpparameters.StatusMap[strings.Split(statusStart, "\"")[0]]
	}

	return ranptpparameters.StatusMap[""]
}

// getValue gets the value of a single metric.
// arguments:		"metricDetails"-	a single metrics string.
// return values:	the value of the single metric if exists as an int64.
//
//	an error if any occurred.
func getValue(metricDetails string) (int64, error) {
	if metricDetails == "" {
		return 0, nil
	}

	var value int64

	metricDetailsArr := strings.Split(metricDetails, " ")
	valueStr := metricDetailsArr[len(metricDetailsArr)-1]
	valueStr = valueStr[:len(valueStr)-1]
	bigFloat, err := strconv.ParseFloat(valueStr, 64)

	if nil != err {
		return value, err
	}

	value = int64(bigFloat)

	return value, nil
}

// getProcess gets the metric process of a single metric.
// arguments:		"metricDetails"-	a single metrics string.
// return values:	the process of the single metric if exists as a ranptpparameters.Process.
func getProcess(metricDetails string) ranptpparameters.Process {
	const processStr = "process="
	if strings.Contains(metricDetails, processStr) {
		procStrPos := strings.Index(metricDetails, processStr)
		processStart := metricDetails[procStrPos+len(processStr)+1:]

		return ranptpparameters.ProcessMap[strings.Split(processStart, "\"")[0]]
	}

	return ranptpparameters.ProcessMap[""]
}

// getCode gets the metric code of a single metric.
// arguments:		"metricDetails"-	a single metrics string.
// return values:	the code of the single metric if exists as an int.
//
//	an error if any occurred.
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

// getSpecificDetail gets a metric detail of a single metric.
// arguments:		"metricDetails"-	a single metrics string.
//
//	"detail"-			the specific detail that the metric has (e.g. "address", "type", "node",
//	"iface", "from", "config", etc.).
//
// return value:	the specific detail of the single metric if exists as a string.
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

// getClockState inserts the value of the ranptpparameters.MetricDetails.ClockStateValue field according to the
// ranptpparameters.MetricDetails.Value.
// this value if for 'clock_state_value' metric key only.
// return value:	an error if the clock state metrics are empty or the state value is undefined.
func getClockState() error {
	if nil == ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpClockState] {
		return fmt.Errorf("openshift_ptp_clock_state metrics didn't get correctly")
	}

	for i := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpClockState] {
		details := &ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpClockState][i]
		value := ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpClockState][i].Value

		switch value {
		case int64(0):
			details.ClockStateValue = ranptpparameters.FreeRunState
		case int64(1):
			details.ClockStateValue = ranptpparameters.LockedState
		case int64(2):
			details.ClockStateValue = ranptpparameters.HoldOverState
		default:
			return fmt.Errorf("an unexpected clock state returned, returned value: %d", value)
		}
	}

	return nil
}

// getInterfaceRoleValue inserts the value of the ranptpparameters.MetricDetails.InterfaceRoleValue field according to
// the ranptpparameters.MetricDetails.Value.
// this value if for 'interface_role_value' metric key only.
// return value:	an error if the interface role metrics are empty or the state value is undefined.
func getInterfaceRoleValue() error {
	if nil == ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpInterfaceRole] {
		return fmt.Errorf("openshift_ptp_interface_role metrics didn't get correctly")
	}

	for i := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpInterfaceRole] {
		details := &ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpInterfaceRole][i]
		value := ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpInterfaceRole][i].Value

		switch value {
		case int64(0):
			details.InterfaceRoleValue = ranptpparameters.PassiveRole
		case int64(1):
			details.InterfaceRoleValue = ranptpparameters.SlaveRole
		case int64(2):
			details.InterfaceRoleValue = ranptpparameters.MasterRole
		case int64(3):
			details.InterfaceRoleValue = ranptpparameters.FaultyRole
		case int64(4):
			details.InterfaceRoleValue = ranptpparameters.UnknownRole
		case int64(5):
			details.InterfaceRoleValue = ranptpparameters.Listening
		default:
			return fmt.Errorf("an unexpected interface role returned, returned value: %d", value)
		}
	}

	return nil
}

// getProcessStatusValue inserts the value of the ranptpparameters.MetricDetails.ProcessStatusValue field according to
// the ranptpparameters.MetricDetails.Value.
// this value if for 'process_status_value' metric key only.
// return value:	an error if the process status metrics are empty or the state value is undefined.
func getProcessStatusValue() error {
	if !ranhelper.IsVersionStringInRange(ranptpparameters.PtpVersion, "4.11", "") {
		return nil
	}

	if nil == ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpProcessStatus] {
		return fmt.Errorf("openshift_ptp_process_status metrics didn't get correctly")
	}

	for i := range ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpProcessStatus] {
		details := &ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpProcessStatus][i]
		value := ranptpparameters.MetricMap[ranptpparameters.OpenshiftPtpProcessStatus][i].Value

		switch value {
		case int64(0):
			details.ProcessStatusValue = ranptpparameters.Down
		case int64(1):
			details.ProcessStatusValue = ranptpparameters.Up
		default:
			return fmt.Errorf("an unexpected process state returned, returned value: %d", value)
		}
	}

	return nil
}
