/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-kni/oran-o2ims/api/common"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OAuthConfig defines the configurable attributes that represent the authentication mechanism.  This is currently
// expected to be a way to acquire a token from an OAuth2 server.
type OAuthConfig struct {
	// URL represents the base URL of the authorization server. (e.g., https://keycloak.example.com/realms/oran)
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="OAuth URL",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	URL string `json:"url"`
	// TokenEndpoint represents the API endpoint used to acquire a token (e.g., /protocol/openid-connect/token) which
	// will be appended to the base URL to form the full URL
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="OAuth Token Endpoint",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	TokenEndpoint string `json:"tokenEndpoint"`
	// ClientSecretName represents the name of a secret (in the current namespace) which contains the client-id and
	// client-secret values used by the OAuth client.
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Client Secret",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	ClientSecretName string `json:"clientSecretName"`
	// Scopes represents the OAuth scope values to request when acquiring a token.  Typically, this should be set to
	// "openid" in addition to any other scopes that the SMO specifically requires (e.g., "roles", "groups", etc...) to
	// authorize our requests
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="OAuth Scopes",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	Scopes []string `json:"scopes"`
	// UsernameClaim represents the claim contained within the OAuth JWT token which holds the username
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="OAuth Username Claim",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	//+kubebuilder:default=preferred_username
	UsernameClaim string `json:"usernameClaim"`
	// GroupsClaim represents the claim contained within the OAuth JWT token which holds the list of groups/roles. This
	// must be a list/array and not a space separated list of names.  It must also be a top level attribute rather than
	// a nested field in the JSON structure of the JWT object.
	//    i.e., {"roles": ["a", "b"]} rather than {"realm": {"roles": ["a", "b"}}.
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="OAuth Groups Claim",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	//+kubebuilder:default=roles
	GroupsClaim string `json:"groupsClaim"`
	// ClientBindingClaim represents the claim contained within the OAuth JWT token which holds the certificate SHA256
	// fingerprint.  This is expected to be a CEL mapper expression.  It should only be changed in advanced scenarios.
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="OAuth Client Binding Claim",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	//+kubebuilder:default="has(claims.cnf) ? claims.cnf['x5t#S256'] : []"
	ClientBindingClaim string `json:"clientBindingClaim"`
}

// SmoConfig defines the configurable attributes to represent the SMO instance
type SmoConfig struct {
	// URL represents the base URL of the SMO instance
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="SMO URL",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	URL string `json:"url"`
	// RegistrationEndpoint represents the API endpoint used to register the O-Cloud Manager with the SMO.
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Registration API Endpoint",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	RegistrationEndpoint string `json:"registrationEndpoint"`
	// OAuthConfig defines the configurable attributes required to access the OAuth2 authorization server
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="SMO OAuth Configuration"
	OAuthConfig *OAuthConfig `json:"oauth,omitempty"`
	// TLSConfig defines the TLS attributes specific to enabling mTLS communication to the SMO and OAuth servers.  If
	// a configuration is provided, then an mTLS connection will be established to the destination; otherwise, a regular
	// TLS connection will be used.
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Client TLS Configuration"
	TLS *common.TLSConfig `json:"tls"`
}

type ServerConfig struct {
}

// ResourceServerConfig contains the configuration for the resource server.
type ResourceServerConfig struct {
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Server Configuration",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:bool"}
	ServerConfig `json:",inline"`
}

// ClusterServerConfig contains the configuration for the cluster server.
type ClusterServerConfig struct {
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Server Configuration",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:bool"}
	ServerConfig `json:",inline"`
}

// AlarmServerConfig contains the configuration for the alarm server.
type AlarmServerConfig struct {
	ServerConfig `json:",inline"`
}

// ArtifactsServerConfig contains the configuration for the artifacts server.
type ArtifactsServerConfig struct {
	ServerConfig `json:",inline"`
}

// ProvisioningServerConfig contains the configuration for the provisioning server.
type ProvisioningServerConfig struct {
	ServerConfig `json:",inline"`
}

// IngressConfig contains the configuration for the Ingress instance.
type IngressConfig struct {
	// IngressHost defines the FQDN for the IMS endpoints.  By default, it is assumed to be "o2ims.apps.<cluster domain name>".
	// If a different DNS domain is used, then it should be customized here.
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Ingress Host",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	IngressHost *string `json:"ingressHost,omitempty"`
	// TLS defines the TLS configuration for the IMS endpoints.  The certificate CN and DNS SAN values must match exactly
	// the value provided by the `IngressHost` value.  If the `IngressHost` value is not provided, then the CN and SAN
	// must match the expected default value.  If the TLS configuration is not provided, then the TLS configuration of
	// the default IngressController will be used.
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="TLS Configuration",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	TLS *common.TLSConfig `json:"tls,omitempty"`
}

// InventorySpec defines the desired state of Inventory
type InventorySpec struct {
	// Image is the full reference of the container image that contains the binary. This is
	// optional and the default will be the value passed to the `--image` command line flag of
	// the controller manager.
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Image",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	Image *string `json:"image,omitempty"`
	// CloudID is the global cloud ID value used to correlate the SMO inventory record with the deployed cloud instance.
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Cloud ID",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	CloudID *string `json:"cloudID"`
	// ResourceServerConfig contains the configuration for the resource server.
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Resource Server Configuration",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	ResourceServerConfig *ResourceServerConfig `json:"resourceServerConfig,omitempty"`
	// ClusterServerConfig contains the configuration for the resource server.
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Cluster Server Configuration",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	ClusterServerConfig *ClusterServerConfig `json:"clusterServerConfig,omitempty"`
	// AlarmServerConfig contains the configuration for the alarm server.
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Alarm Server Configuration",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	AlarmServerConfig *AlarmServerConfig `json:"alarmServerConfig"`
	// ArtifactsServerConfig contains the configuration for the artifacts server.
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Artifacts Server Configuration",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	ArtifactsServerConfig *ArtifactsServerConfig `json:"artifactsServerConfig"`
	// ProvisioningServerConfig contains the configuration for the provisioning server.
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Provisioning Server Configuration",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	ProvisioningServerConfig *ProvisioningServerConfig `json:"provisioningServerConfig"`
	// IngressConfig defines configuration attributes related to the Ingress endpoint.
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Ingress Configuration",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	IngressConfig *IngressConfig `json:"ingress,omitempty"`
	// SmoConfig defines the configurable attributes to represent the SMO instance
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="SMO Configuration"
	SmoConfig *SmoConfig `json:"smo,omitempty"`
	// CaBundleName references a config map that contains a set of custom CA certificates to be used when communicating
	// with any outside entity (e.g., the SMO, the authorization server, etc.) that has its TLS certificate signed by
	// a non-public CA certificate.  The config map is expected to contain a single file called 'ca-bundle.crt'
	// containing all trusted CA certificates in PEM format.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Custom CA Certificates",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	CaBundleName *string `json:"caBundleName,omitempty"`
}

type UsedServerConfig struct {
	ArtifactsServerUsedConfig    []string `json:"artifactsServerUsedConfig,omitempty"`
	AlarmsServerUsedConfig       []string `json:"alarmsServerUsedConfig,omitempty"`
	ClusterServerUsedConfig      []string `json:"clusterServerUsedConfig,omitempty"`
	ResourceServerUsedConfig     []string `json:"resourceServerUsedConfig,omitempty"`
	ProvisioningServerUsedConfig []string `json:"provisioningServerUsedConfig,omitempty"`
}

// InventoryStatus defines the observed state of Inventory
type InventoryStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Conditions"
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Deployed Server Configurations"
	UsedServerConfig UsedServerConfig `json:"usedServerConfig,omitempty"`
	// Stores the ingress host domain resolved at runtime; either from a user override or automatically computed from
	// the default ingress controller.
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Resolved Ingress Host Address"
	IngressHost string `json:"ingressHost,omitempty"`
	// Stores the local cluster ID used as the local Cloud ID value.
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Local Cluster ID"
	ClusterID string `json:"clusterID,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// Inventory is the Schema for the Inventory API
// +operator-sdk:csv:customresourcedefinitions:displayName="O-Cloud Manager Inventory",resources={{Deployment,apps/v1}}
type Inventory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InventorySpec   `json:"spec,omitempty"`
	Status InventoryStatus `json:"status,omitempty"`
}

// InventoryList contains a list of Inventory
//
// +kubebuilder:object:root=true
type InventoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Inventory `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Inventory{}, &InventoryList{})
}
