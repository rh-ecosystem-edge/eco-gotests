package ocm

import (
	"context"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	policiesv1beta1 "open-cluster-management.io/governance-policy-propagator/api/v1beta1"
)

var policySetGVK = policiesv1beta1.GroupVersion.WithKind("PolicySet")

// PolicySetBuilder provides struct for the policySet object containing connection to
// the cluster and the policySet definitions.
type PolicySetBuilder struct {
	common.EmbeddableBuilder[policiesv1beta1.PolicySet, *policiesv1beta1.PolicySet]
	common.EmbeddableCreator[policiesv1beta1.PolicySet, PolicySetBuilder, *policiesv1beta1.PolicySet, *PolicySetBuilder]
	common.EmbeddableDeleteReturner[policiesv1beta1.PolicySet, PolicySetBuilder, *policiesv1beta1.PolicySet, *PolicySetBuilder]
	common.EmbeddableForceUpdater[policiesv1beta1.PolicySet, PolicySetBuilder, *policiesv1beta1.PolicySet, *PolicySetBuilder]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *PolicySetBuilder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleteReturner.SetBase(builder)
	builder.EmbeddableForceUpdater.SetBase(builder)
}

// GetGVK returns the PolicySet GVK for this builder.
func (builder *PolicySetBuilder) GetGVK() schema.GroupVersionKind {
	return policySetGVK
}

// NewPolicySetBuilder creates a new instance of PolicySetBuilder.
func NewPolicySetBuilder(
	apiClient *clients.Settings, name, nsname string, policy policiesv1beta1.NonEmptyString) *PolicySetBuilder {
	builder := common.NewNamespacedBuilder[policiesv1beta1.PolicySet, PolicySetBuilder](
		apiClient, policiesv1beta1.AddToScheme, name, nsname)
	if builder.GetError() != nil {
		return builder
	}

	if policy == "" {
		klog.V(100).Info("The policy of the PolicySet is empty")

		builder.SetError(fmt.Errorf("policyset's 'policy' cannot be empty"))

		return builder
	}

	builder.Definition.Spec.Policies = []policiesv1beta1.NonEmptyString{policy}

	return builder
}

// PullPolicySet pulls existing policySet into Builder struct.
func PullPolicySet(apiClient *clients.Settings, name, nsname string) (*PolicySetBuilder, error) {
	return common.PullNamespacedBuilder[policiesv1beta1.PolicySet, PolicySetBuilder](
		context.TODO(), apiClient, policiesv1beta1.AddToScheme, name, nsname)
}

// WithAdditionalPolicy appends a policy to the policies list in the PolicySet definition.
func (builder *PolicySetBuilder) WithAdditionalPolicy(policy policiesv1beta1.NonEmptyString) *PolicySetBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof(
		"Adding Policy %v to PolicySet %s in namespace %s", policy, builder.Definition.Name, builder.Definition.Namespace)

	if policy == "" {
		klog.V(100).Info("The policy to be added to the PolicySet's Policies is empty")

		builder.SetError(fmt.Errorf("policy in PolicySet Policies spec cannot be empty"))

		return builder
	}

	builder.Definition.Spec.Policies = append(builder.Definition.Spec.Policies, policy)

	return builder
}
