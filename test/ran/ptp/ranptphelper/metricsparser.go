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
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GetPTPMetrics gets the metrics and checks if the all the metrics got correctly if not it will try again
// up to 15 minutes.
// when the metrics are correctly collected, the function will call to a parser function.
// arguments:
// "ptpPod"-	a given ptp pod for getting the metrics from.
// "optionalArgs":	first element (bool): whether to print out ptp metrics
// return value:	an error if any occurred.
func GetPTPMetrics(ptpPod corev1.Pod, optionalArgs ...interface{}) error {
	var (
		buff bytes.Buffer
		err  error
	)

	buff, err = pod.ExecCommand(helper.Apiclient, ptpPod, []string{"bash", "-c", ranptpparameters.PtpMetricsCmd},
		parameters.PtpContainerName)
	if nil != err {
		return err
	}

	var errFromParser error

	err = wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		errFromParser = metricParser(buff)
		if errFromParser != nil {
			log.Println(errFromParser.Error())

			buff, err = pod.ExecCommand(helper.Apiclient, ptpPod, []string{"bash", "-c", ranptpparameters.PtpMetricsCmd},
				parameters.PtpContainerName)
			if err != nil {
				return false, nil
			}

			return false, nil
		}

		return true, nil
	})

	if len(optionalArgs) > 0 {
		printLog, _ := optionalArgs[0].(bool)
		if printLog {
			log.Println("PTP metrics: \n" + buff.String())
		}
	}

	if errFromParser != nil {
		return errFromParser
	}

	return err
}

// metricParser get all metrics details and the store them in the ranptpparameters.MetricMap map.
// arguments:		"ptpMetricsBuff"-	a metrics buffer.
// return value:	an error if any occurred.
func metricParser(ptpMetricsBuff bytes.Buffer) error {
	var (
		metric ranptpparameters.MetricDetails
		err    error
	)

	if ptpMetricsBuff.Len() == 0 {
		return fmt.Errorf("buffer is empty, nothing to parse")
	}

	ranptpparameters.MetricMap = make(map[string][]ranptpparameters.MetricDetails)

	for _, singleMetric := range removeHashSigns(ptpMetricsBuff) {
		singleMetric = strings.Trim(singleMetric, "\r\n")
		if !strings.Contains(singleMetric, "}") {
			// Ignore lines without curly brackets
			continue
		}

		name := getMetricName(singleMetric)
		metric, err = getDetails(singleMetric, metric)

		if nil != err {
			return err
		}
		ranptpparameters.MetricMap[name] = append(ranptpparameters.MetricMap[name], metric)
	}

	// get more special details for specific metrics keys.
	err = getClockState()
	if nil != err {
		return err
	}
	// openshift_ptp_process_status is added in 4.11
	err = getProcessStatusValue()
	if nil != err {
		return err
	}

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

	metric.Value, err = getValue(singleMetric)

	if nil != err {
		return metric, err
	}

	metric.Address = getSpecificDetail(singleMetric, "address")
	metric.Type = getSpecificDetail(singleMetric, "type")
	metric.Node = getSpecificDetail(singleMetric, "node")
	metric.Process = getSpecificDetail(singleMetric, "process")
	metric.Interface = getSpecificDetail(singleMetric, "iface")
	metric.From = getSpecificDetail(singleMetric, "from")
	metric.Config = getSpecificDetail(singleMetric, "config")
	metric.Status = getSpecificDetail(singleMetric, "status")
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
func getMetricName(metricDetails string) string {
	endName := strings.Index(metricDetails, "{")

	if endName == -1 {
		return strings.Split(metricDetails, " ")[0]
	}

	return metricDetails[:endName]
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

	r := regexp.MustCompile(`.*{.*}\s+(-*\d+)`)
	valueStr := r.FindStringSubmatch(metricDetails)

	if len(valueStr) < 2 {
		return 0, fmt.Errorf("no match found in metricDetails: %s", metricDetails)
	}

	return strconv.ParseInt(valueStr[1], 0, 64)
}

// getCode gets the metric code of a single metric.
// arguments:		"metricDetails"-	a single metrics string.
// return values:	the code of the single metric if exists as an int.
//
//	an error if any occurred.
func getCode(metricDetails string) (int, error) {
	r := regexp.MustCompile(`code="(\d+)"`)
	matches := r.FindStringSubmatch(metricDetails)

	if len(matches) > 1 {
		return strconv.Atoi(matches[1])
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
	// match anything in between quotation marks, except comma or curly brackets
	r := regexp.MustCompile(fmt.Sprintf("%s=\"([^,{}]*)\"", detail))
	matches := r.FindStringSubmatch(metricDetails)

	if len(matches) > 1 {
		return matches[1]
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
