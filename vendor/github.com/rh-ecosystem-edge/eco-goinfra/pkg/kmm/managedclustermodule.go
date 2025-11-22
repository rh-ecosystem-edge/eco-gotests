package kmm

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	mcmV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm-hub/v1beta1"
	moduleV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ManagedClusterModuleBuilder provides struct for the managedclustermodule object containing connection
// to cluster and the managedclustermodule definitions.
type ManagedClusterModuleBuilder struct {
	Definition *mcmV1Beta1.ManagedClusterModule
	Object     *mcmV1Beta1.ManagedClusterModule
	errorMsg   string
	apiClient  goclient.Client
}

// ManagedClusterModuleAdditionalOptions additional options for managedclustermodule object.
type ManagedClusterModuleAdditionalOptions func(builder *ManagedClusterModuleBuilder) (
	*ManagedClusterModuleBuilder, error)

// NewManagedClusterModuleBuilder creates a new instance of ManagedClusterModuleBuilder.
func NewManagedClusterModuleBuilder(apiClient *clients.Settings, name, nsname string) *ManagedClusterModuleBuilder {
	klog.V(100).Infof(
		"Initializing new ManagedClusterModule structure with following params: %s, %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil
	}

	err := apiClient.AttachScheme(mcmV1Beta1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add module v1beta1 scheme to client schemes")

		return nil
	}

	builder := &ManagedClusterModuleBuilder{
		apiClient: apiClient.Client,
		Definition: &mcmV1Beta1.ManagedClusterModule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the ManagedClusterModule is empty")

		builder.errorMsg = "managedClusterModule 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the ManagedClusterModule is empty")

		builder.errorMsg = "managedClusterModule 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// WithModuleSpec sets the ModuleSpec.
func (builder *ManagedClusterModuleBuilder) WithModuleSpec(
	moduleSpec moduleV1Beta1.ModuleSpec) *ManagedClusterModuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.ModuleSpec = moduleSpec

	return builder
}

// WithSpokeNamespace sets the namespace where the module will be deployed on the spoke.
func (builder *ManagedClusterModuleBuilder) WithSpokeNamespace(
	spokeNamespace string) *ManagedClusterModuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if spokeNamespace == "" {
		builder.errorMsg = "invalid 'spokeNamespace' argument cannot be nil"

		return builder
	}

	builder.Definition.Spec.SpokeNamespace = spokeNamespace

	return builder
}

// WithSelector sets the selector for the managedclustermodule object.
func (builder *ManagedClusterModuleBuilder) WithSelector(
	selector map[string]string) *ManagedClusterModuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(selector) == 0 {
		builder.errorMsg = "invalid 'selector' argument cannot be empty"

		return builder
	}

	builder.Definition.Spec.Selector = selector

	return builder
}

// WithOptions creates ManagedClusterModule with generic mutation options.
func (builder *ManagedClusterModuleBuilder) WithOptions(
	options ...ManagedClusterModuleAdditionalOptions) *ManagedClusterModuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting ManagedClusterModule additional options")

	for _, option := range options {
		if option != nil {
			builder, err := option(builder)
			if err != nil {
				klog.V(100).Info("Error occurred in mutation function")

				builder.errorMsg = err.Error()

				return builder
			}
		}
	}

	return builder
}

// PullManagedClusterModule pulls existing module from cluster.
func PullManagedClusterModule(apiClient *clients.Settings, name, nsname string) (*ManagedClusterModuleBuilder, error) {
	klog.V(100).Infof("Pulling existing module name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("managedclustermodule 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(mcmV1Beta1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add module v1beta1 scheme to client schemes")

		return nil, err
	}

	builder := &ManagedClusterModuleBuilder{
		apiClient: apiClient.Client,
		Definition: &mcmV1Beta1.ManagedClusterModule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the managedclustermodule is empty")

		return nil, fmt.Errorf("managedclustermodule 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the managedclustermodule is empty")

		return nil, fmt.Errorf("managedclustermodule 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("managedclustermodule object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create builds managedclustermodule in the cluster and stores object in struct.
func (builder *ManagedClusterModuleBuilder) Create() (*ManagedClusterModuleBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating managedclustermodule %s in namespace %s",
		builder.Definition.Name,
		builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Update modifies an existing managedclustermodule on the cluster.
func (builder *ManagedClusterModuleBuilder) Update() (*ManagedClusterModuleBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating managedclustermodule %s in namespace %s",
		builder.Definition.Name,
		builder.Definition.Namespace)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Exists checks whether the given managedclustermodule exists.
func (builder *ManagedClusterModuleBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if managedclustermodule %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Delete removes the managedclustermodule.
func (builder *ManagedClusterModuleBuilder) Delete() (*ManagedClusterModuleBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting managedclustermodule %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Info("managedclustermodule cannot be deleted because it does not exist")

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, err
	}

	builder.Object = nil

	return builder, nil
}

// Get fetches the defined managedclustermodule from the cluster.
func (builder *ManagedClusterModuleBuilder) Get() (*mcmV1Beta1.ManagedClusterModule, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting managedclustermodule %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	mcm := &mcmV1Beta1.ManagedClusterModule{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, mcm)
	if err != nil {
		return nil, err
	}

	return mcm, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ManagedClusterModuleBuilder) validate() (bool, error) {
	resourceCRD := "ManagedClusterModule"

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
