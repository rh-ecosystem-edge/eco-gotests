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

// OCloudSiteBuilder provides a struct for the OCloudSite resource containing a connection to the cluster and the
// OCloudSite definition.
type OCloudSiteBuilder struct {
	common.EmbeddableBuilder[inventoryv1alpha1.OCloudSite, *inventoryv1alpha1.OCloudSite]
	common.EmbeddableCreator[inventoryv1alpha1.OCloudSite, OCloudSiteBuilder, *inventoryv1alpha1.OCloudSite, *OCloudSiteBuilder]
	common.EmbeddableDeleter[inventoryv1alpha1.OCloudSite, *inventoryv1alpha1.OCloudSite]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *OCloudSiteBuilder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleter.SetBase(builder)
}

// GetGVK returns the OCloudSite GVK for this builder.
func (builder *OCloudSiteBuilder) GetGVK() schema.GroupVersionKind {
	return inventoryv1alpha1.GroupVersion.WithKind("OCloudSite")
}

// NewOCloudSiteBuilder creates a new instance of OCloudSiteBuilder.
func NewOCloudSiteBuilder(apiClient *clients.Settings, name, nsname string) *OCloudSiteBuilder {
	return common.NewNamespacedBuilder[inventoryv1alpha1.OCloudSite, OCloudSiteBuilder](
		apiClient, inventoryv1alpha1.AddToScheme, name, nsname)
}

// PullOCloudSite fetches an existing OCloudSite from the cluster by name and namespace.
func PullOCloudSite(apiClient *clients.Settings, name, nsname string) (*OCloudSiteBuilder, error) {
	return common.PullNamespacedBuilder[inventoryv1alpha1.OCloudSite, OCloudSiteBuilder](
		context.TODO(), apiClient, inventoryv1alpha1.AddToScheme, name, nsname)
}

// ListOCloudSites returns all OCloudSite CRs across all namespaces.
func ListOCloudSites(
	apiClient *clients.Settings, options ...runtimeclient.ListOption) ([]*OCloudSiteBuilder, error) {
	return common.List[inventoryv1alpha1.OCloudSite, inventoryv1alpha1.OCloudSiteList, OCloudSiteBuilder](
		context.TODO(), apiClient, inventoryv1alpha1.AddToScheme, options...)
}

// ListReadyOCloudSites returns all OCloudSite CRs across all namespaces that are ready. It relies on the
// IsResourceReady function from the inventoryv1alpha1 package to determine if the OCloudSite is ready.
func ListReadyOCloudSites(
	apiClient *clients.Settings, options ...runtimeclient.ListOption) ([]*OCloudSiteBuilder, error) {
	ocloudSiteBuilders, err := ListOCloudSites(apiClient, options...)
	if err != nil {
		return nil, err
	}

	readyOCloudSiteBuilders := make([]*OCloudSiteBuilder, 0)

	for _, ocloudSiteBuilder := range ocloudSiteBuilders {
		if inventoryv1alpha1.IsResourceReady(ocloudSiteBuilder.Object.Status.Conditions) {
			readyOCloudSiteBuilders = append(readyOCloudSiteBuilders, ocloudSiteBuilder)
		}
	}

	return readyOCloudSiteBuilders, nil
}

// WithGlobalLocationName sets the globalLocationName on the OCloudSite spec.
func (builder *OCloudSiteBuilder) WithGlobalLocationName(globalLocationName string) *OCloudSiteBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Setting globalLocationName on OCloudSite %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if globalLocationName == "" {
		builder.SetError(fmt.Errorf("oCloudSite 'globalLocationName' cannot be empty"))

		return builder
	}

	builder.Definition.Spec.GlobalLocationName = globalLocationName

	return builder
}

// WithDescription sets the description on the OCloudSite spec.
func (builder *OCloudSiteBuilder) WithDescription(description string) *OCloudSiteBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Setting description on OCloudSite %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.Description = description

	return builder
}

// WaitForCondition waits up to the provided timeout for a condition matching expected. It checks only the Type, Status,
// Reason, and Message fields. For the message, it matches if the message contains the expected. Zero fields in the
// expected condition are ignored.
func (builder *OCloudSiteBuilder) WaitForCondition(
	expected metav1.Condition, timeout time.Duration) (*OCloudSiteBuilder, error) {
	if err := common.Validate(builder); err != nil {
		return nil, err
	}

	klog.V(100).Infof("Waiting up to %s until OCloudSite %s in namespace %s has condition %v",
		timeout, builder.Definition.Name, builder.Definition.Namespace, expected)

	object, err := builder.Get()
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.V(100).Infof("OCloudSite %s does not exist in namespace %s",
				builder.Definition.Name, builder.Definition.Namespace)

			return nil, fmt.Errorf("cannot wait for non-existent OCloudSite: %w", err)
		}

		return nil, err
	}

	builder.Object = object

	err = wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			builder.Object, err = builder.Get()
			if err != nil {
				klog.V(100).Infof("Failed to get OCloudSite %s in namespace %s: %v",
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
