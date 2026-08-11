package oran

import (
	"context"

	inventoryv1alpha1 "github.com/openshift-kni/oran-o2ims/api/inventory/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// InventoryBuilder provides a struct for the Inventory resource containing a connection to the cluster and the
// Inventory definition.
type InventoryBuilder struct {
	common.EmbeddableBuilder[inventoryv1alpha1.Inventory, *inventoryv1alpha1.Inventory]
}

// GetGVK returns the Inventory GVK for this builder.
func (builder *InventoryBuilder) GetGVK() schema.GroupVersionKind {
	return inventoryv1alpha1.GroupVersion.WithKind("Inventory")
}

// PullInventory fetches an existing Inventory from the cluster by name and namespace.
func PullInventory(apiClient *clients.Settings, name, nsname string) (*InventoryBuilder, error) {
	return common.PullNamespacedBuilder[inventoryv1alpha1.Inventory, InventoryBuilder](
		context.TODO(), apiClient, inventoryv1alpha1.AddToScheme, name, nsname)
}
