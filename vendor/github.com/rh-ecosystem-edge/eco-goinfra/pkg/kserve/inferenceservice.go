package kserve

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	kservev1beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kserve/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// InferenceServiceBuilder provides struct for the InferenceService object
// containing connection to the cluster and the InferenceService definition.
type InferenceServiceBuilder struct {
	Definition *kservev1beta1.InferenceService
	Object     *kservev1beta1.InferenceService
	apiClient  *clients.Settings
	errorMsg   string
}

// NewInferenceServiceBuilder creates a new instance of InferenceServiceBuilder.
func NewInferenceServiceBuilder(
	apiClient *clients.Settings,
	name, namespace string) *InferenceServiceBuilder {
	klog.V(100).Infof(
		"Initializing new InferenceService structure with name: %s, namespace: %s",
		name, namespace)

	if apiClient == nil {
		klog.V(100).Infof("The apiClient is empty")

		return nil
	}

	err := apiClient.AttachScheme(kservev1beta1.AddToScheme)
	if err != nil {
		klog.V(100).Infof("Failed to add kserve v1beta1 scheme to client schemes")

		return nil
	}

	builder := &InferenceServiceBuilder{
		apiClient: apiClient,
		Definition: &kservev1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}

	if name == "" {
		klog.V(100).Infof("The name of the InferenceService is empty")

		builder.errorMsg = "InferenceService 'name' cannot be empty"

		return builder
	}

	if namespace == "" {
		klog.V(100).Infof("The namespace of the InferenceService is empty")

		builder.errorMsg = "InferenceService 'namespace' cannot be empty"

		return builder
	}

	return builder
}

// WithModelFormat sets the model format name.
func (builder *InferenceServiceBuilder) WithModelFormat(name string) *InferenceServiceBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting InferenceService %s model format: %s",
		builder.Definition.Name, name)

	if name == "" {
		builder.errorMsg = "InferenceService 'modelFormat' cannot be empty"

		return builder
	}

	builder.Definition.Spec.Predictor.Model.ModelFormat = kservev1beta1.ModelFormat{Name: name}

	return builder
}

// WithRuntime sets the serving runtime name.
func (builder *InferenceServiceBuilder) WithRuntime(runtimeName string) *InferenceServiceBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting InferenceService %s runtime: %s",
		builder.Definition.Name, runtimeName)

	if runtimeName == "" {
		builder.errorMsg = "InferenceService 'runtime' cannot be empty"

		return builder
	}

	builder.Definition.Spec.Predictor.Model.Runtime = &runtimeName

	return builder
}

// WithStorageURI sets the model storage URI.
func (builder *InferenceServiceBuilder) WithStorageURI(uri string) *InferenceServiceBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting InferenceService %s storageUri: %s",
		builder.Definition.Name, uri)

	if uri == "" {
		builder.errorMsg = "InferenceService 'storageUri' cannot be empty"

		return builder
	}

	builder.Definition.Spec.Predictor.Model.StorageURI = &uri

	return builder
}

// WithServiceAccountName sets the predictor service account.
func (builder *InferenceServiceBuilder) WithServiceAccountName(serviceAccountName string) *InferenceServiceBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting InferenceService %s serviceAccountName: %s",
		builder.Definition.Name, serviceAccountName)

	builder.Definition.Spec.Predictor.ServiceAccountName = serviceAccountName

	return builder
}

// WithResources sets the resource requests and limits.
func (builder *InferenceServiceBuilder) WithResources(
	requests, limits corev1.ResourceList) *InferenceServiceBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting InferenceService %s resources",
		builder.Definition.Name)

	builder.Definition.Spec.Predictor.Model.Resources = corev1.ResourceRequirements{
		Requests: requests,
		Limits:   limits,
	}

	return builder
}

// WithNeuronResources sets Neuron device count and memory resources.
func (builder *InferenceServiceBuilder) WithNeuronResources(
	neuronDevices int64, memoryRequest, memoryLimit string) *InferenceServiceBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting InferenceService %s Neuron resources: devices=%d",
		builder.Definition.Name, neuronDevices)

	neuronQuantity, err := resource.ParseQuantity(fmt.Sprintf("%d", neuronDevices))
	if err != nil {
		builder.errorMsg = fmt.Sprintf("failed to parse neuron device quantity: %v", err)

		return builder
	}

	memReq, err := resource.ParseQuantity(memoryRequest)
	if err != nil {
		builder.errorMsg = fmt.Sprintf("failed to parse memory request %q: %v", memoryRequest, err)

		return builder
	}

	memLim, err := resource.ParseQuantity(memoryLimit)
	if err != nil {
		builder.errorMsg = fmt.Sprintf("failed to parse memory limit %q: %v", memoryLimit, err)

		return builder
	}

	builder.Definition.Spec.Predictor.Model.Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			"aws.amazon.com/neuron": neuronQuantity,
			corev1.ResourceMemory:   memReq,
		},
		Limits: corev1.ResourceList{
			"aws.amazon.com/neuron": neuronQuantity,
			corev1.ResourceMemory:   memLim,
		},
	}

	return builder
}

// WithAnnotation adds an annotation to the InferenceService.
func (builder *InferenceServiceBuilder) WithAnnotation(
	key, value string) *InferenceServiceBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting InferenceService %s annotation %s=%s",
		builder.Definition.Name, key, value)

	if builder.Definition.Annotations == nil {
		builder.Definition.Annotations = make(map[string]string)
	}

	builder.Definition.Annotations[key] = value

	return builder
}

// WithEnv adds environment variables to the model spec.
func (builder *InferenceServiceBuilder) WithEnv(
	envVars []corev1.EnvVar) *InferenceServiceBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting InferenceService %s env vars",
		builder.Definition.Name)

	builder.Definition.Spec.Predictor.Model.Env = append(
		builder.Definition.Spec.Predictor.Model.Env, envVars...)

	return builder
}

// PullInferenceService retrieves an existing InferenceService from the cluster.
func PullInferenceService(
	apiClient *clients.Settings, name, namespace string) (*InferenceServiceBuilder, error) {
	klog.V(100).Infof("Pulling InferenceService %s from namespace %s", name, namespace)

	if apiClient == nil {
		return nil, fmt.Errorf("inferenceService 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(kservev1beta1.AddToScheme)
	if err != nil {
		return nil, err
	}

	builder := &InferenceServiceBuilder{
		apiClient: apiClient,
		Definition: &kservev1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}

	if name == "" {
		return nil, fmt.Errorf("inferenceService 'name' cannot be empty")
	}

	if namespace == "" {
		return nil, fmt.Errorf("inferenceService 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf(
			"inferenceService %s does not exist in namespace %s", name, namespace)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get retrieves the InferenceService from the cluster.
func (builder *InferenceServiceBuilder) Get() (*kservev1beta1.InferenceService, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting InferenceService %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	isvc := &kservev1beta1.InferenceService{}

	err := builder.apiClient.Get(
		logging.DiscardContext(),
		goclient.ObjectKey{
			Name:      builder.Definition.Name,
			Namespace: builder.Definition.Namespace,
		},
		isvc)
	if err != nil {
		return nil, err
	}

	return isvc, nil
}

// Exists checks whether the InferenceService exists in the cluster.
func (builder *InferenceServiceBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	var err error

	builder.Object, err = builder.Get()

	return err == nil
}

// Create builds the InferenceService in the cluster.
func (builder *InferenceServiceBuilder) Create() (*InferenceServiceBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating InferenceService %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		err := builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err != nil {
			return builder, err
		}

		builder.Object = builder.Definition
	}

	return builder, nil
}

// Delete removes the InferenceService from the cluster.
func (builder *InferenceServiceBuilder) Delete() (*InferenceServiceBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting InferenceService %s from namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("cannot delete InferenceService: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// GetURL returns the inference URL from the InferenceService status.
func (builder *InferenceServiceBuilder) GetURL() (string, error) {
	if valid, err := builder.validate(); !valid {
		return "", err
	}

	isvc, err := builder.Get()
	if err != nil {
		return "", err
	}

	if isvc.Status.URL == "" {
		return "", fmt.Errorf("inferenceService %s has no URL in status",
			builder.Definition.Name)
	}

	return isvc.Status.URL, nil
}

// IsReady checks if the InferenceService has a Ready condition with status True.
func (builder *InferenceServiceBuilder) IsReady() (bool, error) {
	if valid, err := builder.validate(); !valid {
		return false, err
	}

	isvc, err := builder.Get()
	if err != nil {
		return false, err
	}

	for _, condition := range isvc.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == kservev1beta1.ConditionTrue {
			return true, nil
		}
	}

	return false, nil
}

// WaitUntilReady waits until the InferenceService reaches Ready state.
func (builder *InferenceServiceBuilder) WaitUntilReady(
	timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Waiting for InferenceService %s to become Ready (timeout: %s)",
		builder.Definition.Name, timeout)

	return wait.PollUntilContextTimeout(
		context.TODO(), 15*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			ready, err := builder.IsReady()
			if err != nil {
				klog.V(100).Infof("Error checking InferenceService readiness: %v", err)

				return false, nil
			}

			return ready, nil
		})
}

// validate checks that the builder is properly configured.
func (builder *InferenceServiceBuilder) validate() (bool, error) {
	resourceCRD := "InferenceService"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		klog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		klog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message: %s",
			resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
