// Package o2imscluster provides framework-independent verification helpers for the O2IMS Cluster API. It compares API
// responses against hub cluster state: NodeCluster and NodeClusterType objects are derived from node-cluster-eligible
// ManagedCluster resources (labels such as clusterID, vendor, and OpenShift version), while ClusterResource and
// ClusterResourceType objects are derived from ORAN-eligible Agent inventory (CPU architecture and core count, linked
// via the hwMgrNodeId label to resourceId).
//
// Discovery helpers ([CollectNodeClusterTypeKeys], [CollectClusterResourceTypeKeys], [ManagedClusterID],
// [ExpectedInventoryResourceID], [FindClusterResourceForAgent]) encode the same eligibility rules and naming patterns
// the collector uses, so tests can build expected sets before calling the API. Verify* functions compare API payloads
// to those expectations using small struct projections and [cmp.Diff]. Cluster change notifications use
// [MatchModifyNodeCluster], built on [o2imstest.RefersTo] and extension checks in postObjectState.
//
// Shared checks that are not cluster-specific (API version discovery, alarm dictionary shape, extension field parsing)
// live in the o2imstest package.
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
package o2imscluster
