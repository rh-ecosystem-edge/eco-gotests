package rdscorecommon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nto"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	. "github.com/onsi/ginkgo/v2"
)

// isPodReady checks if a pod is in Ready condition.
func isPodReady(p *pod.Builder) bool {
	if p.Object == nil || p.Object.Status.Conditions == nil {
		return false
	}

	for _, condition := range p.Object.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == "True" {
			return true
		}
	}

	return false
}

// discoverPrometheusPod finds a ready Prometheus pod dynamically using label selector.
// Returns the first ready pod found, or error if none available.
func discoverPrometheusPod(apiClient *clients.Settings) (*pod.Builder, error) {
	// List Prometheus pods using label selector
	promPods, err := pod.List(
		apiClient,
		rdscoreparams.PrometheusNamespace,
		metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=prometheus"},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list Prometheus pods: %w", err)
	}

	if len(promPods) == 0 {
		return nil, fmt.Errorf("no Prometheus pods found in namespace %s", rdscoreparams.PrometheusNamespace)
	}

	// Find first ready pod
	for _, p := range promPods {
		if isPodReady(p) {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Discovered ready Prometheus pod: %s", p.Definition.Name)

			return p, nil
		}
	}

	// No ready pod found - return first pod anyway (might become ready soon)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"No ready Prometheus pod found, using first available: %s", promPods[0].Definition.Name)

	return promPods[0], nil
}

// ExecPromQuery executes a Prometheus query via curl in the Prometheus pod with retry logic.
// Uses exponential backoff (k8s wait.ExponentialBackoffWithContext) for retries.
// Returns errors gracefully instead of panicking on timeout.
func ExecPromQuery(apiClient *clients.Settings, query string) ([]rdscoreparams.PromMetric, error) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Executing Prometheus query: %s", query)

	var metrics []rdscoreparams.PromMetric

	// Configure exponential backoff: 2s → 4s → 8s → 16s → 30s (capped)
	// Same pattern as sriov-rootless-dpdk.go
	backoff := wait.Backoff{
		Duration: 2 * time.Second,  // Initial interval
		Factor:   2.0,              // Double each retry
		Steps:    10,               // Max 10 attempts
		Cap:      30 * time.Second, // Cap at 30s between retries
	}

	// Context with 2-minute timeout
	ctx, cancel := context.WithTimeout(context.TODO(), rdscoreparams.PromQueryRetryTimeout)
	defer cancel()

	// Execute with exponential backoff
	err := wait.ExponentialBackoffWithContext(
		ctx,
		backoff,
		func(ctx context.Context) (bool, error) {
			// Discover ready Prometheus pod dynamically
			promPod, err := discoverPrometheusPod(apiClient)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Retry: failed to discover Prometheus pod: %v", err)

				return false, nil // Retry
			}

			if !promPod.Exists() {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Retry: Discovered Prometheus pod does not exist yet")

				return false, nil // Retry
			}

			// URL encode the query
			encodedQuery := url.QueryEscape(query)

			// Execute curl command with timeout
			cmd := []string{
				"curl", "-s",
				"--max-time", rdscoreparams.PromQueryCurlTimeout,
				fmt.Sprintf("http://localhost:9090/api/v1/query?query=%s&timeout=25s", encodedQuery),
			}

			output, err := promPod.ExecCommand(cmd, rdscoreparams.PrometheusContainerName)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Retry: failed to execute Prometheus query: %v", err)

				return false, nil // Retry
			}

			var response rdscoreparams.PromQueryResponse
			if err := json.Unmarshal(output.Bytes(), &response); err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Retry: failed to parse Prometheus response: %v", err)

				return false, nil // Retry
			}

			if response.Status != "success" {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Retry: query failed with status %s: %s", response.Status, response.Error)

				return false, nil // Retry
			}

			// Success - store result
			metrics = response.Data.Result

			return true, nil // Success, stop retrying
		})
	if err != nil {
		// Log warning but return error gracefully (no panic)
		klog.Warningf("Prometheus query failed after exponential backoff retries: %v", err)

		return nil, fmt.Errorf("prometheus query failed after retries: %w", err)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Prometheus query succeeded, returned %d results", len(metrics))

	return metrics, nil
}

// ParseCPUValue parses CPU value from Prometheus result.
func ParseCPUValue(value interface{}) (float64, error) {
	strVal, ok := value.(string)
	if !ok {
		return 0, fmt.Errorf("value is not a string")
	}

	return strconv.ParseFloat(strVal, 64)
}

// ParseMemoryValue parses memory value (bytes) from Prometheus result.
func ParseMemoryValue(value interface{}) (int64, error) {
	strVal, ok := value.(string)
	if !ok {
		return 0, fmt.Errorf("value is not a string")
	}

	floatVal, err := strconv.ParseFloat(strVal, 64)
	if err != nil {
		return 0, err
	}

	return int64(floatVal), nil
}

// CalculateStats calculates minimum, maximum, average for a set of values.
func CalculateStats(values []float64) (minimum, maximum, average float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}

	minimum, maximum = values[0], values[0]
	sum := 0.0

	for _, value := range values {
		if value < minimum {
			minimum = value
		}

		if value > maximum {
			maximum = value
		}

		sum += value
	}

	average = sum / float64(len(values))

	return
}

// DiscoverTargetNodes finds nodes matching the configured label selector with retry logic.
func DiscoverTargetNodes(apiClient *clients.Settings, labelSelector string) ([]*nodes.Builder, error) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Discovering nodes with selector: '%s' (with retry)", labelSelector)

	var listOptions metav1.ListOptions

	if labelSelector != "" {
		listOptions.LabelSelector = labelSelector
	}

	var nodeList []*nodes.Builder

	Eventually(func() error {
		var err error

		nodeList, err = nodes.List(apiClient, listOptions)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Retry: failed to list nodes: %v", err)

			return fmt.Errorf("failed to list nodes: %w", err)
		}

		// Sanity check: ensure we found at least one node
		if len(nodeList) == 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Retry: no nodes found matching selector '%s'", labelSelector)

			return fmt.Errorf("no nodes found matching selector '%s'", labelSelector)
		}

		return nil
	}).WithPolling(rdscoreparams.NodeDiscoveryRetryInterval).
		WithTimeout(rdscoreparams.NodeDiscoveryRetryTimeout).
		Should(Succeed(), "Failed to discover nodes after retries")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node discovery succeeded, found %d nodes", len(nodeList))

	return nodeList, nil
}

// MeasureSystemServicesCPU queries and returns system services CPU usage for a node.
// If the node has systemd.cpu_affinity annotation, also returns per-CPU breakdown.
// Returns average CPU rate (not cumulative counter).
//
//nolint:funlen,gocognit
func MeasureSystemServicesCPU(
	apiClient *clients.Settings,
	nodeName, duration string,
) (map[string]float64, *rdscoreparams.PerCPUMeasurementResult, error) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Measuring system services CPU on node: %s (duration: %s)", nodeName, duration)

	// Pull node to check for CPU affinity annotation
	nodeBuilder, err := nodes.Pull(apiClient, nodeName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to pull node %s: %w", nodeName, err)
	}

	affinityInfo, err := ParseCPUAffinity(apiClient, nodeBuilder)
	if err != nil {
		klog.Warningf("Failed to parse CPU affinity for node %s: %v", nodeName, err)

		affinityInfo = &rdscoreparams.CPUAffinityInfo{HasAffinity: false}
	}

	// Get average CPU usage using rate()
	query := fmt.Sprintf(rdscoreparams.CPUSystemServicesQuery, nodeName, duration)

	metrics, err := ExecPromQuery(apiClient, query)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query system services CPU: %w", err)
	}

	result := make(map[string]float64)

	for _, metric := range metrics {
		serviceName := extractServiceName(metric.Metric["id"])
		if len(metric.Value) > 1 {
			cpuValue, err := ParseCPUValue(metric.Value[1])
			if err != nil {
				klog.Warningf("Failed to parse CPU value for service %s: %v", serviceName, err)

				continue
			}

			result[serviceName] = cpuValue
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d system services (avg)", len(result))

	// If node has CPU affinity, also collect per-CPU data
	var perCPUResult *rdscoreparams.PerCPUMeasurementResult

	if affinityInfo.HasAffinity {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Node has CPU affinity (%s), collecting per-CPU data", affinityInfo.RawValue)

		perCPUResult = &rdscoreparams.PerCPUMeasurementResult{
			NodeName:              nodeName,
			SystemServicesPerCPU:  make(map[string]map[int]float64),
			InfraPodsPerCPU:       make(map[string]map[int]float64),
			CPUUtilizationPerCore: make(map[int]float64),
			MaxCPUSpikePerCore:    make(map[int]float64),
		}

		// Query per-CPU average system services
		perCPUQuery := fmt.Sprintf(rdscoreparams.CPUSystemServicesPerCPUQuery, nodeName, duration)

		perCPUMetrics, err := ExecPromQuery(apiClient, perCPUQuery)
		if err != nil {
			klog.Warningf("Failed to query per-CPU system services: %v", err)
		} else {
			for _, metric := range perCPUMetrics {
				serviceName := extractServiceName(metric.Metric["id"])
				cpuStr := metric.Metric["cpu"]

				cpuID, err := extractCPUID(cpuStr) // e.g., "cpu0" -> 0
				if err != nil {
					klog.Warningf("Failed to parse CPU ID from %s: %v", cpuStr, err)

					continue
				}

				if len(metric.Value) > 1 {
					cpuValue, err := ParseCPUValue(metric.Value[1])
					if err != nil {
						continue
					}

					if perCPUResult.SystemServicesPerCPU[serviceName] == nil {
						perCPUResult.SystemServicesPerCPU[serviceName] = make(map[int]float64)
					}

					perCPUResult.SystemServicesPerCPU[serviceName][cpuID] = cpuValue
				}
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Collected per-CPU avg data for %d services", len(perCPUResult.SystemServicesPerCPU))
		}

		// Query per-CPU average infrastructure pods
		infraPerCPUQuery := fmt.Sprintf(rdscoreparams.CPUInfraPodsPerCPUQuery, nodeName, duration)

		infraPerCPUMetrics, err := ExecPromQuery(apiClient, infraPerCPUQuery)
		if err != nil {
			klog.Warningf("Failed to query per-CPU infra pods: %v", err)
		} else {
			for _, metric := range infraPerCPUMetrics {
				namespace := metric.Metric["namespace"]
				podName := metric.Metric["pod"]
				cpuStr := metric.Metric["cpu"]

				podKey := namespace + "/" + podName

				cpuID, err := extractCPUID(cpuStr) // e.g., "cpu0" -> 0
				if err != nil {
					klog.Warningf("Failed to parse CPU ID from %s: %v", cpuStr, err)

					continue
				}

				if len(metric.Value) > 1 {
					cpuValue, err := ParseCPUValue(metric.Value[1])
					if err != nil {
						continue
					}

					if perCPUResult.InfraPodsPerCPU[podKey] == nil {
						perCPUResult.InfraPodsPerCPU[podKey] = make(map[int]float64)
					}

					perCPUResult.InfraPodsPerCPU[podKey][cpuID] = cpuValue
				}
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Collected per-CPU avg data for %d infra pods", len(perCPUResult.InfraPodsPerCPU))
		}

		// Aggregate per-CPU utilization from all services and pods
		for _, cpuMap := range perCPUResult.SystemServicesPerCPU {
			for cpuID, cpuValue := range cpuMap {
				perCPUResult.CPUUtilizationPerCore[cpuID] += cpuValue
			}
		}

		for _, cpuMap := range perCPUResult.InfraPodsPerCPU {
			for cpuID, cpuValue := range cpuMap {
				perCPUResult.CPUUtilizationPerCore[cpuID] += cpuValue
			}
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Aggregated per-CPU utilization for %d cores", len(perCPUResult.CPUUtilizationPerCore))

		// Query per-CPU max spikes
		spikeQuery := fmt.Sprintf(rdscoreparams.CPUSystemServicesPerCPUSpikeQuery, nodeName, duration)

		spikeMetrics, err := ExecPromQuery(apiClient, spikeQuery)
		if err != nil {
			klog.Warningf("Failed to query per-CPU spikes: %v", err)
		} else {
			for _, metric := range spikeMetrics {
				cpuStr := metric.Metric["cpu"]

				cpuID, err := extractCPUID(cpuStr)
				if err != nil {
					klog.Warningf("Failed to parse CPU ID from %s: %v", cpuStr, err)

					continue
				}

				if len(metric.Value) > 1 {
					spikeValue, err := ParseCPUValue(metric.Value[1])
					if err != nil {
						continue
					}

					perCPUResult.MaxCPUSpikePerCore[cpuID] = spikeValue
				}
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Collected per-CPU spike data for %d cores", len(perCPUResult.MaxCPUSpikePerCore))
		}
	}

	return result, perCPUResult, nil
}

// MeasureInfraPodsCPU queries and returns infrastructure pods CPU usage for a node.
func MeasureInfraPodsCPU(apiClient *clients.Settings, nodeName, duration string) (map[string]float64, error) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Measuring infrastructure pods CPU on node: %s (duration: %s)", nodeName, duration)

	query := fmt.Sprintf(rdscoreparams.CPUInfraPodsQuery, nodeName, duration)

	metrics, err := ExecPromQuery(apiClient, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query infra pods CPU: %w", err)
	}

	result := make(map[string]float64)

	for _, metric := range metrics {
		podKey := fmt.Sprintf("%s/%s",
			metric.Metric["namespace"],
			metric.Metric["pod"])

		if len(metric.Value) > 1 {
			cpuValue, err := ParseCPUValue(metric.Value[1])
			if err != nil {
				klog.Warningf("Failed to parse CPU value for pod %s: %v", podKey, err)

				continue
			}

			result[podKey] = cpuValue
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d infrastructure pods", len(result))

	return result, nil
}

// MeasureSystemServicesMemory queries and returns system services memory usage for a node.
func MeasureSystemServicesMemory(apiClient *clients.Settings, nodeName string) (map[string]int64, error) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Measuring system services memory on node: %s", nodeName)

	query := fmt.Sprintf(rdscoreparams.MemSystemServicesQuery, nodeName)

	metrics, err := ExecPromQuery(apiClient, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query system services memory: %w", err)
	}

	result := make(map[string]int64)

	for _, metric := range metrics {
		serviceName := extractServiceName(metric.Metric["id"])
		if len(metric.Value) > 1 {
			memValue, err := ParseMemoryValue(metric.Value[1])
			if err != nil {
				klog.Warningf("Failed to parse memory value for service %s: %v", serviceName, err)

				continue
			}

			result[serviceName] = memValue
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d system services", len(result))

	return result, nil
}

// MeasureInfraPodsMemory queries and returns infrastructure pods memory usage for a node.
func MeasureInfraPodsMemory(apiClient *clients.Settings, nodeName string) (map[string]int64, error) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Measuring infrastructure pods memory on node: %s", nodeName)

	query := fmt.Sprintf(rdscoreparams.MemInfraPodsQuery, nodeName)

	metrics, err := ExecPromQuery(apiClient, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query infra pods memory: %w", err)
	}

	result := make(map[string]int64)

	for _, metric := range metrics {
		podKey := fmt.Sprintf("%s/%s",
			metric.Metric["namespace"],
			metric.Metric["pod"])

		if len(metric.Value) > 1 {
			memValue, err := ParseMemoryValue(metric.Value[1])
			if err != nil {
				klog.Warningf("Failed to parse memory value for pod %s: %v", podKey, err)

				continue
			}

			result[podKey] = memValue
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d infrastructure pods", len(result))

	return result, nil
}

// CaptureNodeMetricsSnapshot captures current CPU/Memory metrics for all nodes (non-failing).
// Designed to be called from AfterEach blocks for diagnostic purposes.
//
//nolint:gocognit,funlen
func CaptureNodeMetricsSnapshot() {
	By("Capturing node metrics snapshot")

	nodeSelector := RDSCoreConfig.CPUMeasureNodeSelector

	nodeList, err := DiscoverTargetNodes(APIClient, nodeSelector)
	if err != nil {
		klog.Warningf("Failed to discover nodes for metrics snapshot: %v", err)

		return
	}

	if len(nodeList) == 0 {
		klog.Warning("No nodes found for metrics snapshot")

		return
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========== Node Metrics Snapshot ==========")

	for _, node := range nodeList {
		nodeName := node.Object.Name

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("--- Node: %s ---", nodeName)

		// Use config duration instead of hardcoded "15m"
		snapshotDuration := RDSCoreConfig.CPUMeasureDuration
		if snapshotDuration == "" {
			snapshotDuration = rdscoreparams.DefaultMeasurementDuration
		}

		cpuServices, perCPUData, err := MeasureSystemServicesCPU(APIClient, nodeName, snapshotDuration)
		if err != nil {
			klog.Infof("  [WARN] Failed to capture CPU metrics: %v", err)
		} else {
			totalCPU := sumFloat64Values(cpuServices)
			klog.Infof("  System Services CPU (last %s avg): %.6f cores", snapshotDuration, totalCPU)

			// Report per-CPU data if available
			if perCPUData != nil {
				ReportPerCPUMeasurements(perCPUData)
			}
		}

		// Capture CPU spike
		cpuSpikeQuery := fmt.Sprintf(rdscoreparams.CPUSystemServicesSpikeQuery,
			nodeName, snapshotDuration)

		cpuSpikeMetrics, err := ExecPromQuery(APIClient, cpuSpikeQuery)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  [WARN] Failed to capture CPU spike: %v", err)
		} else if len(cpuSpikeMetrics) > 0 && len(cpuSpikeMetrics[0].Value) > 1 {
			cpuSpikeValue, err := ParseCPUValue(cpuSpikeMetrics[0].Value[1])
			if err == nil {
				klog.Infof("  System Services CPU (last %s max): %.6f cores", snapshotDuration, cpuSpikeValue)
			}
		}

		// Capture Memory snapshot (instant)
		memServices, err := MeasureSystemServicesMemory(APIClient, nodeName)
		if err != nil {
			klog.Infof("  [WARN] Failed to capture memory metrics: %v", err)
		} else {
			totalMem := sumInt64Values(memServices)
			klog.Infof("  System Services Memory (instant): %.2f GB",
				float64(totalMem)/float64(rdscoreparams.BytesToGB))
		}

		// Capture Memory average
		memAvgQuery := fmt.Sprintf(rdscoreparams.MemSystemServicesAvgQuery, nodeName, snapshotDuration)

		memAvgMetrics, err := ExecPromQuery(APIClient, memAvgQuery)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  [WARN] Failed to capture memory average: %v", err)
		} else if len(memAvgMetrics) > 0 && len(memAvgMetrics[0].Value) > 1 {
			memAvgValue, err := ParseMemoryValue(memAvgMetrics[0].Value[1])
			if err == nil {
				klog.Infof("  System Services Memory (last %s avg): %.2f GB",
					snapshotDuration, float64(memAvgValue)/float64(rdscoreparams.BytesToGB))
			}
		}

		// Capture Memory spike
		memSpikeQuery := fmt.Sprintf(rdscoreparams.MemSystemServicesSpikeQuery, nodeName, snapshotDuration)

		memSpikeMetrics, err := ExecPromQuery(APIClient, memSpikeQuery)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  [WARN] Failed to capture memory spike: %v", err)
		} else if len(memSpikeMetrics) > 0 && len(memSpikeMetrics[0].Value) > 1 {
			memSpikeValue, err := ParseMemoryValue(memSpikeMetrics[0].Value[1])
			if err == nil {
				klog.Infof("  System Services Memory (last %s max): %.2f GB",
					snapshotDuration, float64(memSpikeValue)/float64(rdscoreparams.BytesToGB))
			}
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("===========================================")
}

// ReportCPUMeasurements logs detailed CPU measurements.
func ReportCPUMeasurements(results map[string]*rdscoreparams.CPUMeasurementResult, topN int, duration string) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("\n========== CPU Usage Report ==========")

	// Sort nodes by name for consistent output
	nodeNames := make([]string, 0, len(results))
	for nodeName := range results {
		nodeNames = append(nodeNames, nodeName)
	}

	sort.Strings(nodeNames)

	for _, nodeName := range nodeNames {
		result := results[nodeName]

		klog.Infof("=== Node: %s ===", nodeName)
		klog.Infof("Average CPU (last %s): %.6f cores (System: %.6f + Pods: %.6f)",
			duration, result.TotalCPU, result.TotalSystemCPU, result.TotalInfraCPU)
		klog.Infof("Max Spike (last %s):   %.6f cores (System: %.6f + Pods: %.6f)\n",
			duration, result.MaxTotalCPU, result.MaxSystemCPU, result.MaxInfraCPU)

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Top System Services (average CPU cores):")

		topServices := getTopNFloat64(result.SystemServices, topN)

		for i, svc := range topServices {
			klog.Infof("  %2d. %-40s: %10.6f cores",
				i+1, svc, result.SystemServices[svc])
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Top Infrastructure Pods (average CPU cores):")

		topPods := getTopNFloat64(result.InfraPods, topN)

		for i, pod := range topPods {
			klog.Infof("  %2d. %-60s: %10.6f cores",
				i+1, pod, result.InfraPods[pod])
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("======================================")
}

// ReportMemoryMeasurements logs detailed memory measurements.
//
//nolint:funlen
func ReportMemoryMeasurements(results map[string]*rdscoreparams.MemoryMeasurementResult, topN int, duration string) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("\n========== Memory Usage Report ==========")

	// Sort nodes by name for consistent output
	nodeNames := make([]string, 0, len(results))
	for nodeName := range results {
		nodeNames = append(nodeNames, nodeName)
	}

	sort.Strings(nodeNames)

	for _, nodeName := range nodeNames {
		result := results[nodeName]

		totalGB := float64(result.TotalMemory) / float64(rdscoreparams.BytesToGB)
		systemGB := float64(result.TotalSystemMem) / float64(rdscoreparams.BytesToGB)
		podsGB := float64(result.TotalInfraMem) / float64(rdscoreparams.BytesToGB)

		klog.Infof("\n=== Node: %s ===", nodeName)
		klog.Infof("Total Memory: %.2f GB (System: %.2f GB + Pods: %.2f GB)\n",
			totalGB, systemGB, podsGB)

		// Report spike data if available
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  System Services:")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Instant: %.2f GB", systemGB)

		if result.AvgSystemMem > 0 {
			avgSystemGB := float64(result.AvgSystemMem) / float64(rdscoreparams.BytesToGB)
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Average (last %s): %.2f GB", duration, avgSystemGB)
		}

		if result.MaxSystemMem > 0 {
			maxSystemGB := float64(result.MaxSystemMem) / float64(rdscoreparams.BytesToGB)
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Max (last %s):     %.2f GB", duration, maxSystemGB)
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Infrastructure Pods:")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Instant: %.2f GB", podsGB)

		if result.AvgInfraMem > 0 {
			avgPodsGB := float64(result.AvgInfraMem) / float64(rdscoreparams.BytesToGB)
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Average (last %s): %.2f GB", duration, avgPodsGB)
		}

		if result.MaxInfraMem > 0 {
			maxPodsGB := float64(result.MaxInfraMem) / float64(rdscoreparams.BytesToGB)
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Max (last %s):     %.2f GB", duration, maxPodsGB)
		}

		if result.MaxTotalMem > 0 {
			maxTotalGB := float64(result.MaxTotalMem) / float64(rdscoreparams.BytesToGB)
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Total Max: %.2f GB", maxTotalGB)
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Top System Services (memory usage):")

		topServices := getTopNInt64(result.SystemServices, topN)

		for i, svc := range topServices {
			memGB := float64(result.SystemServices[svc]) / float64(rdscoreparams.BytesToGB)
			klog.Infof("  %2d. %-40s: %8.2f GB", i+1, svc, memGB)
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("\nTop Infrastructure Pods (memory usage):")

		topPods := getTopNInt64(result.InfraPods, topN)

		for i, pod := range topPods {
			memGB := float64(result.InfraPods[pod]) / float64(rdscoreparams.BytesToGB)
			klog.Infof("  %2d. %-60s: %8.2f GB", i+1, pod, memGB)
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("\n=========================================")
}

// ReportCPUSummary reports aggregated CPU statistics across all nodes.
func ReportCPUSummary(results map[string]*rdscoreparams.CPUMeasurementResult) {
	if len(results) == 0 {
		return
	}

	avgCPUValues := make([]float64, 0, len(results))
	maxCPUValues := make([]float64, 0, len(results))
	nodeAvgDetails := make(map[string]float64)
	nodeMaxDetails := make(map[string]float64)

	for nodeName, result := range results {
		avgCPUValues = append(avgCPUValues, result.TotalCPU)
		maxCPUValues = append(maxCPUValues, result.MaxTotalCPU)
		nodeAvgDetails[nodeName] = result.TotalCPU
		nodeMaxDetails[nodeName] = result.MaxTotalCPU
	}

	avgMin, avgMax, avgAvg := CalculateStats(avgCPUValues)
	maxMin, maxMax, maxAvg := CalculateStats(maxCPUValues)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========== CPU Summary Across Nodes ==========")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Average CPU Usage Statistics (cores):")
	klog.Infof("  Min: %.6f cores", avgMin)
	klog.Infof("  Max: %.6f cores", avgMax)
	klog.Infof("  Avg: %.6f cores", avgAvg)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Max Spike CPU Statistics (cores):")
	klog.Infof("  Min: %.6f cores", maxMin)
	klog.Infof("  Max: %.6f cores", maxMax)
	klog.Infof("  Avg: %.6f cores", maxAvg)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Per-Node Details (Avg | Max):")

	sortedNodes := sortMapKeysFloat64(nodeAvgDetails)

	for _, nodeName := range sortedNodes {
		klog.Infof("  %-50s: %.6f | %.6f cores",
			nodeName, nodeAvgDetails[nodeName], nodeMaxDetails[nodeName])
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("==============================================")
}

// ReportMemorySummary reports aggregated memory statistics across all nodes.
func ReportMemorySummary(results map[string]*rdscoreparams.MemoryMeasurementResult) {
	if len(results) == 0 {
		return
	}

	totalMemValues := make([]float64, 0, len(results))
	nodeDetails := make(map[string]float64)

	for nodeName, result := range results {
		memGB := float64(result.TotalMemory) / float64(rdscoreparams.BytesToGB)
		totalMemValues = append(totalMemValues, memGB)
		nodeDetails[nodeName] = memGB
	}

	minVal, maxVal, avgVal := CalculateStats(totalMemValues)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("\n========== Memory Summary Across Nodes ==========")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Total Memory Usage Statistics (GB):")
	klog.Infof("  Min: %.2f GB", minVal)
	klog.Infof("  Max: %.2f GB", maxVal)
	klog.Infof("  Avg: %.2f GB", avgVal)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("\nPer-Node Details:")

	sortedNodes := sortMapKeysFloat64(nodeDetails)

	for _, nodeName := range sortedNodes {
		klog.Infof("  %-50s: %.2f GB", nodeName, nodeDetails[nodeName])
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("==================================================")
}

// ReportPerCPUMeasurements logs per-CPU measurement data.
//
//nolint:funlen,gocognit
func ReportPerCPUMeasurements(result *rdscoreparams.PerCPUMeasurementResult) {
	if result == nil {
		return
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========== Per-CPU Measurements ==========")
	klog.Infof("Node: %s", result.NodeName)

	if len(result.SystemServicesPerCPU) > 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("System Services Per-CPU Breakdown:")

		for serviceName, cpuMap := range result.SystemServicesPerCPU {
			klog.Infof("  %s:", serviceName)

			// Sort CPU IDs for consistent output
			cpuIDs := make([]int, 0, len(cpuMap))
			for cpuID := range cpuMap {
				cpuIDs = append(cpuIDs, cpuID)
			}

			sort.Ints(cpuIDs)

			for _, cpuID := range cpuIDs {
				klog.Infof("    cpu%d: %.6f", cpuID, cpuMap[cpuID])
			}
		}
	}

	if len(result.InfraPodsPerCPU) > 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("\nInfrastructure Pods Per-CPU Breakdown:")

		for podKey, cpuMap := range result.InfraPodsPerCPU {
			klog.Infof("  %s:", podKey)

			cpuIDs := make([]int, 0, len(cpuMap))
			for cpuID := range cpuMap {
				cpuIDs = append(cpuIDs, cpuID)
			}

			sort.Ints(cpuIDs)

			for _, cpuID := range cpuIDs {
				klog.Infof("    cpu%d: %.6f cores", cpuID, cpuMap[cpuID])
			}
		}
	}

	if len(result.CPUUtilizationPerCore) > 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("\nAverage Utilization Per Core:")

		cpuIDs := make([]int, 0, len(result.CPUUtilizationPerCore))
		for cpuID := range result.CPUUtilizationPerCore {
			cpuIDs = append(cpuIDs, cpuID)
		}

		sort.Ints(cpuIDs)

		for _, cpuID := range cpuIDs {
			klog.Infof("  cpu%d: %.6f cores", cpuID, result.CPUUtilizationPerCore[cpuID])
		}
	}

	if len(result.MaxCPUSpikePerCore) > 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("\nMax Spike Per Core:")

		cpuIDs := make([]int, 0, len(result.MaxCPUSpikePerCore))
		for cpuID := range result.MaxCPUSpikePerCore {
			cpuIDs = append(cpuIDs, cpuID)
		}

		sort.Ints(cpuIDs)

		for _, cpuID := range cpuIDs {
			avgVal := result.CPUUtilizationPerCore[cpuID]
			maxVal := result.MaxCPUSpikePerCore[cpuID]

			if avgVal > 0 {
				klog.Infof("  cpu%d: %.6f cores (spike %.2fx avg)",
					cpuID, maxVal, maxVal/avgVal)
			} else {
				klog.Infof("  cpu%d: %.6f cores (spike [no avg data])",
					cpuID, maxVal)
			}
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("==========================================")
}

// Helper functions

// extractServiceName extracts service name from cgroup path.
func extractServiceName(cgroupPath string) string {
	// Remove /system.slice/ or /ovs.slice/ prefix and .service suffix
	parts := strings.Split(cgroupPath, "/")
	if len(parts) > 0 {
		serviceName := parts[len(parts)-1]

		return strings.TrimSuffix(serviceName, ".service")
	}

	return cgroupPath
}

// DetectCPUAffinityFromPerformanceProfile checks if node is targeted by a PerformanceProfile
// and extracts reserved CPU configuration.
func DetectCPUAffinityFromPerformanceProfile(
	apiClient *clients.Settings,
	nodeBuilder *nodes.Builder,
) (*rdscoreparams.CPUAffinityInfo, error) {
	nodeName := nodeBuilder.Definition.Name

	// List all PerformanceProfiles in the cluster
	profiles, err := nto.ListProfiles(apiClient)
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Failed to list PerformanceProfiles: %v (cluster may not have Node Tuning Operator)", err)

		return &rdscoreparams.CPUAffinityInfo{HasAffinity: false}, nil
	}

	if len(profiles) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("No PerformanceProfiles found on cluster")

		return &rdscoreparams.CPUAffinityInfo{HasAffinity: false}, nil
	}

	// Check each profile's nodeSelector against this node's labels
	nodeLabels := nodeBuilder.Object.Labels

	for _, profile := range profiles {
		profileName := profile.Definition.Name
		nodeSelector := profile.Object.Spec.NodeSelector

		if nodeMatchesSelector(nodeLabels, nodeSelector) {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Node %s matches PerformanceProfile %s", nodeName, profileName)

			// Extract reserved CPUs from PerformanceProfile
			var reservedCPUSet string
			if profile.Object.Spec.CPU.Reserved != nil {
				reservedCPUSet = string(*profile.Object.Spec.CPU.Reserved)
			}

			if reservedCPUSet == "" {
				klog.Warningf("PerformanceProfile %s has empty reserved CPUs", profileName)

				continue
			}

			// Parse CPUSet format (e.g., "0-3,52-55")
			cpuList, err := parseCPUList(reservedCPUSet)
			if err != nil {
				klog.Warningf("Failed to parse reserved CPUs from %s: %v", profileName, err)

				continue
			}

			// Also get isolated CPUs if available
			var isolatedCPUSet string
			if profile.Object.Spec.CPU.Isolated != nil {
				isolatedCPUSet = string(*profile.Object.Spec.CPU.Isolated)
			}

			var isolatedCPUs []int

			if isolatedCPUSet != "" {
				isolatedCPUs, err = parseCPUList(isolatedCPUSet)
				if err != nil {
					klog.Warningf("Failed to parse isolated CPUs: %v", err)
				}
			}

			return &rdscoreparams.CPUAffinityInfo{
				HasAffinity:  true,
				ReservedCPUs: cpuList,
				IsolatedCPUs: isolatedCPUs,
				RawValue:     reservedCPUSet,
				Source:       "PerformanceProfile:" + profileName,
			}, nil
		}
	}

	// No matching PerformanceProfile found
	return &rdscoreparams.CPUAffinityInfo{HasAffinity: false}, nil
}

// ParseCPUAffinity detects CPU affinity configuration from PerformanceProfile.
func ParseCPUAffinity(
	apiClient *clients.Settings,
	nodeBuilder *nodes.Builder,
) (*rdscoreparams.CPUAffinityInfo, error) {
	nodeName := nodeBuilder.Definition.Name

	// Check PerformanceProfile CR
	affinityInfo, err := DetectCPUAffinityFromPerformanceProfile(apiClient, nodeBuilder)
	if err != nil {
		klog.Warningf("Failed to detect CPU affinity from PerformanceProfile: %v", err)
		// Fall through to no affinity
	} else if affinityInfo.HasAffinity {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Node %s: CPU affinity detected from %s: reserved=%v",
			nodeName, affinityInfo.Source, affinityInfo.ReservedCPUs)

		return affinityInfo, nil
	}

	// No affinity detected
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Node %s: No CPU affinity configuration found", nodeName)

	return &rdscoreparams.CPUAffinityInfo{HasAffinity: false}, nil
}

// nodeMatchesSelector checks if node labels match the given label selector.
// Returns true if all selector labels are present in node labels with matching values.
// Empty string values in selector mean "label must exist with any value".
func nodeMatchesSelector(nodeLabels map[string]string, selector map[string]string) bool {
	if len(selector) == 0 {
		return false // Empty selector matches nothing
	}

	for key, value := range selector {
		nodeValue, exists := nodeLabels[key]
		if !exists {
			return false // Required label not present
		}

		// Empty string in selector means "label must exist" (any value)
		if value != "" && nodeValue != value {
			return false // Value mismatch
		}
	}

	return true // All selector labels match
}

// parseCPUList parses CPU list string (e.g., "0-3,52-55") into []int.
func parseCPUList(cpuStr string) ([]int, error) {
	var cpus []int

	parts := strings.Split(cpuStr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)

		if strings.Contains(part, "-") {
			// Range format: "0-3"
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid CPU range format: %s", part)
			}

			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid start CPU in range %s: %w", part, err)
			}

			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid end CPU in range %s: %w", part, err)
			}

			for cpuID := start; cpuID <= end; cpuID++ {
				cpus = append(cpus, cpuID)
			}
		} else {
			// Single CPU: "0"
			cpuID, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid CPU number %s: %w", part, err)
			}

			cpus = append(cpus, cpuID)
		}
	}

	return cpus, nil
}

// extractCPUID extracts CPU number from cpu label (e.g., "cpu0" -> 0, "cpu15" -> 15).
func extractCPUID(cpuLabel string) (int, error) {
	// Remove "cpu" prefix
	cpuNumStr := strings.TrimPrefix(cpuLabel, "cpu")

	cpuID, err := strconv.Atoi(cpuNumStr)
	if err != nil {
		return 0, fmt.Errorf("invalid CPU label format '%s': %w", cpuLabel, err)
	}

	return cpuID, nil
}

// getTopNFloat64 returns top N keys from a map sorted by value (descending).
func getTopNFloat64(valuesMap map[string]float64, topCount int) []string {
	type keyValue struct {
		Key   string
		Value float64
	}

	sorted := make([]keyValue, 0, len(valuesMap))

	for key, val := range valuesMap {
		sorted = append(sorted, keyValue{key, val})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Value > sorted[j].Value
	})

	result := make([]string, 0, topCount)

	for idx := 0; idx < len(sorted) && idx < topCount; idx++ {
		result = append(result, sorted[idx].Key)
	}

	return result
}

// getTopNInt64 returns top N keys from a map sorted by value (descending).
func getTopNInt64(valuesMap map[string]int64, topCount int) []string {
	type keyValue struct {
		Key   string
		Value int64
	}

	sorted := make([]keyValue, 0, len(valuesMap))

	for key, val := range valuesMap {
		sorted = append(sorted, keyValue{key, val})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Value > sorted[j].Value
	})

	result := make([]string, 0, topCount)

	for idx := 0; idx < len(sorted) && idx < topCount; idx++ {
		result = append(result, sorted[idx].Key)
	}

	return result
}

// sortMapKeysFloat64 returns sorted keys from a float64 map.
func sortMapKeysFloat64(valuesMap map[string]float64) []string {
	keys := make([]string, 0, len(valuesMap))

	for key := range valuesMap {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

// sumFloat64Values sums all values in a map.
func sumFloat64Values(valuesMap map[string]float64) float64 {
	sum := 0.0

	for _, value := range valuesMap {
		sum += value
	}

	return sum
}

// sumInt64Values sums all values in a map.
func sumInt64Values(valuesMap map[string]int64) int64 {
	var sum int64

	for _, value := range valuesMap {
		sum += value
	}

	return sum
}

// MeasureAndValidateCPUUsage is the main test function to measure and validate CPU usage.
//
//nolint:funlen,gocognit
func MeasureAndValidateCPUUsage() {
	By("=== Starting CPU usage measurement and validation ===")

	By("Discovering target nodes based on label selector")

	nodeSelector := RDSCoreConfig.CPUMeasureNodeSelector
	targetNodes, err := DiscoverTargetNodes(APIClient, nodeSelector)
	Expect(err).ToNot(HaveOccurred(), "Failed to discover target nodes: %v", err)
	Expect(targetNodes).ToNot(BeEmpty(),
		"No nodes found matching selector: '%s'", nodeSelector)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Found %d nodes for CPU measurement", len(targetNodes))

	duration := RDSCoreConfig.CPUMeasureDuration
	if duration == "" {
		duration = rdscoreparams.DefaultMeasurementDuration
	}

	results := make(map[string]*rdscoreparams.CPUMeasurementResult)

	By(fmt.Sprintf("Measuring CPU usage on %d nodes (duration: %s)", len(targetNodes), duration))

	for _, node := range targetNodes {
		nodeName := node.Object.Name

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing node: %s", nodeName)

		result := &rdscoreparams.CPUMeasurementResult{
			NodeName:       nodeName,
			SystemServices: make(map[string]float64),
			InfraPods:      make(map[string]float64),
		}

		// Measure system services CPU (average)
		systemServices, perCPUData, err := MeasureSystemServicesCPU(APIClient, nodeName, duration)
		if err != nil {
			klog.Warningf("Failed to measure system services CPU on node %s (non-fatal): %v", nodeName, err)
			// Skip this measurement, continue with other measurements
		} else {
			result.SystemServices = systemServices
			result.TotalSystemCPU = sumFloat64Values(systemServices)

			// Report per-CPU data if available
			if perCPUData != nil {
				ReportPerCPUMeasurements(perCPUData)
			}
		}

		// Measure system services CPU spike (max)
		maxSystemQuery := fmt.Sprintf(rdscoreparams.CPUSystemServicesSpikeQuery, nodeName, duration)

		maxSystemMetrics, err := ExecPromQuery(APIClient, maxSystemQuery)
		if err != nil {
			klog.Warningf("Failed to query system services max spike: %v", err)
		} else if len(maxSystemMetrics) > 0 && len(maxSystemMetrics[0].Value) > 1 {
			maxSystemCPU, err := ParseCPUValue(maxSystemMetrics[0].Value[1])
			if err == nil {
				result.MaxSystemCPU = maxSystemCPU
			}
		}

		// Measure infrastructure pods CPU (average)
		infraPods, err := MeasureInfraPodsCPU(APIClient, nodeName, duration)
		if err != nil {
			klog.Warningf("Failed to measure infrastructure pods CPU on node %s (non-fatal): %v", nodeName, err)
			// Skip this measurement, continue with other measurements
		} else {
			result.InfraPods = infraPods
			result.TotalInfraCPU = sumFloat64Values(infraPods)
		}

		// Measure infrastructure pods CPU spike (max)
		maxInfraQuery := fmt.Sprintf(rdscoreparams.CPUInfraPodsSpikeQuery, nodeName, duration)

		maxInfraMetrics, err := ExecPromQuery(APIClient, maxInfraQuery)
		if err != nil {
			klog.Warningf("Failed to query infra pods max spike: %v", err)
		} else if len(maxInfraMetrics) > 0 && len(maxInfraMetrics[0].Value) > 1 {
			maxInfraCPU, err := ParseCPUValue(maxInfraMetrics[0].Value[1])
			if err == nil {
				result.MaxInfraCPU = maxInfraCPU
			}
		}

		// Calculate total CPU metrics.
		// TotalCPU: Sum of average system and infra CPU usage.
		// MaxTotalCPU: Sum of individual peak system and peak infra CPU.
		// Note: MaxTotalCPU is an upper bound representing worst-case if both
		// subsystems spike simultaneously. It may exceed the actual observed
		// peak of the combined time series.
		result.TotalCPU = result.TotalSystemCPU + result.TotalInfraCPU
		result.MaxTotalCPU = result.MaxSystemCPU + result.MaxInfraCPU

		results[nodeName] = result

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Node %s total CPU: %.6f cores (System: %.6f + Pods: %.6f)",
			nodeName, result.TotalCPU, result.TotalSystemCPU, result.TotalInfraCPU)
	}

	By("Reporting CPU measurements")
	ReportCPUMeasurements(results, 10, duration)
	ReportCPUSummary(results)

	// Validate against threshold if configured
	if RDSCoreConfig.CPUMeasureThresholdCores != nil {
		threshold := *RDSCoreConfig.CPUMeasureThresholdCores

		By(fmt.Sprintf("Validating CPU usage against threshold: %.2f cores", threshold))

		failures := []string{}

		for nodeName, result := range results {
			if result.TotalCPU > threshold {
				failureMsg := fmt.Sprintf("Node %s exceeded CPU threshold: %.6f > %.2f cores",
					nodeName, result.TotalCPU, threshold)
				failures = append(failures, failureMsg)

				klog.Warningf("%s", failureMsg)

				// Log top consumers
				GinkgoWriter.Printf("\n[THRESHOLD EXCEEDED] %s\n", failureMsg)
				GinkgoWriter.Println("Top 5 System Services:")

				topServices := getTopNFloat64(result.SystemServices, 5)

				for i, svc := range topServices {
					GinkgoWriter.Printf("  %d. %-40s: %.6f\n",
						i+1, svc, result.SystemServices[svc])
				}

				GinkgoWriter.Println("Top 5 Infrastructure Pods:")

				topPods := getTopNFloat64(result.InfraPods, 5)

				for i, pod := range topPods {
					GinkgoWriter.Printf("  %d. %-60s: %.6f cores\n",
						i+1, pod, result.InfraPods[pod])
				}
			}
		}

		Expect(failures).To(BeEmpty(),
			"CPU threshold validation failed:\n%s", strings.Join(failures, "\n"))
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"No CPU threshold configured, reporting values only")
	}
}

// MeasureAndValidateMemoryUsage is the main test function to measure and validate memory usage.
//
//nolint:funlen,gocognit
func MeasureAndValidateMemoryUsage() {
	By("=== Starting memory usage measurement and validation ===")

	By("Discovering target nodes based on label selector")

	nodeSelector := RDSCoreConfig.CPUMeasureNodeSelector
	targetNodes, err := DiscoverTargetNodes(APIClient, nodeSelector)
	Expect(err).ToNot(HaveOccurred(), "Failed to discover target nodes: %v", err)
	Expect(targetNodes).ToNot(BeEmpty(),
		"No nodes found matching selector: '%s'", nodeSelector)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Found %d nodes for memory measurement", len(targetNodes))

	// Use same duration as CPU measurements for spike/average queries
	duration := RDSCoreConfig.CPUMeasureDuration
	if duration == "" {
		duration = rdscoreparams.DefaultMeasurementDuration
	}

	results := make(map[string]*rdscoreparams.MemoryMeasurementResult)

	By(fmt.Sprintf("Measuring memory usage on %d nodes (duration: %s)", len(targetNodes), duration))

	for _, node := range targetNodes {
		nodeName := node.Object.Name

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing node: %s", nodeName)

		result := &rdscoreparams.MemoryMeasurementResult{
			NodeName:       nodeName,
			SystemServices: make(map[string]int64),
			InfraPods:      make(map[string]int64),
		}

		// Measure system services memory
		systemServices, err := MeasureSystemServicesMemory(APIClient, nodeName)
		if err != nil {
			klog.Warningf("Failed to measure system services memory on node %s (non-fatal): %v", nodeName, err)
			// Skip this measurement, continue with other measurements
		} else {
			result.SystemServices = systemServices
			result.TotalSystemMem = sumInt64Values(systemServices)
		}

		// Measure infrastructure pods memory
		infraPods, err := MeasureInfraPodsMemory(APIClient, nodeName)
		if err != nil {
			klog.Warningf("Failed to measure infrastructure pods memory on node %s (non-fatal): %v", nodeName, err)
			// Skip this measurement, continue with other measurements
		} else {
			result.InfraPods = infraPods
			result.TotalInfraMem = sumInt64Values(infraPods)
		}

		// Measure system services memory average
		avgSystemQuery := fmt.Sprintf(rdscoreparams.MemSystemServicesAvgQuery, nodeName, duration)

		avgSystemMetrics, err := ExecPromQuery(APIClient, avgSystemQuery)
		if err != nil {
			klog.Warningf("Failed to query system services avg memory: %v", err)
		} else if len(avgSystemMetrics) > 0 && len(avgSystemMetrics[0].Value) > 1 {
			avgSystemMem, err := ParseMemoryValue(avgSystemMetrics[0].Value[1])
			if err == nil {
				result.AvgSystemMem = avgSystemMem
			}
		}

		// Measure system services memory spike (max)
		maxSystemQuery := fmt.Sprintf(rdscoreparams.MemSystemServicesSpikeQuery, nodeName, duration)

		maxSystemMetrics, err := ExecPromQuery(APIClient, maxSystemQuery)
		if err != nil {
			klog.Warningf("Failed to query system services max spike: %v", err)
		} else if len(maxSystemMetrics) > 0 && len(maxSystemMetrics[0].Value) > 1 {
			maxSystemMem, err := ParseMemoryValue(maxSystemMetrics[0].Value[1])
			if err == nil {
				result.MaxSystemMem = maxSystemMem
			}
		}

		// Measure infrastructure pods memory average
		avgInfraQuery := fmt.Sprintf(rdscoreparams.MemInfraPodsAvgQuery, nodeName, duration)

		avgInfraMetrics, err := ExecPromQuery(APIClient, avgInfraQuery)
		if err != nil {
			klog.Warningf("Failed to query infra pods avg memory: %v", err)
		} else if len(avgInfraMetrics) > 0 && len(avgInfraMetrics[0].Value) > 1 {
			avgInfraMem, err := ParseMemoryValue(avgInfraMetrics[0].Value[1])
			if err == nil {
				result.AvgInfraMem = avgInfraMem
			}
		}

		// Measure infrastructure pods memory spike (max)
		maxInfraQuery := fmt.Sprintf(rdscoreparams.MemInfraPodsSpikeQuery, nodeName, duration)

		maxInfraMetrics, err := ExecPromQuery(APIClient, maxInfraQuery)
		if err != nil {
			klog.Warningf("Failed to query infra pods max spike: %v", err)
		} else if len(maxInfraMetrics) > 0 && len(maxInfraMetrics[0].Value) > 1 {
			maxInfraMem, err := ParseMemoryValue(maxInfraMetrics[0].Value[1])
			if err == nil {
				result.MaxInfraMem = maxInfraMem
			}
		}

		// Calculate total spike
		result.MaxTotalMem = result.MaxSystemMem + result.MaxInfraMem

		result.TotalMemory = result.TotalSystemMem + result.TotalInfraMem

		results[nodeName] = result

		totalGB := float64(result.TotalMemory) / float64(rdscoreparams.BytesToGB)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Node %s total instant memory: %.2f GB", nodeName, totalGB)

		if result.MaxTotalMem > 0 {
			maxTotalGB := float64(result.MaxTotalMem) / float64(rdscoreparams.BytesToGB)
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Node %s total max memory: %.2f GB", nodeName, maxTotalGB)
		}
	}

	By("Reporting memory measurements")
	ReportMemoryMeasurements(results, 10, duration)
	ReportMemorySummary(results)

	// Validate against threshold if configured
	if RDSCoreConfig.MemMeasureThresholdGB != nil {
		threshold := *RDSCoreConfig.MemMeasureThresholdGB

		By(fmt.Sprintf("Validating memory usage against threshold: %.2f GB", threshold))

		failures := []string{}

		for nodeName, result := range results {
			// Use duration-based averages if both queries succeeded, otherwise fall back to instantaneous
			var (
				memBytes  int64
				isAverage bool
			)

			if result.AvgSystemMem > 0 && result.AvgInfraMem > 0 {
				memBytes = result.AvgSystemMem + result.AvgInfraMem
				isAverage = true
			} else {
				memBytes = result.TotalMemory
				isAverage = false
			}

			memGB := float64(memBytes) / float64(rdscoreparams.BytesToGB)

			if memGB > threshold {
				metricType := "average"
				if !isAverage {
					metricType = "instantaneous"
				}

				failureMsg := fmt.Sprintf("Node %s exceeded memory threshold (%s): %.2f > %.2f GB",
					nodeName, metricType, memGB, threshold)
				failures = append(failures, failureMsg)

				klog.Warningf("%s", failureMsg)

				// Log top consumers
				GinkgoWriter.Printf("\n[THRESHOLD EXCEEDED] %s\n", failureMsg)
				GinkgoWriter.Println("Top 5 System Services:")

				topServices := getTopNInt64(result.SystemServices, 5)

				for i, svc := range topServices {
					svcGB := float64(result.SystemServices[svc]) / float64(rdscoreparams.BytesToGB)
					GinkgoWriter.Printf("  %d. %-40s: %.2f GB\n", i+1, svc, svcGB)
				}

				GinkgoWriter.Println("Top 5 Infrastructure Pods:")

				topPods := getTopNInt64(result.InfraPods, 5)

				for i, pod := range topPods {
					podGB := float64(result.InfraPods[pod]) / float64(rdscoreparams.BytesToGB)
					GinkgoWriter.Printf("  %d. %-60s: %.2f GB\n", i+1, pod, podGB)
				}
			}
		}

		Expect(failures).To(BeEmpty(),
			"Memory threshold validation failed:\n%s", strings.Join(failures, "\n"))
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"No memory threshold configured, reporting values only")
	}
}

// formatPrometheusDuration converts Go duration to Prometheus duration format.
// Examples: 1h30m → "90m", 2h → "2h", 45m → "45m", 30s → "5m" (capped).
func formatPrometheusDuration(d time.Duration) string {
	totalMinutes := int(d.Minutes())
	hours := totalMinutes / 60
	minutes := totalMinutes % 60

	if hours > 0 && minutes > 0 {
		// Simplify to minutes: 1h30m → "90m"
		return fmt.Sprintf("%dm", totalMinutes)
	}

	if hours > 0 {
		// Hours only: 2h → "2h"
		return fmt.Sprintf("%dh", hours)
	}

	if minutes > 0 {
		// Minutes only: 45m → "45m"
		return fmt.Sprintf("%dm", minutes)
	}

	// Less than 1 minute - should not happen with min cap
	return "5m"
}

// getMinDuration returns minimum duration cap from config or default (5m).
func getMinDuration() time.Duration {
	if RDSCoreConfig.MinMeasureDuration != "" {
		if d, err := time.ParseDuration(RDSCoreConfig.MinMeasureDuration); err == nil {
			return d
		}

		klog.Warningf("Invalid MinMeasureDuration value: %s, using default 5m", RDSCoreConfig.MinMeasureDuration)
	}

	return 5 * time.Minute
}

// getMaxDuration returns maximum duration cap from config or default (2h).
func getMaxDuration() time.Duration {
	if RDSCoreConfig.MaxMeasureDuration != "" {
		if d, err := time.ParseDuration(RDSCoreConfig.MaxMeasureDuration); err == nil {
			return d
		}

		klog.Warningf("Invalid MaxMeasureDuration value: %s, using default 2h", RDSCoreConfig.MaxMeasureDuration)
	}

	return 2 * time.Hour
}

// prometheusRetentionCache caches the Prometheus retention period to avoid repeated queries.
var prometheusRetentionCache *time.Duration

// getPrometheusRetention queries Prometheus for its retention period.
// Returns the retention duration, or error if query fails.
// Result is cached to avoid repeated queries.
func getPrometheusRetention(apiClient *clients.Settings) (time.Duration, error) {
	// Return cached value if available
	if prometheusRetentionCache != nil {
		return *prometheusRetentionCache, nil
	}

	// Query Prometheus runtime info for retention
	query := "prometheus_tsdb_retention_limit_seconds"

	metrics, err := ExecPromQuery(apiClient, query)
	if err != nil {
		return 0, fmt.Errorf("failed to query Prometheus retention: %w", err)
	}

	if len(metrics) == 0 {
		return 0, fmt.Errorf("prometheus_tsdb_retention_limit_seconds metric not found")
	}

	// Parse retention value (in seconds)
	// Value is []interface{} where [0] is timestamp and [1] is the value
	if len(metrics[0].Value) < 2 {
		return 0, fmt.Errorf("invalid metric value format: %v", metrics[0].Value)
	}

	valueStr, ok := metrics[0].Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("metric value is not a string: %v", metrics[0].Value[1])
	}

	retentionSeconds, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse retention value %s: %w", valueStr, err)
	}

	retention := time.Duration(retentionSeconds) * time.Second

	// Cache the result
	prometheusRetentionCache = &retention

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Prometheus retention period: %v", retention)

	return retention, nil
}

// CalculateDynamicDuration calculates duration since startTime with configurable min/max caps.
// Priority: ECO_RDSCORE_CPU_MEASURE_DURATION > dynamic > YAML > default
// Returns Prometheus-formatted duration string (e.g., "90m", "2h")
// Exported so test files can use it.
func CalculateDynamicDuration(startTime time.Time) string {
	// Priority 1: Check env var override and validate against Prometheus retention
	if RDSCoreConfig.CPUMeasureDuration != "" {
		configuredDuration := RDSCoreConfig.CPUMeasureDuration

		// Parse configured duration to validate it
		parsedDuration, err := time.ParseDuration(configuredDuration)
		if err != nil {
			klog.Warningf(
				"Invalid configured duration %s: %v, falling back to dynamic calculation",
				configuredDuration, err)
		} else {
			// Check against Prometheus retention
			if promRetention, err := getPrometheusRetention(APIClient); err == nil {
				if parsedDuration > promRetention {
					klog.Warningf(
						"Configured duration %s exceeds Prometheus retention %v, capping at retention",
						configuredDuration, promRetention)

					return formatPrometheusDuration(promRetention)
				}
			}

			// Configured duration is valid and within Prometheus retention
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Using configured duration (env/YAML): %s", configuredDuration)

			return configuredDuration
		}
	}

	// Priority 2: Calculate dynamic duration
	elapsed := time.Since(startTime)

	// Get caps from config or use defaults
	minDuration := getMinDuration()   // Default: 5m
	configuredMax := getMaxDuration() // Default: 2h

	var effectiveMax time.Duration

	// Query Prometheus retention - this is the hard ceiling
	if promRetention, err := getPrometheusRetention(APIClient); err == nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Prometheus retention: %v", promRetention)

		// Use Prometheus retention as the effective max (it's the hard ceiling)
		effectiveMax = promRetention

		// Warn if user's configured max exceeds what Prometheus can actually provide
		if configuredMax > promRetention {
			klog.Warningf(
				"Configured max %v exceeds Prometheus retention %v, will cap at retention",
				configuredMax, promRetention)
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Effective max cap: %v (Prometheus retention)", effectiveMax)
	} else {
		// Prometheus query failed - fall back to configured max
		klog.Warningf(
			"Failed to query Prometheus retention (%v), using configured max %v as fallback (may exceed actual retention)",
			err, configuredMax)
		effectiveMax = configuredMax

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Effective max cap: %v (configured, Prometheus unavailable)", effectiveMax)
	}

	// Apply caps
	if elapsed < minDuration {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Elapsed time %v below minimum %v, using minimum", elapsed, minDuration)
		elapsed = minDuration
	}

	if elapsed > effectiveMax {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Elapsed time %v exceeds effective maximum %v, capping at maximum", elapsed, effectiveMax)
		elapsed = effectiveMax
	}

	duration := formatPrometheusDuration(elapsed)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Using dynamic duration (since suite start): %s (elapsed: %v)", duration, elapsed)

	return duration
}

// MeasureCPUWithDynamicDuration wraps MeasureAndValidateCPUUsage
// with dynamic duration calculation based on suite start time.
func MeasureCPUWithDynamicDuration(startTime time.Time) {
	duration := CalculateDynamicDuration(startTime)

	// Temporarily override config
	originalDuration := RDSCoreConfig.CPUMeasureDuration

	RDSCoreConfig.CPUMeasureDuration = duration

	defer func() {
		RDSCoreConfig.CPUMeasureDuration = originalDuration
	}()

	MeasureAndValidateCPUUsage()
}

// MeasureMemoryWithDynamicDuration wraps MeasureAndValidateMemoryUsage
// with dynamic duration calculation based on suite start time.
func MeasureMemoryWithDynamicDuration(startTime time.Time) {
	duration := CalculateDynamicDuration(startTime)

	// Temporarily override config
	originalDuration := RDSCoreConfig.CPUMeasureDuration

	RDSCoreConfig.CPUMeasureDuration = duration

	defer func() {
		RDSCoreConfig.CPUMeasureDuration = originalDuration
	}()

	MeasureAndValidateMemoryUsage()
}

// CaptureSnapshotWithDynamicDuration wraps CaptureNodeMetricsSnapshot
// with dynamic duration calculation based on suite start time.
// This modifies the hardcoded "15m" in CaptureNodeMetricsSnapshot to use dynamic duration.
func CaptureSnapshotWithDynamicDuration(startTime time.Time) {
	duration := CalculateDynamicDuration(startTime)

	// Temporarily override config (snapshot will pick it up)
	originalDuration := RDSCoreConfig.CPUMeasureDuration

	RDSCoreConfig.CPUMeasureDuration = duration

	defer func() {
		RDSCoreConfig.CPUMeasureDuration = originalDuration
	}()

	CaptureNodeMetricsSnapshot()
}
