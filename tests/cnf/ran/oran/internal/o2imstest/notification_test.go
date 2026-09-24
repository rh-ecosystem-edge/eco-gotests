//go:build unit_test

package o2imstest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRefersTo(t *testing.T) {
	t.Parallel()

	const (
		objectID   = "pool-1"
		objectName = "test-pool"
	)

	tests := []struct {
		name        string
		objectRef   *string
		post, prior *map[string]any
		want        bool
	}{
		{name: "objectRef contains id", objectRef: new("/resourcePools/" + objectID), want: true},
		{name: "id in post state", post: new(map[string]any{"resourcePoolId": objectID, "name": objectName}), want: true},
		{name: "id in prior state", prior: new(map[string]any{"resourcePoolId": objectID}), want: true},
		{name: "name in post state", post: new(map[string]any{"name": objectName}), want: true},
		{name: "objectRef does not contain id", objectRef: new("/resourcePools/other")},
		{name: "all nil"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := RefersTo(
				testCase.objectRef, testCase.post, testCase.prior,
				"resourcePoolId", objectID, "name", objectName,
			)

			assert.Equal(t, testCase.want, got)
		})
	}
}
