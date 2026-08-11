package inventory

import (
	"fmt"
	"strings"

	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
)

// NotificationRefersToPool reports whether the notification references the given ResourcePool.
func NotificationRefersToPool(
	notification *oranapi.InventoryChangeNotification,
	poolID, poolName string,
) bool {
	if notification.ObjectRef != nil && strings.Contains(*notification.ObjectRef, poolID) {
		return true
	}

	if notification.PostObjectState != nil {
		if value, ok := (*notification.PostObjectState)["resourcePoolId"]; ok && fmt.Sprint(value) == poolID {
			return true
		}

		if value, ok := (*notification.PostObjectState)["name"]; ok && fmt.Sprint(value) == poolName {
			return true
		}
	}

	if notification.PriorObjectState != nil {
		if value, ok := (*notification.PriorObjectState)["resourcePoolId"]; ok && fmt.Sprint(value) == poolID {
			return true
		}

		if value, ok := (*notification.PriorObjectState)["name"]; ok && fmt.Sprint(value) == poolName {
			return true
		}
	}

	return false
}
