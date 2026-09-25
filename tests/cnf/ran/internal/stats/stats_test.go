//go:build unit_test

package stats

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

const epsilon float64 = 1e-9

func TestMean(t *testing.T) {
	testCases := []struct {
		input          []float64
		expectedOutput float64
		expectedError  error
	}{
		{
			input:          []float64{1, 2, 3, 4, 5},
			expectedOutput: 3,
			expectedError:  nil,
		},
		{
			input:          []float64{},
			expectedOutput: math.NaN(),
			expectedError:  fmt.Errorf("input array must have at least 1 element"),
		},
	}

	for _, testCase := range testCases {
		output, err := Mean(testCase.input)
		assert.Equal(t, testCase.expectedError, err)

		if testCase.expectedError == nil {
			assert.InDelta(t, testCase.expectedOutput, output, epsilon)
		}
	}
}

func TestStdDev(t *testing.T) {
	testCases := []struct {
		input          []float64
		expectedOutput float64
		expectedError  error
	}{
		{
			input:          []float64{1, 2, 3, 4, 5},
			expectedOutput: math.Sqrt2,
			expectedError:  nil,
		},
		{
			input:          []float64{},
			expectedOutput: math.NaN(),
			expectedError:  fmt.Errorf("input array must have at least 1 element"),
		},
	}

	for _, testCase := range testCases {
		output, err := StdDev(testCase.input)
		assert.Equal(t, testCase.expectedError, err)

		if testCase.expectedError == nil {
			assert.InDelta(t, testCase.expectedOutput, output, epsilon)
		}
	}
}

func TestMedian(t *testing.T) {
	testCases := []struct {
		input          []float64
		expectedOutput float64
		expectedError  error
	}{
		{
			input:          []float64{1, 2, 3, 4, 5},
			expectedOutput: 3,
			expectedError:  nil,
		},
		{
			input:          []float64{},
			expectedOutput: math.NaN(),
			expectedError:  fmt.Errorf("input array must have at least 1 element"),
		},
	}

	for _, testCase := range testCases {
		output, err := Median(testCase.input)
		assert.Equal(t, testCase.expectedError, err)

		if testCase.expectedError == nil {
			assert.InDelta(t, testCase.expectedOutput, output, epsilon)
		}
	}
}
