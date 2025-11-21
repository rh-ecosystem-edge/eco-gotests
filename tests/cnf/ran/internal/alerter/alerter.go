// Package alerter provides functions for accessing the ACM Observability Alertmanager instance.
package alerter

import (
	"crypto/x509"
	"fmt"

	openapiruntime "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	alertmanagerv2 "github.com/prometheus/alertmanager/api/v2/client"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/route"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
)

// FindAlertmanagerAddress finds the address of the ACM Observability Alertmanager instance using the route on the
// provided cluster.
func FindAlertmanagerAddress(client *clients.Settings) (string, error) {
	routeBuilder, err := route.Pull(client, ranparam.ACMObservabilityAMRouteName, ranparam.ACMObservabilityNamespace)
	if err != nil {
		return "", err
	}

	if len(routeBuilder.Definition.Status.Ingress) == 0 {
		return "", fmt.Errorf("cannot find address for alertmanager route: no ingresses found")
	}

	return routeBuilder.Definition.Status.Ingress[0].Host, nil
}

// GetAlertmanagerToken gets the token from the ACM Observability Alertmanager secret. It fails fast on a missing token.
func GetAlertmanagerToken(client *clients.Settings) (string, error) {
	secretBuilder, err := secret.Pull(client, ranparam.ACMObservabilityAMSecretName, ranparam.ACMObservabilityNamespace)
	if err != nil {
		return "", fmt.Errorf("failed to pull alertmanager secret: %w", err)
	}

	token := secretBuilder.Definition.Data["token"]

	if len(token) == 0 {
		return "", fmt.Errorf("token is empty")
	}

	return string(token), nil
}

// CreateAlertmanagerClient creates a new AlertmanagerAPI client for the given address and token. The address will use
// scheme https if it is not specified. The provided token is used as a bearer token.
func CreateAlertmanagerClient(address, token string, caPool *x509.CertPool) (*alertmanagerv2.AlertmanagerAPI, error) {
	runtime := openapiruntime.New(address, alertmanagerv2.DefaultBasePath, []string{"https", "http"})

	transport, err := openapiruntime.TLSTransport(openapiruntime.TLSClientOptions{LoadedCAPool: caPool})
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS client: %w", err)
	}

	runtime.Transport = transport

	authInfoWriter := openapiruntime.BearerToken(token)
	runtime.DefaultAuthentication = authInfoWriter

	apiClient := alertmanagerv2.New(runtime, strfmt.Default)

	return apiClient, nil
}

// CreateAlerterClientForCluster creates a new Alertmanager API client for the cluster using the Alertmanager address
// and token. It first finds the address of the Alertmanager route then attempts to get the token and CA pool from the
// secret to build the Alertmanager API client. No cleanup is necessary by callers.
func CreateAlerterClientForCluster(client *clients.Settings) (*alertmanagerv2.AlertmanagerAPI, error) {
	address, err := FindAlertmanagerAddress(client)
	if err != nil {
		return nil, fmt.Errorf("failed to find alertmanager address: %w", err)
	}

	token, err := GetAlertmanagerToken(client)
	if err != nil {
		return nil, fmt.Errorf("failed to get alerter token: %w", err)
	}

	caPool, err := rancluster.GetClusterDefaultRouterCAPool(client)
	if err != nil {
		return nil, fmt.Errorf("failed to get default router CA pool: %w", err)
	}

	return CreateAlertmanagerClient(address, token, caPool)
}
