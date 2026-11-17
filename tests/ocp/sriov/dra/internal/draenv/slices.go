// Package draenv implements DRA test observations without test-framework dependencies.
package draenv

import (
	"fmt"

	resourcev1 "k8s.io/api/resource/v1"
)

// Driver and ResourceAttribute identify the SR-IOV DRA advertisement.
const (
	Driver            = "sriovnetwork.k8snetworkplumbingwg.io"
	ResourceAttribute = "k8s.cni.cncf.io/resourceName"
)

// CountDevices counts only complete, latest generations, avoiding transient double-counting.
func CountDevices(slices []resourcev1.ResourceSlice, node, resource string) (int, error) {
	generations := map[string]int64{}

	for _, slice := range slices {
		if slice.Spec.Driver != Driver || slice.Spec.NodeName == nil || *slice.Spec.NodeName != node {
			continue
		}

		old, found := generations[slice.Spec.Pool.Name]
		if !found || slice.Spec.Pool.Generation > old {
			generations[slice.Spec.Pool.Name] = slice.Spec.Pool.Generation
		}
	}

	counts := map[string]int64{}
	expected := map[string]int64{}
	total := 0

	for _, slice := range slices {
		if slice.Spec.Driver != Driver || slice.Spec.NodeName == nil || *slice.Spec.NodeName != node ||
			slice.Spec.Pool.Generation != generations[slice.Spec.Pool.Name] {
			continue
		}

		pool := slice.Spec.Pool.Name
		if value, found := expected[pool]; found && value != slice.Spec.Pool.ResourceSliceCount {
			return 0, fmt.Errorf("inconsistent slice counts in pool %s", pool)
		}

		expected[pool] = slice.Spec.Pool.ResourceSliceCount
		counts[pool]++

		for _, device := range slice.Spec.Devices {
			attr := device.Attributes[resourcev1.QualifiedName(ResourceAttribute)]
			if attr.StringValue != nil && *attr.StringValue == resource {
				total++
			}
		}
	}

	for pool, count := range counts {
		if expected[pool] <= 0 || count != expected[pool] {
			return 0, fmt.Errorf("pool %s incomplete: have %d slices, expect %d", pool, count, expected[pool])
		}
	}

	return total, nil
}
