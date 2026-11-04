package apiservers

import (
	"context"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

const apiServerName = "cluster"

// APIServerBuilder provides a struct for the config.openshift.io/v1 APIServer object.
type APIServerBuilder struct {
	common.EmbeddableBuilder[configv1.APIServer, *configv1.APIServer]
	common.EmbeddableUpdater[configv1.APIServer, APIServerBuilder, *configv1.APIServer, *APIServerBuilder]
}

// AttachMixins wires the embedded mixins to this builder instance.
func (builder *APIServerBuilder) AttachMixins() {
	builder.SetBase(builder)
}

// GetGVK returns the APIServer GVK for this builder.
func (builder *APIServerBuilder) GetGVK() schema.GroupVersionKind {
	return configv1.GroupVersion.WithKind("APIServer")
}

// PullAPIServer pulls the existing config.openshift.io/v1 APIServer singleton from the cluster.
func PullAPIServer(apiClient *clients.Settings) (*APIServerBuilder, error) {
	return common.PullClusterScopedBuilder[configv1.APIServer, APIServerBuilder](
		context.TODO(), apiClient, configv1.Install, apiServerName)
}

// WithTLSAdherence sets the TLS adherence policy on the APIServer definition.
func (builder *APIServerBuilder) WithTLSAdherence(
	policy configv1.TLSAdherencePolicy) *APIServerBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Setting TLS adherence policy %q on APIServer %s", policy, builder.Definition.Name)

	builder.Definition.Spec.TLSAdherence = policy

	return builder
}

// WithTLSSecurityProfile sets the TLS security profile on the APIServer definition.
func (builder *APIServerBuilder) WithTLSSecurityProfile(
	profile *configv1.TLSSecurityProfile) *APIServerBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Setting TLS security profile on APIServer %s", builder.Definition.Name)

	if profile == nil {
		builder.SetError(fmt.Errorf("apiserver TLS security profile cannot be nil"))

		return builder
	}

	builder.Definition.Spec.TLSSecurityProfile = profile

	return builder
}
