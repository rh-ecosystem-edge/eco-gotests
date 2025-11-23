package rbac

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// RoleBuilder provides a struct for role object containing connection to the cluster and the role definitions.
type RoleBuilder struct {
	// Role definition. Used to create a role object
	Definition *rbacv1.Role
	// Created role object
	Object *rbacv1.Role

	// Used in functions that define or mutate role definition. errorMsg is processed
	// before the role object is created
	errorMsg  string
	apiClient *clients.Settings
}

// RoleAdditionalOptions additional options for Role object.
type RoleAdditionalOptions func(builder *RoleBuilder) (*RoleBuilder, error)

// NewRoleBuilder create a new instance of RoleBuilder.
func NewRoleBuilder(apiClient *clients.Settings, name, nsname string, rule rbacv1.PolicyRule) *RoleBuilder {
	klog.V(100).Infof(
		"Initializing new role structure with the following params: "+
			"name: %s, namespace: %s, rule %v", name, nsname, rule)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil
	}

	builder := &RoleBuilder{
		apiClient: apiClient,
		Definition: &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the role is empty")

		builder.errorMsg = "role 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the role is empty")

		builder.errorMsg = "role 'nsname' cannot be empty"

		return builder
	}

	builder.WithRules([]rbacv1.PolicyRule{rule})

	return builder
}

// WithRules adds the specified PolicyRule to the Role.
func (builder *RoleBuilder) WithRules(rules []rbacv1.PolicyRule) *RoleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Adding to role %s the following rules: %v",
		builder.Definition.Name, rules)

	if len(rules) == 0 {
		klog.V(100).Info("The list of rules is empty")

		builder.errorMsg = "cannot create role with empty rule"

		return builder
	}

	for _, rule := range rules {
		if len(rule.Verbs) == 0 {
			klog.V(100).Info("The role has no verbs")

			builder.errorMsg = "role must contain at least one Verb"

			return builder
		}

		if len(rule.Resources) == 0 {
			klog.V(100).Info("The role has no resources")

			builder.errorMsg = "role must contain at least one Resource"

			return builder
		}

		if len(rule.APIGroups) == 0 {
			klog.V(100).Info("The role has no apigroups")

			builder.errorMsg = "role must contain at least one APIGroup"

			return builder
		}
	}

	if builder.Definition.Rules == nil {
		builder.Definition.Rules = rules
	} else {
		builder.Definition.Rules = append(builder.Definition.Rules, rules...)
	}

	return builder
}

// WithOptions creates Role with generic mutation options.
func (builder *RoleBuilder) WithOptions(
	options ...RoleAdditionalOptions) *RoleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting Role additional options")

	for _, option := range options {
		if option != nil {
			builder, err := option(builder)
			if err != nil {
				klog.V(100).Info("Error occurred in mutation function")

				builder.errorMsg = err.Error()

				return builder
			}
		}
	}

	return builder
}

// PullRole pulls existing role from cluster.
func PullRole(apiClient *clients.Settings, name, nsname string) (*RoleBuilder, error) {
	klog.V(100).Infof("Pulling existing role name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient cannot be nil")
	}

	builder := &RoleBuilder{
		apiClient: apiClient,
		Definition: &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the role is empty")

		return nil, fmt.Errorf("role 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the role is empty")

		return nil, fmt.Errorf("role 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("role object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create makes a Role in the cluster and stores the created object in struct.
func (builder *RoleBuilder) Create() (*RoleBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating role %s under namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		builder.Object, err = builder.apiClient.Roles(builder.Definition.Namespace).Create(
			logging.DiscardContext(), builder.Definition, metav1.CreateOptions{})
	}

	return builder, err
}

// Delete removes a Role.
func (builder *RoleBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Removing role %s under namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("Role %s namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Roles(builder.Definition.Namespace).Delete(
		logging.DiscardContext(), builder.Definition.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// Update modifies the existing Role object with role definition in builder.
func (builder *RoleBuilder) Update() (*RoleBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating role %s under namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return nil, fmt.Errorf("role object %s does not exist, fail to update", builder.Definition.Name)
	}

	var err error

	builder.Object, err = builder.apiClient.Roles(builder.Definition.Namespace).Update(
		logging.DiscardContext(), builder.Definition, metav1.UpdateOptions{})

	return builder, err
}

// Exists checks whether the given Role exists.
func (builder *RoleBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if role %s exists under namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.apiClient.Roles(builder.Definition.Namespace).Get(
		logging.DiscardContext(), builder.Definition.Name, metav1.GetOptions{})

	return err == nil || !k8serrors.IsNotFound(err)
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *RoleBuilder) validate() (bool, error) {
	resourceCRD := "role"

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
