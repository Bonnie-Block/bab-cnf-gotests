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
	var ptpProfile []ptpv1.PtpProfile

	newProfile := ptpv1.PtpProfile{
		Name:                  ptpConfig.Spec.Profile[0].Name,
		Interface:             ptpConfig.Spec.Profile[0].Interface,
		Ptp4lOpts:             ptpConfig.Spec.Profile[0].Ptp4lOpts,
		Phc2sysOpts:           ptpConfig.Spec.Profile[0].Phc2sysOpts,
		Ptp4lConf:             ptpConfig.Spec.Profile[0].Ptp4lConf,
		PtpSchedulingPolicy:   ptpConfig.Spec.Profile[0].PtpSchedulingPolicy,
		PtpSchedulingPriority: ptpConfig.Spec.Profile[0].PtpSchedulingPriority,
		PtpClockThreshold:     newClockThresholds,
	}

	fmt.Printf("MinOffsetThreshold: %d\nMaxOffsetThreshold: %d\nHoldOverTimeout: %d\n",
		newProfile.PtpClockThreshold.MinOffsetThreshold,
		newProfile.PtpClockThreshold.MaxOffsetThreshold,
		newProfile.PtpClockThreshold.HoldOverTimeout)
	ptpProfile = append(ptpProfile, newProfile)

	policy := ptpv1.PtpConfig{
		TypeMeta:   ptpConfig.TypeMeta,
		ObjectMeta: ptpConfig.ObjectMeta,
		Spec: ptpv1.PtpConfigSpec{
			Profile:   ptpProfile,
			Recommend: ptpConfig.Spec.Recommend,
		},
		Status: ptpConfig.Status,
	}

	_, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).Update(context.Background(),
		&policy, metav1.UpdateOptions{})
	if nil != err {
		return err
	}

	ptpConfigUpdated, err := helper.Apiclient.PtpConfigs(parameters.PtpOperatorNamespace).Get(context.Background(),
		ptpConfig.Name, metav1.GetOptions{})
	if nil != err {
		return err
	}

	fmt.Printf("MinOffsetThreshold: %d\nMaxOffsetThreshold: %d\nHoldOverTimeout: %d\n",
		ptpConfigUpdated.Spec.Profile[0].PtpClockThreshold.MinOffsetThreshold,
		ptpConfigUpdated.Spec.Profile[0].PtpClockThreshold.MaxOffsetThreshold,
		ptpConfigUpdated.Spec.Profile[0].PtpClockThreshold.HoldOverTimeout)

	return nil
}

// ThresholdsMetricsValsValidation validates the correct clock threshold values inside the metrics.
// arguments:		"thresholdMetrics"-	a single clock threshold metrics string.
//                  "thresholdVals"-	the expected values.
// return value:	an error if the threshold value is not one of the clock threshold parameters.
func ThresholdsMetricsValsValidation(thresholdMetrics ranptpparameters.MetricDetails,
	thresholdVals ptpv1.PtpClockThreshold) error {
	switch thresholdMetrics.Threshold {
	case ranptpparameters.HoldOverTimeout:
		if thresholdMetrics.Value == thresholdVals.HoldOverTimeout {
			return nil
		}
	case ranptpparameters.MaxOffsetThreshold:
		if thresholdMetrics.Value == thresholdVals.MaxOffsetThreshold {
			return nil
		}
	case ranptpparameters.MinOffsetThreshold:
		if thresholdMetrics.Value != thresholdVals.MinOffsetThreshold {
			return nil
		}
	default:
		return fmt.Errorf("threshold value %s is undefined", thresholdMetrics.Threshold)
	}

	return fmt.Errorf("metric threshold %s value %d is not equal to %d",
		thresholdMetrics.Threshold, thresholdMetrics.Value, thresholdVals.HoldOverTimeout)
}
