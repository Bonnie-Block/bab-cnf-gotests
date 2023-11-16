package ranptpparameters

import ptpoperatorv1 "github.com/openshift/ptp-operator/api/v1"

var OriginalThresholdsValues map[string]*ptpoperatorv1.PtpClockThreshold

var ModifiedThresholdsValues = ptpoperatorv1.PtpClockThreshold{
	HoldOverTimeout:    120,
	MaxOffsetThreshold: 1,
	MinOffsetThreshold: -1,
}

var MlxThresholdsValues = ptpoperatorv1.PtpClockThreshold{
	MaxOffsetThreshold: 200,
	MinOffsetThreshold: -200,
}
