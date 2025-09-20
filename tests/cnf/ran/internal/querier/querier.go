package querier

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	prometheusapi "github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/config"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/route"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
)

// FindQuerierAddress returns the address of the Thanos Querier route in the OpenShift Monitoring namespace. Note that
// the returned address does not include the scheme.
//
// Uses the route and namespace from this documentation:
// https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/monitoring/accessing-metrics#viewing
// -a-list-of-available-metrics_accessing-metrics-as-a-developer.
func FindQuerierAddress(client *clients.Settings) (string, error) {
	routeBuilder, err := route.Pull(client, ranparam.ThanosQuerierRouteName, ranparam.OpenshiftMonitoringNamespace)
	if err != nil {
		return "", fmt.Errorf("failed to get thanos-querier route: %w", err)
	}

	if len(routeBuilder.Definition.Status.Ingress) == 0 {
		return "", fmt.Errorf("cannot find address for thanos-querier route: no ingresses found")
	}

	return routeBuilder.Definition.Status.Ingress[0].Host, nil
}

// GetQuerierToken creates a ServiceAccount and ClusterRoleBinding to access the Thanos Querier API, then returns a
// bearer token valid for 24 hours that can be used to access the API.
func GetQuerierToken(client *clients.Settings) (string, error) {
	// If the ServiceAccount already exists, then this is equivalent to pulling it. Likewise with the
	// ClusterRoleBinding, these are really assertions that the ServiceAccount and ClusterRoleBinding exist.
	saBuilder, err := serviceaccount.NewBuilder(
		client, ranparam.QuerierServiceAccountName, ranparam.OpenshiftMonitoringNamespace).Create()
	if err != nil {
		return "", fmt.Errorf("failed to create querier service account: %w", err)
	}

	saSubject := rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      ranparam.QuerierServiceAccountName,
		Namespace: ranparam.OpenshiftMonitoringNamespace,
	}
	_, err = rbac.NewClusterRoleBindingBuilder(
		client, ranparam.QuerierCRBName, ranparam.OpenshiftMonitoringViewRole, saSubject).Create()

	if err != nil {
		return "", fmt.Errorf("failed to create querier cluster role binding: %w", err)
	}

	token, err := saBuilder.CreateToken(24 * time.Hour)
	if err != nil {
		return "", fmt.Errorf("failed to create querier token: %w", err)
	}

	return token, nil
}

// CleanupQuerierResources deletes the querier ServiceAccount and ClusterRoleBinding that were created when getting a
// new token. It is idempotent and will not fail if the resources do not exist.
func CleanupQuerierResources(client *clients.Settings) error {
	crbBuilder, err := rbac.PullClusterRoleBinding(client, ranparam.QuerierCRBName)
	if err == nil {
		err = crbBuilder.Delete()
		if err != nil {
			return fmt.Errorf("failed to delete querier cluster role binding: %w", err)
		}
	}

	saBuilder, err := serviceaccount.Pull(
		client, ranparam.QuerierServiceAccountName, ranparam.OpenshiftMonitoringNamespace)
	if err == nil {
		err = saBuilder.Delete()
		if err != nil {
			return fmt.Errorf("failed to delete querier service account: %w", err)
		}
	}

	return nil
}

// CreatePrometheusAPI creates a new Prometheus API client using the given address and token. The address will use
// scheme https if it is not specified. The provided token is used as a bearer token and TLS verification is disabled.
func CreatePrometheusAPI(address, token string) (prometheusv1.API, error) {
	if !strings.HasPrefix(address, "http") {
		address = "https://" + address
	}

	client, err := prometheusapi.NewClient(prometheusapi.Config{
		Address: address,
		RoundTripper: config.NewAuthorizationCredentialsRoundTripper(
			"Bearer",
			config.NewInlineSecret(token),
			&http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
		),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus client: %w", err)
	}

	return prometheusv1.NewAPI(client), nil
}

// CreatePrometheusAPIForCluster creates a new Prometheus API client for the cluster using the Thanos Querier address
// and token. It first finds the address of the Thanos Querier route, creates a ServiceAccount and ClusterRoleBinding to
// access the API, and then creates the Prometheus API client using the address and a token generated for the
// ServiceAccount.
func CreatePrometheusAPIForCluster(client *clients.Settings) (prometheusv1.API, error) {
	address, err := FindQuerierAddress(client)
	if err != nil {
		return nil, fmt.Errorf("failed to find querier address: %w", err)
	}

	token, err := GetQuerierToken(client)
	if err != nil {
		return nil, fmt.Errorf("failed to get querier token: %w", err)
	}

	return CreatePrometheusAPI(address, token)
}
