package neuronhelpers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

const (
	kmmBuildNamespace  = "openshift-kmm"
	diagnosticLogLines = int64(200)
)

var (
	buildGVR = schema.GroupVersionResource{
		Group: "build.openshift.io", Version: "v1", Resource: "builds",
	}
	buildConfigGVR = schema.GroupVersionResource{
		Group: "build.openshift.io", Version: "v1", Resource: "buildconfigs",
	}
	imageStreamGVR = schema.GroupVersionResource{
		Group: "image.openshift.io", Version: "v1", Resource: "imagestreams",
	}
	moduleBuildSignConfigGVR = schema.GroupVersionResource{
		Group: "kmm.sigs.x-k8s.io", Version: "v1beta1", Resource: "modulebuildsignconfigs",
	}
	sensitiveDiagnosticKey = regexp.MustCompile(
		`(?i)(secret|password|token|authorization|credential|dockerconfig|pullsecret|auth)`,
	)
	sensitiveDiagnosticText = regexp.MustCompile(
		`(?i)(authorization\s*[=:]\s*)(?:bearer|basic)\s+[^\s,"]+|` +
			`(bearer\s+)[^\s,"]+|` +
			`((?:password|token|authorization|secret|credential|auth|dockerconfig|pullsecret)` +
			`\s*[=:]\s*)(?:"[^"]*"|'[^']*'|[^\s,]+)|` +
			`([a-z][a-z0-9+.-]*://[^/\s:@]+:)[^@\s]+@`,
	)
)

// LogDRAInClusterBuildDiagnostics records the KMM Module, build resources,
// image streams, build ConfigMap, build-related pods, and recent Events after
// an in-cluster DRA build does not converge. Resource fields and pod logs are
// sanitized so credentials are never printed.
func LogDRAInClusterBuildDiagnostics(apiClient *clients.Settings) {
	ctx := context.TODO()

	deviceConfig, err := apiClient.K8sClient.CoreV1().ConfigMaps(params.NeuronNamespace).Get(
		ctx, params.BuildConfigMapPrefix+params.DefaultDeviceConfigName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("DRA in-cluster build diagnostics: build ConfigMap lookup failed: %v", err)
	} else {
		klog.Infof("DRA in-cluster build diagnostics: build ConfigMap %s has data keys: %s",
			deviceConfig.Name, strings.Join(sortedKeys(deviceConfig.Data), ", "))
	}

	for _, namespace := range []string{params.NeuronNamespace, kmmBuildNamespace} {
		logDynamicResource(ctx, apiClient, buildGVR, namespace)
		logDynamicResource(ctx, apiClient, buildConfigGVR, namespace)
		logDynamicResource(ctx, apiClient, imageStreamGVR, namespace)
		logDynamicResource(ctx, apiClient, moduleBuildSignConfigGVR, namespace)
	}

	module, err := kmm.Pull(apiClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
	if err != nil {
		klog.Errorf("DRA in-cluster build diagnostics: Module lookup failed: %v", err)
	} else {
		logJSON("DRA in-cluster build diagnostics: Module spec", sanitizeDiagnosticObject(module.Object.Spec))
		logJSON("DRA in-cluster build diagnostics: Module status", sanitizeDiagnosticObject(module.Object.Status))
	}

	for _, namespace := range []string{params.NeuronNamespace, kmmBuildNamespace} {
		logPodDiagnostics(ctx, apiClient, namespace)
		logEventDiagnostics(ctx, apiClient, namespace)
	}
}

func logPodDiagnostics(ctx context.Context, apiClient *clients.Settings, namespace string) {
	pods, err := apiClient.K8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("DRA in-cluster build diagnostics: pod listing failed in %s: %v", namespace, err)

		return
	}

	for _, pod := range pods.Items {
		if !isBuildRelatedPod(pod) {
			continue
		}

		klog.Infof("DRA in-cluster build diagnostics: pod %s/%s phase=%s reason=%s message=%s",
			namespace, pod.Name, pod.Status.Phase, pod.Status.Reason, redactDiagnosticText(pod.Status.Message))

		for _, status := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
			state := containerState(status.State)
			previous := containerState(status.LastTerminationState)
			klog.Infof("DRA in-cluster build diagnostics: pod %s/%s container=%s ready=%t restarts=%d state=%s previous=%s",
				namespace, pod.Name, status.Name, status.Ready, status.RestartCount,
				redactDiagnosticText(state), redactDiagnosticText(previous))
		}

		if isBuildLogPod(pod) {
			logPodContainerLogs(ctx, apiClient, namespace, pod)
		}
	}
}

func logDynamicResource(ctx context.Context, apiClient *clients.Settings,
	gvr schema.GroupVersionResource, namespace string) {
	resources, err := apiClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "the server could not find the requested resource") ||
			strings.Contains(err.Error(), "the server has asked for the client to provide credentials") {
			return
		}

		klog.Errorf("DRA in-cluster build diagnostics: %s listing failed in %s: %v",
			gvr.Resource, namespace, err)

		return
	}

	for _, item := range resources.Items {
		logJSON(fmt.Sprintf("DRA in-cluster build diagnostics: %s/%s", gvr.Resource, item.GetName()),
			sanitizeDiagnosticValue(map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": item.GetName(), "namespace": item.GetNamespace(),
				},
				"spec": item.Object["spec"], "status": item.Object["status"],
			}))
	}
}

func logPodContainerLogs(ctx context.Context, apiClient *clients.Settings, namespace string, pod corev1.Pod) {
	tailLines := diagnosticLogLines
	containerNames := make([]string, 0, len(pod.Spec.InitContainers)+len(pod.Spec.Containers))

	for _, container := range append(pod.Spec.InitContainers, pod.Spec.Containers...) {
		containerNames = append(containerNames, container.Name)
	}

	for _, previous := range []bool{false, true} {
		for _, containerName := range containerNames {
			options := &corev1.PodLogOptions{Previous: previous, TailLines: &tailLines}

			if len(containerNames) > 1 {
				options.Container = containerName
			}

			stream, err := apiClient.K8sClient.CoreV1().Pods(namespace).GetLogs(pod.Name, options).Stream(ctx)
			if err != nil {
				continue
			}

			logs, readErr := io.ReadAll(stream)
			closeErr := stream.Close()

			if readErr != nil {
				klog.Errorf("DRA in-cluster build diagnostics: logs read failed for %s/%s container=%s previous=%t: %v",
					namespace, pod.Name, containerName, previous, readErr)

				continue
			}

			if closeErr != nil {
				klog.Errorf("DRA in-cluster build diagnostics: logs close failed for %s/%s container=%s previous=%t: %v",
					namespace, pod.Name, containerName, previous, closeErr)
			}

			klog.Infof("DRA in-cluster build diagnostics: pod %s/%s container=%s previous=%t logs:\n%s",
				namespace, pod.Name, containerName, previous, redactDiagnosticText(string(logs)))
		}
	}
}

func logEventDiagnostics(ctx context.Context, apiClient *clients.Settings, namespace string) {
	events, err := apiClient.K8sClient.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("DRA in-cluster build diagnostics: Event listing failed in %s: %v", namespace, err)

		return
	}

	for _, event := range events.Items {
		klog.Infof("DRA in-cluster build diagnostics: event %s/%s type=%s reason=%s involved=%s/%s message=%s",
			namespace, event.Name, event.Type, event.Reason, event.InvolvedObject.Kind,
			event.InvolvedObject.Name, redactDiagnosticText(event.Message))
	}
}

func isBuildRelatedPod(pod corev1.Pod) bool {
	name := strings.ToLower(pod.Name)

	return strings.Contains(name, "build") || strings.Contains(name, "kmm") ||
		strings.Contains(name, params.DefaultDeviceConfigName) || strings.Contains(name, "pull")
}

func isBuildLogPod(pod corev1.Pod) bool {
	name := strings.ToLower(pod.Name)

	return strings.Contains(name, "build") || strings.Contains(name, "pull")
}

func containerState(state corev1.ContainerState) string {
	switch {
	case state.Waiting != nil:
		return fmt.Sprintf("waiting(reason=%s message=%s)", state.Waiting.Reason, state.Waiting.Message)
	case state.Running != nil:
		return "running"
	case state.Terminated != nil:
		return fmt.Sprintf("terminated(reason=%s exit=%d message=%s)",
			state.Terminated.Reason, state.Terminated.ExitCode, state.Terminated.Message)
	default:
		return "unknown"
	}
}

func logJSON(label string, value interface{}) {
	encoded, err := json.Marshal(value)
	if err != nil {
		klog.Errorf("%s: JSON encoding failed: %v", label, err)

		return
	}

	klog.Infof("%s: %s", label, encoded)
}

func sanitizeDiagnosticValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(typed))
		sensitiveNamedValue := false

		for _, nameKey := range []string{"name", "key"} {
			if name, ok := typed[nameKey].(string); ok && sensitiveDiagnosticKey.MatchString(name) {
				sensitiveNamedValue = true
			}
		}

		for key, nested := range typed {
			if sensitiveDiagnosticKey.MatchString(key) ||
				(sensitiveNamedValue &&
					(key == "value" || key == "valueFrom" || key == "data" || key == "stringData")) {
				result[key] = "<redacted>"

				continue
			}

			result[key] = sanitizeDiagnosticValue(nested)
		}

		return result
	case []interface{}:
		result := make([]interface{}, len(typed))
		for index, nested := range typed {
			result[index] = sanitizeDiagnosticValue(nested)
		}

		return result
	default:
		return value
	}
}

func sanitizeDiagnosticObject(value interface{}) interface{} {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "<unavailable>"
	}

	var generic interface{}
	if err := json.Unmarshal(encoded, &generic); err != nil {
		return "<unavailable>"
	}

	return sanitizeDiagnosticValue(generic)
}

func redactDiagnosticText(value string) string {
	return sensitiveDiagnosticText.ReplaceAllStringFunc(value, func(match string) string {
		groups := sensitiveDiagnosticText.FindStringSubmatch(match)
		for _, prefix := range groups[1:] {
			if prefix != "" {
				return prefix + "<redacted>"
			}
		}

		return "<redacted>"
	})
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	// ConfigMap key order is not semantically meaningful; keep diagnostics
	// stable without exposing ConfigMap values.
	sort.Strings(keys)

	return keys
}
