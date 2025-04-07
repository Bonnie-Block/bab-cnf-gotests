package ranptphelper

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/expfmt"
	openmetrics "github.com/prometheus/common/model"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ranhelper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/pod"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// NewMetric creates a metric.
// arguments:
// "metricName": Metric name; title. e.g. temperature_celsius.
// "metricsLabels": key-value label pairs. e.g. "city":"haifa".
// return value: Prometheus OpenMetrics Metric.
func NewMetric(metricName string, metricLabels map[string]string) openmetrics.Metric {
	metric := make(openmetrics.Metric, 0)

	metric[openmetrics.MetricNameLabel] = openmetrics.LabelValue(metricName)

	for name, value := range metricLabels {
		metric[openmetrics.LabelName(name)] = openmetrics.LabelValue(value)
	}

	return metric
}

// NewMetricVector creates a metrics slice. acts as a wrapper to avoid importing openmetrics elsewhere.
// arguments:
// "samples": a list of samples.
// return value: Prometheus OpenMetrics Vector.
func NewMetrics(samples ...openmetrics.Metric) []openmetrics.Metric {
	return samples
}

// NewMetricSample creates a metric sample.
// arguments:
// "metricName": Metric name; title. e.g. temperature_celsius.
// "metricValue": Metric value. e.g. 25.0 .
// "metricsLabels": key-value label pairs. e.g. "city":"haifa".
// return value: Prometheus OpenMetrics Sample.
func NewMetricSample(
	metricName string,
	metricSampleValue float64,
	metricLabels map[string]string) *openmetrics.Sample {

	metric := NewMetric(metricName, metricLabels)

	s := &openmetrics.Sample{
		Value:  openmetrics.SampleValue(metricSampleValue),
		Metric: metric,
	}

	return s
}

// NewMetricVector creates a metric vector. acts as a wrapper to avoid importing openmetrics elsewhere.
// arguments:
// "samples": a list of samples.
// return value: Prometheus OpenMetrics Vector.
func NewMetricVector(samples ...*openmetrics.Sample) openmetrics.Vector {
	return openmetrics.Vector(samples)
}

// FilterMetricSampleByMetric checks if the sample matches the metric filter.
// arguments:
// "sample": sample to be filtered.
// "filter": metric filter. empty filter fields will be ignored.
// return value: error on mismatch.
func FilterMetricSampleByMetric(sample *openmetrics.Sample, filter openmetrics.Metric) error {
	for fLabelName, fLabelValue := range filter {
		labelValue, nameOk := sample.Metric[fLabelName]
		if !nameOk {
			return fmt.Errorf("metric label name not found in sample keys: %s", fLabelName)
		}

		if labelValue != fLabelValue {
			return fmt.Errorf("metric label `%s` value: want %s, got %s", fLabelName, fLabelValue, labelValue)
		}
	}

	return nil
}

// FilterMetricSampleBySample checks if the sample matches the sample filter.
// arguments:
// "sample": sample to be filtered.
// "filter": sample filter. empty filter fields will be ignored.
// return value: error on mismatch.
func FilterMetricSampleBySample(sample, filter *openmetrics.Sample) error {
	if err := FilterMetricSampleByMetric(sample, filter.Metric); err != nil {
		return err
	}

	if sample.Value != filter.Value {
		return fmt.Errorf("metric value: want %s, got %s", filter.Value, sample.Value)
	}

	return nil
}

// FilterMetricsVectorByMetrics checks if the sample matches the filter.
// arguments:
// "sample": vector to be filtered.
// "filter": metrics filter.
// return value: vector metrics sample of the matching samples, error on mismatched or unused filter.
func FilterMetricsVectorByMetrics(vector openmetrics.Vector, filter []openmetrics.Metric) (openmetrics.Vector, error) {
	matching := make(openmetrics.Vector, 0)
	remaining := vector

	for _, filterSample := range filter {
		matched := false

		for remainingIndex, metricSample := range remaining {
			// skip for non-matching metric names
			if filterSample[openmetrics.MetricNameLabel] != metricSample.Metric[openmetrics.MetricNameLabel] {
				continue
			}

			if err := FilterMetricSampleByMetric(metricSample, filterSample); err != nil {
				continue
			}

			matched = true
			matching = append(matching, metricSample)
			remaining = append(remaining[:remainingIndex], remaining[remainingIndex+1:]...)

			break
		}

		if !matched {
			return nil, fmt.Errorf("exhausted all metrics with no matches: %s", filterSample.String())
		}
	}

	return matching, nil
}

// FilterMetricsVectorByVector checks if the sample matches the filter.
// arguments:
// "sample": vector to be filtered.
// "filter": vector filter.
// return value: vector metrics sample of the matching samples, error on mismatched or unused filter.
func FilterMetricsVectorByVector(vector, filter openmetrics.Vector) (openmetrics.Vector, error) {
	matching := make(openmetrics.Vector, 0)
	remaining := vector

	for _, filterSample := range filter {
		matched := false

		for remainingIndex, metricSample := range remaining {
			// skip for non-matching metric names
			if filterSample.Metric[openmetrics.MetricNameLabel] != metricSample.Metric[openmetrics.MetricNameLabel] {
				continue
			}

			if err := FilterMetricSampleBySample(metricSample, filterSample); err != nil {
				continue
			}

			matched = true
			matching = append(matching, metricSample)
			remaining = append(remaining[:remainingIndex], remaining[remainingIndex+1:]...)

			break
		}

		if !matched {
			return nil, fmt.Errorf("exhausted all metrics with no matches: %s", filterSample.String())
		}
	}

	return matching, nil
}

// decodeRawMetrics decodes the metrics output into an OpenMetrics Vector.
// arguments:
// "openMetricsBuff": a bytes buffer which holds the output from OpenMetrics API.
// return value: OpenMetrics Vector. error on decode failer.
func decodeRawMetrics(openMetricsBuff bytes.Buffer) (openmetrics.Vector, error) {

	if openMetricsBuff.Len() == 0 {
		return nil, fmt.Errorf("metric buffer is empty, nothing to parse")
	}

	openMetricsBuff, err := removeRawMetricsComments(openMetricsBuff)
	if err != nil {
		return nil, err
	}

	dec := expfmt.SampleDecoder{
		Dec: expfmt.NewDecoder(&openMetricsBuff, expfmt.FmtText),
		Opts: &expfmt.DecodeOptions{
			Timestamp: openmetrics.Now(),
		},
	}

	var vector openmetrics.Vector

	for {
		var decodedVector openmetrics.Vector

		if err := dec.Decode(&decodedVector); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, err
		}

		vector = append(vector, decodedVector...)
	}

	return vector, nil
}

// removeRawMetricsComments remove commented (#) lines from the metrics output.
// arguments:
// "openMetricsBuff": bytes buffer which holds the output from OpenMetrics API.
// return value: cleaned bytes buffer from comments.
func removeRawMetricsComments(openMetricsBuff bytes.Buffer) (bytes.Buffer, error) {
	var output bytes.Buffer
	scanner := bufio.NewScanner(&openMetricsBuff)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "#") {
			output.WriteString(line + "\n")
		}
	}

	return output, scanner.Err()
}

// GetPtpMetrics gets the PTP metrics & returns them as map constructed of
// metric name & slice of metric details.
func GetPtpMetrics(ptpPod corev1.Pod) (
	openmetrics.Vector,
	error) {

	buff, err := pod.ExecCommand(helper.Apiclient, ptpPod, []string{"bash", "-c", ranptpparameters.PtpMetricsCmd},
		parameters.PtpContainerName)

	if err != nil {
		return nil, err
	}

	metrics, err := decodeRawMetrics(buff)
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

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

	metric.Profile = getSpecificDetail(singleMetric, "profile")
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
		return 0, fmt.Errorf("no match found in metricDetails: `%s`", metricDetails)
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
			details.InterfaceRoleValue = ranptpparameters.ListeningRole
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
