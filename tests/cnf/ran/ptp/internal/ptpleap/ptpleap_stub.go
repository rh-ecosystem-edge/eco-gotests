//go:build unit_test

package ptpleap

import "time"

// WaitForConfigmapToBeUpdated is a stub for golangci typecheck under the unit_test build tag.
// The integration implementation lives in ptpleap.go (!unit_test).
func WaitForConfigmapToBeUpdated(_ string, _ string, _ time.Duration, _ time.Duration) error {
	return nil
}
