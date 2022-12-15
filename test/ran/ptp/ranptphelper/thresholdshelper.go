package ranptphelper

import (
	ptpv1 "github.com/openshift/ptp-operator/api/v1"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/parameters"
	"gitlab.cee.redhat.com/cnf/cnf-gotests/test/ran/ptp/ranptpparameters"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"context"
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

	return nil
}
