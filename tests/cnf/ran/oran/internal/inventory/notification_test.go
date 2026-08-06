package inventory

import (
	"testing"

	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/stretchr/testify/assert"
)

func TestNotificationRefersToPool(t *testing.T) {
	t.Parallel()

	const (
		poolID   = "abc-123-pool-id"
		poolName = "test-inventory-pool"
	)

	tests := []struct {
		name         string
		notification *oranapi.InventoryChangeNotification
		want         bool
	}{
		{
			name: "match via ObjectRef",
			notification: &oranapi.InventoryChangeNotification{
				ObjectRef: new("/resourcePools/" + poolID),
			},
			want: true,
		},
		{
			name: "match via PostObjectState resourcePoolId",
			notification: &oranapi.InventoryChangeNotification{
				PostObjectState: &map[string]any{
					"resourcePoolId": poolID,
				},
			},
			want: true,
		},
		{
			name: "match via PostObjectState name",
			notification: &oranapi.InventoryChangeNotification{
				PostObjectState: &map[string]any{
					"name": poolName,
				},
			},
			want: true,
		},
		{
			name: "match via PriorObjectState resourcePoolId",
			notification: &oranapi.InventoryChangeNotification{
				PriorObjectState: &map[string]any{
					"resourcePoolId": poolID,
				},
			},
			want: true,
		},
		{
			name: "match via PriorObjectState name",
			notification: &oranapi.InventoryChangeNotification{
				PriorObjectState: &map[string]any{
					"name": poolName,
				},
			},
			want: true,
		},
		{
			name: "no match",
			notification: &oranapi.InventoryChangeNotification{
				ObjectRef: new("/resourcePools/other-id"),
				PostObjectState: &map[string]any{
					"resourcePoolId": "other-id",
					"name":           "other-pool",
				},
			},
			want: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := NotificationRefersToPool(testCase.notification, poolID, poolName)
			assert.Equal(t, testCase.want, got)
		})
	}
}
