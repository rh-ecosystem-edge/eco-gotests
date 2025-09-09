// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
//
// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package kacv1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KlusterletAddonConfigSpec defines the desired state of KlusterletAddonConfig
type KlusterletAddonConfigSpec struct {
	// DEPRECATED in release 2.4 and will be removed in the future since not used anymore.
	// +optional
	Version string `json:"version,omitempty"`

	// DEPRECATED in release 2.4 and will be removed in the future since not used anymore.
	// +kubebuilder:validation:MinLength=1
	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// DEPRECATED in release 2.4 and will be removed in the future since not used anymore.
	// +kubebuilder:validation:MinLength=1
	// +optional
	ClusterNamespace string `json:"clusterNamespace,omitempty"`

	// DEPRECATED in release 2.4 and will be removed in the future since not used anymore.
	// +optional
	ClusterLabels map[string]string `json:"clusterLabels,omitempty"`

	// ProxyConfig defines the cluster-wide proxy configuration of the OCP managed cluster.
	// +optional
	ProxyConfig ProxyConfig `json:"proxyConfig,omitempty"`

	// SearchCollectorConfig defines the configurations of SearchCollector addon agent.
	SearchCollectorConfig KlusterletAddonAgentConfigSpec `json:"searchCollector"`

	// PolicyController defines the configurations of PolicyController addon agent.
	PolicyController KlusterletAddonAgentConfigSpec `json:"policyController"`

	// ApplicationManagerConfig defines the configurations of ApplicationManager addon agent.
	ApplicationManagerConfig KlusterletAddonAgentConfigSpec `json:"applicationManager"`

	// CertPolicyControllerConfig defines the configurations of CertPolicyController addon agent.
	CertPolicyControllerConfig KlusterletAddonAgentConfigSpec `json:"certPolicyController"`

	// DEPRECATED in release 2.11 and will be removed in the future since not used anymore.
	IAMPolicyControllerConfig KlusterletAddonAgentConfigSpec `json:"iamPolicyController,omitempty"`
}

// ProxyConfig defines the global proxy env for OCP cluster
type ProxyConfig struct {
	// HTTPProxy is the URL of the proxy for HTTP requests.  Empty means unset and will not result in an env var.
	// +optional
	HTTPProxy string `json:"httpProxy,omitempty"`

	// HTTPSProxy is the URL of the proxy for HTTPS requests.  Empty means unset and will not result in an env var.
	// +optional
	HTTPSProxy string `json:"httpsProxy,omitempty"`

	// NoProxy is a comma-separated list of hostnames and/or CIDRs for which the proxy should not be used.
	// Empty means unset and will not result in an env var.
	// The API Server of Hub cluster should be added here.
	// And If you scale up workers that are not included in the network defined by the networking.machineNetwork[].cidr
	// field from the installation configuration, you must add them to this list to prevent connection issues.
	// +optional
	NoProxy string `json:"noProxy,omitempty"`
}

type ProxyPolicy string

const (
	ProxyPolicyDisable        ProxyPolicy = "Disabled"
	ProxyPolicyOCPGlobalProxy ProxyPolicy = "OCPGlobalProxy"
	ProxyPolicyCustomProxy    ProxyPolicy = "CustomProxy"
)

// KlusterletAddonAgentConfigSpec defines configuration for each addon agent.
type KlusterletAddonAgentConfigSpec struct {
	// Enabled is the flag to enable/disable the addon. default is false.
	// +optional
	Enabled bool `json:"enabled"`

	// ProxyPolicy defines the policy to set proxy for each addon agent. default is Disabled.
	// Disabled means that the addon agent pods do not configure the proxy env variables.
	// OCPGlobalProxy means that the addon agent pods use the cluster-wide proxy config of OCP cluster provisioned by ACM.
	// CustomProxy means that the addon agent pods use the ProxyConfig specified in KlusterletAddonConfig.
	// +kubebuilder:validation:Enum=Disabled;OCPGlobalProxy;CustomProxy
	// +optional
	ProxyPolicy ProxyPolicy `json:"proxyPolicy,omitempty"`
}

const (
	OCPGlobalProxyDetected           string = "OCPGlobalProxyDetected"
	ReasonOCPGlobalProxyDetected     string = "OCPGlobalProxyDetected"
	ReasonOCPGlobalProxyNotDetected  string = "OCPGlobalProxyNotDetected"
	ReasonOCPGlobalProxyDetectedFail string = "OCPGlobalProxyNotDetectedFail"
)

// KlusterletAddonConfigStatus defines the observed state of KlusterletAddonConfig
type KlusterletAddonConfigStatus struct {
	// OCPGlobalProxy is the cluster-wide proxy config of the OCP cluster provisioned by ACM
	// +optional
	OCPGlobalProxy ProxyConfig `json:"ocpGlobalProxy,omitempty"`

	// Conditions contains condition information for the klusterletAddonConfig
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KlusterletAddonConfig is the Schema for the klusterletaddonconfigs API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=klusterletaddonconfigs,scope=Namespaced
type KlusterletAddonConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KlusterletAddonConfigSpec   `json:"spec,omitempty"`
	Status KlusterletAddonConfigStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KlusterletAddonConfigList contains a list of klusterletAddonConfig
type KlusterletAddonConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KlusterletAddonConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KlusterletAddonConfig{}, &KlusterletAddonConfigList{})
}
