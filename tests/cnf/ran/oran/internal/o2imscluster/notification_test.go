//go:build unit_test

package o2imscluster

import (
	"testing"

	"github.com/google/uuid"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"github.com/stretchr/testify/assert"
)

func TestMatchModifyNodeCluster(t *testing.T) {
	t.Parallel()

	const (
		nodeClusterName = "spoke-cluster"
		labelValue      = "test-label-value"
	)

	nodeClusterID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	subscriptionID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	matchingPost := map[string]any{
		"nodeClusterId": nodeClusterID.String(),
		"name":          nodeClusterName,
		"extensions": map[string]any{
			tsparams.TestNotificationLabel: labelValue,
		},
	}
	matchingPrior := map[string]any{"nodeClusterId": nodeClusterID.String()}

	match := MatchModifyNodeCluster(
		subscriptionID,
		nodeClusterID,
		nodeClusterName,
		tsparams.TestNotificationLabel,
		labelValue,
	)

	tests := []struct {
		name         string
		notification *oranapi.ClusterChangeNotification
		want         bool
	}{
		{
			name: "matching modify",
			notification: &oranapi.ClusterChangeNotification{
				NotificationEventType:  oranapi.ClusterChangeNotificationEventTypeModify,
				ConsumerSubscriptionId: &subscriptionID,
				PostObjectState:        &matchingPost,
				PriorObjectState:       &matchingPrior,
			},
			want: true,
		},
		{name: "nil notification"},
		{
			name: "non-modify event",
			notification: &oranapi.ClusterChangeNotification{
				NotificationEventType:  oranapi.ClusterChangeNotificationEventTypeCreate,
				ConsumerSubscriptionId: &subscriptionID,
				PostObjectState:        &matchingPost,
				PriorObjectState:       &matchingPrior,
			},
		},
		{
			name: "wrong extension value",
			notification: &oranapi.ClusterChangeNotification{
				NotificationEventType:  oranapi.ClusterChangeNotificationEventTypeModify,
				ConsumerSubscriptionId: &subscriptionID,
				PostObjectState: new(map[string]any{
					"nodeClusterId": nodeClusterID.String(),
					"name":          nodeClusterName,
					"extensions": map[string]any{
						tsparams.TestNotificationLabel: "other-value",
					},
				}),
				PriorObjectState: &matchingPrior,
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.want, match(testCase.notification))
		})
	}
}
