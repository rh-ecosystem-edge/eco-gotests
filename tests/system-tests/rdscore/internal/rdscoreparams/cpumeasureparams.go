package rdscoreparams

import (
	"time"
)

const (
	// LabelCPUMeasurements label to select CPU/Memory measurement tests.
	LabelCPUMeasurements = "rds-core-cpu-measurements"

	// PrometheusNamespace Prometheus monitoring namespace.
	PrometheusNamespace = "openshift-monitoring"

	// PrometheusPodName name of Prometheus pod.
	PrometheusPodName = "prometheus-k8s-0"

	// PrometheusContainerName container name in Prometheus pod.
	PrometheusContainerName = "prometheus"

	// DefaultMeasurementDuration default time window for rate calculations.
	DefaultMeasurementDuration = "10m"

	// BytesToGB conversion factor from bytes to gigabytes.
	BytesToGB = 1024 * 1024 * 1024

	// PromQueryRetryInterval polling interval between retries for Prometheus queries.
	//
	// Deprecated: No longer used with wait.ExponentialBackoffWithContext.
	// Kept for backward compatibility.
	PromQueryRetryInterval = 5 * time.Second
	// PromQueryRetryTimeout total timeout for Prometheus query retries.
	// Increased from 30s to 120s to accommodate exponential backoff retry strategy.
	// Used as context timeout for wait.ExponentialBackoffWithContext.
	PromQueryRetryTimeout = 120 * time.Second

	// NodeDiscoveryRetryInterval polling interval for node list operations.
	NodeDiscoveryRetryInterval = 3 * time.Second
	// NodeDiscoveryRetryTimeout total timeout for node discovery operations.
	NodeDiscoveryRetryTimeout = 20 * time.Second

	// PromQueryCurlTimeout timeout in seconds for curl command.
	// Set to 30s to exceed Prometheus query timeout (25s) while allowing multiple retries
	// within PromQueryRetryTimeout of 120s.
	PromQueryCurlTimeout = "30"
)

// Prometheus query templates (node name will be inserted via fmt.Sprintf).
const (
	// CPUSystemServicesQuery queries average CPU rate for system services (cgroups under /ovs.slice or /system.slice).
	// Returns: CPU cores per second (rate) for each system service on a specific node.
	// Uses /ovs.slice/.+|/system.slice/.+ pattern as fallback (CNF-GoTests proven pattern).
	// Parameters: nodeName, duration (e.g., "10m").
	CPUSystemServicesQuery = `rate(container_cpu_usage_seconds_total{cpu="total",` +
		`id=~"/ovs.slice/.+|/system.slice/.+",id!~"/ovs.slice|/system.slice",node="%s"}[%s])`

	// CPUSystemServicesPerCPUQuery queries per-CPU average rate for system services.
	// Returns: CPU cores per second (rate) per CPU core for each system service.
	// Used when systemd.cpu_affinity annotation is present on node.
	// Parameters: nodeName, duration (e.g., "10m").
	CPUSystemServicesPerCPUQuery = `rate(container_cpu_usage_seconds_total{cpu!="total",` +
		`id=~"/ovs.slice/.+|/system.slice/.+",id!~"/ovs.slice|/system.slice",node="%s"}[%s])`

	// CPUInfraPodsQuery queries CPU rate for infrastructure pods over the measurement window.
	// Excludes: core-*, rds* namespaces.
	// Returns: CPU cores per second, grouped by namespace and pod.
	CPUInfraPodsQuery = `sum by (namespace,pod) (rate(container_cpu_usage_seconds_total{` +
		`namespace=~"openshift-.*|kube-.*",namespace!~"core-.*|rds.*",` +
		`pod!="",container!="",` +
		`id=~"/kubepods.slice/.*pod.*",node="%s"}[%s]))`

	// CPUInfraPodsPerCPUQuery queries per-CPU rate for infrastructure pods.
	// Returns: CPU cores per second per CPU core, grouped by namespace, pod, and cpu.
	// Used when systemd.cpu_affinity annotation is present on node.
	CPUInfraPodsPerCPUQuery = `sum by (namespace,pod,cpu) (rate(container_cpu_usage_seconds_total{` +
		`namespace=~"openshift-.*|kube-.*",namespace!~"core-.*|rds.*",` +
		`pod!="",container!="",cpu!="total",` +
		`id=~"/kubepods.slice/.*pod.*",node="%s"}[%s]))`

	// CPUSystemServicesSpikeQuery queries maximum CPU spike for system services.
	// Uses max_over_time with 30s resolution over the measurement duration.
	// Inner rate uses short 30s window to capture spikes accurately.
	// Returns: Maximum CPU cores per second seen during the measurement window.
	// Parameters: nodeName, duration (for outer max_over_time only).
	CPUSystemServicesSpikeQuery = `max_over_time(sum(rate(container_cpu_usage_seconds_total{` +
		`cpu="total",id=~"/ovs.slice/.+|/system.slice/.+",id!~"/ovs.slice|/system.slice",` +
		`node="%s"}[30s]))[%s:30s])`

	// CPUInfraPodsSpikeQuery queries maximum CPU spike for infrastructure pods.
	// Uses max_over_time with 30s resolution over the measurement duration.
	// Inner rate uses short 30s window to capture spikes accurately.
	// Returns: Maximum CPU cores per second seen during the measurement window.
	// Parameters: nodeName, duration (for outer max_over_time only).
	CPUInfraPodsSpikeQuery = `max_over_time(sum(rate(container_cpu_usage_seconds_total{` +
		`namespace=~"openshift-.*|kube-.*",namespace!~"core-.*|rds.*",` +
		`pod!="",container!="",` +
		`id=~"/kubepods.slice/.*pod.*",node="%s"}[30s]))[%s:30s])`

	// CPUSystemServicesPerCPUSpikeQuery queries maximum per-CPU spike for system services.
	// Aggregates service rates with sum by (cpu) before applying max_over_time.
	// Inner rate uses short 30s window to capture spikes accurately.
	// Returns max spike per CPU core. Used when systemd.cpu_affinity is present.
	// Parameters: nodeName, duration (for outer max_over_time only).
	CPUSystemServicesPerCPUSpikeQuery = `max by (cpu) (max_over_time(` +
		`sum by (cpu) (rate(container_cpu_usage_seconds_total{` +
		`cpu!="total",id=~"/ovs.slice/.+|/system.slice/.+",id!~"/ovs.slice|/system.slice",` +
		`node="%s"}[30s]))[%s:30s]))`

	// MemInfraPodsQuery queries current memory usage for infrastructure pods.
	// Excludes: core-*, rds* namespaces.
	// Returns: bytes used, grouped by namespace and pod.
	MemInfraPodsQuery = `sum by (namespace,pod) (container_memory_usage_bytes{` +
		`namespace=~"openshift-.*|kube-.*",namespace!~"core-.*|rds.*",` +
		`pod!="",container!="",` +
		`id=~"/kubepods.slice/.*pod.*",node="%s"})`

	// MemSystemServicesQuery queries current memory usage for system services.
	// Returns: bytes used for each system service on a specific node.
	// Uses /ovs.slice/.+|/system.slice/.+ pattern as fallback (CNF-GoTests proven pattern).
	MemSystemServicesQuery = `container_memory_usage_bytes{` +
		`id=~"/ovs.slice/.+|/system.slice/.+",id!~"/ovs.slice|/system.slice",node="%s"}`

	// MemSystemServicesSpikeQuery queries maximum memory spike for system services.
	// Uses max_over_time to capture peak memory usage over the measurement duration.
	// Returns: Maximum bytes used during the measurement window.
	// Parameters: nodeName, duration (for max_over_time window).
	MemSystemServicesSpikeQuery = `max_over_time(sum(container_memory_usage_bytes{` +
		`id=~"/ovs.slice/.+|/system.slice/.+",id!~"/ovs.slice|/system.slice",` +
		`node="%s"})[%s:30s])`

	// MemInfraPodsSpikeQuery queries maximum memory spike for infrastructure pods.
	// Uses max_over_time to capture peak memory usage over the measurement duration.
	// Returns: Maximum bytes used during the measurement window.
	// Parameters: nodeName, duration (for max_over_time window).
	MemInfraPodsSpikeQuery = `max_over_time(sum(container_memory_usage_bytes{` +
		`namespace=~"openshift-.*|kube-.*",namespace!~"core-.*|rds.*",` +
		`pod!="",container!="",` +
		`id=~"/kubepods.slice/.*pod.*",node="%s"})[%s:30s])`

	// MemSystemServicesAvgQuery queries average memory usage for system services.
	// Uses avg_over_time to smooth out transient spikes over the measurement duration.
	// Returns: Average bytes used during the measurement window.
	// Parameters: nodeName, duration (for avg_over_time window).
	MemSystemServicesAvgQuery = `avg_over_time(sum(container_memory_usage_bytes{` +
		`id=~"/ovs.slice/.+|/system.slice/.+",id!~"/ovs.slice|/system.slice",` +
		`node="%s"})[%s:30s])`

	// MemInfraPodsAvgQuery queries average memory usage for infrastructure pods.
	// Uses avg_over_time to smooth out transient spikes over the measurement duration.
	// Returns: Average bytes used during the measurement window.
	// Parameters: nodeName, duration (for avg_over_time window).
	MemInfraPodsAvgQuery = `avg_over_time(sum(container_memory_usage_bytes{` +
		`namespace=~"openshift-.*|kube-.*",namespace!~"core-.*|rds.*",` +
		`pod!="",container!="",` +
		`id=~"/kubepods.slice/.*pod.*",node="%s"})[%s:30s])`

	// NodeCPUTotalQuery queries total node CPU usage from node_exporter.
	// Returns: Total CPU cores in use on the node.
	NodeCPUTotalQuery = `sum(rate(node_cpu_seconds_total{mode!="idle",instance=~"%s.*"}[%s]))`

	// NodeMemoryTotalQuery queries total node memory usage.
	// Returns: Total memory in bytes used on the node.
	NodeMemoryTotalQuery = `node_memory_Active_bytes{instance=~"%s.*"}`
)

// CPUMeasurementResult holds per-node CPU measurement data.
type CPUMeasurementResult struct {
	NodeName       string
	SystemServices map[string]float64 // service name -> CPU cores (average rate)
	InfraPods      map[string]float64 // "namespace/pod" -> CPU cores (average rate)
	TotalSystemCPU float64            // Total average CPU from system services
	TotalInfraCPU  float64            // Total average CPU from infrastructure pods
	TotalCPU       float64            // Combined total average
	MaxSystemCPU   float64            // Maximum spike from system services
	MaxInfraCPU    float64            // Maximum spike from infrastructure pods
	MaxTotalCPU    float64            // Maximum combined spike
}

// MemoryMeasurementResult holds per-node memory measurement data.
type MemoryMeasurementResult struct {
	NodeName       string
	SystemServices map[string]int64 // service name -> bytes (instant snapshot)
	InfraPods      map[string]int64 // "namespace/pod" -> bytes (instant snapshot)
	TotalSystemMem int64            // Total instant memory from system services
	TotalInfraMem  int64            // Total instant memory from infrastructure pods
	TotalMemory    int64            // Combined total instant memory
	// Spike tracking fields (over measurement duration)
	AvgSystemMem int64 // Average memory from system services
	AvgInfraMem  int64 // Average memory from infrastructure pods
	MaxSystemMem int64 // Maximum spike from system services
	MaxInfraMem  int64 // Maximum spike from infrastructure pods
	MaxTotalMem  int64 // Maximum combined spike
}

// MeasurementSummary holds aggregated statistics across nodes.
type MeasurementSummary struct {
	MetricName string
	MinValue   float64
	MaxValue   float64
	AvgValue   float64
	Unit       string
	NodeValues map[string]float64 // node name -> value for detailed reporting
}

// PerCPUMeasurementResult holds per-CPU measurement data for a node.
type PerCPUMeasurementResult struct {
	NodeName              string
	SystemServicesPerCPU  map[string]map[int]float64 // service -> cpu_id -> average value
	InfraPodsPerCPU       map[string]map[int]float64 // "namespace/pod" -> cpu_id -> average value
	CPUUtilizationPerCore map[int]float64            // cpu_id -> average total utilization
	MaxCPUSpikePerCore    map[int]float64            // cpu_id -> maximum spike value
}

// CPUAffinityInfo holds reserved/isolated CPU set information.
type CPUAffinityInfo struct {
	HasAffinity  bool
	ReservedCPUs []int
	IsolatedCPUs []int
	RawValue     string // Raw annotation value for logging
	Source       string // Source of detection (e.g., "PerformanceProfile:name")
}

// PromQueryResponse represents Prometheus API response structure.
type PromQueryResponse struct {
	Status string        `json:"status"`
	Data   PromQueryData `json:"data"`
	Error  string        `json:"error,omitempty"`
}

// PromQueryData represents the data section of Prometheus response.
type PromQueryData struct {
	ResultType string       `json:"resultType"`
	Result     []PromMetric `json:"result"`
}

// PromMetric represents an individual metric from Prometheus.
type PromMetric struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value"`
}
