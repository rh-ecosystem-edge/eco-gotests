/*
Copyright 2023.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicfgv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/imagebasedinstall/hive/api/v1"
)

const (
	ConfigurationPendingReason = "ConfigurationPending"
	ConfigurationFailedReason  = "ConfigurationFailed"

	ImageCreationFailedReason  = "ImageCreationFailed"
	ImageCreationPendingReason = "ImageCreationPending"

	HostConfigurationPendingReason   = "HostConfigurationPending"
	HostConfigurationFailedReason    = "HostConfigurationFailed"
	HostConfigurationSucceededReason = "HostConfigurationSucceeded"

	InstallTimedoutReason  = "ClusterInstallationTimedOut"
	InstallTimedoutMessage = "Cluster installation is taking longer than expected"

	InstallInProgressReason  = "ClusterInstallationInProgress"
	InstallInProgressMessage = "Cluster installation is in progress"

	InstallSucceededReason  = "ClusterInstallationSucceeded"
	InstallSucceededMessage = "Cluster installation has succeeded"

	HostValidationFailedReason  = "HostValidationFailed"
	HostValidationPendingReason = "HostValidationPending"
)

// ImageClusterInstallSpec defines the desired state of ImageClusterInstall
type ImageClusterInstallSpec struct {
	// ClusterDeploymentRef is a reference to the ClusterDeployment.
	// +optional
	ClusterDeploymentRef *corev1.LocalObjectReference `json:"clusterDeploymentRef"`

	// ImageSetRef is a reference to a ClusterImageSet.
	ImageSetRef hivev1.ClusterImageSetReference `json:"imageSetRef"`

	// ClusterMetadata contains metadata information about the installed cluster.
	// This must be set as soon as all the information is available.
	// +optional
	ClusterMetadata *hivev1.ClusterMetadata `json:"clusterMetadata"`

	// NodeIP is the desired IP for the host
	// +optional
	// Deprecated: this field is ignored (will be removed in a future release).
	NodeIP string `json:"nodeIP,omitempty"`

	// Hostname is the desired hostname for the host
	Hostname string `json:"hostname,omitempty"`

	// SSHKey is the public Secure Shell (SSH) key to provide access to
	// instances. Equivalent to install-config.yaml's sshKey.
	// This key will be added to the host to allow ssh access
	SSHKey string `json:"sshKey,omitempty"`

	// ImageDigestSources lists sources/repositories for the release-image content.
	// +optional
	ImageDigestSources []apicfgv1.ImageDigestMirrors `json:"imageDigestSources,omitempty"`

	// CABundle is a reference to a config map containing the new bundle of trusted certificates for the host.
	// The tls-ca-bundle.pem entry in the config map will be written to /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem
	CABundleRef *corev1.LocalObjectReference `json:"caBundleRef,omitempty"`

	// ExtraManifestsRefs is list of config map references containing additional manifests to be applied to the relocated cluster.
	// +optional
	ExtraManifestsRefs []corev1.LocalObjectReference `json:"extraManifestsRefs,omitempty"`

	// BareMetalHostRef identifies a BareMetalHost object to be used to attach the configuration to the host.
	// +optional
	BareMetalHostRef *BareMetalHostReference `json:"bareMetalHostRef,omitempty"`

	// MachineNetwork is the subnet provided by user for the ocp cluster.
	// This will be used to create the node network and choose ip address for the node.
	// Equivalent to install-config.yaml's machineNetwork.
	// Deprecated: Use MachineNetworks instead.
	// +optional.
	MachineNetwork string `json:"machineNetwork,omitempty"`

	// MachineNetworks is the list of IP address pools for machines.
	// This enables dual-stack support by allowing multiple networks.
	// If both MachineNetwork and MachineNetworks are specified, MachineNetwork should match
	// the first element of MachineNetworks.
	// Use instead of MachineNetwork.
	// +optional
	MachineNetworks []MachineNetworkEntry `json:"machineNetworks,omitempty"`

	// Proxy defines the proxy settings to be applied in relocated cluster
	// +optional
	Proxy *Proxy `json:"proxy,omitempty"`

	// AdditionalNTPSources is a list of NTP sources (hostname or IP) to be added to all cluster
	// hosts. They are added to any NTP sources that were configured through other means.
	// +optional
	AdditionalNTPSources []string `json:"additionalNTPSources,omitempty"`
}

// ImageClusterInstallStatus defines the observed state of ImageClusterInstall
type ImageClusterInstallStatus struct {
	// Conditions is a list of conditions associated with syncing to the cluster.
	// +optional
	Conditions []hivev1.ClusterInstallCondition `json:"conditions,omitempty"`

	// InstallRestarts is the total count of container restarts on the clusters install job.
	InstallRestarts int `json:"installRestarts,omitempty"`

	BareMetalHostRef *BareMetalHostReference `json:"bareMetalHostRef,omitempty"`

	// BootTime indicates the time at which the host was requested to boot. Used to determine install timeouts.
	BootTime metav1.Time `json:"bootTime,omitempty"`
}

type BareMetalHostReference struct {
	// Name identifies the BareMetalHost within a namespace
	Name string `json:"name"`
	// Namespace identifies the namespace containing the referenced BareMetalHost
	Namespace string `json:"namespace"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:path=imageclusterinstalls,shortName=ici
// +kubebuilder:printcolumn:name="RequirementsMet",type="string",JSONPath=".status.conditions[?(@.type=='RequirementsMet')].reason"
// +kubebuilder:printcolumn:name="Completed",type="string",JSONPath=".status.conditions[?(@.type=='Completed')].reason"
// +kubebuilder:printcolumn:name="BareMetalHostRef",type="string",JSONPath=".spec.bareMetalHostRef.name"

// ImageClusterInstall is the Schema for the imageclusterinstall API
type ImageClusterInstall struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageClusterInstallSpec   `json:"spec,omitempty"`
	Status ImageClusterInstallStatus `json:"status,omitempty"`
}

// Proxy defines the proxy settings for the cluster.
// At least one of HTTPProxy or HTTPSProxy is required.
type Proxy struct {
	// HTTPProxy is the URL of the proxy for HTTP requests.
	// +optional
	HTTPProxy string `json:"httpProxy,omitempty"`

	// HTTPSProxy is the URL of the proxy for HTTPS requests.
	// +optional
	HTTPSProxy string `json:"httpsProxy,omitempty"`

	// NoProxy is a comma-separated list of domains and CIDRs for which the proxy should not be
	// used.
	// +optional
	NoProxy string `json:"noProxy,omitempty"`
}

// MachineNetworkEntry is a single IP address block for node IP blocks.
type MachineNetworkEntry struct {
	// CIDR is the IP block address pool for machines within the cluster.
	CIDR string `json:"cidr"`
}

//+kubebuilder:object:root=true

// ImageClusterInstallList contains a list of ImageClusterInstall
type ImageClusterInstallList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImageClusterInstall `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImageClusterInstall{}, &ImageClusterInstallList{})
}
