package msg

import "fmt"

// UndefinedCrdObjectErrString returns an error message for an undefined CR.
func UndefinedCrdObjectErrString(crName string) string {
	return fmt.Sprintf("can not redefine the undefined %s", crName)
}

// FailToUpdateNotification returns a notification message fail to update cr.
func FailToUpdateNotification(crName, objName string, nsName ...string) string {
	msg := fmt.Sprintf("Failed to update the %s object %s", crName, objName)
	if len(nsName) > 0 {
		msg += fmt.Sprintf(" in namespace %s", nsName[0])
	}

	return msg + ". Note: Force flag set, executed delete/create methods instead"
}

// FailToUpdateError returns an error message error to update due to failure in delete function.
func FailToUpdateError(crName, objName string, nsName ...string) string {
	msg := fmt.Sprintf("Failed to update the %s object %s", crName, objName)
	if len(nsName) > 0 {
		msg += fmt.Sprintf(" in namespace %s", nsName[0])
	}

	return msg + ", due to error in delete function"
}
