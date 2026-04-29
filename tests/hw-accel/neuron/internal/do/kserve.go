package do

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// KServeInferenceConfig holds configuration for KServe inference requests.
type KServeInferenceConfig struct {
	InferenceServiceURL string
	Namespace           string
	ModelName           string
	Timeout             time.Duration
}

// ExecuteKServeInference sends an inference request to a KServe InferenceService endpoint.
// It creates a temporary curl pod, sends the request, and retries until success or timeout.
func ExecuteKServeInference(apiClient *clients.Settings, config KServeInferenceConfig) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	jsonBody, err := buildInferenceRequestBody(config.ModelName)
	if err != nil {
		return "", fmt.Errorf("failed to marshal inference request: %w", err)
	}

	podName := "kserve-inference-test-curl"

	if err := ensureCurlPod(ctx, apiClient, podName, config.Namespace); err != nil {
		return "", fmt.Errorf("failed to create curl pod: %w", err)
	}

	defer cleanupCurlPod(apiClient, podName, config.Namespace)

	endpoint := fmt.Sprintf("%s/v1/chat/completions", config.InferenceServiceURL)

	curlCmd := []string{
		"curl", "-sk",
		"-X", "POST",
		endpoint,
		"-H", "Content-Type: application/json",
		"-d", string(jsonBody),
		"--max-time", "60",
	}

	const retryInterval = 30 * time.Second

	var inferenceResult string

	pollErr := wait.PollUntilContextTimeout(
		ctx, retryInterval, config.Timeout, true,
		func(pollCtx context.Context) (bool, error) {
			execCtx, execCancel := context.WithTimeout(pollCtx, 90*time.Second)
			defer execCancel()

			response, execErr := executeInPod(execCtx, apiClient, podName, config.Namespace, "curl", curlCmd)
			if execErr != nil {
				klog.V(params.NeuronLogLevel).Infof(
					"KServe inference attempt failed (model may still be compiling): %v", execErr)

				return false, nil
			}

			content, extractErr := extractInferenceContent(response)
			if extractErr != nil {
				klog.V(params.NeuronLogLevel).Infof(
					"KServe inference response not ready: %v", extractErr)

				return false, nil
			}

			inferenceResult = content

			return true, nil
		})
	if pollErr != nil {
		return "", fmt.Errorf("KServe inference failed after %v: %w", config.Timeout, pollErr)
	}

	return inferenceResult, nil
}

// ensureCurlPod creates a long-running curl pod for executing inference requests.
func ensureCurlPod(ctx context.Context, apiClient *clients.Settings, name, namespace string) error {
	_, err := apiClient.Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"sidecar.istio.io/inject": "false",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "curl",
					Image:   "registry.access.redhat.com/ubi9/ubi-minimal:latest",
					Command: []string{"sleep", "3600"},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	_, err = apiClient.Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	return wait.PollUntilContextTimeout(ctx, 5*time.Second, 2*time.Minute, true,
		func(pollCtx context.Context) (bool, error) {
			p, getErr := apiClient.Pods(namespace).Get(pollCtx, name, metav1.GetOptions{})
			if getErr != nil {
				return false, nil
			}

			return p.Status.Phase == corev1.PodRunning, nil
		})
}

// cleanupCurlPod removes the temporary curl pod.
func cleanupCurlPod(apiClient *clients.Settings, name, namespace string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := apiClient.Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		klog.V(params.NeuronLogLevel).Infof("Failed to delete curl pod: %v", err)
	}
}

// ParseInferenceResponse parses a raw chat completions JSON response.
func ParseInferenceResponse(response string) (string, error) {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w, raw: %s", err, response)
	}

	if errMsg, ok := result["error"]; ok {
		raw, _ := json.Marshal(errMsg)

		return "", fmt.Errorf("inference returned error: %s", string(raw))
	}

	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")

	if err := enc.Encode(result); err != nil {
		return fmt.Sprintf("%v", result), nil
	}

	return buf.String(), nil
}
