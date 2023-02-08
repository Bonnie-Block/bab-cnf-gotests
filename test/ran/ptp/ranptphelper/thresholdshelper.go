package ranptphelper

import (
	"context"
	"fmt"

	ptpv1 "github.com/openshift/ptp-operator/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SaveOriginalValues saves the original clock threshold values inside ranptpparameters.OriginalThresholdsValues map.
// return value:	an error if any occurred.
func SaveOriginalValues() error {
	ptpConfigMap, err := GetPtpConfigs()
	if nil != err {
		return err
	}

	ranptpparameters.OriginalThresholdsValues = make(map[string]*ptpv1.PtpClockThreshold)

	for name, config := range ptpConfigMap {
		ranptpparameters.OriginalThresholdsValues[name] = config.Spec.Profile[0].PtpClockThreshold
	}

	return nil
}

// RestoreThresholdsValues updates the clock threshold values for a ptp configuration to the original values
// that are saved inside ranptpparameters.OriginalThresholdsValues.
// arguments:		"ptpConfig"-	the ptp configuration.
// return value:	an error if any occurred.
func RestoreThresholdsValues(ptpConfig *ptpv1.PtpConfig) error {
	return setThresholdValues(ptpConfig, ranptpparameters.OriginalThresholdsValues[ptpConfig.Name])
}

// SetThresholdsValAllConfigs updates ALL ptp configuration clock threshold values.
// arguments:		"ptpConfigs"-		a list with all ptp configurations.
//
//	"newClockThresholds"-	the new clock threshold values.
//
// return value:	an error if any occurred.
func SetThresholdsValAllConfigs(ptpConfigs *ptpv1.PtpConfigList, newClockThresholds *ptpv1.PtpClockThreshold) error {
	for _, ptpConfig := range ptpConfigs.Items {
		err := setThresholdValues(&ptpConfig, newClockThresholds)
		if nil != err {
			return err
		}
	}

	return nil
}

// setThresholdValues updates the clock threshold values for a ptp configuration, all other parameters are saved.
// arguments:		"ptpConfig"-			the given ptp configuration.
//
//	"newClockThresholds"-	the new clock threshold values.
//
// return value:	an error if any occurred.
func setThresholdValues(ptpConfig *ptpv1.PtpConfig, newClockThresholds *ptpv1.PtpClockThreshold) error {
	ptpConfig.Spec.Profile[0].PtpClockThreshold = newClockThresholds
	_, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).Update(context.Background(),
		ptpConfig, metav1.UpdateOptions{})

	if err != nil {
		return err
	}

	return nil
}

// ThresholdsMetricsValsValidation validates the correct clock threshold values inside the metrics.
// arguments:		"thresholdMetrics"-	a single clock threshold metrics string.
//
//	"thresholdVals"-	the expected values.
//
// return value:	an error if the threshold value is not one of the clock threshold parameters.
func ThresholdsMetricsValsValidation(thresholdMetrics ranptpparameters.MetricDetails,
	thresholdVals ptpv1.PtpClockThreshold) error {
	var exptVal int64

	switch thresholdMetrics.Threshold {
	case ranptpparameters.HoldOverTimeout:
		exptVal = thresholdVals.HoldOverTimeout
		if thresholdMetrics.Value == exptVal {
			return nil
		}
	case ranptpparameters.MaxOffsetThreshold:
		exptVal = thresholdVals.MaxOffsetThreshold
		if thresholdMetrics.Value == exptVal {
			return nil
		}
	case ranptpparameters.MinOffsetThreshold:
		exptVal = thresholdVals.MinOffsetThreshold
		if thresholdMetrics.Value == exptVal {
			return nil
		}
	default:
		return fmt.Errorf("threshold value is undefined")
	}

	return fmt.Errorf("metric threshold %s value %d is not equal to %d",
		thresholdMetrics.Threshold, thresholdMetrics.Value, exptVal)
}
