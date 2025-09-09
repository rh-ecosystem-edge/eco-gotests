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

// ClusterInstance preservation label constants
const (
	PreservationLabelKey      = Group + "/preserve"
	ClusterIdentityLabelValue = "cluster-identity"
)

// PausedAnnotation is the annotation that pauses the reconciliation.
const PausedAnnotation = "clusterinstance." + Group + "/paused"

const LastClusterInstanceSpecAnnotation = "clusterinstance." + Group + "/last-observed-spec"
