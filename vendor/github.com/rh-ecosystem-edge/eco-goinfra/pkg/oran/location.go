package oran

import (
	"context"
	"fmt"
	"strings"
	"time"

	inventoryv1alpha1 "github.com/openshift-kni/oran-o2ims/api/inventory/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// LocationBuilder provides a struct for the Location resource containing a connection to the cluster and the Location
// definition.
type LocationBuilder struct {
	common.EmbeddableBuilder[inventoryv1alpha1.Location, *inventoryv1alpha1.Location]
	common.EmbeddableCreator[inventoryv1alpha1.Location, LocationBuilder, *inventoryv1alpha1.Location, *LocationBuilder]
	common.EmbeddableDeleter[inventoryv1alpha1.Location, *inventoryv1alpha1.Location]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *LocationBuilder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleter.SetBase(builder)
}

// GetGVK returns the Location GVK for this builder.
func (builder *LocationBuilder) GetGVK() schema.GroupVersionKind {
	return inventoryv1alpha1.GroupVersion.WithKind("Location")
}

// NewLocationBuilder creates a new instance of LocationBuilder.
func NewLocationBuilder(apiClient *clients.Settings, name, nsname string) *LocationBuilder {
	return common.NewNamespacedBuilder[inventoryv1alpha1.Location, LocationBuilder](
		apiClient, inventoryv1alpha1.AddToScheme, name, nsname)
}

// PullLocation fetches an existing Location from the cluster by name and namespace.
func PullLocation(apiClient *clients.Settings, name, nsname string) (*LocationBuilder, error) {
	return common.PullNamespacedBuilder[inventoryv1alpha1.Location, LocationBuilder](
		context.TODO(), apiClient, inventoryv1alpha1.AddToScheme, name, nsname)
}

// ListLocations returns all Location CRs across all namespaces.
func ListLocations(apiClient *clients.Settings, options ...runtimeclient.ListOption) ([]*LocationBuilder, error) {
	return common.List[inventoryv1alpha1.Location, inventoryv1alpha1.LocationList, LocationBuilder](
		context.TODO(), apiClient, inventoryv1alpha1.AddToScheme, options...)
}

// ListReadyLocations returns all Location CRs across all namespaces that are ready. It relies on the IsResourceReady
// function from the inventoryv1alpha1 package to determine if the Location is ready.
func ListReadyLocations(apiClient *clients.Settings, options ...runtimeclient.ListOption) ([]*LocationBuilder, error) {
	locationBuilders, err := common.List[inventoryv1alpha1.Location, inventoryv1alpha1.LocationList, LocationBuilder](
		context.TODO(), apiClient, inventoryv1alpha1.AddToScheme, options...)
	if err != nil {
		return nil, err
	}

	readyLocationBuilders := make([]*LocationBuilder, 0)

	for _, locationBuilder := range locationBuilders {
		if inventoryv1alpha1.IsResourceReady(locationBuilder.Object.Status.Conditions) {
			readyLocationBuilders = append(readyLocationBuilders, locationBuilder)
		}
	}

	return readyLocationBuilders, nil
}

// WithDescription sets the description on the Location spec.
func (builder *LocationBuilder) WithDescription(description string) *LocationBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Setting description on Location %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.Description = description

	return builder
}

// WithAddress sets the human-readable address on the Location spec.
func (builder *LocationBuilder) WithAddress(address string) *LocationBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Setting address on Location %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if address == "" {
		builder.SetError(fmt.Errorf("location 'address' cannot be empty"))

		return builder
	}

	builder.Definition.Spec.Address = &address

	return builder
}

// WithCoordinate sets the geographic coordinates on the Location spec.
func (builder *LocationBuilder) WithCoordinate(coordinate inventoryv1alpha1.GeoLocation) *LocationBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Setting coordinate on Location %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if coordinate.Latitude == "" {
		builder.SetError(fmt.Errorf("location coordinate 'latitude' cannot be empty"))

		return builder
	}

	if coordinate.Longitude == "" {
		builder.SetError(fmt.Errorf("location coordinate 'longitude' cannot be empty"))

		return builder
	}

	builder.Definition.Spec.Coordinate = &coordinate

	return builder
}

// WithCivicAddress sets the civic address elements on the Location spec.
func (builder *LocationBuilder) WithCivicAddress(elements ...inventoryv1alpha1.CivicAddressElement) *LocationBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Setting civic address on Location %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if len(elements) == 0 {
		builder.SetError(fmt.Errorf("location 'civicAddress' cannot be empty"))

		return builder
	}

	builder.Definition.Spec.CivicAddress = elements

	return builder
}

// WaitForCondition waits up to the provided timeout for a condition matching expected. It checks only the Type, Status,
// Reason, and Message fields. For the message, it matches if the message contains the expected. Zero fields in the
// expected condition are ignored.
func (builder *LocationBuilder) WaitForCondition(
	expected metav1.Condition, timeout time.Duration) (*LocationBuilder, error) {
	if err := common.Validate(builder); err != nil {
		return nil, err
	}

	klog.V(100).Infof("Waiting up to %s until Location %s in namespace %s has condition %v",
		timeout, builder.Definition.Name, builder.Definition.Namespace, expected)

	if !builder.Exists() {
		klog.V(100).Infof("Location %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, fmt.Errorf("cannot wait for non-existent Location")
	}

	err := wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			builder.Object, err = builder.Get()
			if err != nil {
				klog.V(100).Infof("Failed to get Location %s in namespace %s: %v",
					builder.Definition.Name, builder.Definition.Namespace, err)

				return false, nil
			}

			builder.Definition = builder.Object

			for _, condition := range builder.Object.Status.Conditions {
				if expected.Type != "" && condition.Type != expected.Type {
					continue
				}

				if expected.Status != "" && condition.Status != expected.Status {
					continue
				}

				if expected.Reason != "" && condition.Reason != expected.Reason {
					continue
				}

				if expected.Message != "" && !strings.Contains(condition.Message, expected.Message) {
					continue
				}

				return true, nil
			}

			return false, nil
		})
	if err != nil {
		return nil, err
	}

	return builder, nil
}
