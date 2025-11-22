// Package auth provides functionality for authenticating with the O2IMS API. It assumes that there is a Keycloak
// instance running in the cluster and that the O2IMS API is accessible via the O2IMS base URL.
package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"k8s.io/klog/v2"
)

// oAuthScopes are the OAuth scopes required for the O2IMS API. These are specific to the Keycloak configuration; what
// is required is that the access token has the o2ims-admin role and the audience is o2ims-client.
var oAuthScopes = []string{"openid", "roles", "role:o2ims-admin", "o2ims-audience"}

// NewClientBuilderForConfig creates a new ClientBuilder for the O2IMS API using the provided configuration. If the
// OAuth client id and client secret are not provided, the builder will use the bearer token provided. Otherwise, the
// builder will attempt to use mTLS and OAuth for authentication and authorization.
func NewClientBuilderForConfig(config *ranconfig.RANConfig) (*oranapi.ClientBuilder, error) {
	o2imsBaseURL := "https://" + config.GetAppsURL("o2ims")
	oAuthURL := "https://" + config.GetAppsURL("keycloak") + "/realms/oran/protocol/openid-connect/token"

	if config.O2IMSOAuthClientID == "" || config.O2IMSOAuthClientSecret == "" {
		if config.O2IMSToken == "" {
			klog.V(tsparams.LogLevel).Info("No OAuth credentials or token found for O2IMS API")

			return nil, fmt.Errorf("no OAuth credentials or token found for O2IMS API")
		}

		clientBuilder := oranapi.NewClientBuilder(o2imsBaseURL).
			WithToken(config.O2IMSToken).
			WithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true})

		return clientBuilder, nil
	}

	tlsConfig, err := getTLSConfigFromCertificateSecret(
		config.HubAPIClient, config.O2IMSClientCertSecret, config.O2IMSClientCertSecretNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get TLS config from certificate secret: %w", err)
	}

	httpClient := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}}
	oAuthConfig := clientcredentials.Config{
		ClientID:     config.O2IMSOAuthClientID,
		ClientSecret: config.O2IMSOAuthClientSecret,
		TokenURL:     oAuthURL,
		Scopes:       oAuthScopes,
	}

	ctx := context.WithValue(context.TODO(), oauth2.HTTPClient, httpClient)
	httpClient = oAuthConfig.Client(ctx)

	clientBuilder := oranapi.NewClientBuilder(o2imsBaseURL).
		WithHTTPClient(httpClient)

	return clientBuilder, nil
}

func getTLSConfigFromCertificateSecret(
	hubClient *clients.Settings, certSecretName string, certSecretNamespace string) (*tls.Config, error) {
	certSecret, err := secret.Pull(hubClient, certSecretName, certSecretNamespace)
	if err != nil {
		return nil, err
	}

	if len(certSecret.Definition.Data["tls.crt"]) == 0 || len(certSecret.Definition.Data["tls.key"]) == 0 {
		return nil, fmt.Errorf("tls.crt or tls.key not found in certificate secret %q", certSecretName)
	}

	cert, err := tls.X509KeyPair(certSecret.Definition.Data["tls.crt"], certSecret.Definition.Data["tls.key"])
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate from secret %q: %w", certSecretName, err)
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}

	if len(certSecret.Definition.Data["ca.crt"]) > 0 {
		klog.V(tsparams.LogLevel).Infof("Adding CA certificate to certificate pool from secret %q", certSecretName)

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(certSecret.Definition.Data["ca.crt"]) {
			return nil, fmt.Errorf("failed to append CA certificate to certificate pool from secret %q", certSecretName)
		}

		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}
