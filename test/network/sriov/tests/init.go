package tests

import (
	"log"
	"os"
)

var (
	sriovSmokeTestMode bool
)

func init() {
	sriovSmokeTestModeEnvVar := os.Getenv("CNF_GOTESTS_SRIOV_SMOKE")
	if sriovSmokeTestModeEnvVar == "true" {
		sriovSmokeTestMode = true

		log.Print("Run sriov tests in smoke mode")
	}
}
