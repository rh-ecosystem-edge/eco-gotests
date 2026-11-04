package argocd

import (
	"context"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	argocdoperator "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/argocd/argocdoperator"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Builder provides struct for the argocd object containing connection to
// the cluster and the argocd definitions.
type Builder struct {
	common.EmbeddableBuilder[argocdoperator.ArgoCD, *argocdoperator.ArgoCD]
	common.EmbeddableCreator[argocdoperator.ArgoCD, Builder, *argocdoperator.ArgoCD, *Builder]
	common.EmbeddableDeleteReturner[argocdoperator.ArgoCD, Builder, *argocdoperator.ArgoCD, *Builder]
	common.EmbeddableForceUpdater[argocdoperator.ArgoCD, Builder, *argocdoperator.ArgoCD, *Builder]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *Builder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleteReturner.SetBase(builder)
	builder.EmbeddableForceUpdater.SetBase(builder)
}

// GetGVK returns the ArgoCD GVK for this builder.
func (builder *Builder) GetGVK() schema.GroupVersionKind {
	return argocdoperator.GroupVersion.WithKind("ArgoCD")
}

// NewBuilder creates a new instance of Builder.
func NewBuilder(apiClient *clients.Settings, name, nsname string) *Builder {
	return common.NewNamespacedBuilder[argocdoperator.ArgoCD, Builder](
		apiClient, argocdoperator.AddToScheme, name, nsname)
}

// Pull pulls existing argocd from cluster.
func Pull(apiClient *clients.Settings, name, nsname string) (*Builder, error) {
	return common.PullNamespacedBuilder[argocdoperator.ArgoCD, Builder](
		context.TODO(), apiClient, argocdoperator.AddToScheme, name, nsname)
}
