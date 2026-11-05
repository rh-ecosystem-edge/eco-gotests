package ocm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ocm/clusterv1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

var managedClusterGVK = schema.GroupVersion{Group: clusterv1.GroupName, Version: "v1"}.WithKind("ManagedCluster")

// ManagedClusterBuilder provides a struct for the ManagedCluster object containing connection to the cluster and the
// ManagedCluster definitions.
type ManagedClusterBuilder struct {
	common.EmbeddableBuilder[clusterv1.ManagedCluster, *clusterv1.ManagedCluster]
	common.EmbeddableCreator[clusterv1.ManagedCluster, ManagedClusterBuilder, *clusterv1.ManagedCluster, *ManagedClusterBuilder]
	common.EmbeddableDeleter[clusterv1.ManagedCluster, *clusterv1.ManagedCluster]
	common.EmbeddableUpdater[
		clusterv1.ManagedCluster, ManagedClusterBuilder, *clusterv1.ManagedCluster, *ManagedClusterBuilder]
	common.EmbeddableWithOptions[
		clusterv1.ManagedCluster, ManagedClusterBuilder, *clusterv1.ManagedCluster, *ManagedClusterBuilder, ManagedClusterAdditionalOptions]
}

// ManagedClusterAdditionalOptions additional options for ManagedCluster object.
type ManagedClusterAdditionalOptions func(builder *ManagedClusterBuilder) (*ManagedClusterBuilder, error)

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *ManagedClusterBuilder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleter.SetBase(builder)
	builder.EmbeddableUpdater.SetBase(builder)
	builder.EmbeddableWithOptions.SetBase(builder)
}

// GetGVK returns the ManagedCluster GVK for this builder.
func (builder *ManagedClusterBuilder) GetGVK() schema.GroupVersionKind {
	return managedClusterGVK
}

// NewManagedClusterBuilder creates a new instance of ManagedClusterBuilder.
func NewManagedClusterBuilder(apiClient *clients.Settings, name string) *ManagedClusterBuilder {
	return common.NewClusterScopedBuilder[clusterv1.ManagedCluster, ManagedClusterBuilder](
		apiClient, clusterv1.Install, name)
}

// PullManagedCluster loads an existing ManagedCluster into ManagedClusterBuilder struct.
func PullManagedCluster(apiClient *clients.Settings, name string) (*ManagedClusterBuilder, error) {
	return common.PullClusterScopedBuilder[clusterv1.ManagedCluster, ManagedClusterBuilder](
		context.TODO(), apiClient, clusterv1.Install, name)
}

// WithHubAcceptsClient sets the hubAcceptsClient field in the managedcluster resource.
func (builder *ManagedClusterBuilder) WithHubAcceptsClient(accept bool) *ManagedClusterBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Setting ManagedCluster hubAcceptsClient field to %t", accept)

	builder.Definition.Spec.HubAcceptsClient = accept

	return builder
}

// DeleteAndWait deletes the Cluster then waits up to timeout until the Cluster no longer
// exists.
func (builder *ManagedClusterBuilder) DeleteAndWait(timeout time.Duration) error {
	if err := common.Validate(builder); err != nil {
		return err
	}

	klog.V(100).Infof("Deleting cluster %s and waiting up to %s until it is deleted",
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

// WaitForLabel waits up to timeout until label exists on the ManagedCluster.
func (builder *ManagedClusterBuilder) WaitForLabel(
	label string, timeout time.Duration) (*ManagedClusterBuilder, error) {
	if err := common.Validate(builder); err != nil {
		return nil, err
	}

	klog.V(100).Infof("Waiting up to %s until ManagedCluster %s has label %s", timeout, builder.Definition.Name, label)

	if !builder.Exists() {
		klog.V(100).Infof("ManagedCluster %s does not exist", builder.Definition.Name)

		return nil, fmt.Errorf("managedCluster object %s does not exist", builder.Definition.Name)
	}

	err := wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			builder.Object, err = builder.Get()
			if err != nil {
				klog.V(100).Infof("Failed to get ManagedCluster %s: %v", builder.Definition.Name, err)

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

// WaitForCondition waits up to the provided timeout for a condition matching expected. It checks only the Type, Status,
// Reason, and Message fields of the expected condition. Empty fields in the expected condition are ignored.
func (builder *ManagedClusterBuilder) WaitForCondition(expected metav1.Condition, timeout time.Duration) (
	*ManagedClusterBuilder, error) {
	if err := common.Validate(builder); err != nil {
		return nil, err
	}

	klog.V(100).Infof("Waiting up to %s until ManagedCluster %s has condition %v",
		timeout, builder.Definition.Name, expected)

	if !builder.Exists() {
		klog.V(100).Infof("ManagedCluster %s does not exist", builder.Definition.Name)

		return nil, fmt.Errorf("cannot wait for non-existent ManagedCluster")
	}

	err := wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			builder.Object, err = builder.Get()
			if err != nil {
				klog.V(100).Infof("Failed to get ManagedCluster %s: %v", builder.Definition.Name, err)

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
