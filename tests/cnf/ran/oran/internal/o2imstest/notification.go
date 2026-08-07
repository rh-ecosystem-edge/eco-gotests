package o2imstest

import (
	"fmt"
	"strings"
)

// RefersTo reports whether a change notification refers to an object with the given id and name. It matches if
// objectRef contains objectID, or if idField/nameField match in post or prior object state.
func RefersTo(
	objectRef *string,
	post, prior *map[string]any,
	idField, objectID, nameField, name string,
) bool {
	if objectRef != nil && strings.Contains(*objectRef, objectID) {
		return true
	}

	for _, state := range []*map[string]any{post, prior} {
		if state == nil {
			continue
		}

		if value, ok := (*state)[idField]; ok && fmt.Sprint(value) == objectID {
			return true
		}

		if value, ok := (*state)[nameField]; ok && fmt.Sprint(value) == name {
			return true
		}
	}

	return false
}
