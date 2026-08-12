package capi

import (
	"context"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	clusterv1beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/capi/cluster/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

// AdditionalOptions are optional mutations applied via WithOptions.
type AdditionalOptions func(builder *ClusterBuilder) (*ClusterBuilder, error)

// ClusterBuilder provides a CAPI Cluster builder backed by the shared common builder framework.
type ClusterBuilder struct {
	common.EmbeddableBuilder[clusterv1beta1.Cluster, *clusterv1beta1.Cluster]
	common.EmbeddableWithOptions[
		clusterv1beta1.Cluster, ClusterBuilder, *clusterv1beta1.Cluster, *ClusterBuilder, AdditionalOptions]
	common.EmbeddableCreator[clusterv1beta1.Cluster, ClusterBuilder, *clusterv1beta1.Cluster, *ClusterBuilder]
	common.EmbeddableDeleter[clusterv1beta1.Cluster, *clusterv1beta1.Cluster]
	common.EmbeddableUpdater[clusterv1beta1.Cluster, ClusterBuilder, *clusterv1beta1.Cluster, *ClusterBuilder]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *ClusterBuilder) AttachMixins() {
	builder.EmbeddableWithOptions.SetBase(builder)
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleter.SetBase(builder)
	builder.EmbeddableUpdater.SetBase(builder)
}

// GetGVK returns the CAPI Cluster GVK for this builder.
func (builder *ClusterBuilder) GetGVK() schema.GroupVersionKind {
	return clusterv1beta1.GroupVersion.WithKind("Cluster")
}

// NewClusterBuilder creates a new instance of ClusterBuilder.
func NewClusterBuilder(apiClient *clients.Settings, name, nsname string) *ClusterBuilder {
	return common.NewNamespacedBuilder[clusterv1beta1.Cluster, ClusterBuilder](
		apiClient, clusterv1beta1.AddToScheme, name, nsname)
}

// PullCluster retrieves an existing CAPI Cluster object from the cluster.
func PullCluster(apiClient *clients.Settings, name, nsname string) (*ClusterBuilder, error) {
	return common.PullNamespacedBuilder[clusterv1beta1.Cluster, ClusterBuilder](
		context.TODO(), apiClient, clusterv1beta1.AddToScheme, name, nsname)
}

// WithControlPlaneEndpoint sets the control plane endpoint host and port.
func (builder *ClusterBuilder) WithControlPlaneEndpoint(host string, port int32) *ClusterBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Setting control plane endpoint %s:%d on CAPI Cluster %s/%s",
		host, port, builder.Definition.Namespace, builder.Definition.Name)

	if host == "" {
		builder.SetError(fmt.Errorf("'host' cannot be empty"))

		return builder
	}

	if port < 1 || port > 65535 {
		builder.SetError(fmt.Errorf("'port' must be between 1 and 65535, got %d", port))

		return builder
	}

	builder.Definition.Spec.ControlPlaneEndpoint = clusterv1beta1.APIEndpoint{
		Host: host,
		Port: port,
	}

	return builder
}

// WithControlPlaneRef sets the control plane object reference.
func (builder *ClusterBuilder) WithControlPlaneRef(
	apiVersion, kind, name string) *ClusterBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Setting control plane ref %s/%s on CAPI Cluster %s/%s",
		kind, name, builder.Definition.Namespace, builder.Definition.Name)

	if apiVersion == "" {
		builder.SetError(fmt.Errorf("'apiVersion' cannot be empty"))

		return builder
	}

	if kind == "" {
		builder.SetError(fmt.Errorf("'kind' cannot be empty"))

		return builder
	}

	if name == "" {
		builder.SetError(fmt.Errorf("'name' cannot be empty"))

		return builder
	}

	builder.Definition.Spec.ControlPlaneRef = &clusterv1beta1.ObjectReference{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
	}

	return builder
}

// WithInfrastructureRef sets the infrastructure object reference.
func (builder *ClusterBuilder) WithInfrastructureRef(
	apiVersion, kind, name string) *ClusterBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Setting infrastructure ref %s/%s on CAPI Cluster %s/%s",
		kind, name, builder.Definition.Namespace, builder.Definition.Name)

	if apiVersion == "" {
		builder.SetError(fmt.Errorf("'apiVersion' cannot be empty"))

		return builder
	}

	if kind == "" {
		builder.SetError(fmt.Errorf("'kind' cannot be empty"))

		return builder
	}

	if name == "" {
		builder.SetError(fmt.Errorf("'name' cannot be empty"))

		return builder
	}

	builder.Definition.Spec.InfrastructureRef = &clusterv1beta1.ObjectReference{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
	}

	return builder
}

// WithPaused sets the paused field on the Cluster spec.
func (builder *ClusterBuilder) WithPaused(paused bool) *ClusterBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Setting paused=%v on CAPI Cluster %s/%s",
		paused, builder.Definition.Namespace, builder.Definition.Name)

	builder.Definition.Spec.Paused = paused

	return builder
}
