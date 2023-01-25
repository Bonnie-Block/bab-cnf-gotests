package ranhelper

import (
	"testing"
)

// Run me to test the helper function:
// e.g. `go test test/ran/ranhelper/ranhelper_test.go test/ran/ranhelper/ranhelper.go`.
func TestIsVersionStringInRange(t *testing.T) {
	// Test cases
	var tests = []struct {
		description    string
		version        string
		minimum        string
		maximum        string
		expectedResult bool
		shouldPanic    bool
	}{
		{"Case 1 - should not pass",
			"4.9.4", "4.10", "4.11", false, false},
		{"Case 2 - should pass",
			"4.10.4", "4.10", "4.11", true, false},
		{"Case 3 - should pass with no max",
			"4.10.4", "4.10", "", true, false},
		{"Case 4 - should not pass with no max",
			"4.9.4", "4.10", "", false, false},
		{"Case 5 - should pass with no min",
			"4.10.1", "", "4.11", true, false},
		{"Case 6 - should not pass with no min",
			"4.12.1", "", "4.11", false, false},
		{"Case 7 - should pass with no min or max",
			"4.9.1", "", "", true, false},
		{"Case 8 - should pass with no version or max",
			"", "4.9", "", true, false},
		{"Case 9 - should pass with nothing",
			"", "", "", true, false},
		{"Case 10 - should assume empty with no max defined",
			"invalid_input", "4.9", "", true, false},
		{"Case 10 - should assume empty with max defined",
			"invalid_input", "4.9", "4.11", false, false},
		{"Case 11 - should panic on invalid minimum and maximum",
			"4.10.1", "invalid_input", "invalid_input", false, true},
		{"Case 12 - should panic on invalid minimum",
			"4.10.1", "invalid_input", "4.11", false, true},
		{"Case 13 - should panic on invalid maximum",
			"4.10.1", "4.11", "invalid_input", false, true},
	}

	// Execution
	for _, test := range tests {
		// If the test should panic we need to run a deferred function to recover and capture the result
		if test.shouldPanic {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("Test '%s' did not panic as expected", test.description)
				}
			}()
		}

		result := IsVersionStringInRange(test.version, test.minimum, test.maximum)

		if result != test.expectedResult {
			t.Errorf("Result '%t' not equal to expected '%t' for test '%s'", result, test.expectedResult, test.description)
		}
	}
}
