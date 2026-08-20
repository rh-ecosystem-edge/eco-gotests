// Package resource models K8s PTP resources as an identity (a coordinate to find or write the real CR
// again) paired with parsed facts -- the strangler fig replacement for profiles.ProfileInfo/
// InterfaceInfo's own conflation of the two. Metadata is always a plain reference, never a live handle:
// resolving it against a freshly-fetched resource list is the caller's job, not something Resource does
// itself.
package resource

import (
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource pairs one parsed K8s resource's own identity with its parsed facts.
type Resource[M any, D any] struct {
	Metadata M
	Data     D
}

// ConfigReference identifies one PtpConfig CR.
type ConfigReference struct {
	Name runtimeclient.ObjectKey
}

// ProfileReference locates one profile within a PtpConfig CR.
type ProfileReference struct {
	ConfigReference
	ProfileIndex int
}

// HardwareConfigReference identifies one HardwareConfig CR.
type HardwareConfigReference struct {
	Name runtimeclient.ObjectKey
}
