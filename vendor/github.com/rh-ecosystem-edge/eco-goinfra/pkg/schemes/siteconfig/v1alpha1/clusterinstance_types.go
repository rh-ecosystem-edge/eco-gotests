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

// BmcCredentialsName is a reference to a BareMetalHost BMC credentials Secret.
type BmcCredentialsName struct {
	// name is the name of the Secret containing the BMC credentials
	// (requires keys "username" and "password").
	// +required
	Name string `json:"name"`
}

// IronicInspect controls automatic hardware introspection performed by Ironic
// during BareMetalHost registration. The default empty string enables
// inspection. Set to "disabled" to skip hardware inspection.
// +kubebuilder:validation:Enum="";disabled
type IronicInspect string

// PlatformType specifies the infrastructure platform for installation.
// Only applicable to the Assisted Installer flow. When empty, the field
// is omitted from the rendered AgentClusterInstall, allowing the installer
// to select a default.
// +kubebuilder:validation:Enum="";BareMetal;None;VSphere;Nutanix;External
type PlatformType string

// TangConfig holds the configuration for a single Tang server used for
// network-bound disk encryption (NBDE).
type TangConfig struct {
	// url is the URL of the Tang server.
	URL string `json:"url,omitempty"`
	// thumbprint is the thumbprint of the Tang server's signing key.
	Thumbprint string `json:"thumbprint,omitempty"`
}

// DiskEncryption configures disk encryption for cluster nodes. Use the
// type sub-field to select the encryption mode and tang to provide
// Tang server details when using network-bound disk encryption.
type DiskEncryption struct {
	// type specifies the disk encryption mode. The default "none" disables
	// encryption. Use "tpmv2" for TPM 2.0 or "tang" for network-bound disk
	// encryption via Tang servers (requires the tang field).
	// +kubebuilder:default:=none
	// +optional
	Type string `json:"type,omitempty"`
	// tang is a list of Tang server configurations used for network-bound
	// disk encryption (NBDE). Each entry specifies a Tang server URL and
	// its thumbprint.
	// +optional
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

// NodeSpec defines the desired configuration for a single node (host) in
// a ClusterInstance, including bare-metal host details, network settings,
// and node-level template overrides.
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
	// This MAC address should correspond to one of the interfaces
	// defined in nodeNetwork so that the host can be provisioned
	// on the correct network.
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
	// If it is not defined here then it is inherited from the ClusterInstanceSpec.
	// +kubebuilder:validation:Enum=x86_64;aarch64
	// +optional
	CPUArchitecture CPUArchitecture `json:"cpuArchitecture,omitempty"`

	// bootMode selects the method of initializing the hardware during boot.
	// Defaults to UEFI.
	// +kubebuilder:default:=UEFI
	// +optional
	BootMode bmh_v1alpha1.BootMode `json:"bootMode,omitempty"`

	// installerArgs is a JSON-formatted string of arguments passed to the
	// coreos-installer via the bare metal agent controller. Rendered as the
	// bmac.agent-install.openshift.io/installer-args annotation on the
	// BareMetalHost resource. Must be valid JSON when provided.
	// +optional
	InstallerArgs string `json:"installerArgs,omitempty"`

	// ignitionConfigOverride is a JSON-formatted string containing
	// host-specific Ignition config overrides. Rendered as the
	// bmac.agent-install.openshift.io/ignition-config-overrides annotation
	// on the BareMetalHost resource. Must be valid JSON when provided.
	// +optional
	IgnitionConfigOverride string `json:"ignitionConfigOverride,omitempty"`

	// role specifies the role of this node in the cluster. Defaults to "master".
	// +kubebuilder:validation:Enum=master;worker;arbiter
	// +kubebuilder:default:=master
	// +optional
	Role string `json:"role,omitempty"`

	// extraAnnotations specifies additional node-level annotations keyed by
	// the Kubernetes resource Kind (e.g., "BareMetalHost", "NMStateConfig").
	// The key is matched against the kind field of each rendered manifest.
	// If a Kind is present in the node-level map, those annotations are
	// used for this node instead of any cluster-level annotations for the
	// same Kind. Kinds not defined at node level fall back to cluster-level
	// extraAnnotations. Annotations already defined in the template take
	// precedence and are not overwritten.
	// +optional
	ExtraAnnotations map[string]map[string]string `json:"extraAnnotations,omitempty"`

	// extraLabels specifies additional node-level labels keyed by the
	// Kubernetes resource Kind (e.g., "BareMetalHost", "NMStateConfig").
	// The key is matched against the kind field of each rendered manifest.
	// If a Kind is present in the node-level map, those labels are used
	// for this node instead of any cluster-level labels for the same Kind.
	// Kinds not defined at node level fall back to cluster-level
	// extraLabels. Labels already defined in the template take precedence
	// and are not overwritten.
	// +optional
	ExtraLabels map[string]map[string]string `json:"extraLabels,omitempty"`

	// suppressedManifests is a list of Kubernetes resource Kinds to exclude
	// from rendering for this node. Combined with cluster-level
	// suppressedManifests. Matching manifests are not applied and are
	// recorded as "suppressed" in status. Previously applied resources
	// are not deleted; use pruneManifests to remove them.
	// +optional
	SuppressedManifests []string `json:"suppressedManifests,omitempty"`

	// pruneManifests is a list of resource references (apiVersion + kind)
	// identifying node-level manifests that should be skipped during
	// rendering and deleted from the cluster if previously applied.
	// Combined with cluster-level pruneManifests.
	// +optional
	PruneManifests []ResourceRef `json:"pruneManifests,omitempty"`

	// ironicInspect controls automatic hardware introspection performed by
	// Ironic during BareMetalHost registration. The default empty string
	// enables inspection. Set to "disabled" to skip hardware inspection.
	// +kubebuilder:default:=""
	// +optional
	IronicInspect IronicInspect `json:"ironicInspect,omitempty"`

	// TemplateRefs is a list of references to node-level templates. A node-level template consists of a ConfigMap
	// in which the keys of the data field represent the kind of the installation manifest(s).
	// Node-level templates are instantiated once for each node in the ClusterInstance CR.
	// +required
	TemplateRefs []TemplateRef `json:"templateRefs"`
}

// ClusterType specifies the topology of the cluster.
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

// ClusterInstanceSpec defines the desired state for a cluster, including
// topology, networking, platform, node roles, and installation settings.
type ClusterInstanceSpec struct {
	// clusterName is the name of the cluster. It is used as the base name
	// for generated resources and their namespace. Must be a valid
	// Kubernetes namespace name: lowercase alphanumeric characters or
	// hyphens, must start and end with an alphanumeric character, and at
	// most 63 characters long.
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
	// Not applicable to Single Node OpenShift (SNO) clusters.
	// +kubebuilder:validation:MaxItems=2
	// +optional
	ApiVIPs []string `json:"apiVIPs,omitempty"`

	// IngressVIPs are the virtual IPs used for cluster ingress traffic.
	// Enter one IP address for single-stack clusters, or up to two for dual-stack clusters (at
	// most one IP address per IP stack used). The order of stacks should be the same as order
	// of subnets in Cluster Networks, Service Networks, and Machine Networks.
	// Not applicable to Single Node OpenShift (SNO) clusters.
	// +kubebuilder:validation:MaxItems=2
	// +optional
	IngressVIPs []string `json:"ingressVIPs,omitempty"`

	// holdInstallation prevents the Assisted Installer from starting the
	// installation when set to true. Inspection and validation proceed
	// normally, but installation is held until this field is set to false.
	// Can only be enabled at creation time. Defaults to false.
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

	// networkType specifies the Container Network Interface (CNI) plugin
	// to install. Defaults to OVNKubernetes.
	// +kubebuilder:validation:Enum=OpenShiftSDN;OVNKubernetes
	// +kubebuilder:default:=OVNKubernetes
	// +optional
	NetworkType string `json:"networkType,omitempty"`

	// platformType specifies the infrastructure platform for installation.
	// Only applicable to the Assisted Installer flow. When empty, the field
	// is omitted from the rendered AgentClusterInstall, allowing the installer
	// to select a default.
	// +optional
	PlatformType PlatformType `json:"platformType,omitempty"`

	// extraAnnotations specifies additional cluster-level annotations keyed by
	// resource Kind (e.g., "ManagedCluster": {"key": "value"}). For each Kind,
	// the annotations are merged into the matching rendered manifest's metadata.
	// Annotations already defined in the template take precedence and are not
	// overwritten by extraAnnotations.
	// +optional
	ExtraAnnotations map[string]map[string]string `json:"extraAnnotations,omitempty"`

	// extraLabels specifies additional cluster-level labels keyed by resource
	// Kind (e.g., "ManagedCluster": {"key": "value"}). For each Kind, the
	// labels are merged into the matching rendered manifest's metadata.
	// Labels already defined in the template take precedence and are not
	// overwritten by extraLabels.
	// +optional
	ExtraLabels map[string]map[string]string `json:"extraLabels,omitempty"`

	// installConfigOverrides is a JSON-formatted string of install-config
	// parameters. The controller automatically merges networkType and
	// cpuPartitioningMode into this value. Rendered as the
	// agent-install.openshift.io/install-config-overrides annotation on
	// AgentClusterInstall. Applies to the Assisted Installer flow only.
	// +optional
	InstallConfigOverrides string `json:"installConfigOverrides,omitempty"`

	// ignitionConfigOverride is a JSON-formatted string containing overrides
	// for the discovery-phase Ignition config. Rendered into
	// InfraEnv.spec.ignitionConfigOverride. Applies to Assisted Installer
	// and Hosted Control Plane flows.
	// +optional
	IgnitionConfigOverride string `json:"ignitionConfigOverride,omitempty"`

	// diskEncryption configures disk encryption for cluster nodes. Use the
	// type sub-field to select the encryption mode and tang to provide
	// Tang server details when using network-bound disk encryption.
	// These fields are not consumed by the default provided templates
	// and require custom templates (via templateRefs) to take effect.
	// +optional
	DiskEncryption *DiskEncryption `json:"diskEncryption,omitempty"`

	// Proxy defines the proxy settings used for the install config
	// +optional
	Proxy *aiv1beta1.Proxy `json:"proxy,omitempty"`

	// ExtraManifestsRefs is list of config map references containing additional manifests to be applied to the cluster.
	// +optional
	ExtraManifestsRefs []corev1.LocalObjectReference `json:"extraManifestsRefs,omitempty"`

	// suppressedManifests is a list of Kubernetes resource Kinds to exclude
	// from rendering. Manifests whose rendered Kind matches an entry in this
	// list are not applied to the cluster and are recorded as "suppressed" in
	// status. Cluster-level suppression also applies to node-level manifests.
	// Previously applied resources are not deleted; use pruneManifests to
	// remove them.
	// +optional
	SuppressedManifests []string `json:"suppressedManifests,omitempty"`

	// pruneManifests is a list of resource references (apiVersion + kind)
	// identifying manifests that should be skipped during rendering and
	// deleted from the cluster if they were previously applied. Unlike
	// suppressedManifests, pruning actively removes existing resources.
	// Cluster-level prune entries also apply to node-level manifests.
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

	// clusterType specifies the topology of the cluster.
	// One of: SNO (Single Node OpenShift — exactly 1 control-plane node; worker nodes
	// may also be specified and are provisioned independently of the initial installation),
	// HighlyAvailable (multiple control-plane and worker nodes),
	// HostedControlPlane (control plane hosted externally via HyperShift, no control-plane
	// nodes in this spec),
	// HighlyAvailableArbiter (at least 2 control-plane nodes with at least 1 arbiter node
	// for stretched clusters).
	// +kubebuilder:validation:Enum=SNO;HighlyAvailable;HostedControlPlane;HighlyAvailableArbiter
	// +optional
	ClusterType ClusterType `json:"clusterType,omitempty"`

	// TemplateRefs is a list of references to cluster-level templates. A cluster-level template consists of a ConfigMap
	// in which the keys of the data field represent the kind of the installation manifest(s).
	// Cluster-level templates are instantiated once per cluster (ClusterInstance CR).
	// +required
	TemplateRefs []TemplateRef `json:"templateRefs"`

	// caBundleRef is a reference to a ConfigMap containing a bundle of
	// trusted CA certificates. The controller passes this reference to
	// the downstream resource (HostedCluster or ImageClusterInstall). Not
	// applicable to the Assisted Installer flow.
	// +optional
	CaBundleRef *corev1.LocalObjectReference `json:"caBundleRef,omitempty"`

	// nodes is the list of nodes to provision for this cluster. The number
	// and roles of nodes must be compatible with the selected clusterType.
	// +required
	Nodes []NodeSpec `json:"nodes"`

	// reinstall triggers a cluster reinstallation workflow when populated.
	// This is a destructive operation: all rendered resources (except
	// ManagedCluster) are deleted and reprovisioned. Requires the controller
	// to be configured with allowReinstalls enabled. Each reinstall must use
	// a unique generation value.
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

// ClusterInstance defines the desired configuration for provisioning an
// OpenShift cluster. Based on the selected templateRefs and clusterType,
// the controller renders and applies the Kubernetes resources needed for
// cluster installation (e.g., ClusterDeployment, ManagedCluster,
// BareMetalHost, or HostedCluster depending on the installation flow).
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
