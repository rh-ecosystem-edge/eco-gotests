// Package inventory provides helpers for O2IMS inventory API tests: verification against hub cluster state (CRs, BMHs,
// ManagedClusters), test fixture setup, hub-state discovery, and notification matching. Verification helpers accumulate
// errors so that test assertions capture all possible mismatches.
package inventory
