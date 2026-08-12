package v1alpha3

import (
	hiveext "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/hiveextension/v1beta1"
	aiv1beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const InstallConfigOverrideAnnotation = "controlplane.cluster.x-k8s.io/install-config-override"

// ObjectMeta is a subset of the Cluster API ObjectMeta type with labels and annotations.
type ObjectMeta struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ContractVersionedObjectReference is a reference to another object with kind, name, and apiGroup.
type ContractVersionedObjectReference struct {
	Kind     string `json:"kind,omitempty"`
	Name     string `json:"name,omitempty"`
	APIGroup string `json:"apiGroup,omitempty"`
}

// MachineDeletionSpec defines timeouts for machine deletion operations.
type MachineDeletionSpec struct {
	NodeDrainTimeoutSeconds        *int32 `json:"nodeDrainTimeoutSeconds,omitempty"`
	NodeVolumeDetachTimeoutSeconds *int32 `json:"nodeVolumeDetachTimeoutSeconds,omitempty"`
	NodeDeletionTimeoutSeconds     *int32 `json:"nodeDeletionTimeoutSeconds,omitempty"`
}

// NodeRegistrationOptions holds fields related to registering nodes to the cluster.
type NodeRegistrationOptions struct {
	Name               string   `json:"name,omitempty"`
	KubeletExtraLabels []string `json:"kubeletExtraLabels,omitempty"`
	ProviderID         string   `json:"providerID,omitempty"`
}

// OpenshiftAssistedConfigSpec defines the desired state of OpenshiftAssistedConfig.
type OpenshiftAssistedConfigSpec struct {
	Proxy                       *aiv1beta1.Proxy            `json:"proxy,omitempty"`
	PullSecretRef               *corev1.LocalObjectReference `json:"pullSecretRef,omitempty"`
	AdditionalNTPSources        []string                     `json:"additionalNTPSources,omitempty"`
	SSHAuthorizedKey            string                       `json:"sshAuthorizedKey,omitempty"`
	NMStateConfigLabelSelector  metav1.LabelSelector         `json:"nmStateConfigLabelSelector,omitempty"`
	CpuArchitecture             string                       `json:"cpuArchitecture,omitempty"`
	KernelArguments             []aiv1beta1.KernelArgument   `json:"kernelArguments,omitempty"`
	AdditionalTrustBundle       string                       `json:"additionalTrustBundle,omitempty"`
	OSImageVersion              string                       `json:"osImageVersion,omitempty"`
	PreBootstrapCommands        []string                     `json:"preBootstrapCommands,omitempty"`
	PostBootstrapCommands       []string                     `json:"postBootstrapCommands,omitempty"`
	BootstrapCommandSentinelDir string                       `json:"bootstrapCommandSentinelDir,omitempty"`
	PostBootstrapKubeconfigPath string                       `json:"postBootstrapKubeconfigPath,omitempty"`
	NodeRegistration            NodeRegistrationOptions      `json:"nodeRegistration,omitempty"`
}

type OpenshiftAssistedControlPlaneMachineTemplate struct {
	ObjectMeta        ObjectMeta                       `json:"metadata,omitempty"`
	InfrastructureRef ContractVersionedObjectReference `json:"infrastructureRef"`
	Deletion          MachineDeletionSpec              `json:"deletion,omitempty"`
}

// OpenshiftAssistedControlPlaneSpec defines the desired state of OpenshiftAssistedControlPlane.
type OpenshiftAssistedControlPlaneSpec struct {
	Config                      OpenshiftAssistedControlPlaneConfigSpec      `json:"config,omitempty"`
	MachineTemplate             OpenshiftAssistedControlPlaneMachineTemplate `json:"machineTemplate"`
	OpenshiftAssistedConfigSpec OpenshiftAssistedConfigSpec                  `json:"openshiftAssistedConfigSpec,omitempty"`
	Replicas                    int32                                        `json:"replicas,omitempty"`
	DistributionVersion         string                                       `json:"distributionVersion"`
}

// OpenshiftAssistedControlPlaneConfigSpec defines configuration for the agent-provisioned cluster.
type OpenshiftAssistedControlPlaneConfigSpec struct {
	APIVIPs                []string                              `json:"apiVIPs,omitempty"`
	IngressVIPs            []string                              `json:"ingressVIPs,omitempty"`
	ManifestsConfigMapRefs []hiveext.ManifestsConfigMapReference `json:"manifestsConfigMapRefs,omitempty"`
	DiskEncryption         *hiveext.DiskEncryption               `json:"diskEncryption,omitempty"`
	Proxy                  *hiveext.Proxy                        `json:"proxy,omitempty"`
	MastersSchedulable     bool                                  `json:"mastersSchedulable,omitempty"`
	NetworkType            string                                `json:"networkType,omitempty"`
	SSHAuthorizedKey       string                                `json:"sshAuthorizedKey,omitempty"`
	ClusterName            string                                `json:"clusterName"`
	BaseDomain             string                                `json:"baseDomain"`
	PullSecretRef          *corev1.LocalObjectReference          `json:"pullSecretRef,omitempty"`
	ImageRegistryRef       *corev1.LocalObjectReference          `json:"imageRegistryRef,omitempty"`
	Capabilities           Capabilities                          `json:"capabilities,omitempty"`
}

type Capabilities struct {
	BaselineCapability            string   `json:"baselineCapability,omitempty"`
	AdditionalEnabledCapabilities []string `json:"additionalEnabledCapabilities,omitempty"`
}

type OpenshiftAssistedControlPlaneInitializationStatus struct {
	ControlPlaneInitialized *bool `json:"controlPlaneInitialized,omitempty"`
}

type OpenshiftAssistedControlPlaneStatus struct {
	Conditions          []metav1.Condition                                `json:"conditions,omitempty"`
	Initialization      OpenshiftAssistedControlPlaneInitializationStatus `json:"initialization,omitempty,omitzero"`
	Selector            string                                            `json:"selector,omitempty"`
	Replicas            *int32                                            `json:"replicas,omitempty"`
	ReadyReplicas       *int32                                            `json:"readyReplicas,omitempty"`
	AvailableReplicas   *int32                                            `json:"availableReplicas,omitempty"`
	UpToDateReplicas    *int32                                            `json:"upToDateReplicas,omitempty"`
	Version             string                                            `json:"version,omitempty"`
	DistributionVersion string                                            `json:"distributionVersion,omitempty"`
	ObservedGeneration  int64                                             `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=oacp;oacps

// OpenshiftAssistedControlPlane is the Schema for the openshiftassistedcontrolplane API.
type OpenshiftAssistedControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenshiftAssistedControlPlaneSpec   `json:"spec,omitempty"`
	Status OpenshiftAssistedControlPlaneStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenshiftAssistedControlPlaneList contains a list of OpenshiftAssistedControlPlane.
type OpenshiftAssistedControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenshiftAssistedControlPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenshiftAssistedControlPlane{}, &OpenshiftAssistedControlPlaneList{})
}
