package ranstats

import (
	"math"
	"testing"
)

func TestMin(t *testing.T) {
	// Test cases
	var tests = []struct {
		description    string
		input          []float64
		expectedResult float64
		returnError    bool
	}{
		{"Case 1 - empty input array", []float64{}, math.NaN(), true},
		{"Case 2 - should pass", []float64{1, 2.2, 4.5, 9.0, 0.1}, 0.1, false},
		{"Case 3 - should pass", []float64{1.2345, 10.987, 3.189, 1.0001}, 1.0001, false},
	}

	// Execution
	for _, test := range tests {
		result, err := Min(test.input)

		if test.returnError {
			if err == nil {
				t.Errorf("Empty input array did not return an error")
			}
		} else {
			if !float64Equality(result, test.expectedResult) {
				t.Errorf("Result '%f' not equal to expected '%f' for test '%s'", result, test.expectedResult, test.description)
			}
		}
	}
}

func TestMax(t *testing.T) {
	// Test cases
	var tests = []struct {
		description    string
		input          []float64
		expectedResult float64
		returnError    bool
	}{
		{"Case 1 - empty input array", []float64{}, math.NaN(), true},
		{"Case 2 - should pass", []float64{1, 2.2, 4.5, 9.0, 0.1}, 9.0, false},
		{"Case 3 - should pass", []float64{1.2345, 10.987, 3.189, 1.0001}, 10.987, false},
	}

	// Execution
	for _, test := range tests {
		result, err := Max(test.input)

		if test.returnError {
			if err == nil {
				t.Errorf("Empty input array did not return an error")
			}
		} else {
			if !float64Equality(result, test.expectedResult) {
				t.Errorf("Result '%f' not equal to expected '%f' for test '%s'", result, test.expectedResult, test.description)
			}
		}
	}
}

func TestMean(t *testing.T) {
	// Test cases
	var tests = []struct {
		description    string
		input          []float64
		expectedResult float64
		returnError    bool
	}{
		{"Case 1 - empty input array", []float64{}, math.NaN(), true},
		{"Case 2 - should pass", []float64{1, 2.2, 4.5, 9.0, 0.1}, 3.36, false},
		{"Case 3 - should pass", []float64{1.2345, 10.987, 3.189, 1.0001}, 4.10265, false},
	}

	// Execution
	for _, test := range tests {
		result, err := Mean(test.input)

		if test.returnError {
			if err == nil {
				t.Errorf("Empty input array did not return an error")
			}
		} else {
			if !float64Equality(result, test.expectedResult) {
				t.Errorf("Result '%f' not equal to expected '%f' for test '%s'", result, test.expectedResult, test.description)
			}
		}
	}
}

func TestStdDev(t *testing.T) {
	// Test cases
	var tests = []struct {
		description    string
		input          []float64
		expectedResult float64
		returnError    bool
	}{
		{"Case 1 - empty input array", []float64{}, math.NaN(), true},
		{"Case 2 - should pass", []float64{1, 2.2, 4.5, 9.0, 0.1}, 3.18282893037, false},
		{"Case 3 - should pass", []float64{1.2345, 10.987, 3.189, 1.0001}, 4.0645151054585, false},
	}

	// ExecutionR
	for _, test := range tests {
		result, err := StdDev(test.input)

		if test.returnError {
			if err == nil {
				t.Errorf("Empty input array did not return an error")
			}
		} else {
			if !float64Equality(result, test.expectedResult) {
				t.Errorf("Result '%f' not equal to expected '%f' for test '%s'", result, test.expectedResult, test.description)
			}
		}
	}
}

func TestMedian(t *testing.T) {
	// Test cases
	var tests = []struct {
		description    string
		input          []float64
		expectedResult float64
		returnError    bool
	}{
		{"Case 1 - empty input array", []float64{}, math.NaN(), true},
		{"Case 2 - should pass", []float64{1, 2.2, 4.5, 9.0, 0.1}, 2.2, false},
		{"Case 3 - should pass", []float64{1.2345, 10.987, 3.189, 1.0001}, 2.21175, false},
	}

	// ExecutionR
	for _, test := range tests {
		result, err := Median(test.input)

		if test.returnError {
			if err == nil {
				t.Errorf("Empty input array did not return an error")
			}
		} else {
			if !float64Equality(result, test.expectedResult) {
				t.Errorf("Result '%f' not equal to expected '%f' for test '%s'", result, test.expectedResult, test.description)
			}
		}
	}
}

func float64Equality(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9
}
