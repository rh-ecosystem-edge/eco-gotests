package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerifyAPIVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version *string
		wantErr bool
	}{
		{
			name:    "valid version",
			version: new("2.0.0"),
		},
		{
			name:    "wrong version",
			version: new("1.0.0"),
			wantErr: true,
		},
		{
			name:    "missing version",
			version: nil,
			wantErr: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := verifyAPIVersion(testCase.version)

			if testCase.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestVerifyAPIURIPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		uriPrefix *string
		wantErr   bool
	}{
		{
			name:      "valid uriPrefix",
			uriPrefix: new("/o2ims-infrastructureInventory/v2"),
		},
		{
			name:      "wrong uriPrefix",
			uriPrefix: new("/wrong"),
			wantErr:   true,
		},
		{
			name:    "missing uriPrefix",
			wantErr: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := verifyAPIURIPrefix(testCase.uriPrefix)

			if testCase.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
