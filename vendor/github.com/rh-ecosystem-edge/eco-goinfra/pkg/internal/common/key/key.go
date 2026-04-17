package key

import (
	"fmt"
)

// ResourceKey is a standard set of information required to uniquely identify a resource in a cluster.
type ResourceKey struct {
	Kind      string
	Name      string
	Namespace string
}

func (k ResourceKey) String() string {
	if k.Name == "" {
		return k.Kind
	}

	if k.Namespace == "" {
		return fmt.Sprintf("%s %s", k.Kind, k.Name)
	}

	return fmt.Sprintf("%s %s/%s", k.Kind, k.Namespace, k.Name)
}

// NewResourceKey creates a new ResourceKey from the given kind, name, and namespace. It does not validate the input.
func NewResourceKey(kind string, name string, namespace string) ResourceKey {
	return ResourceKey{Kind: kind, Name: name, Namespace: namespace}
}
