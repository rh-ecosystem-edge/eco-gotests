package helper

import "time"

// GetStartTime returns the current time after waiting two seconds. Kubernetes timestamps have second precision, so
// callers use this delay before waiting for a phase transition that may begin and end within the same second.
func GetStartTime() time.Time {
	startTime := time.Now()

	// Since we cannot verify that a second starts at the same time on the cluster versus the test executor, it is
	// most reliable to wait for two seconds to ensure the time we count as starting is almost certainly not the
	// same second on both machines.
	time.Sleep(2 * time.Second)

	return startTime
}
