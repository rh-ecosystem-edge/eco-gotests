package helper

import "time"

// GetStartTime returns the current time after waiting two seconds. Kubernetes timestamps have second precision, so
// callers use this delay before waiting for a phase transition that may begin and end within the same second.
func GetStartTime() time.Time {
	startTime := time.Now()

	time.Sleep(2 * time.Second)

	return startTime
}
