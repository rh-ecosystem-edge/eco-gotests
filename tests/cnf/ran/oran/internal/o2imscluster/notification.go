package o2imscluster

import (
	"github.com/google/uuid"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/o2imstest"
)

// MatchModifyNodeCluster returns a matcher for MODIFY notifications on the given NodeCluster.
//
// A notification matches when the subscription ID agrees, both prior and post object state are present, the cluster is
// referenced in objectRef or via nodeClusterId/name in either state, and postObjectState.extensions contains the given
// key=value.
func MatchModifyNodeCluster(
	consumerSubscriptionID, nodeClusterID uuid.UUID,
	nodeClusterName, extensionKey, extensionValue string,
) func(*oranapi.ClusterChangeNotification) bool {
	return func(notification *oranapi.ClusterChangeNotification) bool {
		if notification == nil {
			return false
		}

		if notification.NotificationEventType != oranapi.ClusterChangeNotificationEventTypeModify {
			return false
		}

		if notification.ConsumerSubscriptionId == nil || *notification.ConsumerSubscriptionId != consumerSubscriptionID {
			return false
		}

		if notification.PriorObjectState == nil || notification.PostObjectState == nil {
			return false
		}

		if !o2imstest.RefersTo(
			notification.ObjectRef,
			notification.PostObjectState,
			notification.PriorObjectState,
			"nodeClusterId", nodeClusterID.String(),
			"name", nodeClusterName,
		) {
			return false
		}

		extensions, ok := o2imstest.AsStringKeyedMap((*notification.PostObjectState)["extensions"])

		return ok && extensions[extensionKey] == extensionValue
	}
}
