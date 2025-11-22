package hive

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	hiveV1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/hive/api/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterImageSetBuilder provides struct for the clusterimageset object containing connection to
// the cluster and the clusterimageset definitions.
type ClusterImageSetBuilder struct {
	Definition *hiveV1.ClusterImageSet
	Object     *hiveV1.ClusterImageSet
	errorMsg   string
	apiClient  goclient.Client
}

// ClusterImageSetAdditionalOptions additional options for ClusterImageSet object.
type ClusterImageSetAdditionalOptions func(builder *ClusterImageSetBuilder) (*ClusterImageSetBuilder, error)

// NewClusterImageSetBuilder creates a new instance of ClusterImageSetBuilder.
func NewClusterImageSetBuilder(apiClient *clients.Settings, name, releaseImage string) *ClusterImageSetBuilder {
	klog.V(100).Infof(
		`Initializing new clusterimageset structure with the following params: name: %s, releaseImage: %s`,
		name, releaseImage)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(hiveV1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add hive v1 scheme to client schemes")

		return nil
	}

	builder := &ClusterImageSetBuilder{
		apiClient: apiClient.Client,
		Definition: &hiveV1.ClusterImageSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: hiveV1.ClusterImageSetSpec{
				ReleaseImage: releaseImage,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the clusterimageset is empty")

		builder.errorMsg = "clusterimageset 'name' cannot be empty"

		return builder
	}

	if releaseImage == "" {
		klog.V(100).Info("The releaseImage of the clusterimageset is empty")

		builder.errorMsg = "clusterimageset 'releaseImage' cannot be empty"

		return builder
	}

	return builder
}

// PullClusterImageSet loads an existing clusterimageset into ClusterImageSetBuilder struct.
func PullClusterImageSet(apiClient *clients.Settings, name string) (*ClusterImageSetBuilder, error) {
	klog.V(100).Infof("Pulling existing clusterimageset name: %s", name)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("clusterImageSet 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(hiveV1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add hive v1 scheme to client schemes")

		return nil, err
	}

	builder := &ClusterImageSetBuilder{
		apiClient: apiClient.Client,
		Definition: &hiveV1.ClusterImageSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the clusterimageset is empty")

		return nil, fmt.Errorf("clusterimageset 'name' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("clusterimageset object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get fetches the defined clusterimageset from the cluster.
func (builder *ClusterImageSetBuilder) Get() (*hiveV1.ClusterImageSet, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting clusterimageset %s", builder.Definition.Name)

	clusterimageset := &hiveV1.ClusterImageSet{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name: builder.Definition.Name,
	}, clusterimageset)
	if err != nil {
		return nil, err
	}

	return clusterimageset, err
}

// Create generates a clusterimageset on the cluster.
func (builder *ClusterImageSetBuilder) Create() (*ClusterImageSetBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the clusterimageset %s", builder.Definition.Name)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Update modifies an existing clusterimageset on the cluster.
func (builder *ClusterImageSetBuilder) Update(force bool) (*ClusterImageSetBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating clusterimageset %s", builder.Definition.Name)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%v", msg.FailToUpdateNotification("clusterimageset", builder.Definition.Name, builder.Definition.Namespace))

			err := builder.Delete()
			if err != nil {
				klog.V(100).Infof("%v", msg.FailToUpdateError("clusterimageset", builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}
	}

	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Delete removes a clusterimageset from the cluster.
func (builder *ClusterImageSetBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the clusterimageset %s", builder.Definition.Name)

	if !builder.Exists() {
		klog.V(100).Info("clusterimageset cannot be deleted because it does not exist")

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return fmt.Errorf("cannot delete clusterimageset: %w", err)
	}

	builder.Object = nil
	builder.Definition.ResourceVersion = ""
	builder.Definition.CreationTimestamp = metav1.Time{}

	return nil
}

// Exists checks if the defined clusterimageset has already been created.
func (builder *ClusterImageSetBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if clusterimageset %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// WithReleaseImage sets the releaseImage for the clusterimageset.
func (builder *ClusterImageSetBuilder) WithReleaseImage(image string) *ClusterImageSetBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting clusterimageset %s releaseImage to %s",
		builder.Definition.Name, image)

	if image == "" {
		klog.V(100).Info("The clusterimageset releaseImage is empty")

		builder.errorMsg = "cannot set releaseImage to empty string"

		return builder
	}

	builder.Definition.Spec.ReleaseImage = image

	return builder
}

// WithOptions creates ClusterDeployment with generic mutation options.
func (builder *ClusterImageSetBuilder) WithOptions(
	options ...ClusterImageSetAdditionalOptions) *ClusterImageSetBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting ClusterImageSet additional options")

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

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ClusterImageSetBuilder) validate() (bool, error) {
	resourceCRD := "ClusterImageSet"

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
