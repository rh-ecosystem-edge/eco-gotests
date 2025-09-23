package ocm

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ocm/clusterv1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ManagedClusterBuilder provides a struct for the ManagedCluster object containing connection to the cluster and the
// ManagedCluster definitions.
type ManagedClusterBuilder struct {
	Definition *clusterv1.ManagedCluster
	Object     *clusterv1.ManagedCluster
	errorMsg   string
	apiClient  runtimeclient.Client
}

// ManagedClusterAdditionalOptions additional options for ManagedCluster object.
type ManagedClusterAdditionalOptions func(builder *ManagedClusterBuilder) (*ManagedClusterBuilder, error)

// NewManagedClusterBuilder creates a new instance of ManagedClusterBuilder.
func NewManagedClusterBuilder(apiClient *clients.Settings, name string) *ManagedClusterBuilder {
	glog.V(100).Infof(
		"Initializing new ManagedCluster structure with the following params: name: %s", name)

	if apiClient == nil {
		glog.V(100).Info("The apiClient of the ManagedCluster is nil")

		return nil
	}

	err := apiClient.AttachScheme(clusterv1.Install)
	if err != nil {
		glog.V(100).Info("Failed to add ManagedCluster scheme to client schemes")

		return nil
	}

	builder := &ManagedClusterBuilder{
		apiClient: apiClient.Client,
		Definition: &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the ManagedCluster is empty")

		builder.errorMsg = "managedCluster 'name' cannot be empty"

		return builder
	}

	return builder
}

// WithHubAcceptsClient sets the hubAcceptsClient field in the managedcluster resource.
func (builder *ManagedClusterBuilder) WithHubAcceptsClient(accept bool) *ManagedClusterBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting ManagedCluster hubAcceptsClient field to %t", accept)

	builder.Definition.Spec.HubAcceptsClient = accept

	return builder
}

// WithOptions creates ManagedCluster with generic mutation options.
func (builder *ManagedClusterBuilder) WithOptions(options ...ManagedClusterAdditionalOptions) *ManagedClusterBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting ManagedCluster additional options")

	for _, option := range options {
		if option != nil {
			builder, err := option(builder)
			if err != nil {
				glog.V(100).Infof("Error occurred in mutation function")

				builder.errorMsg = err.Error()

				return builder
			}
		}
	}

	return builder
}

// PullManagedCluster loads an existing ManagedCluster into ManagedClusterBuilder struct.
func PullManagedCluster(apiClient *clients.Settings, name string) (*ManagedClusterBuilder, error) {
	glog.V(100).Infof("Pulling existing ManagedCluster name: %s", name)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient for the ManagedCluster is empty")

		return nil, fmt.Errorf("managedCluster 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(clusterv1.Install)
	if err != nil {
		glog.V(100).Info("Failed to add ManagedCluster scheme to client schemes")

		return nil, err
	}

	builder := &ManagedClusterBuilder{
		apiClient: apiClient.Client,
		Definition: &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		glog.V(100).Info("The name of the ManagedCluster is empty")

		return nil, fmt.Errorf("managedCluster 'name' cannot be empty")
	}

	if !builder.Exists() {
		glog.V(100).Info("The ManagedCluster does not exist")

		return nil, fmt.Errorf("managedCluster object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Update modifies an existing ManagedCluster on the cluster.
func (builder *ManagedClusterBuilder) Update() (*ManagedClusterBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating ManagedCluster %s", builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("ManagedCluster %s does not exist", builder.Definition.Name)

		return nil, fmt.Errorf("cannot update non-existent managedCluster")
	}

	builder.Definition.ResourceVersion = builder.Object.ResourceVersion

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Delete removes a ManagedCluster from the cluster.
func (builder *ManagedClusterBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Deleting the ManagedCluster %s", builder.Definition.Name)

	if !builder.Exists() {
		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return fmt.Errorf("cannot delete managedCluster: %w", err)
	}

	builder.Object = nil

	return nil
}

// DeleteAndWait deletes the Cluster then waits up to timeout until the Cluster no longer
// exists.
func (builder *ManagedClusterBuilder) DeleteAndWait(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Deleting cluster %s and waiting up to %s until it is deleted",
		builder.Definition.Name, timeout)

	err := builder.Delete()
	if err != nil {
		return err
	}

	return wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			return !builder.Exists(), nil
		})
}

// Get returns the ManagedCluster object if found.
func (builder *ManagedClusterBuilder) Get() (*clusterv1.ManagedCluster, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting ManagedCluster object %s", builder.Definition.Name)

	managedCluster := &clusterv1.ManagedCluster{}

	err := builder.apiClient.Get(context.TODO(), runtimeclient.ObjectKey{
		Name: builder.Definition.Name,
	}, managedCluster)
	if err != nil {
		glog.V(100).Infof("Failed to get ManagedCluster object %s: %v", builder.Definition.Name, err)

		return nil, err
	}

	return managedCluster, nil
}

// Exists checks if the defined ManagedCluster has already been created.
func (builder *ManagedClusterBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if ManagedCluster %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Create makes a managedcluster in the cluster and stores the created object in struct.
func (builder *ManagedClusterBuilder) Create() (*ManagedClusterBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the managedcluster %s",
		builder.Definition.Name)

	if builder.Exists() {
		return builder, nil
	}

	err := builder.apiClient.Create(context.TODO(), builder.Definition)
	if err != nil {
		return builder, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WaitForLabel waits up to timeout until label exists on the ManagedCluster.
func (builder *ManagedClusterBuilder) WaitForLabel(
	label string, timeout time.Duration) (*ManagedClusterBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Waiting up to %s until ManageddCluster %s has label %s", timeout, builder.Definition.Name, label)

	if !builder.Exists() {
		glog.V(100).Infof("ManagedCluster %s does not exist", builder.Definition.Name)

		return nil, fmt.Errorf("managedCluster object %s does not exist", builder.Definition.Name)
	}

	err := wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			builder.Object, err = builder.Get()
			if err != nil {
				glog.V(100).Info("Failed to get ManagedCluster %s: %v", builder.Definition.Name, err)

				return false, nil
			}

			builder.Definition = builder.Object

			if builder.Definition.Labels == nil {
				return false, nil
			}

			_, exists := builder.Definition.Labels[label]

			return exists, nil
		})
	if err != nil {
		return nil, err
	}

	return builder, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ManagedClusterBuilder) validate() (bool, error) {
	resourceCRD := "managedCluster"

	if builder == nil {
		glog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		glog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		glog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		glog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
