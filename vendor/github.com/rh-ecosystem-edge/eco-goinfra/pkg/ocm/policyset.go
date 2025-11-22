package ocm

import (
	"context"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	policiesv1beta1 "open-cluster-management.io/governance-policy-propagator/api/v1beta1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// PolicySetBuilder provides struct for the policySet object containing connection to
// the cluster and the policySet definitions.
type PolicySetBuilder struct {
	// policySet Definition, used to create the policySet object.
	Definition *policiesv1beta1.PolicySet
	// created policySet object.
	Object *policiesv1beta1.PolicySet
	// api client to interact with the cluster.
	apiClient runtimeclient.Client
	// used to store latest error message upon defining or mutating policySet definition.
	errorMsg string
}

// NewPolicySetBuilder creates a new instance of PolicySetBuilder.
func NewPolicySetBuilder(
	apiClient *clients.Settings, name, nsname string, policy policiesv1beta1.NonEmptyString) *PolicySetBuilder {
	klog.V(100).Infof(
		"Initializing new policy set structure with the following params: name: %s, nsname: %s, policy: %v",
		name, nsname, policy)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is nil")

		return nil
	}

	err := apiClient.AttachScheme(policiesv1beta1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add PolicySet scheme to client schemes")

		return nil
	}

	builder := &PolicySetBuilder{
		apiClient: apiClient.Client,
		Definition: &policiesv1beta1.PolicySet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
			Spec: policiesv1beta1.PolicySetSpec{
				Policies: []policiesv1beta1.NonEmptyString{policy},
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the PolicySet is empty")

		builder.errorMsg = "policyset's 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the PolicySet is empty")

		builder.errorMsg = "policyset's 'nsname' cannot be empty"

		return builder
	}

	if policy == "" {
		klog.V(100).Info("The policy of the PolicySet is empty")

		builder.errorMsg = "policyset's 'policy' cannot be empty"

		return builder
	}

	return builder
}

// PullPolicySet pulls existing policySet into Builder struct.
func PullPolicySet(apiClient *clients.Settings, name, nsname string) (*PolicySetBuilder, error) {
	klog.V(100).Infof("Pulling existing policySet name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is nil")

		return nil, fmt.Errorf("policyset's 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(policiesv1beta1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add PolicySet scheme to client schemes")

		return nil, err
	}

	builder := &PolicySetBuilder{
		apiClient: apiClient.Client,
		Definition: &policiesv1beta1.PolicySet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the policyset is empty")

		return nil, fmt.Errorf("policyset's 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the policyset is empty")

		return nil, fmt.Errorf("policyset's 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("policyset object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Exists checks whether the given policySet exists.
func (builder *PolicySetBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if policySet %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get returns a policySet object if found.
func (builder *PolicySetBuilder) Get() (*policiesv1beta1.PolicySet, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting policySet %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	policySet := &policiesv1beta1.PolicySet{}

	err := builder.apiClient.Get(context.TODO(), runtimeclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, policySet)
	if err != nil {
		klog.V(100).Infof("Failed to get policySet %s in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return nil, err
	}

	return policySet, nil
}

// Create makes a policySet in the cluster and stores the created object in struct.
func (builder *PolicySetBuilder) Create() (*PolicySetBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the policySet %s in namespace %s",
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

// Delete removes a policySet from a cluster.
func (builder *PolicySetBuilder) Delete() (*PolicySetBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting the policySet %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("PolicySet %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete policySet: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Update renovates the existing policySet object with the policySet's definition in builder.
func (builder *PolicySetBuilder) Update(force bool) (*PolicySetBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	if !builder.Exists() {
		klog.V(100).Infof(
			"PolicySet %s does not exist in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

		return nil, fmt.Errorf("cannot update non-existent policySet")
	}

	klog.V(100).Infof("Updating the policySet object: %s in namespace: %s",
		builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.ResourceVersion = builder.Object.ResourceVersion

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%v", msg.FailToUpdateNotification("policySet", builder.Definition.Name, builder.Definition.Namespace))

			builder, err := builder.Delete()
			builder.Definition.ResourceVersion = ""

			if err != nil {
				klog.V(100).Infof("%v", msg.FailToUpdateError("policySet", builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithAdditionalPolicy appends a policy to the policies list in the PolicySet definition.
func (builder *PolicySetBuilder) WithAdditionalPolicy(policy policiesv1beta1.NonEmptyString) *PolicySetBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Adding Policy %v to PolicySet %s in namespace %s", policy, builder.Definition.Name, builder.Definition.Namespace)

	if policy == "" {
		klog.V(100).Info("The policy to be added to the PolicySet's Policies is empty")

		builder.errorMsg = "policy in PolicySet Policies spec cannot be empty"

		return builder
	}

	builder.Definition.Spec.Policies = append(builder.Definition.Spec.Policies, policy)

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *PolicySetBuilder) validate() (bool, error) {
	resourceCRD := "policySet"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		klog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		klog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
