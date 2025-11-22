package pfstatus

import (
	"context"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	pfstatustypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/pfstatus/pfstatustypes"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// PfStatusConfigurationBuilder provides struct for the PfStatusConfiguration object containing connection to
// the cluster and the PFLACPMonitor definitions.
type PfStatusConfigurationBuilder struct {
	Definition *pfstatustypes.PFLACPMonitor
	Object     *pfstatustypes.PFLACPMonitor
	apiClient  runtimeClient.Client
	errorMsg   string
}

// NewPfStatusConfigurationBuilder creates a new instance of PfStatusConfiguration.
func NewPfStatusConfigurationBuilder(
	apiClient *clients.Settings, name, nsname string) *PfStatusConfigurationBuilder {
	klog.V(100).Infof(
		"Initializing new NewPfStatusConfiguration structure with the following params: %s, %s",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("failed to initialize the apiclient is empty")

		return nil
	}

	err := apiClient.AttachScheme(pfstatustypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add pfstatus scheme to client schemes")

		return nil
	}

	builder := &PfStatusConfigurationBuilder{
		apiClient: apiClient,
		Definition: &pfstatustypes.PFLACPMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the pfStatusConfiguration is empty")

		builder.errorMsg = "pfStatusConfiguration 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the pfStatusConfiguration is empty")

		builder.errorMsg = "pfStatusConfiguration 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// Exists checks whether the given PfStatusConfiguration exists.
func (builder *PfStatusConfigurationBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof(
		"Checking if pfStatusConfiguration %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Create makes a PfStatusConfiguration in the cluster and stores the created object in struct.
func (builder *PfStatusConfigurationBuilder) Create() (*PfStatusConfigurationBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the pfStatusConfiguration %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(context.TODO(), builder.Definition)
		if err != nil {
			klog.V(100).Info("Failed to create pfStatusConfiguration")

			return nil, err
		}

		builder.Object = builder.Definition
	}

	return builder, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *PfStatusConfigurationBuilder) validate() (bool, error) {
	resourceCRD := "pflacpmonitors"

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
		klog.V(100).Infof("the %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}

// Get returns PfStatusConfiguration object if found.
func (builder *PfStatusConfigurationBuilder) Get() (*pfstatustypes.PFLACPMonitor, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Collecting pfStatusConfiguration object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	pfstatusConfig := &pfstatustypes.PFLACPMonitor{}

	err := builder.apiClient.Get(context.TODO(),
		runtimeClient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace}, pfstatusConfig)
	if err != nil {
		klog.V(100).Infof(
			"pfStatusConfiguration object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return pfstatusConfig, nil
}

// PullPfStatusConfiguration pulls existing pfStatusConfiguration from cluster.
func PullPfStatusConfiguration(
	apiClient *clients.Settings, name, nsname string) (*PfStatusConfigurationBuilder, error) {
	klog.V(100).Infof(
		"Pulling existing pfStatusConfiguration name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("pfStatusConfiguration 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(pfstatustypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add pfStatusConfiguration scheme to client schemes")

		return nil, err
	}

	builder := &PfStatusConfigurationBuilder{
		apiClient: apiClient,
		Definition: &pfstatustypes.PFLACPMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the pfStatusConfiguration is empty")

		return nil, fmt.Errorf("pfStatusConfiguration 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the pfStatusConfiguration is empty")

		return nil, fmt.Errorf("pfStatusConfiguration 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("pfStatusConfiguration object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Delete removes PfStatusConfiguration object from a cluster.
func (builder *PfStatusConfigurationBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the pfStatusConfiguration object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	if !builder.Exists() {
		klog.V(100).Infof(
			"pfStatusConfiguration %s in namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return fmt.Errorf("can not delete pfStatusConfiguration: %w", err)
	}

	builder.Object = nil

	return nil
}

// WithNodeSelector defines the nodeSelector placed in the PfStatusConfiguration spec.
func (builder *PfStatusConfigurationBuilder) WithNodeSelector(
	nodeSelector map[string]string) *PfStatusConfigurationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating pfStatusConfiguration %s in namespace %s with this nodeSelector: %s",
		builder.Definition.Name, builder.Definition.Namespace, nodeSelector)

	if len(nodeSelector) == 0 {
		klog.V(100).Info("Can not redefine pfStatusConfiguration with empty nodeSelector map")

		builder.errorMsg = "pfStatusConfiguration 'nodeSelector' cannot be empty map"

		return builder
	}

	builder.Definition.Spec.NodeSelector = nodeSelector

	return builder
}

// WithInterface defines the interface to be used in the PfStatusConfiguration.
func (builder *PfStatusConfigurationBuilder) WithInterface(interfaceName string) *PfStatusConfigurationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating pfStatusConfiguration %s in namespace %s with interface: %s",
		builder.Definition.Name, builder.Definition.Namespace, interfaceName)

	if interfaceName == "" {
		klog.V(100).Info("Can not redefine pfStatusConfiguration with empty interface string")

		builder.errorMsg = "interface can not be empty string"

		return builder
	}

	// Append the new unique interface
	builder.Definition.Spec.Interfaces = append(builder.Definition.Spec.Interfaces, interfaceName)

	return builder
}

// WithPollingInterval defines the polling time between LACP messages.
func (builder *PfStatusConfigurationBuilder) WithPollingInterval(pollingInterval int) *PfStatusConfigurationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating pfStatusConfiguration %s in namespace %s with this pollingInterval: %d",
		builder.Definition.Name, builder.Definition.Namespace, pollingInterval)

	if pollingInterval < 100 || pollingInterval > 65535 {
		klog.V(100).Info("A valid polling interval is between 100-65535")

		builder.errorMsg = "pfStatusConfiguration 'pollingInterval' value is not valid"

		return builder
	}

	builder.Definition.Spec.PollingInterval = pollingInterval

	return builder
}

// GetPfStatusConfigurationGVR returns PfStatusConfiguration GroupVersionResource.
func GetPfStatusConfigurationGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group: "pfstatusrelay.openshift.io", Version: "v1alpha1", Resource: "pflacpmonitors",
	}
}
