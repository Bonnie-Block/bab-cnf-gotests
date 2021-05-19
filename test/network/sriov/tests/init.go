package tests

import (
	"os"
)

var (
	sriovSmokeTestMode bool
)

func init() {
	sriovSmokeTestModeEnvVar := os.Getenv("CNF_GOTESTS_SRIOV_SMOKE")
	if sriovSmokeTestModeEnvVar == "true" {
		sriovSmokeTestMode = true
	}
}
