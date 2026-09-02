package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerifyStringSetEqual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		want    []string
		got     []string
		wantErr bool
	}{
		{
			name: "matching order independent",
			want: []string{"a", "b"},
			got:  []string{"b", "a"},
		},
		{
			name: "matching with duplicates",
			want: []string{"a", "a", "b"},
			got:  []string{"b", "a", "a"},
		},
		{
			name:    "missing element",
			want:    []string{"a", "b"},
			got:     []string{"a"},
			wantErr: true,
		},
		{
			name:    "extra element",
			want:    []string{"a"},
			got:     []string{"a", "b"},
			wantErr: true,
		},
		{
			name:    "duplicate mismatch",
			want:    []string{"a", "a"},
			got:     []string{"a"},
			wantErr: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := verifyStringSetEqual(testCase.want, testCase.got, "testField")
			if testCase.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAreCoordinatesEqual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		left  []float64
		right []float64
		want  bool
	}{
		{
			name:  "matching coordinates within epsilon",
			left:  []float64{-73.935242, 40.730610},
			right: []float64{-73.935242, 40.730615},
			want:  true,
		},
		{
			name:  "longitude within epsilon",
			left:  []float64{1.0, 2.0},
			right: []float64{1.00005, 2.0},
			want:  true,
		},
		{
			name:  "longitude out of tolerance",
			left:  []float64{1.0, 2.0},
			right: []float64{1.001, 2.0},
			want:  false,
		},
		{
			name:  "latitude out of tolerance",
			left:  []float64{1.0, 2.0},
			right: []float64{1.0, 2.001},
			want:  false,
		},
		{
			name:  "wrong length want",
			left:  []float64{1.0},
			right: []float64{1.0, 2.0},
			want:  false,
		},
		{
			name:  "wrong length got",
			left:  []float64{1.0, 2.0},
			right: []float64{1.0, 2.0, 3.0},
			want:  false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.want, areCoordinatesEqual(testCase.left, testCase.right))
		})
	}
}
