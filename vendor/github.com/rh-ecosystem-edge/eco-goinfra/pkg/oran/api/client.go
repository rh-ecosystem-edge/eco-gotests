package api

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/internal/alarms"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/internal/artifacts"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/internal/provisioning"
)

// ClientType is a string that represents the type of client being used. It is used to identify the client type in error
// messages and for logging purposes.
type ClientType string

const (
	// ProvisioningClientType is the client type for the provisioning client.
	ProvisioningClientType ClientType = "ProvisioningClient"
	// ArtifactsClientType is the client type for the artifacts client.
	ArtifactsClientType ClientType = "ArtifactsClient"
	// AlarmsClientType is the client type for the alarms client.
	AlarmsClientType ClientType = "AlarmsClient"
)

// ClientBuilder is a builder for creating clients that correspond to different parts of the O-RAN O2IMS API. Unlike
// other builders in this repository, the builder on its own does not allow interaction with the API. Instead, it is
// used only to build other clients.
//
// Error messages are stored in the builder and only returned when the builder is used to create a client. If an error
// message is present, all methods on the builder become no-ops.
type ClientBuilder struct {
	baseURL string
	client  *http.Client
	token   string
	err     error
}

// NewClientBuilder creates a new ClientBuilder with the given base URL. It should include the scheme and will likely be
// a form similar to `https://o2ims.apps.example.com`.
func NewClientBuilder(baseURL string) *ClientBuilder {
	return &ClientBuilder{
		baseURL: baseURL,
	}
}

// WithHTTPClient sets the HTTP client to be used by the clients created by this builder. Note that this will override
// the TLSConfig if one has been set using WithTLSConfig. All clients created by this builder will use the same HTTP
// client. Updating this will not affect clients that have already been created.
func (builder *ClientBuilder) WithHTTPClient(client *http.Client) *ClientBuilder {
	if builder.validate() != nil {
		return builder
	}

	builder.client = client

	return builder
}

// WithBaseURL allows changing the base URL used by the builder, but will not affect any clients that have already been
// created.
func (builder *ClientBuilder) WithBaseURL(baseURL string) *ClientBuilder {
	if builder.validate() != nil {
		return builder
	}

	builder.baseURL = baseURL

	return builder
}

// WithTLSConfig sets the TLS configuration to be used by the clients created by this builder. If an HTTP client has not
// been set using WithHTTPClient, a new zero value HTTP client will be created with the given TLS configuration.
// Otherwise, the existing HTTP client has its transport replaced with a new one that uses the given TLS configuration.
func (builder *ClientBuilder) WithTLSConfig(tlsConfig *tls.Config) *ClientBuilder {
	if builder.validate() != nil {
		return builder
	}

	if builder.client == nil {
		builder.client = &http.Client{}
	}

	builder.client.Transport = &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return builder
}

// WithToken sets the bearer token to be used by the clients created by this builder. It does not affect any clients
// that have already been created.
func (builder *ClientBuilder) WithToken(token string) *ClientBuilder {
	if builder.validate() != nil {
		return builder
	}

	builder.token = token

	return builder
}

// BuildProvisioning creates a new ProvisioningClient using the configuration set on this builder. If the builder has an
// error message, it will be returned here. This method does not modify the builder so the builder can be reused.
func (builder *ClientBuilder) BuildProvisioning() (*ProvisioningClient, error) {
	if err := builder.validate(); err != nil {
		return nil, err
	}

	var opts []provisioning.ClientOption

	if builder.client != nil {
		opts = append(opts, provisioning.WithHTTPClient(builder.client))
	}

	if builder.token != "" {
		opts = append(opts, provisioning.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+builder.token)

			return nil
		}))
	}

	client, err := provisioning.NewClientWithResponses(builder.baseURL, opts...)
	if err != nil {
		return nil, err
	}

	return &ProvisioningClient{client}, nil
}

// BuildArtifacts creates a new ArtifactsClient using the configuration set on this builder. If the builder has an
// error message, it will be returned here. This method does not modify the builder so the builder can be reused.
//
// Unlike the provisioning client, the artifacts client does not serve as a runtimeclient.Client.
func (builder *ClientBuilder) BuildArtifacts() (*ArtifactsClient, error) {
	if err := builder.validate(); err != nil {
		return nil, err
	}

	var opts []artifacts.ClientOption

	if builder.client != nil {
		opts = append(opts, artifacts.WithHTTPClient(builder.client))
	}

	if builder.token != "" {
		opts = append(opts, artifacts.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+builder.token)

			return nil
		}))
	}

	client, err := artifacts.NewClientWithResponses(builder.baseURL, opts...)
	if err != nil {
		return nil, err
	}

	return &ArtifactsClient{client}, nil
}

// BuildAlarms creates a new AlarmsClient using the configuration set on this builder. If the builder has an
// error message, it will be returned here. This method does not modify the builder so the builder can be reused.
//
// Unlike the provisioning client, the alarms client does not serve as a runtimeclient.Client.
func (builder *ClientBuilder) BuildAlarms() (*AlarmsClient, error) {
	if err := builder.validate(); err != nil {
		return nil, err
	}

	var opts []alarms.ClientOption

	if builder.client != nil {
		opts = append(opts, alarms.WithHTTPClient(builder.client))
	}

	if builder.token != "" {
		opts = append(opts, alarms.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+builder.token)

			return nil
		}))
	}

	client, err := alarms.NewClientWithResponses(builder.baseURL, opts...)
	if err != nil {
		return nil, err
	}

	return &AlarmsClient{client}, nil
}

// validate checks if the builder is valid and returns an error if not. A valid builder is defined as being non-nil,
// having a nil error, and having a non-empty base URL.
func (builder *ClientBuilder) validate() error {
	if builder == nil {
		// need better error handling
		return fmt.Errorf("error: received nil ClientBuilder")
	}

	if builder.err != nil {
		return builder.err
	}

	if builder.baseURL == "" {
		return fmt.Errorf("error: missing base URL")
	}

	return nil
}
