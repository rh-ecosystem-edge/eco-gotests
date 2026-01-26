package neuronmetrics

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
)

// PrometheusQueryResult represents the result from a Prometheus query.
type PrometheusQueryResult struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// ServiceMonitorExists checks if a ServiceMonitor exists.
// Returns (true, nil) if exists, (false, nil) if not found, (false, err) for other errors.
func ServiceMonitorExists(apiClient *clients.Settings, name, namespace string) (bool, error) {
	klog.V(params.NeuronLogLevel).Infof("Checking if ServiceMonitor %s exists in namespace %s", name, namespace)

	_, err := apiClient.Resource(neuronparams.ServiceMonitorGVR).
		Namespace(namespace).
		Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.V(params.NeuronLogLevel).Infof("ServiceMonitor %s not found in namespace %s", name, namespace)

			return false, nil
		}

		return false, fmt.Errorf("failed to get ServiceMonitor %s in namespace %s: %w", name, namespace, err)
	}

	return true, nil
}

// GetServiceMonitor retrieves a ServiceMonitor.
func GetServiceMonitor(apiClient *clients.Settings, name, namespace string) (*unstructured.Unstructured, error) {
	return apiClient.Resource(neuronparams.ServiceMonitorGVR).
		Namespace(namespace).
		Get(context.Background(), name, metav1.GetOptions{})
}

// ListServiceMonitors lists all ServiceMonitors in a namespace.
func ListServiceMonitors(apiClient *clients.Settings, namespace string) (*unstructured.UnstructuredList, error) {
	return apiClient.Resource(neuronparams.ServiceMonitorGVR).
		Namespace(namespace).
		List(context.Background(), metav1.ListOptions{})
}

// GetPrometheusToken gets a token for Prometheus API access.
func GetPrometheusToken(apiClient *clients.Settings) (string, error) {
	// Get the prometheus-k8s service account token
	secrets, err := apiClient.CoreV1Interface.Secrets(neuronparams.PrometheusNamespace).List(
		context.Background(), metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list secrets: %w", err)
	}

	for _, secret := range secrets.Items {
		if secret.Type == "kubernetes.io/service-account-token" {
			if token, ok := secret.Data["token"]; ok {
				return string(token), nil
			}
		}
	}

	return "", fmt.Errorf("prometheus token not found")
}

// QueryPrometheus queries Prometheus for a specific metric.
func QueryPrometheus(apiClient *clients.Settings, query string) (*PrometheusQueryResult, error) {
	klog.V(params.NeuronLogLevel).Infof("Querying Prometheus for: %s", query)

	// Get the thanos-querier route or use the internal service
	prometheusURL := fmt.Sprintf("https://%s.%s.svc:9091/api/v1/query",
		neuronparams.ThanosQuerierServiceName, neuronparams.PrometheusNamespace)

	token, err := GetPrometheusToken(apiClient)
	if err != nil {
		klog.V(params.NeuronLogLevel).Infof("Failed to get Prometheus token: %v", err)
		// Continue without token for in-cluster access

		return queryPrometheusInternal(prometheusURL, query, "")
	}

	return queryPrometheusInternal(prometheusURL, query, token)
}

// queryPrometheusInternal performs the actual HTTP query to Prometheus.
func queryPrometheusInternal(baseURL, query, token string) (*PrometheusQueryResult, error) {
	queryURL := fmt.Sprintf("%s?query=%s", baseURL, url.QueryEscape(query))

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query Prometheus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf("prometheus query failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result PrometheusQueryResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// MetricExists checks if a metric exists in Prometheus.
func MetricExists(apiClient *clients.Settings, metricName string) (bool, error) {
	result, err := QueryPrometheus(apiClient, metricName)
	if err != nil {
		return false, err
	}

	return result.Status == "success" && len(result.Data.Result) > 0, nil
}

// GetMetricValue gets the value of a metric from Prometheus.
func GetMetricValue(apiClient *clients.Settings, metricName string) ([]map[string]interface{}, error) {
	result, err := QueryPrometheus(apiClient, metricName)
	if err != nil {
		return nil, err
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("query failed with status: %s", result.Status)
	}

	var values []map[string]interface{}

	for _, r := range result.Data.Result {
		value := map[string]interface{}{
			"metric": r.Metric,
		}
		if len(r.Value) >= 2 {
			value["value"] = r.Value[1]
		}

		values = append(values, value)
	}

	return values, nil
}

// VerifyNeuronMetricsAvailable checks if all expected Neuron metrics are available.
func VerifyNeuronMetricsAvailable(apiClient *clients.Settings) ([]string, []string, error) {
	var availableMetrics, missingMetrics []string

	for _, metric := range params.NeuronMetrics {
		exists, err := MetricExists(apiClient, metric)
		if err != nil {
			klog.V(params.NeuronLogLevel).Infof("Error checking metric %s: %v", metric, err)
			missingMetrics = append(missingMetrics, metric)

			continue
		}

		if exists {
			klog.V(params.NeuronLogLevel).Infof("Metric %s is available", metric)
			availableMetrics = append(availableMetrics, metric)
		} else {
			klog.V(params.NeuronLogLevel).Infof("Metric %s is not available", metric)
			missingMetrics = append(missingMetrics, metric)
		}
	}

	return availableMetrics, missingMetrics, nil
}

// GetNeuronHardwareInfo retrieves the neuron_hardware_info metric.
func GetNeuronHardwareInfo(apiClient *clients.Settings) ([]map[string]interface{}, error) {
	return GetMetricValue(apiClient, "neuron_hardware_info")
}

// GetNeuroncoreUtilization retrieves the neuroncore utilization metric.
func GetNeuroncoreUtilization(apiClient *clients.Settings) ([]map[string]interface{}, error) {
	return GetMetricValue(apiClient, "neuroncore_utilization_ratio")
}

// GetNeuronMemoryUsed retrieves the neuron runtime memory used metric.
func GetNeuronMemoryUsed(apiClient *clients.Settings) ([]map[string]interface{}, error) {
	return GetMetricValue(apiClient, "neuron_runtime_memory_used_bytes")
}
