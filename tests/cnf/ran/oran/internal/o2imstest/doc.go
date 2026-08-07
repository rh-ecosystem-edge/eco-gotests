// Package o2imstest provides shared, hub-agnostic helpers used by the o2imscluster and o2imsinventory packages. It does
// not list or compare Kubernetes resources; callers supply API responses or notification payloads.
//
// The exported surface falls into a few groups:
//   - Service metadata: [VerifyAPIVersions] asserts the first entry in apiVersions and uriPrefix for cluster or
//     inventory version endpoints.
//   - Alarms: [VerifyAlarmDictionaryStructure] validates AlarmDictionary top-level fields and per-definition
//     required keys (including severity in additionalFields).
//   - Change notifications: [RefersTo] matches objectRef or id/name fields in prior and post object state maps.
//   - O2IMS extensions: [ExtensionString] and [AsStringKeyedMap] normalize JSON-decoded extension maps for
//     comparisons in the domain-specific Verify* helpers.
//
// Domain packages (o2imscluster, o2imsinventory) own mapping hub state to API types and call into o2imstest for common
// parsing and cross-cutting assertions. They follow the same design: accumulate errors with errors.Join, prefer
// [cmp.Diff] on small views where full object comparison is needed, and unit-test pure logic without constructing full
// test builders—integration coverage remains in the oran Ginkgo suite.
package o2imstest
