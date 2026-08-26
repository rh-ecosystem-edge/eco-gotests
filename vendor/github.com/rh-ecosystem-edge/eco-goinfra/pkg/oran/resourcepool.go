package oran

import (
	"context"
	"fmt"
	"strings"
	"time"

	inventoryv1alpha1 "github.com/openshift-kni/oran-o2ims/api/inventory/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourcePoolBuilder provides a struct for the ResourcePool resource containing a connection to the cluster and the
// ResourcePool definition.
type ResourcePoolBuilder struct {
	common.EmbeddableBuilder[inventoryv1alpha1.ResourcePool, *inventoryv1alpha1.ResourcePool]
	common.EmbeddableCreator[inventoryv1alpha1.ResourcePool, ResourcePoolBuilder, *inventoryv1alpha1.ResourcePool, *ResourcePoolBuilder]
	common.EmbeddableDeleter[inventoryv1alpha1.ResourcePool, *inventoryv1alpha1.ResourcePool]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *ResourcePoolBuilder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleter.SetBase(builder)
}

// GetGVK returns the ResourcePool GVK for this builder.
func (builder *ResourcePoolBuilder) GetGVK() schema.GroupVersionKind {
	return inventoryv1alpha1.GroupVersion.WithKind("ResourcePool")
}

// NewResourcePoolBuilder creates a new instance of ResourcePoolBuilder.
func NewResourcePoolBuilder(apiClient *clients.Settings, name, nsname string) *ResourcePoolBuilder {
	return common.NewNamespacedBuilder[inventoryv1alpha1.ResourcePool, ResourcePoolBuilder](
		apiClient, inventoryv1alpha1.AddToScheme, name, nsname)
}

// PullResourcePool fetches an existing ResourcePool from the cluster by name and namespace.
func PullResourcePool(apiClient *clients.Settings, name, nsname string) (*ResourcePoolBuilder, error) {
	return common.PullNamespacedBuilder[inventoryv1alpha1.ResourcePool, ResourcePoolBuilder](
		context.TODO(), apiClient, inventoryv1alpha1.AddToScheme, name, nsname)
}

// ListResourcePools returns all ResourcePool CRs across all namespaces.
func ListResourcePools(
	apiClient *clients.Settings, options ...runtimeclient.ListOption) ([]*ResourcePoolBuilder, error) {
	return common.List[inventoryv1alpha1.ResourcePool, inventoryv1alpha1.ResourcePoolList, ResourcePoolBuilder](
		context.TODO(), apiClient, inventoryv1alpha1.AddToScheme, options...)
}

// ListReadyResourcePools returns all ResourcePool CRs across all namespaces that are ready. It relies on the
// IsResourceReady function from the inventoryv1alpha1 package to determine if the ResourcePool is ready.
func ListReadyResourcePools(
	apiClient *clients.Settings, options ...runtimeclient.ListOption) ([]*ResourcePoolBuilder, error) {
	resourcePoolBuilders, err := ListResourcePools(apiClient, options...)
	if err != nil {
		return nil, err
	}

	readyResourcePoolBuilders := make([]*ResourcePoolBuilder, 0)

	for _, resourcePoolBuilder := range resourcePoolBuilders {
		if inventoryv1alpha1.IsResourceReady(resourcePoolBuilder.Object.Status.Conditions) {
			readyResourcePoolBuilders = append(readyResourcePoolBuilders, resourcePoolBuilder)
		}
	}

	return readyResourcePoolBuilders, nil
}

// WithOCloudSiteName sets the oCloudSiteName on the ResourcePool spec.
func (builder *ResourcePoolBuilder) WithOCloudSiteName(oCloudSiteName string) *ResourcePoolBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Setting oCloudSiteName on ResourcePool %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if oCloudSiteName == "" {
		builder.SetError(fmt.Errorf("resourcePool 'oCloudSiteName' cannot be empty"))

		return builder
	}

	builder.Definition.Spec.OCloudSiteName = oCloudSiteName

	return builder
}

// WithDescription sets the description on the ResourcePool spec.
func (builder *ResourcePoolBuilder) WithDescription(description string) *ResourcePoolBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Setting description on ResourcePool %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.Description = description

	return builder
}

// WaitForCondition waits up to the provided timeout for a condition matching expected. It checks only the Type, Status,
// Reason, and Message fields. For the message, it matches if the message contains the expected. Zero fields in the
// expected condition are ignored.
func (builder *ResourcePoolBuilder) WaitForCondition(
	expected metav1.Condition, timeout time.Duration) (*ResourcePoolBuilder, error) {
	if err := common.Validate(builder); err != nil {
		return nil, err
	}

	klog.V(100).Infof("Waiting up to %s until ResourcePool %s in namespace %s has condition %v",
		timeout, builder.Definition.Name, builder.Definition.Namespace, expected)

	object, err := builder.Get()
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.V(100).Infof("ResourcePool %s does not exist in namespace %s",
				builder.Definition.Name, builder.Definition.Namespace)

			return nil, fmt.Errorf("cannot wait for non-existent ResourcePool: %w", err)
		}

		return nil, err
	}

	builder.Object = object

	err = wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			builder.Object, err = builder.Get()
			if err != nil {
				klog.V(100).Infof("Failed to get ResourcePool %s in namespace %s: %v",
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
