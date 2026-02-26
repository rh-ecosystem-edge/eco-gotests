/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExtraAnnotationSearch Looks up a specific manifest Annotation for this cluster
func (c *ClusterInstanceSpec) ExtraAnnotationSearch(kind string) (map[string]string, bool) {
	annotations, ok := c.ExtraAnnotations[kind]
	return annotations, ok
}

// ExtraAnnotationSearch Looks up a specific manifest annotation for this node, with fallback to cluster
func (node *NodeSpec) ExtraAnnotationSearch(kind string, cluster *ClusterInstanceSpec) (map[string]string, bool) {
	annotations, ok := node.ExtraAnnotations[kind]
	if ok {
		return annotations, ok
	}
	return cluster.ExtraAnnotationSearch(kind)
}

// ExtraLabelSearch Looks up a specific manifest label for this cluster
func (c *ClusterInstanceSpec) ExtraLabelSearch(kind string) (map[string]string, bool) {
	labels, ok := c.ExtraLabels[kind]
	return labels, ok
}

// ExtraLabelSearch Looks up a specific manifest label for this node, with fallback to cluster
func (node *NodeSpec) ExtraLabelSearch(kind string, cluster *ClusterInstanceSpec) (map[string]string, bool) {
	labels, ok := node.ExtraLabels[kind]
	if ok {
		return labels, ok
	}
	return cluster.ExtraLabelSearch(kind)
}

// MatchesIdentity checks if two ManifestReference objects are equal based on identifying fields.
// These fields are APIGroup, Kind, Name, and Namespace.
func (m *ManifestReference) MatchesIdentity(other *ManifestReference) bool {
	if m == nil || other == nil {
		return false
	}

	// Safely compare APIGroup pointers
	APIGroupMatches := m.APIGroup == nil && other.APIGroup == nil ||
		m.APIGroup != nil && other.APIGroup != nil && *m.APIGroup == *other.APIGroup

	// Compare identifying fields
	return APIGroupMatches &&
		m.Kind == other.Kind &&
		m.Name == other.Name &&
		m.Namespace == other.Namespace
}

func (m *ManifestReference) UpdateStatus(status, message string) {
	if m.Status != status || m.Message != message {
		m.Status = status
		m.Message = message
		m.LastAppliedTime = metav1.Now()
	}
}

// String generates a Kubernetes style resource path string from the ManifestReference object
func (m *ManifestReference) String() string {
	apiGroup := ""
	if m.APIGroup != nil && *m.APIGroup != "" {
		apiGroup = fmt.Sprintf(".%s", *m.APIGroup)
	}

	// Handle Namespace if present
	if m.Namespace != "" {
		return fmt.Sprintf("%s%s/namespaces/%s/%s", strings.ToLower(m.Kind), apiGroup, m.Namespace, m.Name)
	}

	// Return without Namespace if it is empty
	return fmt.Sprintf("%s%s/%s", strings.ToLower(m.Kind), apiGroup, m.Name)
}

// IndexOfManifestByIdentity searches for a ManifestReference in the given list based on identity fields
// and returns its index. It returns -1 and a not found error if the target is not found.
func IndexOfManifestByIdentity(target *ManifestReference, manifestRefs []ManifestReference) (int, error) {
	for i, ref := range manifestRefs {
		if ref.MatchesIdentity(target) {
			return i, nil
		}
	}
	return -1, fmt.Errorf("manifestReference (%s) not found", target.String())
}

// IsPaused checks if the ClusterInstance has the paused annotation.
func (ci *ClusterInstance) IsPaused() bool {
	annotations := ci.ObjectMeta.Annotations
	if annotations == nil {
		return false
	}
	_, exists := annotations[PausedAnnotation]
	return exists
}
