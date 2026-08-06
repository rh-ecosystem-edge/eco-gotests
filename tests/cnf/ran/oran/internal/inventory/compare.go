package inventory

import (
	"fmt"
	"maps"
	"math"
)

// appendMismatch appends a field mismatch error to errs when want and got differ.
func appendMismatch(errs []error, field string, want, got any) []error {
	if want != got {
		return append(errs, fmt.Errorf("%s: want %#v, got %#v", field, want, got))
	}

	return errs
}

// appendError appends err to errs when err is non-nil.
func appendError(errs []error, err error) []error {
	if err != nil {
		return append(errs, err)
	}

	return errs
}

// derefSlice returns the slice pointed to by s, treating a nil pointer to a slice as a nil slice.
func derefSlice[T any](s *[]T) []T {
	if s == nil {
		return nil
	}

	return *s
}

// verifyStringSetEqual reports an error when want and got contain different string multisets.
func verifyStringSetEqual(want, got []string, field string) error {
	if len(want) != len(got) || !maps.Equal(countStrings(want), countStrings(got)) {
		return fmt.Errorf("%s: want %v, got %v", field, want, got)
	}

	return nil
}

// countStrings returns occurrence counts for each value in values.
func countStrings(values []string) map[string]int {
	counts := make(map[string]int, len(values))
	for _, value := range values {
		counts[value]++
	}

	return counts
}

// coordinateEpsilon is the tolerance for comparing floating-point coordinates.
const coordinateEpsilon = 0.0001

// areCoordinatesEqual reports whether got is within [coordinateEpsilon] of want for both latitude and longitude.
func areCoordinatesEqual(want, got []float64) bool {
	if len(want) != 2 || len(got) != 2 {
		return false
	}

	return math.Abs(want[0]-got[0]) <= coordinateEpsilon && math.Abs(want[1]-got[1]) <= coordinateEpsilon
}
