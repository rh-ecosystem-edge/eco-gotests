/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package common

// +genclient

// +kubebuilder:object:generate=true
// TLSConfig defines the configuration for TLS-specific attributes.
type TLSConfig struct {
	// SecretName specifies the name of a secret (in the current namespace) containing an X.509 certificate and
	// private key. The secret must include 'tls.key' and 'tls.crt' keys. If the certificate is signed by
	// intermediate CA(s), the full certificate chain should be included in the certificate file, with the
	// leaf certificate first and the root CA last. The certificate's Common Name (CN) or Subject Alternative
	// Name (SAN) should align with the service's fully qualified domain name to support both ingress and
	// outgoing client certificate use cases.
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="TLS Certificate"
	SecretName *string `json:"secretName"`
}

// +kubebuilder:object:generate=true
// OAuthClientConfig defines the configurable client attributes that represent the authentication mechanism.  This is currently
// expected to be a way to acquire a token from an OAuth2 server.
type OAuthClientConfig struct {
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
}

// +kubebuilder:object:generate=true
// AuthType defines the authorization type used for authentication.
type AuthType string

const (
	// ServiceAccount represents authentication using a Kubernetes ServiceAccount token. This method assumes
	// the server provides a service running within the same Kubernetes cluster and leverages a ServiceAccount
	// to authenticate requests. It is used for internal cluster communication, where the client presents a
	// JWT token issued by the cluster's Kubernetes API server.
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="ServiceAccount Authentication"
	ServiceAccount AuthType = "ServiceAccount"

	// Basic represents authentication using HTTP Basic Authentication. This method uses a username and password
	// combination to authenticate requests, typically encoded in the Authorization header of HTTP requests.
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Basic Authentication"
	Basic AuthType = "Basic"

	// OAuth represents authentication using an OAuth2-based mechanism. This method involves acquiring a token
	// from an OAuth2 server using the client_credentials grant type, which is then used to authenticate
	// requests, typically via a Bearer token in the Authorization header. Only the client_credentials grant
	// type is supported. Configuration details for OAuth, including the authorization server URL and client
	// credentials, are specified in the OAuthConfig struct.
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="OAuth Authentication"
	OAuth AuthType = "OAuth"
)

// +kubebuilder:object:generate=true
// AuthConfig defines the configuration for different authentication types.
// This struct encapsulates the settings required for ServiceAccount, Basic, and OAuth authentication mechanisms.
type AuthClientConfig struct {
	// Type specifies the authentication type to be used (e.g., ServiceAccount, Basic, or OAuth).
	//+kubebuilder:validation:Enum=ServiceAccount;Basic;OAuth
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Authentication Type",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:select:ServiceAccount","urn:alm:descriptor:com.tectonic.ui:select:Basic","urn:alm:descriptor:com.tectonic.ui:select:OAuth"}
	Type AuthType `json:"type"`

	// BasicAuthSecret represents the name of a secret (in the current namespace) containing the username
	// and password for Basic authentication. The secret is expected to contain 'username' and 'password' keys.
	// This field is required when Type is set to "Basic".
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Basic Auth Secret",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	BasicAuthSecret *string `json:"basicAuthSecret,omitempty"`

	// OAuthConfig holds the configuration for OAuth2-based authentication, including the authorization server
	// URL, token endpoint, and client credentials. This field is required when Type is set to "OAuth".
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="OAuth Configuration"
	OAuthClientConfig *OAuthClientConfig `json:"oauthConfig,omitempty"`

	// TLSConfig specifies the TLS configuration for secure communication, including the certificate and private
	// key. This field is optional and can be used with any authentication type to enable TLS for the connection.
	//+optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="TLS Configuration"
	TLSConfig *TLSConfig `json:"tlsConfig,omitempty"`
}
