package certmanager

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/route"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
)

// BuildCertRenewalPrometheusRule constructs a PrometheusRule CR for cert-manager renewal alerts
// with configurable thresholds for info, warning, and critical severity levels.
func BuildCertRenewalPrometheusRule(namespace, certName string,
	infoThreshold, warningThreshold, criticalThreshold int) *unstructured.Unstructured {
	klog.V(100).Infof("Building PrometheusRule for cert %s with thresholds info=%d, warning=%d, critical=%d",
		certName, infoThreshold, warningThreshold, criticalThreshold)

	metricSelector := fmt.Sprintf(`certmanager_certificate_renewal_timestamp_seconds{name="%s"}`, certName)

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "monitoring.coreos.com/v1",
			"kind":       "PrometheusRule",
			"metadata": map[string]interface{}{
				"name":      "cert-renewal-alert-test",
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"groups": []interface{}{
					map[string]interface{}{
						"name": "cert-manager-alert-test",
						"rules": []interface{}{
							map[string]interface{}{
								"alert": AlertNameInfo,
								"expr":  fmt.Sprintf(`(%s - time()) < %d`, metricSelector, infoThreshold),
								"for":   "0m",
								"labels": map[string]interface{}{
									"severity": "info",
								},
								"annotations": map[string]interface{}{
									"description": fmt.Sprintf(
										"Certificate %s will renew in less than %d seconds", certName, infoThreshold),
								},
							},
							map[string]interface{}{
								"alert": AlertNameWarning,
								"expr":  fmt.Sprintf(`(%s - time()) < %d`, metricSelector, warningThreshold),
								"for":   "0m",
								"labels": map[string]interface{}{
									"severity": "warning",
								},
								"annotations": map[string]interface{}{
									"description": fmt.Sprintf(
										"Certificate %s will renew in less than %d seconds", certName, warningThreshold),
								},
							},
							map[string]interface{}{
								"alert": AlertNameCritical,
								"expr":  fmt.Sprintf(`(%s - time()) < %d`, metricSelector, criticalThreshold),
								"for":   "0m",
								"labels": map[string]interface{}{
									"severity": "critical",
								},
								"annotations": map[string]interface{}{
									"description": fmt.Sprintf(
										"Certificate %s will renew in less than %d seconds", certName, criticalThreshold),
								},
							},
						},
					},
				},
			},
		},
	}
}

// QueryPrometheusAlertState queries the state of a specific alert rule.
func QueryPrometheusAlertState(promAPI promv1.API, alertName string) (string, error) {
	klog.V(100).Infof("Querying Prometheus alert state for %s", alertName)

	result, err := promAPI.Rules(context.TODO())
	if err != nil {
		return "", fmt.Errorf("failed to query Prometheus rules: %w", err)
	}

	for _, group := range result.Groups {
		for _, rule := range group.Rules {
			alertRule, ok := rule.(promv1.AlertingRule)
			if !ok {
				continue
			}

			if alertRule.Name == alertName {
				klog.V(100).Infof("Alert %s state: %s", alertName, alertRule.State)

				return alertRule.State, nil
			}
		}
	}

	klog.V(100).Infof("Alert %s not found in Prometheus rules, returning inactive", alertName)

	return AlertStateInactive, nil
}

// QueryPrometheusRenewalMetric queries Prometheus for the certmanager_certificate_renewal_timestamp_seconds metric
// and returns the number of seconds remaining until renewal (renewal_timestamp - current_time).
func QueryPrometheusRenewalMetric(promAPI promv1.API, certName string) (float64, error) {
	klog.V(100).Infof("Querying Prometheus renewal metric for certificate %s", certName)

	query := fmt.Sprintf(`certmanager_certificate_renewal_timestamp_seconds{name="%s"} - time()`, certName)

	result, _, err := promAPI.Query(context.TODO(), query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to query Prometheus renewal metric: %w", err)
	}

	vector, ok := result.(model.Vector)
	if !ok || len(vector) == 0 {
		return 0, fmt.Errorf("no renewal metric data found for certificate %s", certName)
	}

	remaining := float64(vector[0].Value)

	klog.V(100).Infof("Certificate %s has %.0f seconds remaining until renewal", certName, remaining)

	return remaining, nil
}

// NewPrometheusAPI returns a Prometheus v1 API interface, creating necessary authentication
// resources (ServiceAccount and ClusterRoleBinding) if they do not exist. It connects via
// the Thanos Querier route and falls back to dialing via the API server hostname when the
// route hostname is not DNS-resolvable.
func NewPrometheusAPI(apiClient *clients.Settings,
	saName, crbName, namespace string) (promv1.API, error) {
	klog.V(100).Infof("Creating Prometheus API client via thanos-querier route in namespace %s", namespace)

	thanosRoute, err := route.Pull(apiClient, "thanos-querier", namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to pull thanos-querier route: %w", err)
	}

	if !thanosRoute.Exists() {
		return nil, fmt.Errorf("thanos-querier route not found")
	}

	if len(thanosRoute.Object.Status.Ingress) == 0 {
		return nil, fmt.Errorf("thanos-querier route has no ingress")
	}

	address := thanosRoute.Object.Status.Ingress[0].Host

	klog.V(100).Infof("Thanos Querier route address: %s", address)

	token, err := ensurePrometheusAuth(apiClient, saName, crbName, namespace)
	if err != nil {
		return nil, err
	}

	caPool, err := GetClusterDefaultRouterCAPool(apiClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get router CA pool: %w", err)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: caPool,
		},
	}

	_, dnsErr := net.LookupHost(address)
	if dnsErr != nil {
		dialHost, parseErr := ExtractAPIServerHostname(apiClient)
		if parseErr != nil {
			return nil, fmt.Errorf(
				"route hostname %s is not DNS-resolvable and failed to extract API server hostname as fallback: %w",
				address, parseErr)
		}

		klog.V(100).Infof("Route hostname %s not resolvable, using API server hostname %s as dial target",
			address, dialHost)

		transport.TLSClientConfig.ServerName = address
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, _ := net.SplitHostPort(addr)
			if port == "" {
				port = "443"
			}

			return (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, network, net.JoinHostPort(dialHost, port))
		}
	}

	client, err := promapi.NewClient(promapi.Config{
		Address: "https://" + address,
		RoundTripper: config.NewAuthorizationCredentialsRoundTripper(
			"Bearer",
			config.NewInlineSecret(token),
			transport,
		),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Prometheus client: %w", err)
	}

	klog.V(100).Infof("Successfully created Prometheus API client for %s", address)

	return promv1.NewAPI(client), nil
}

// ExtractAPIServerHostname returns the hostname (or IP) from the KUBECONFIG API server URL.
func ExtractAPIServerHostname(apiClient *clients.Settings) (string, error) {
	apiURL, err := url.Parse(apiClient.Config.Host)
	if err != nil {
		return "", fmt.Errorf("failed to parse API server URL %q: %w", apiClient.Config.Host, err)
	}

	hostname := apiURL.Hostname()
	if hostname == "" {
		return "", fmt.Errorf("api server URL %q has no hostname", apiClient.Config.Host)
	}

	return hostname, nil
}

// GetClusterDefaultRouterCAPool retrieves the default router CA pool.
func GetClusterDefaultRouterCAPool(apiClient *clients.Settings) (*x509.CertPool, error) {
	klog.V(100).Infof("Retrieving default router CA pool from openshift-ingress/router-certs-default")

	routerCASecret, err := secret.Pull(apiClient, "router-certs-default", "openshift-ingress")
	if err != nil {
		return nil, fmt.Errorf("failed to pull router-certs-default secret: %w", err)
	}

	if routerCASecret == nil || !routerCASecret.Exists() {
		return nil, fmt.Errorf("router-certs-default secret not found")
	}

	caPEM := routerCASecret.Object.Data["tls.crt"]
	if len(caPEM) == 0 {
		return nil, fmt.Errorf("tls.crt not found in router-certs-default secret")
	}

	caPool, err := x509.SystemCertPool()
	if err != nil {
		caPool = x509.NewCertPool()
	}

	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("failed to append router CA to cert pool")
	}

	return caPool, nil
}

func ensurePrometheusAuth(apiClient *clients.Settings,
	saName, crbName, namespace string) (string, error) {
	klog.V(100).Infof("Ensuring Prometheus authentication resources exist (SA: %s, CRB: %s)", saName, crbName)

	saBuilder := serviceaccount.NewBuilder(apiClient, saName, namespace)

	if !saBuilder.Exists() {
		klog.V(100).Infof("Creating ServiceAccount %s/%s", namespace, saName)

		_, err := saBuilder.Create()
		if err != nil {
			return "", fmt.Errorf("failed to create ServiceAccount: %w", err)
		}
	}

	crbBuilder := rbac.NewClusterRoleBindingBuilder(
		apiClient,
		crbName,
		"cluster-monitoring-view",
		rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      saName,
			Namespace: namespace,
		},
	)

	if !crbBuilder.Exists() {
		klog.V(100).Infof("Creating ClusterRoleBinding %s", crbName)

		_, err := crbBuilder.Create()
		if err != nil {
			return "", fmt.Errorf("failed to create ClusterRoleBinding: %w", err)
		}
	}

	klog.V(100).Infof("Creating token for ServiceAccount %s/%s", namespace, saName)

	token, err := saBuilder.CreateToken(24 * time.Hour)
	if err != nil {
		return "", fmt.Errorf("failed to create token: %w", err)
	}

	return token, nil
}
