package clusterlogging

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	lokiv1 "github.com/grafana/loki/operator/apis/loki/v1"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// LokiStackBuilder provides struct for the lokiStack object.
type LokiStackBuilder struct {
	// LokiStack definition. Used to create lokiStack object with minimum set of required elements.
	Definition *lokiv1.LokiStack
	// Created lokiStack object on the cluster.
	Object *lokiv1.LokiStack
	// api client to interact with the cluster.
	apiClient goclient.Client
	// errorMsg is processed before lokiStack object is created.
	errorMsg string
}

// NewLokiStackBuilder creates new instance of builder.
func NewLokiStackBuilder(
	apiClient *clients.Settings, name, nsname string) *LokiStackBuilder {
	klog.V(100).Infof("Initializing new lokiStack structure with the following params: name: %s, namespace: %s",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("lokiStack 'apiClient' cannot be empty")

		return nil
	}

	err := apiClient.AttachScheme(lokiv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add lokv1 scheme to client schemes")

		return nil
	}

	builder := &LokiStackBuilder{
		apiClient: apiClient.Client,
		Definition: &lokiv1.LokiStack{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the lokiStack is empty")

		builder.errorMsg = "lokiStack 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The nsname of the lokiStack is empty")

		builder.errorMsg = "lokiStack 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullLokiStack retrieves an existing lokiStack object from the cluster.
func PullLokiStack(apiClient *clients.Settings, name, nsname string) (*LokiStackBuilder, error) {
	klog.V(100).Infof(
		"Pulling lokiStack object name: %s in namespace: %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("lokiStack 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(lokiv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add lokv1 scheme to client schemes")

		return nil, err
	}

	builder := &LokiStackBuilder{
		apiClient: apiClient.Client,
		Definition: &lokiv1.LokiStack{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the lokiStack is empty")

		return nil, fmt.Errorf("lokiStack 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the lokiStack is empty")

		return nil, fmt.Errorf("lokiStack 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("lokiStack object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns lokiStack object if found.
func (builder *LokiStackBuilder) Get() (*lokiv1.LokiStack, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting lokiStack %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	lokiStackObj := &lokiv1.LokiStack{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, lokiStackObj)
	if err != nil {
		return nil, err
	}

	return lokiStackObj, err
}

// Create makes a lokiStack in the cluster and stores the created object in struct.
func (builder *LokiStackBuilder) Create() (*LokiStackBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the lokiStack %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Delete removes lokiStack from a cluster.
func (builder *LokiStackBuilder) Delete() (*LokiStackBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Deleting the lokiStack %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("lokiStack %s in namespace %s cannot be deleted"+
			" because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete lokiStack: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Exists checks whether the given lokiStack exists.
func (builder *LokiStackBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if lokiStack %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Update renovates the existing lokiStack object with lokiStack definition in builder.
func (builder *LokiStackBuilder) Update() (*LokiStackBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating lokiStack %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("%v", msg.FailToUpdateError("lokiStack", builder.Definition.Name, builder.Definition.Namespace))

		return nil, err
	}

	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// WithSize sets the lokiStack operator's size.
func (builder *LokiStackBuilder) WithSize(
	size lokiv1.LokiStackSizeType) *LokiStackBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting lokiStack %s in namespace %s with the size: %v",
		builder.Definition.Name, builder.Definition.Namespace, size)

	if size == "" {
		klog.V(100).Info("'size' argument cannot be empty")

		builder.errorMsg = "'size' argument cannot be empty"

		return builder
	}

	builder.Definition.Spec.Size = size

	return builder
}

// WithStorage sets the lokiStack operator's storage configuration.
func (builder *LokiStackBuilder) WithStorage(
	storage lokiv1.ObjectStorageSpec) *LokiStackBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting lokiStack %s in namespace %s with the storage config: %v",
		builder.Definition.Name, builder.Definition.Namespace, storage)

	builder.Definition.Spec.Storage = storage

	return builder
}

// WithStorageClassName sets the lokiStack operator's storage class name configuration.
func (builder *LokiStackBuilder) WithStorageClassName(
	storageClassName string) *LokiStackBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting lokiStack %s in namespace %s with the storage class name config: %v",
		builder.Definition.Name, builder.Definition.Namespace, storageClassName)

	if storageClassName == "" {
		klog.V(100).Info("'storageClassName' argument cannot be empty")

		builder.errorMsg = "'storageClassName' argument cannot be empty"

		return builder
	}

	builder.Definition.Spec.StorageClassName = storageClassName

	return builder
}

// WithTenants sets the lokiStack operator's tenants configuration.
func (builder *LokiStackBuilder) WithTenants(
	tenants lokiv1.TenantsSpec) *LokiStackBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting lokiStack %s in namespace %s with the tenants config: %v",
		builder.Definition.Name, builder.Definition.Namespace, tenants)

	builder.Definition.Spec.Tenants = &tenants

	return builder
}

// WithRules sets the lokiStack operator's rules configuration.
func (builder *LokiStackBuilder) WithRules(
	rules lokiv1.RulesSpec) *LokiStackBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting lokiStack %s in namespace %s with the rules config: %v",
		builder.Definition.Name, builder.Definition.Namespace, rules)

	builder.Definition.Spec.Rules = &rules

	return builder
}

// WithManagementState sets the lokiStack operator's rules configuration.
func (builder *LokiStackBuilder) WithManagementState(
	managementState lokiv1.ManagementStateType) *LokiStackBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting lokiStack %s in namespace %s with the managementState config: %v",
		builder.Definition.Name, builder.Definition.Namespace, managementState)

	builder.Definition.Spec.ManagementState = managementState

	return builder
}

// WithLimits sets the lokiStack operator's limits configuration.
func (builder *LokiStackBuilder) WithLimits(
	limits lokiv1.LimitsSpec) *LokiStackBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting lokiStack %s in namespace %s with the limits config: %v",
		builder.Definition.Name, builder.Definition.Namespace, limits)

	builder.Definition.Spec.Limits = &limits

	return builder
}

// WithTemplate sets the lokiStack operator's template configuration.
func (builder *LokiStackBuilder) WithTemplate(
	template lokiv1.LokiTemplateSpec) *LokiStackBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting lokiStack %s in namespace %s with the template config: %v",
		builder.Definition.Name, builder.Definition.Namespace, template)

	builder.Definition.Spec.Template = &template

	return builder
}

// IsReady checks for the duration of timeout if the lokiStack state is Ready.
func (builder *LokiStackBuilder) IsReady(timeout time.Duration) bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	err := wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			if !builder.Exists() {
				return false, nil
			}

			for _, condition := range builder.Definition.Status.Conditions {
				if condition.Type == "Ready" {
					if condition.Status == metav1.ConditionTrue {
						return true, nil
					}
				}
			}

			return false, nil
		})

	return err == nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *LokiStackBuilder) validate() (bool, error) {
	resourceCRD := "LokiStack"

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
		klog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
