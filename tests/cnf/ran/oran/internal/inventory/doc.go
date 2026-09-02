// Package inventory provides framework-independent verification helpers that compare O2IMS inventory API responses
// against hub cluster state (CRs, BMHs, ManagedClusters).
//
// The helpers are designed to accumulate errors so that test assertions capture all possible mismatches. When the
// helpers require cluster resources, they generally move the comparison logic into unexported functions that may be
// unit tested independently.
//
// Ideally, comparisons use projections with [cmp.Diff] to handle the differences between the API and hub cluster state.
// This avoids turning the package into an assertion framework and focuses on the core logic translating between types
// in the O2IMS and Kubernetes APIs.
//
// Unit tests which would require constructing builders are excluded and these cases are covered by the broader oran
// test suite.
package inventory
