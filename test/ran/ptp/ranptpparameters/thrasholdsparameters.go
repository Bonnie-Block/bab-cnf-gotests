package ranptpparameters

import "gitlab.cee.redhat.com/cnf/cnf-gotests/test/util/schemes/ptp/ptpv1"

var OriginalThresholdsValues map[string]*ptpv1.PtpClockThreshold

var ModifiedThresholdsValues = ptpv1.PtpClockThreshold{
	HoldOverTimeout:    120,
	MaxOffsetThreshold: 1,
	MinOffsetThreshold: -1,
}

var MlxThresholdsValues = ptpv1.PtpClockThreshold{
	MaxOffsetThreshold: 200,
	MinOffsetThreshold: -200,
}
