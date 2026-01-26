package do

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// VLLMDeploymentConfig holds configuration for creating a vLLM deployment.
type VLLMDeploymentConfig struct {
	Name             string
	Namespace        string
	Image            string
	ModelName        string
	ServedModelName  string
	Port             int
	Labels           map[string]string
	PVCName          string
	HFSecretName     string
	TensorParallel   int
	MaxModelLen      int
	MaxNumSeqs       int
	NeuronDevices    int
	MemoryLimit      string
	MemoryRequest    string
	StorageClassName string
	StorageSize      string
}

// DefaultVLLMConfig returns default configuration for vLLM deployment.
func DefaultVLLMConfig(namespace string) VLLMDeploymentConfig {
	return VLLMDeploymentConfig{
		Name:             "neuron-vllm-test",
		Namespace:        namespace,
		Image:            "public.ecr.aws/neuron/pytorch-inference-vllm-neuronx:0.7.2-neuronx-py310-sdk2.24.1-ubuntu22.04",
		ModelName:        "meta-llama/Llama-3.1-8B-Instruct",
		ServedModelName:  "meta-llama/Llama-3.1-8B-Instruct",
		Port:             8000,
		Labels:           map[string]string{"app": "neuron-vllm-test"},
		PVCName:          "model-cache",
		HFSecretName:     "hf-token",
		TensorParallel:   2,
		MaxModelLen:      4096,
		MaxNumSeqs:       4,
		NeuronDevices:    1,
		MemoryLimit:      "100Gi",
		MemoryRequest:    "10Gi",
		StorageClassName: "gp3-csi",
		StorageSize:      "50Gi",
	}
}

// CreateVLLMPVC creates a PersistentVolumeClaim for model storage.
func CreateVLLMPVC(config VLLMDeploymentConfig) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.PVCName,
			Namespace: config.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(config.StorageSize),
				},
			},
			StorageClassName: &config.StorageClassName,
		},
	}
}

// CreateHFTokenSecret creates a secret for Hugging Face authentication.
func CreateHFTokenSecret(namespace, secretName, token string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"token": token,
		},
	}
}

// buildVLLMVolumes creates the volumes for the vLLM deployment.
func buildVLLMVolumes(pvcName string) []corev1.Volume {
	shmSizeLimit := resource.MustParse("2Gi")

	return []corev1.Volume{
		{
			Name: "model-volume",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		},
		{
			Name: "shm",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium:    corev1.StorageMediumMemory,
					SizeLimit: &shmSizeLimit,
				},
			},
		},
	}
}

// buildInitContainer creates the init container for model download.
func buildInitContainer(modelName, hfSecretName string) corev1.Container {
	initScript := fmt.Sprintf(`set -ex
echo "--- SCRIPT STARTED ---"
echo "--- CHECKING /model DIRECTORY PERMISSIONS AND CONTENTS ---"
# Only pull if /model is empty
if [ ! -f "/model/config.json" ]; then
  export PYTHONUSERBASE="/tmp/pip"
  pip install --no-cache-dir --user "huggingface_hub>=1.0"
  echo "Pulling model %s ..."
  # The hf script is installed to /tmp/pip/bin/hf
  /tmp/pip/bin/hf download %s --local-dir /model
else
  echo "Model already present, skipping model pull"
fi`, modelName, modelName)

	return corev1.Container{
		Name:  "fetch-model",
		Image: "python:3.11-slim",
		Env: []corev1.EnvVar{
			{Name: "DOCKER_CONFIG", Value: "/auth"},
			{Name: "HF_HOME", Value: "/model"},
			{
				Name: "HF_TOKEN",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: hfSecretName},
						Key:                  "token",
					},
				},
			},
		},
		Command:      []string{"/bin/sh", "-c"},
		Args:         []string{initScript},
		VolumeMounts: []corev1.VolumeMount{{Name: "model-volume", MountPath: "/model"}},
	}
}

// buildVLLMContainer creates the main vLLM container.
func buildVLLMContainer(config VLLMDeploymentConfig) corev1.Container {
	return corev1.Container{
		Name:            "vllm",
		Image:           config.Image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		WorkingDir:      "/model",
		Env: []corev1.EnvVar{
			{Name: "VLLM_SERVER_DEV_MODE", Value: "1"},
			{Name: "NEURON_CACHE_URL", Value: "/model/neuron_cache"},
		},
		Command: []string{"python", "-m", "vllm.entrypoints.openai.api_server"},
		Args: []string{
			fmt.Sprintf("--port=%d", config.Port),
			"--model=/model",
			fmt.Sprintf("--served-model-name=%s", config.ServedModelName),
			fmt.Sprintf("--tensor-parallel-size=%d", config.TensorParallel),
			"--device", "neuron",
			fmt.Sprintf("--max-num-seqs=%d", config.MaxNumSeqs),
			fmt.Sprintf("--max-model-len=%d", config.MaxModelLen),
		},
		Ports: []corev1.ContainerPort{{ContainerPort: int32(config.Port), Protocol: corev1.ProtocolTCP}},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory:                        resource.MustParse(config.MemoryLimit),
				corev1.ResourceName(params.NeuronCapacityID): resource.MustParse(fmt.Sprintf("%d", config.NeuronDevices)),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceMemory:                        resource.MustParse(config.MemoryRequest),
				corev1.ResourceName(params.NeuronCapacityID): resource.MustParse(fmt.Sprintf("%d", config.NeuronDevices)),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "model-volume", MountPath: "/model"},
			{Name: "shm", MountPath: "/dev/shm"},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt(config.Port)},
			},
			InitialDelaySeconds: 600,
			PeriodSeconds:       30,
			TimeoutSeconds:      10,
			FailureThreshold:    60,
		},
	}
}

// CreateVLLMDeployment creates a vLLM Deployment.
func CreateVLLMDeployment(config VLLMDeploymentConfig) *appsv1.Deployment {
	replicas := int32(1)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.Name,
			Namespace: config.Namespace,
			Labels:    config.Labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: config.Labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: config.Labels},
				Spec: corev1.PodSpec{
					SchedulerName:      params.SchedulerDeploymentName,
					ServiceAccountName: "default",
					Volumes:            buildVLLMVolumes(config.PVCName),
					InitContainers:     []corev1.Container{buildInitContainer(config.ModelName, config.HFSecretName)},
					Containers:         []corev1.Container{buildVLLMContainer(config)},
					RestartPolicy:      corev1.RestartPolicyAlways,
				},
			},
		},
	}
}

// VLLMServiceConfig holds configuration for creating a vLLM service.
type VLLMServiceConfig struct {
	Name      string
	Namespace string
	Port      int
	Labels    map[string]string
}

// CreateVLLMService creates a vLLM service specification.
func CreateVLLMService(config VLLMServiceConfig) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.Name,
			Namespace: config.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: config.Labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "vllm-port",
					Port:       80,
					TargetPort: intstr.FromInt(config.Port),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// InferenceConfig holds configuration for executing an inference request.
type InferenceConfig struct {
	ServiceName      string
	Namespace        string
	Port             int
	ModelName        string
	Timeout          time.Duration
	PodLabelSelector string
}

// buildInferenceRequestBody creates the JSON request body for inference.
func buildInferenceRequestBody(modelName string) ([]byte, error) {
	requestBody := map[string]interface{}{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "user", "content": "Hello, how are you?"},
		},
		"max_tokens":  50,
		"temperature": 0.7,
	}

	return json.Marshal(requestBody)
}

// findRunningVLLMPod finds a running vLLM pod in the specified namespace.
func findRunningVLLMPod(ctx context.Context, apiClient *clients.Settings,
	namespace, labelSelector string) (string, error) {
	podList, err := apiClient.Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil || len(podList.Items) == 0 {
		return "", fmt.Errorf("no vLLM pods found in namespace %s with selector %s", namespace, labelSelector)
	}

	for _, p := range podList.Items {
		if p.Status.Phase == corev1.PodRunning {
			return p.Name, nil
		}
	}

	return "", fmt.Errorf("no running vLLM pods found with selector %s", labelSelector)
}

// executeInPod runs a command inside a pod and returns stdout.
// The command is passed as a slice to avoid shell injection vulnerabilities.
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

	if stdout.String() == "" {
		return "", fmt.Errorf("empty response, stderr: %s", stderr.String())
	}

	return stdout.String(), nil
}

// extractInferenceContent parses the chat completions response and extracts content.
func extractInferenceContent(response string) (string, error) {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w, raw: %s", err, response)
	}

	if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].(string); ok {
					return content, nil
				}
			}
		}
	}

	return fmt.Sprintf("%v", result), nil
}

// ExecuteInferenceFromCluster executes an inference request from within the cluster.
func ExecuteInferenceFromCluster(apiClient *clients.Settings, config InferenceConfig) (string, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	jsonBody, err := buildInferenceRequestBody(config.ModelName)
	if err != nil {
		return "", fmt.Errorf("failed to marshal inference request: %w", err)
	}

	// Build service URL with explicit port
	serviceURL := fmt.Sprintf("http://%s.%s.svc.cluster.local:80/v1/chat/completions",
		config.ServiceName, config.Namespace)

	// Build curl command as arg slice to avoid shell injection
	curlCmd := []string{
		"curl",
		"-s",
		"-X", "POST",
		serviceURL,
		"-H", "Content-Type: application/json",
		"-d", string(jsonBody),
	}

	// Use configurable label selector, default to "app=neuron-vllm-test" if empty
	labelSelector := config.PodLabelSelector
	if labelSelector == "" {
		labelSelector = "app=neuron-vllm-test"
	}

	targetPod, err := findRunningVLLMPod(ctx, apiClient, config.Namespace, labelSelector)
	if err != nil {
		return "", err
	}

	response, err := executeInPod(ctx, apiClient, targetPod, config.Namespace, "vllm", curlCmd)
	if err != nil {
		return "", fmt.Errorf("inference request failed: %w", err)
	}

	return extractInferenceContent(response)
}
