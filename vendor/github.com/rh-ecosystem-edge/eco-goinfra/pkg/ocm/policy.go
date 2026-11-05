package ocm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

var policyGVK = policiesv1.GroupVersion.WithKind("Policy")

// PolicyBuilder provides struct for the policy object containing connection to
// the cluster and the policy definitions.
type PolicyBuilder struct {
	common.EmbeddableBuilder[policiesv1.Policy, *policiesv1.Policy]
	common.EmbeddableCreator[policiesv1.Policy, PolicyBuilder, *policiesv1.Policy, *PolicyBuilder]
	common.EmbeddableDeleteReturner[policiesv1.Policy, PolicyBuilder, *policiesv1.Policy, *PolicyBuilder]
	common.EmbeddableForceUpdater[policiesv1.Policy, PolicyBuilder, *policiesv1.Policy, *PolicyBuilder]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *PolicyBuilder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleteReturner.SetBase(builder)
	builder.EmbeddableForceUpdater.SetBase(builder)
}

// GetGVK returns the Policy GVK for this builder.
func (builder *PolicyBuilder) GetGVK() schema.GroupVersionKind {
	return policyGVK
}

// NewPolicyBuilder creates a new instance of PolicyBuilder.
func NewPolicyBuilder(
	apiClient *clients.Settings, name, nsname string, template *policiesv1.PolicyTemplate) *PolicyBuilder {
	builder := common.NewNamespacedBuilder[policiesv1.Policy, PolicyBuilder](
		apiClient, policiesv1.AddToScheme, name, nsname)
	if builder.GetError() != nil {
		return builder
	}

	if template == nil {
		klog.V(100).Info("The PolicyTemplate of the Policy is nil")

		builder.SetError(fmt.Errorf("policy 'template' cannot be nil"))

		return builder
	}

	builder.Definition.Spec.PolicyTemplates = []*policiesv1.PolicyTemplate{template}

	return builder
}

// PullPolicy pulls existing policy into Builder struct.
func PullPolicy(apiClient *clients.Settings, name, nsname string) (*PolicyBuilder, error) {
	return common.PullNamespacedBuilder[policiesv1.Policy, PolicyBuilder](
		context.TODO(), apiClient, policiesv1.AddToScheme, name, nsname)
}

// WithRemediationAction sets a RemediationAction in the policy definition.
func (builder *PolicyBuilder) WithRemediationAction(action policiesv1.RemediationAction) *PolicyBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Setting RemediationAction for policy %s to %v", builder.Definition.Name, action)

	// Lowercase versions are allowed even if there's no constant for them in policiesv1.
	if action != policiesv1.Inform && action != policiesv1.Enforce && action != "inform" && action != "enforce" {
		klog.V(100).Info("The RemediationAction to be set in the Policy spec is neither 'Inform' nor 'Enforce'")

		builder.SetError(fmt.Errorf("remediation action in policy spec must be either 'Inform' or 'Enforce'"))

		return builder
	}

	builder.Definition.Spec.RemediationAction = action

	return builder
}

// WithAdditionalPolicyTemplate appends a PolicyTemplate to the PolicyTemplates in the policy definition.
func (builder *PolicyBuilder) WithAdditionalPolicyTemplate(template *policiesv1.PolicyTemplate) *PolicyBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Adding PolicyTemplate to policy %s", builder.Definition.Name)

	if template == nil {
		klog.V(100).Info("The PolicyTemplate to be added to the Policy's PolicyTemplates is nil")

		builder.SetError(fmt.Errorf("policy template in policy policytemplates cannot be nil"))

		return builder
	}

	builder.Definition.Spec.PolicyTemplates = append(builder.Definition.Spec.PolicyTemplates, template)

	return builder
}

// WaitUntilDeleted waits for the duration of the defined timeout or until the policy is deleted.
func (builder *PolicyBuilder) WaitUntilDeleted(timeout time.Duration) error {
	if err := common.Validate(builder); err != nil {
		return err
	}

	klog.V(100).Infof(
		"Waiting for the defined period until policy %s in namespace %s is deleted",
		builder.Definition.Name, builder.Definition.Namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := builder.Get()
			if err == nil {
				klog.V(100).Infof("policy %s/%s still present", builder.Definition.Name, builder.Definition.Namespace)

				return false, nil
			}

			if k8serrors.IsNotFound(err) {
				klog.V(100).Infof("policy %s/%s is gone", builder.Definition.Name, builder.Definition.Namespace)

				return true, nil
			}

			klog.V(100).Infof("failed to get policy %s/%s: %v", builder.Definition.Name, builder.Definition.Namespace, err)

			return false, err
		})
}

// WaitUntilComplianceState waits for the duration of the defined timeout or until the policy is in the provided
// compliance state.
func (builder *PolicyBuilder) WaitUntilComplianceState(state policiesv1.ComplianceState, timeout time.Duration) error {
	if err := common.Validate(builder); err != nil {
		return err
	}

	klog.V(100).Infof(
		"Waiting for the defined period until policy %s in namespace %s is in compliance state %v",
		builder.Definition.Name, builder.Definition.Namespace, state)

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			updatedPolicy, err := builder.Get()
			if err != nil {
				klog.V(100).Infof(
					"error getting policy %s in namespace %s: %v", builder.Definition.Name, builder.Definition.Namespace, err)

				return false, nil
			}

			return updatedPolicy.Status.ComplianceState == state, nil
		})
}

// WaitForStatusMessageToContain waits up to the specified timeout for the policy message to contain the
// expectedMessage.
func (builder *PolicyBuilder) WaitForStatusMessageToContain(
	expectedMessage string, timeout time.Duration) (*PolicyBuilder, error) {
	if err := common.Validate(builder); err != nil {
		return nil, err
	}

	if expectedMessage == "" {
		klog.V(100).Info("expectedMessage for policy cannot be empty")

		return nil, fmt.Errorf("policy expectedMessage is empty")
	}

	klog.V(100).Infof(
		"Waiting until status message of policy %s in namespace %s contains '%s'",
		builder.Definition.Name, builder.Definition.Namespace, expectedMessage)

	if !builder.Exists() {
		return nil, fmt.Errorf(
			"policy object %s does not exist in namespace %s", builder.Definition.Name, builder.Definition.Namespace)
	}

	var err error

	err = wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			builder.Object, err = builder.Get()
			if err != nil {
				return false, nil
			}

			details := builder.Object.Status.Details
			if len(details) > 0 && len(details[0].History) > 0 {
				message := details[0].History[0].Message

				klog.V(100).Infof("Checking if message '%s' contains substring '%s'", message, expectedMessage)

				return strings.Contains(message, expectedMessage), nil
			}

			return false, nil
		})
	if err != nil {
		return nil, err
	}

	return builder, nil
}
