package ocm

import (
	"context"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	"k8s.io/apimachinery/pkg/runtime/schema"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
)

var placementRuleGVK = placementrulev1.SchemeGroupVersion.WithKind("PlacementRule")

// PlacementRuleBuilder provides struct for the PlacementRule object containing connection to
// the cluster and the PlacementRule definitions.
type PlacementRuleBuilder struct {
	common.EmbeddableBuilder[placementrulev1.PlacementRule, *placementrulev1.PlacementRule]
	common.EmbeddableCreator[placementrulev1.PlacementRule, PlacementRuleBuilder, *placementrulev1.PlacementRule, *PlacementRuleBuilder]
	common.EmbeddableDeleteReturner[placementrulev1.PlacementRule, PlacementRuleBuilder, *placementrulev1.PlacementRule, *PlacementRuleBuilder]
	common.EmbeddableForceUpdater[placementrulev1.PlacementRule, PlacementRuleBuilder, *placementrulev1.PlacementRule, *PlacementRuleBuilder]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *PlacementRuleBuilder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleteReturner.SetBase(builder)
	builder.EmbeddableForceUpdater.SetBase(builder)
}

// GetGVK returns the PlacementRule GVK for this builder.
func (builder *PlacementRuleBuilder) GetGVK() schema.GroupVersionKind {
	return placementRuleGVK
}

// NewPlacementRuleBuilder creates a new instance of PlacementRuleBuilder.
func NewPlacementRuleBuilder(apiClient *clients.Settings, name, nsname string) *PlacementRuleBuilder {
	return common.NewNamespacedBuilder[placementrulev1.PlacementRule, PlacementRuleBuilder](
		apiClient, placementrulev1.AddToScheme, name, nsname)
}

// PullPlacementRule pulls existing placementrule into Builder struct.
func PullPlacementRule(apiClient *clients.Settings, name, nsname string) (*PlacementRuleBuilder, error) {
	return common.PullNamespacedBuilder[placementrulev1.PlacementRule, PlacementRuleBuilder](
		context.TODO(), apiClient, placementrulev1.AddToScheme, name, nsname)
}
