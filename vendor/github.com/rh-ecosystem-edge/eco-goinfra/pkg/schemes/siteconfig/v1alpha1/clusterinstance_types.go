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
	bmh_v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	aiv1beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	hivev1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/hive/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MachineNetworkEntry is a single IP address block for node IP blocks.
type MachineNetworkEntry struct {
	// CIDR is the IP block address pool for machines within the cluster.
	// +required
	CIDR string `json:"cidr"`
}

// ClusterNetworkEntry is a single IP address block for pod IP blocks. IP blocks
// are allocated with size 2^HostSubnetLength.
type ClusterNetworkEntry struct {
	// CIDR is the IP block address pool.
	// +required
	CIDR string `json:"cidr"`

	// HostPrefix is the prefix size to allocate to each node from the CIDR.
	// For example, 24 would allocate 2^8=256 adresses to each node. If this
	// field is not used by the plugin, it can be left unset.
	// +optional
	HostPrefix int32 `json:"hostPrefix,omitempty"`
}

// ServiceNetworkEntry is a single IP address block for node IP blocks.
type ServiceNetworkEntry struct {
	// CIDR is the IP block address pool for machines within the cluster.
	// +required
	CIDR string `json:"cidr"`
}

// BmcCredentialsName
type BmcCredentialsName struct {
	// +required
	Name string `json:"name"`
}

// IronicInspect is used to specify if automatic introspection carried out during registration
// of BMH is enabled or disabled.
// +kubebuilder:validation:Enum="";disabled
type IronicInspect string

// PlatformType is a specific supported infrastructure provider.
// +kubebuilder:validation:Enum="";BareMetal;None;VSphere;Nutanix;External
type PlatformType string

type TangConfig struct {
	URL        string `json:"url,omitempty"`
	Thumbprint string `json:"thumbprint,omitempty"`
}

type DiskEncryption struct {
	// +kubebuilder:default:=none
	Type string       `json:"type,omitempty"`
	Tang []TangConfig `json:"tang,omitempty"`
}

// CPUPartitioningMode is used to drive how a cluster nodes CPUs are Partitioned.
type CPUPartitioningMode string

const (
	// The only supported configurations are an all or nothing configuration.
	CPUPartitioningNone     CPUPartitioningMode = "None"
	CPUPartitioningAllNodes CPUPartitioningMode = "AllNodes"
)

// CpuArchitecture is used to define the software architecture of a host.
type CPUArchitecture string

const (
	// Supported architectures are x86, arm, or multi
	CPUArchitectureX86_64  CPUArchitecture = "x86_64"
	CPUArchitectureAarch64 CPUArchitecture = "aarch64"
	CPUArchitectureMulti   CPUArchitecture = "multi"
)

// Reference represents a namespaced reference to a Kubernetes object.
// It is commonly used to specify dependencies or related objects in different namespaces.
type Reference struct {
	// Name specifies the name of the referenced object.
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`

	// Namespace specifies the namespace of the referenced object.
	// +kubebuilder:validation:MinLength=1
	// +required
	Namespace string `json:"namespace"`
}

// TemplateRef is a reference to an installation Custom Resource (CR) template.
// It provides a way to specify the template to be used for an installation process.
type TemplateRef Reference

// HostRef is a reference to a BareMetalHost node located in another namespace.
// It is used to link a resource to a specific BareMetalHost instance.
type HostRef Reference

// ResourceRef represents the API version and kind of a Kubernetes resource
type ResourceRef struct {
	// APIVersion is the version of the Kubernetes API to use when interacting
	// with the resource. It includes both the API group and the version, such
	// as "v1" for core resources or "apps/v1" for deployments.
	// +required
	APIVersion string `json:"apiVersion"`

	// Kind is the type of Kubernetes resource being referenced.
	// +required
	Kind string `json:"kind"`
}

// NodeSpec
type NodeSpec struct {
	// BmcAddress holds the URL for accessing the controller on the network.
	// +required
	BmcAddress string `json:"bmcAddress"`

	// BmcCredentialsName is the name of the secret containing the BMC credentials (requires keys "username"
	// and "password").
	// +required
	BmcCredentialsName BmcCredentialsName `json:"bmcCredentialsName"`

	// Which MAC address will PXE boot? This is optional for some
	// types, but required for libvirt VMs driven by vbmc.
	// +kubebuilder:validation:Pattern=`[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}`
	// +required
	BootMACAddress string `json:"bootMACAddress"`

	// When set to disabled, automated cleaning will be avoided during provisioning and deprovisioning.
	// Set the value to metadata to enable the removal of the disk’s partitioning table only, without fully wiping
	// the disk. The default value is disabled.
	// +optional
	// +kubebuilder:default:=disabled
	AutomatedCleaningMode bmh_v1alpha1.AutomatedCleaningMode `json:"automatedCleaningMode,omitempty"`

	// RootDeviceHints specifies the device for deployment.
	// Identifiers that are stable across reboots are recommended, for example, wwn: <disk_wwn> or
	// deviceName: /dev/disk/by-path/<device_path>
	// +optional
	RootDeviceHints *bmh_v1alpha1.RootDeviceHints `json:"rootDeviceHints,omitempty"`

	// NodeNetwork is a set of configurations pertaining to the network settings for the node.
	// +optional
	NodeNetwork *aiv1beta1.NMStateConfigSpec `json:"nodeNetwork,omitempty"`

	// NodeLabels allows the specification of custom roles for your nodes in your managed clusters.
	// These are additional roles that are not used by any OpenShift Container Platform components, only by the user.
	// When you add a custom role, it can be associated with a custom machine config pool that references a specific
	// configuration for that role.
	// Adding custom labels or roles during installation makes the deployment process more effective and prevents the
	// need for additional reboots after the installation is complete.
	// +optional
	NodeLabels map[string]string `json:"nodeLabels,omitempty"`

	// Hostname is the desired hostname for the host
	// +required
	HostName string `json:"hostName"`

	// HostRef is used to specify a reference to a BareMetalHost resource.
	// +optional
	HostRef *HostRef `json:"hostRef,omitempty"`

	// CPUArchitecture is the software architecture of the node.
	// If it is not defined here then it is inheirited from the ClusterInstanceSpec.
	// +kubebuilder:validation:Enum=x86_64;aarch64
	// +optional
	CPUArchitecture CPUArchitecture `json:"cpuArchitecture,omitempty"`

	// Provide guidance about how to choose the device for the image being provisioned.
	// +kubebuilder:default:=UEFI
	// +optional
	BootMode bmh_v1alpha1.BootMode `json:"bootMode,omitempty"`

	// Json formatted string containing the user overrides for the host's coreos installer args
	// +optional
	InstallerArgs string `json:"installerArgs,omitempty"`

	// Json formatted string containing the user overrides for the host's ignition config
	// IgnitionConfigOverride enables the assignment of partitions for persistent storage.
	// Adjust disk ID and size to the specific hardware.
	// +optional
	IgnitionConfigOverride string `json:"ignitionConfigOverride,omitempty"`

	// +kubebuilder:validation:Enum=master;worker;arbiter
	// +kubebuilder:default:=master
	// +optional
	Role string `json:"role,omitempty"`

	// Additional node-level annotations to be applied to the rendered templates
	// +optional
	ExtraAnnotations map[string]map[string]string `json:"extraAnnotations,omitempty"`

	// Additional node-level labels to be applied to the rendered templates
	// +optional
	ExtraLabels map[string]map[string]string `json:"extraLabels,omitempty"`

	// SuppressedManifests is a list of node-level manifest names to be excluded from the template rendering process
	// +optional
	SuppressedManifests []string `json:"suppressedManifests,omitempty"`

	// PruneManifests represents a list of Kubernetes resource references that indicates which "node-level" manifests
	// should be pruned (removed).
	// +optional
	PruneManifests []ResourceRef `json:"pruneManifests,omitempty"`

	// IronicInspect is used to specify if automatic introspection carried out during registration of BMH is enabled or
	// disabled
	// +kubebuilder:default:=""
	// +optional
	IronicInspect IronicInspect `json:"ironicInspect,omitempty"`

	// TemplateRefs is a list of references to node-level templates. A node-level template consists of a ConfigMap
	// in which the keys of the data field represent the kind of the installation manifest(s).
	// Node-level templates are instantiated once for each node in the ClusterInstance CR.
	// +required
	TemplateRefs []TemplateRef `json:"templateRefs"`
}

// ClusterType is a string representing the cluster type
type ClusterType string

const (
	ClusterTypeSNO                    ClusterType = "SNO"
	ClusterTypeHighlyAvailable        ClusterType = "HighlyAvailable"
	ClusterTypeHostedControlPlane     ClusterType = "HostedControlPlane"
	ClusterTypeHighlyAvailableArbiter ClusterType = "HighlyAvailableArbiter"
)

// PreservationMode represents the modes of data preservation for a ClusterInstance during reinstallation.
type PreservationMode string

// Supported modes of data preservation for reinstallation.
const (
	// PreservationModeNone indicates that no data preservation will be performed.
	PreservationModeNone PreservationMode = "None"

	// PreservationModeAll indicates that all resources labeled with PreservationLabelKey will be preserved.
	PreservationModeAll PreservationMode = "All"

	// PreservationModeClusterIdentity indicates that only cluster identity resources labeled with
	// PreservationLabelKey and ClusterIdentityLabelValue will be preserved.
	PreservationModeClusterIdentity PreservationMode = "ClusterIdentity"
)

// ReinstallSpec defines the configuration for reinstallation of a ClusterInstance.
type ReinstallSpec struct {
	// Generation specifies the desired generation for the reinstallation operation.
	// Updating this field triggers a new reinstall request.
	// +required
	Generation string `json:"generation"`

	// PreservationMode defines the strategy for data preservation during reinstallation.
	// Supported values:
	// - None: No data will be preserved.
	// - All: All Secrets and ConfigMaps in the ClusterInstance namespace labeled with the PreservationLabelKey will be
	//   preserved.
	// - ClusterIdentity: Only Secrets and ConfigMaps in the ClusterInstance namespace labeled with both the
	//   PreservationLabelKey and the ClusterIdentityLabelValue will be preserved.
	// This field ensures critical cluster identity data is preserved when required.
	// +kubebuilder:validation:Enum=None;All;ClusterIdentity
	// +kubebuilder:default=None
	// +required
	PreservationMode PreservationMode `json:"preservationMode"`
}

// ClusterInstanceSpec defines the desired state of ClusterInstance
type ClusterInstanceSpec struct {
	// Desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ClusterName is the name of the cluster.
	// +required
	ClusterName string `json:"clusterName"`

	// PullSecretRef is the reference to the secret to use when pulling images.
	// +required
	PullSecretRef corev1.LocalObjectReference `json:"pullSecretRef"`

	// ClusterImageSetNameRef is the name of the ClusterImageSet resource indicating which
	// OpenShift version to deploy.
	// +required
	ClusterImageSetNameRef string `json:"clusterImageSetNameRef"`

	// SSHPublicKey is the public Secure Shell (SSH) key to provide access to instances.
	// This key will be added to the host to allow ssh access
	// +optional
	SSHPublicKey string `json:"sshPublicKey,omitempty"`

	// BaseDomain is the base domain to use for the deployed cluster.
	// +required
	BaseDomain string `json:"baseDomain"`

	// APIVIPs are the virtual IPs used to reach the OpenShift cluster's API.
	// Enter one IP address for single-stack clusters, or up to two for dual-stack clusters (at
	// most one IP address per IP stack used). The order of stacks should be the same as order
	// of subnets in Cluster Networks, Service Networks, and Machine Networks.
	// +kubebuilder:validation:MaxItems=2
	// +optional
	ApiVIPs []string `json:"apiVIPs,omitempty"`

	// IngressVIPs are the virtual IPs used for cluster ingress traffic.
	// Enter one IP address for single-stack clusters, or up to two for dual-stack clusters (at
	// most one IP address per IP stack used). The order of stacks should be the same as order
	// of subnets in Cluster Networks, Service Networks, and Machine Networks.
	// +kubebuilder:validation:MaxItems=2
	// +optional
	IngressVIPs []string `json:"ingressVIPs,omitempty"`

	// HoldInstallation will prevent installation from happening when true.
	// Inspection and validation will proceed as usual, but once the RequirementsMet condition is true,
	// installation will not begin until this field is set to false.
	// +kubebuilder:default:=false
	// +optional
	HoldInstallation bool `json:"holdInstallation,omitempty"`

	// AdditionalNTPSources is a list of NTP sources (hostname or IP) to be added to all cluster
	// hosts. They are added to any NTP sources that were configured through other means.
	// +optional
	AdditionalNTPSources []string `json:"additionalNTPSources,omitempty"`

	// MachineNetwork is the list of IP address pools for machines.
	// +optional
	MachineNetwork []MachineNetworkEntry `json:"machineNetwork,omitempty"`

	// ClusterNetwork is the list of IP address pools for pods.
	// +optional
	ClusterNetwork []ClusterNetworkEntry `json:"clusterNetwork,omitempty"`

	// ServiceNetwork is the list of IP address pools for services.
	// +optional
	ServiceNetwork []ServiceNetworkEntry `json:"serviceNetwork,omitempty"`

	// NetworkType is the Container Network Interface (CNI) plug-in to install
	// The default value is OpenShiftSDN for IPv4, and OVNKubernetes for IPv6 or SNO
	// +kubebuilder:validation:Enum=OpenShiftSDN;OVNKubernetes
	// +kubebuilder:default:=OVNKubernetes
	// +optional
	NetworkType string `json:"networkType,omitempty"`

	// PlatformType is the name for the specific platform upon which to perform the installation.
	// +optional
	PlatformType PlatformType `json:"platformType,omitempty"`

	// Additional cluster-wide annotations to be applied to the rendered templates
	// +optional
	ExtraAnnotations map[string]map[string]string `json:"extraAnnotations,omitempty"`

	// Additional cluster-wide labels to be applied to the rendered templates
	// +optional
	ExtraLabels map[string]map[string]string `json:"extraLabels,omitempty"`

	// InstallConfigOverrides is a Json formatted string that provides a generic way of passing
	// install-config parameters.
	// +optional
	InstallConfigOverrides string `json:"installConfigOverrides,omitempty"`

	// Json formatted string containing the user overrides for the initial ignition config
	// +optional
	IgnitionConfigOverride string `json:"ignitionConfigOverride,omitempty"`

	// DiskEncryption is the configuration to enable/disable disk encryption for cluster nodes.
	// +optional
	DiskEncryption *DiskEncryption `json:"diskEncryption,omitempty"`

	// Proxy defines the proxy settings used for the install config
	// +optional
	Proxy *aiv1beta1.Proxy `json:"proxy,omitempty"`

	// ExtraManifestsRefs is list of config map references containing additional manifests to be applied to the cluster.
	// +optional
	ExtraManifestsRefs []corev1.LocalObjectReference `json:"extraManifestsRefs,omitempty"`

	// SuppressedManifests is a list of manifest names to be excluded from the template rendering process
	// +optional
	SuppressedManifests []string `json:"suppressedManifests,omitempty"`

	// PruneManifests represents a list of Kubernetes resource references that indicates which manifests should be
	// pruned (removed).
	// +optional
	PruneManifests []ResourceRef `json:"pruneManifests,omitempty"`

	// CPUPartitioning determines if a cluster should be setup for CPU workload partitioning at install time.
	// When this field is set the cluster will be flagged for CPU Partitioning allowing users to segregate workloads to
	// specific CPU Sets. This does not make any decisions on workloads it only configures the nodes to allow CPU
	// Partitioning.
	// The "AllNodes" value will setup all nodes for CPU Partitioning, the default is "None".
	// +kubebuilder:validation:Enum=None;AllNodes
	// +kubebuilder:default=None
	// +optional
	CPUPartitioning CPUPartitioningMode `json:"cpuPartitioningMode,omitempty"`

	// CPUArchitecture is the default software architecture used for nodes that do not have an architecture defined.
	// +kubebuilder:validation:Enum=x86_64;aarch64;multi
	// +kubebuilder:default:=x86_64
	// +optional
	CPUArchitecture CPUArchitecture `json:"cpuArchitecture,omitempty"`

	// +kubebuilder:validation:Enum=SNO;HighlyAvailable;HostedControlPlane;HighlyAvailableArbiter
	// +optional
	ClusterType ClusterType `json:"clusterType,omitempty"`

	// TemplateRefs is a list of references to cluster-level templates. A cluster-level template consists of a ConfigMap
	// in which the keys of the data field represent the kind of the installation manifest(s).
	// Cluster-level templates are instantiated once per cluster (ClusterInstance CR).
	// +required
	TemplateRefs []TemplateRef `json:"templateRefs"`

	// CABundle is a reference to a config map containing the new bundle of trusted certificates for the host.
	// +optional
	CaBundleRef *corev1.LocalObjectReference `json:"caBundleRef,omitempty"`

	// List of node objects
	// +required
	Nodes []NodeSpec `json:"nodes"`

	// Reinstall specifications
	// +optional
	Reinstall *ReinstallSpec `json:"reinstall,omitempty"`
}

const (
	ManifestRenderedSuccess    = "rendered"
	ManifestRenderedFailure    = "failed"
	ManifestRenderedValidated  = "validated"
	ManifestSuppressed         = "suppressed"
	ManifestDeleted            = "deleted"
	ManifestDeletionInProgress = "deletion-in-progress"
	ManifestDeletionFailure    = "deletion-failed"
	ManifestDeletionTimedOut   = "deletion-attempt-timed-out"
)

// ManifestReference contains enough information to let you locate the
// typed referenced object inside the same namespace.
// +structType=atomic
type ManifestReference struct {
	// APIGroup is the group for the resource being referenced.
	// If APIGroup is not specified, the specified Kind must be in the core API group.
	// For any other third-party types, APIGroup is required.
	// +required
	APIGroup *string `json:"apiGroup"`
	// Kind is the type of resource being referenced
	// +required
	Kind string `json:"kind"`
	// Name is the name of the resource being referenced
	// +required
	Name string `json:"name"`
	// Namespace is the namespace of the resource being referenced
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// SyncWave is the order in which the resource should be processed: created in ascending order, deleted in
	// descending order.
	// +required
	SyncWave int `json:"syncWave"`
	// Status is the status of the manifest
	// +required
	Status string `json:"status"`
	// lastAppliedTime is the last time the manifest was applied.
	// This should be when the underlying manifest changed.  If that is not known, then using the time when the API
	// field changed is acceptable.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=date-time
	// +required
	LastAppliedTime metav1.Time `json:"lastAppliedTime"`
	// message is a human readable message indicating details about the transition.
	// This may be an empty string.
	// +kubebuilder:validation:MaxLength=32768
	// +optional
	Message string `json:"message,omitempty"`
}

// ReinstallHistory represents a record of a reinstallation event for a ClusterInstance.
type ReinstallHistory struct {
	// Generation specifies the generation of the ClusterInstance at the time of the reinstallation.
	// This value corresponds to the ReinstallSpec.Generation field associated with the reinstallation request.
	// +required
	Generation string `json:"generation"`

	// RequestStartTime indicates the time at which SiteConfig was requested to reinstall.
	// +required
	RequestStartTime metav1.Time `json:"requestStartTime,omitempty"`

	// RequestEndTime indicates the time at which SiteConfig completed processing the reinstall request.
	// +required
	RequestEndTime metav1.Time `json:"requestEndTime,omitempty"`

	// ClusterInstanceSpecDiff provides a JSON representation of the differences between the
	// ClusterInstance spec at the time of reinstallation and the previous spec.
	// This field helps in tracking changes that triggered the reinstallation.
	// +required
	ClusterInstanceSpecDiff string `json:"clusterInstanceSpecDiff"`
}

// ReinstallStatus represents the current state and historical details of reinstall operations for a ClusterInstance.
type ReinstallStatus struct {

	// List of conditions pertaining to reinstall requests.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// InProgressGeneration is the generation of the ClusterInstance that is being processed for reinstallation.
	// It corresponds to the Generation field in ReinstallSpec and indicates the latest reinstall request that
	// the controller is acting upon.
	// +optional
	InProgressGeneration string `json:"inProgressGeneration,omitempty"`

	// ObservedGeneration is the generation of the ClusterInstance that has been processed for reinstallation.
	// It corresponds to the Generation field in ReinstallSpec and indicates the latest reinstall request that
	// the controller has acted upon.
	// +optionsl
	ObservedGeneration string `json:"observedGeneration,omitempty"`

	// RequestStartTime indicates the time at which SiteConfig was requested to reinstall.
	// +optional
	RequestStartTime metav1.Time `json:"requestStartTime,omitempty"`

	// RequestEndTime indicates the time at which SiteConfig completed processing the reinstall request.
	// +optional
	RequestEndTime metav1.Time `json:"requestEndTime,omitempty"`

	// History maintains a record of all previous reinstallation attempts.
	// Each entry captures details such as the generation, timestamp, and the differences in the ClusterInstance
	// specification that triggered the reinstall.
	// This field is useful for debugging, auditing, and tracking reinstallation events over time.
	// +optional
	History []ReinstallHistory `json:"history,omitempty"`
}

type PausedStatus struct {
	// TimeSet indicates when the paused annotation was applied.
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=date-time
	TimeSet metav1.Time `json:"timeSet"`

	// Reason provides an explanation for why the paused annotation was applied.
	// This field may not be empty.
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=32768
	Reason string `json:"reason"`
}

// ClusterInstanceStatus defines the observed state of ClusterInstance
type ClusterInstanceStatus struct {
	// Important: Run "make" to regenerate code after modifying this file

	// List of conditions pertaining to actions performed on the ClusterInstance resource.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Reference to the associated ClusterDeployment resource.
	// +optional
	ClusterDeploymentRef *corev1.LocalObjectReference `json:"clusterDeploymentRef,omitempty"`

	// List of hive status conditions associated with the ClusterDeployment resource.
	// +optional
	DeploymentConditions []hivev1.ClusterDeploymentCondition `json:"deploymentConditions,omitempty"`

	// List of manifests that have been rendered along with their status.
	// +optional
	ManifestsRendered []ManifestReference `json:"manifestsRendered,omitempty"`

	// Track the observed generation to avoid unnecessary reconciles
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Reinstall status information.
	// +optional
	Reinstall *ReinstallStatus `json:"reinstall,omitempty"`

	// Paused provides information about the pause annotation set by the controller
	// to temporarily pause reconciliation of the ClusterInstance.
	// +optional
	Paused *PausedStatus `json:"paused,omitempty"`
}

//nolint:lll
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=clusterinstances,scope=Namespaced
//+kubebuilder:printcolumn:name="Paused",type="date",JSONPath=".status.paused.timeSet"
//+kubebuilder:printcolumn:name="ProvisionStatus",type="string",JSONPath=".status.conditions[?(@.type=='Provisioned')].reason"
//+kubebuilder:printcolumn:name="ProvisionDetails",type="string",JSONPath=".status.conditions[?(@.type=='Provisioned')].message"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterInstance is the Schema for the clusterinstances API
type ClusterInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterInstanceSpec   `json:"spec,omitempty"`
	Status ClusterInstanceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClusterInstanceList contains a list of ClusterInstance
type ClusterInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterInstance{}, &ClusterInstanceList{})
}
