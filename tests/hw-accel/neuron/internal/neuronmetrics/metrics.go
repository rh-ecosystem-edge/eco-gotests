package neuronmetrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
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

// monitoringPodInfo contains information about a pod to use for queries.
type monitoringPodInfo struct {
	Name      string
	Container string
}

// findMonitoringPod finds a running pod in the monitoring namespace to execute queries from.
func findMonitoringPod(ctx context.Context, apiClient *clients.Settings) (*monitoringPodInfo, error) {
	// Try thanos-querier pods first - queries are executed in the thanos-query container
	thanosPodslist, err := apiClient.CoreV1Interface.Pods(neuronparams.PrometheusNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=thanos-querier",
	})
	if err == nil {
		for _, pod := range thanosPodslist.Items {
			if pod.Status.Phase == corev1.PodRunning {
				return &monitoringPodInfo{Name: pod.Name, Container: "thanos-query"}, nil
			}
		}
	}

	// Fallback to prometheus pods
	promPodList, err := apiClient.CoreV1Interface.Pods(neuronparams.PrometheusNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=prometheus",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list monitoring pods: %w", err)
	}

	for _, pod := range promPodList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return &monitoringPodInfo{Name: pod.Name, Container: "prometheus"}, nil
		}
	}

	return nil, fmt.Errorf("no running monitoring pods found")
}

// executeInPod runs a command inside a pod and returns stdout.
func executeInPod(ctx context.Context, apiClient *clients.Settings,
	podName, namespace, container string, command []string) (string, error) {
	execReq := apiClient.CoreV1Interface.RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(apiClient.Config, "POST", execReq.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer

	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return "", fmt.Errorf("exec failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// QueryPrometheus queries Prometheus for a specific metric by executing a query inside a cluster pod.
func QueryPrometheus(apiClient *clients.Settings, query string) (*PrometheusQueryResult, error) {
	klog.V(params.NeuronLogLevel).Infof("Querying Prometheus for: %s", query)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Find a running pod in the monitoring namespace to execute the query from
	podInfo, err := findMonitoringPod(ctx, apiClient)
	if err != nil {
		return nil, fmt.Errorf("failed to find monitoring pod: %w", err)
	}

	klog.V(params.NeuronLogLevel).Infof("Using monitoring pod: %s (container: %s)", podInfo.Name, podInfo.Container)

	encodedQuery := url.QueryEscape(query)

	// Try localhost endpoints inside the thanos-query container
	endpoints := []struct {
		name string
		url  string
	}{
		{"localhost:9090", fmt.Sprintf("http://localhost:9090/api/v1/query?query=%s", encodedQuery)},
		{"localhost:9095", fmt.Sprintf("http://localhost:9095/api/v1/query?query=%s", encodedQuery)},
		{"localhost:10902", fmt.Sprintf("http://localhost:10902/api/v1/query?query=%s", encodedQuery)},
	}

	var response string

	var lastErr error

	for _, endpoint := range endpoints {
		queryCmd := []string{"sh", "-c", fmt.Sprintf("curl -sf '%s' 2>/dev/null", endpoint.url)}
		resp, err := executeInPod(ctx, apiClient, podInfo.Name, neuronparams.PrometheusNamespace, podInfo.Container, queryCmd)

		if err == nil && resp != "" && !isUnauthorized(resp) {
			response = resp

			klog.V(params.NeuronLogLevel).Infof("Endpoint %s succeeded", endpoint.name)

			break
		}

		lastErr = fmt.Errorf("endpoint %s failed: %w", endpoint.name, err)
	}

	if response == "" {
		// Fallback: Try with service account token for authenticated endpoint
		tokenCmd := "cat /var/run/secrets/kubernetes.io/serviceaccount/token 2>/dev/null || echo ''"
		token, _ := executeInPod(ctx, apiClient, podInfo.Name, neuronparams.PrometheusNamespace, podInfo.Container,
			[]string{"sh", "-c", tokenCmd})
		token = trimNewline(token)

		if token != "" {
			authURL := fmt.Sprintf("https://%s.%s.svc:9091/api/v1/query?query=%s",
				neuronparams.ThanosQuerierServiceName, neuronparams.PrometheusNamespace, encodedQuery)
		authCmd := []string{"sh", "-c", fmt.Sprintf(
			"curl -sf -k -H 'Authorization: Bearer %s' '%s' 2>/dev/null", token, authURL)}

			resp, err := executeInPod(ctx, apiClient, podInfo.Name, neuronparams.PrometheusNamespace, podInfo.Container, authCmd)

			if err == nil && resp != "" && !isUnauthorized(resp) {
				response = resp
			}
		}
	}

	if response == "" {
		return nil, fmt.Errorf("failed to query prometheus, all endpoints failed: %w", lastErr)
	}

	var result PrometheusQueryResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w, raw: %s", err, response)
	}

	return &result, nil
}

func isUnauthorized(resp string) bool {
	return resp == "Unauthorized" || resp == "Unauthorized\n"
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}

	return s
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
