package tsparams

import (
	"os"
	"strconv"
)

const (
	// LabelSuite represents systemreserved test suite label.
	LabelSuite = "systemreserved"
	// LabelSystemReservedValidation represents systemreserved validation test label.
	LabelSystemReservedValidation = "systemreserved-validation"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{LabelSuite, LabelSystemReservedValidation}

	// Default systemReserved values based on CNF-16828 and scale lab data
	// These can be overridden via environment variables

	// ControlPlaneCPUReserved is the default CPU cores reserved for control plane nodes (40 cores based on scale lab max 39.1)
	ControlPlaneCPUReserved = getEnvOrDefault("CONTROL_PLANE_CPU_RESERVED", "40")
	// ControlPlaneMemoryReserved is the default memory reserved for control plane nodes (30Gi per CNF-16828)
	ControlPlaneMemoryReserved = getEnvOrDefault("CONTROL_PLANE_MEMORY_RESERVED", "30Gi")
	// WorkerCPUReserved is the default CPU reserved for worker nodes (1 core / 1000m)
	WorkerCPUReserved = getEnvOrDefault("WORKER_CPU_RESERVED", "1000m")
	// WorkerMemoryReserved is the default memory reserved for worker nodes (11Gi per CNF-16828)
	WorkerMemoryReserved = getEnvOrDefault("WORKER_MEMORY_RESERVED", "11Gi")

	// TargetNodeRole specifies which node role to test (master, worker, or both)
	TargetNodeRole = getEnvOrDefault("TARGET_NODE_ROLE", "both")

	// PerformanceProfileName is the name of the PerformanceProfile to validate
	PerformanceProfileName = getEnvOrDefault("PERFORMANCE_PROFILE_NAME", "")

	// NodeSizingEnvPath is the path to node-sizing.env file on nodes
	NodeSizingEnvPath = "/host/etc/node-sizing.env"

	// Scale lab baseline data
	// ScaleLabControlPlaneCPUMax is the maximum CPU usage observed in scale lab for control plane (OCP 4.16)
	ScaleLabControlPlaneCPUMax = 39.1
	// ScaleLabControlPlaneMemoryMax is the maximum memory usage observed in scale lab for control plane (OCP 4.16) in GB
	ScaleLabControlPlaneMemoryMax = 33.9
	// ScaleLabWorkerCPUMax is the maximum CPU usage observed in scale lab for workers (OCP 4.16)
	ScaleLabWorkerCPUMax = 16.2
	// ScaleLabWorkerMemoryMax is the maximum memory usage observed in scale lab for workers (OCP 4.16) in GB
	ScaleLabWorkerMemoryMax = 216.0

	// ReporterNamespacesToDump tells reporter which namespaces to collect pod logs from.
	ReporterNamespacesToDump = map[string]string{
		"openshift-performance-addon-operator": "openshift-performance-addon-operator",
		"openshift-cluster-node-tuning-operator": "openshift-cluster-node-tuning-operator",
	}

	// ReporterCRDsToDump tells reporter which CRDs to dump.
	ReporterCRDsToDump = []string{
		"performanceprofiles",
		"tuneds",
		"machineconfigs",
		"machineconfigpools",
	}
)

// getEnvOrDefault retrieves environment variable or returns default value.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetControlPlaneCPUCores returns control plane CPU reserved as numeric cores.
func GetControlPlaneCPUCores() (float64, error) {
	return strconv.ParseFloat(ControlPlaneCPUReserved, 64)
}

// GetWorkerCPUMillicores returns worker CPU reserved in millicores.
// Handles formats like "1000m", "1", "2000m".
func GetWorkerCPUMillicores() (int, error) {
	cpuStr := WorkerCPUReserved

	// If ends with 'm', parse as millicores
	if len(cpuStr) > 0 && cpuStr[len(cpuStr)-1] == 'm' {
		return strconv.Atoi(cpuStr[:len(cpuStr)-1])
	}

	// Otherwise parse as cores and convert to millicores
	cores, err := strconv.ParseFloat(cpuStr, 64)
	if err != nil {
		return 0, err
	}
	return int(cores * 1000), nil
}
