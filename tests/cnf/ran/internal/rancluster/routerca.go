package rancluster

import (
	"crypto/x509"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
)

// GetClusterDefaultRouterCAPool gets the CA pool for the default openshift ingress router for the provided cluster. It
// always appends the default router CA to the system CA pool.
func GetClusterDefaultRouterCAPool(client *clients.Settings) (*x509.CertPool, error) {
	secretBuilder, err := secret.Pull(client, ranparam.IngressDefaultRouterCASecret, ranparam.OpenshiftIngressNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to pull default router CA secret: %w", err)
	}

	caPool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("failed to get system CA pool: %w", err)
	}

	if !caPool.AppendCertsFromPEM(secretBuilder.Definition.Data[ranparam.IngressDefaultRouterCAKey]) {
		return nil, fmt.Errorf("failed to append default router CA to pool")
	}

	return caPool, nil
}
