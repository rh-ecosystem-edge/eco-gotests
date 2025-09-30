package nodehealthparams

import (
	"github.com/openshift-kni/k8sreporter"
	corev1 "k8s.io/api/core/v1"
)

const (
	// Label is used to select tests for Node Health monitoring.
	Label = "node-health"

	// NodeHealthLogLevel configures logging level for Node Health related tests.
	NodeHealthLogLevel = 90

	// LabelValidateNodeReadiness is a test selector for node readiness validation.
	LabelValidateNodeReadiness = "node-health-readiness"

	// LabelValidateNodePressure is a test selector for node pressure validation.
	LabelValidateNodePressure = "node-health-pressure"

	// LabelValidateNodeResources is a test selector for node resource usage validation.
	LabelValidateNodeResources = "node-health-resources"

	// LabelValidateKubeletStatus is a test selector for kubelet status validation.
	LabelValidateKubeletStatus = "node-health-kubelet"

	// LabelValidateNodeConditions is a test selector for node conditions validation.
	LabelValidateNodeConditions = "node-health-conditions"

	// DefaultDiskPressureThreshold is the default threshold for disk pressure (percentage).
	DefaultDiskPressureThreshold = 85.0

	// DefaultMemoryPressureThreshold is the default threshold for memory pressure (percentage).
	DefaultMemoryPressureThreshold = 85.0

	// DefaultDiskUsageThreshold is the default threshold for disk usage (percentage).
	DefaultDiskUsageThreshold = 80.0

	// DefaultMemoryUsageThreshold is the default threshold for memory usage (percentage).
	DefaultMemoryUsageThreshold = 80.0

	// KubeletHealthCheckTimeout is the timeout for kubelet health checks.
	KubeletHealthCheckTimeout = 30

	// NodeConditionCheckTimeout is the timeout for node condition checks.
	NodeConditionCheckTimeout = 60

	// ResourceCheckInterval is the interval between resource checks.
	ResourceCheckInterval = 10

	// ConditionTypeReadyString constant to fix linter warning.
	ConditionTypeReadyString = "Ready"

	// ConstantTrueString constant to fix linter warning.
	ConstantTrueString = "True"

	// ConstantFalseString constant to fix linter warning.
	ConstantFalseString = "False"

	// KubeletNamespace is the namespace where kubelet runs.
	KubeletNamespace = "kube-system"

	// KubeletPodSelector is the label selector for kubelet pods.
	KubeletPodSelector = "k8s-app=kubelet"

	// MachineConfigDaemonPodSelector is a label selector for all machine-config-daemon pods.
	MachineConfigDaemonPodSelector = "k8s-app=machine-config-daemon"

	// MachineConfigDaemonContainerName is a name of container within machine-config-daemon pod.
	MachineConfigDaemonContainerName = "machine-config-daemon"
)

// Labels contains all the labels for node health tests.
var Labels = []string{
	Label,
	LabelValidateNodeReadiness,
	LabelValidateNodePressure,
	LabelValidateNodeResources,
	LabelValidateKubeletStatus,
	LabelValidateNodeConditions,
}

// ReporterNamespacesToDump contains namespaces to dump on test failure.
var ReporterNamespacesToDump = map[string]string{
	"kube-system":                       "kube-system",
	"openshift-machine-config-operator": "openshift-machine-config-operator",
	"openshift-node":                    "openshift-node",
}

// ReporterCRDsToDump contains CRDs to dump on test failure.
var ReporterCRDsToDump = []k8sreporter.CRData{
	{Cr: &corev1.NodeList{}},
	{Cr: &corev1.PodList{}},
	{Cr: &corev1.EventList{}},
}
