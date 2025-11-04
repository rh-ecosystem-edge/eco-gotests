package ocm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// PolicyBuilder provides struct for the policy object containing connection to
// the cluster and the policy definitions.
type PolicyBuilder struct {
	// policy Definition, used to create the policy object.
	Definition *policiesv1.Policy
	// created policy object.
	Object *policiesv1.Policy
	// api client to interact with the cluster.
	apiClient runtimeclient.Client
	// used to store latest error message upon defining or mutating application definition.
	errorMsg string
}

// NewPolicyBuilder creates a new instance of PolicyBuilder.
func NewPolicyBuilder(
	apiClient *clients.Settings, name, nsname string, template *policiesv1.PolicyTemplate) *PolicyBuilder {
	glog.V(100).Infof(
		"Initializing new policy structure with the following params: name: %s, nsname: %s",
		name, nsname)

	if apiClient == nil {
		glog.V(100).Info("The apiClient of the Policy is nil")

		return nil
	}

	err := apiClient.AttachScheme(policiesv1.AddToScheme)
	if err != nil {
		glog.V(100).Info("Failed to add Policy scheme to client schemes")

		return nil
	}

	builder := &PolicyBuilder{
		apiClient: apiClient.Client,
		Definition: &policiesv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
			Spec: policiesv1.PolicySpec{
				PolicyTemplates: []*policiesv1.PolicyTemplate{template},
			},
		},
	}

	if name == "" {
		glog.V(100).Info("The name of the Policy is empty")

		builder.errorMsg = "policy 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Info("The namespace of the Policy is empty")

		builder.errorMsg = "policy 'nsname' cannot be empty"

		return builder
	}

	if template == nil {
		glog.V(100).Info("The PolicyTemplate of the Policy is nil")

		builder.errorMsg = "policy 'template' cannot be nil"

		return builder
	}

	return builder
}

// PullPolicy pulls existing policy into Builder struct.
func PullPolicy(apiClient *clients.Settings, name, nsname string) (*PolicyBuilder, error) {
	glog.V(100).Infof("Pulling existing policy name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is nil")

		return nil, fmt.Errorf("policy 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(policiesv1.AddToScheme)
	if err != nil {
		glog.V(100).Info("Failed to add Policy scheme to client schemes")

		return nil, err
	}

	builder := &PolicyBuilder{
		apiClient: apiClient.Client,
		Definition: &policiesv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the policy is empty")

		return nil, fmt.Errorf("policy's 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the policy is empty")

		return nil, fmt.Errorf("policy's 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("policy object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Exists checks whether the given policy exists.
func (builder *PolicyBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if policy %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get returns a policy object if found.
func (builder *PolicyBuilder) Get() (*policiesv1.Policy, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting policy %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	policy := &policiesv1.Policy{}

	err := builder.apiClient.Get(context.TODO(), runtimeclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, policy)
	if err != nil {
		glog.V(100).Infof("Failed to get policy %s in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return nil, err
	}

	return policy, nil
}

// Create makes a policy in the cluster and stores the created object in struct.
func (builder *PolicyBuilder) Create() (*PolicyBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the policy %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if builder.Exists() {
		return builder, nil
	}

	err := builder.apiClient.Create(context.TODO(), builder.Definition)
	if err != nil {
		return builder, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Delete removes a policy from a cluster.
func (builder *PolicyBuilder) Delete() (*PolicyBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Deleting the policy %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof("policy %s namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete policy: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Update renovates the existing policy object with the policy definition in builder.
func (builder *PolicyBuilder) Update(force bool) (*PolicyBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	if !builder.Exists() {
		glog.V(100).Infof("Policy %s does not exist in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

		return nil, fmt.Errorf("cannot update non-existent policy")
	}

	glog.V(100).Infof("Updating the policy object: %s in namespace: %s",
		builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.ResourceVersion = builder.Object.ResourceVersion

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		if force {
			glog.V(100).Infof(
				msg.FailToUpdateNotification("policy", builder.Definition.Name, builder.Definition.Namespace))

			builder, err := builder.Delete()
			builder.Definition.ResourceVersion = ""

			if err != nil {
				glog.V(100).Infof(
					msg.FailToUpdateError("policy", builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithRemediationAction sets a RemediationAction in the policy definition.
func (builder *PolicyBuilder) WithRemediationAction(action policiesv1.RemediationAction) *PolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting RemediationAction for policy %s to %v", builder.Definition.Name, action)

	// Lowercase versions are allowed even if there's no constant for them in policiesv1.
	if action != policiesv1.Inform && action != policiesv1.Enforce && action != "inform" && action != "enforce" {
		glog.V(100).Info("The RemediationAction to be set in the Policy spec is neither 'Inform' nor 'Enforce'")

		builder.errorMsg = "remediation action in policy spec must be either 'Inform' or 'Enforce'"

		return builder
	}

	builder.Definition.Spec.RemediationAction = action

	return builder
}

// WithAdditionalPolicyTemplate appends a PolicyTemplate to the PolicyTemplates in the policy definition.
func (builder *PolicyBuilder) WithAdditionalPolicyTemplate(template *policiesv1.PolicyTemplate) *PolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding PolicyTemplate to policy %s", builder.Definition.Name)

	if template == nil {
		glog.V(100).Info("The PolicyTemplate to be added to the Policy's PolicyTemplates is nil")

		builder.errorMsg = "policy template in policy policytemplates cannot be nil"

		return builder
	}

	builder.Definition.Spec.PolicyTemplates = append(builder.Definition.Spec.PolicyTemplates, template)

	return builder
}

// WaitUntilDeleted waits for the duration of the defined timeout or until the policy is deleted.
func (builder *PolicyBuilder) WaitUntilDeleted(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof(
		"Waiting for the defined period until policy %s in namespace %s is deleted",
		builder.Definition.Name, builder.Definition.Namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := builder.Get()
			if err == nil {
				glog.V(100).Infof("policy %s/%s still present", builder.Definition.Name, builder.Definition.Namespace)

				return false, nil
			}

			if k8serrors.IsNotFound(err) {
				glog.V(100).Infof("policy %s/%s is gone", builder.Definition.Name, builder.Definition.Namespace)

				return true, nil
			}

			glog.V(100).Infof("failed to get policy %s/%s: %w", builder.Definition.Name, builder.Definition.Namespace, err)

			return false, err
		})
}

// WaitUntilComplianceState waits for the duration of the defined timeout or until the policy is in the provided
// compliance state.
func (builder *PolicyBuilder) WaitUntilComplianceState(state policiesv1.ComplianceState, timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof(
		"Waiting for the defined period until policy %s in namespace %s is in compliance state %v",
		builder.Definition.Name, builder.Definition.Namespace, state)

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			updatedPolicy, err := builder.Get()
			if err != nil {
				glog.V(100).Infof(
					"error getting policy %s in namespace %s: %w", builder.Definition.Name, builder.Definition.Namespace, err)

				return false, nil
			}

			return updatedPolicy.Status.ComplianceState == state, nil
		})
}

// WaitForStatusMessageToContain waits up to the specified timeout for the policy message to contain the
// expectedMessage.
func (builder *PolicyBuilder) WaitForStatusMessageToContain(
	expectedMessage string, timeout time.Duration) (*PolicyBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	if expectedMessage == "" {
		glog.V(100).Info("expectedMessage for policy cannot be empty")

		return nil, fmt.Errorf("policy expectedMessage is empty")
	}

	glog.V(100).Infof(
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

				glog.V(100).Infof("Checking if message '%s' contains substring '%s'", message, expectedMessage)

				return strings.Contains(message, expectedMessage), nil
			}

			return false, nil
		})
	if err != nil {
		return nil, err
	}

	return builder, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *PolicyBuilder) validate() (bool, error) {
	resourceCRD := "policy"

	if builder == nil {
		glog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		glog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		glog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		glog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
