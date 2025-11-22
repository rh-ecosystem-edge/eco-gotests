package rbac

import (
	"context"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	"golang.org/x/exp/slices"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// RoleBindingBuilder provides struct for RoleBinding object containing connection
// to the cluster RoleBinding definition.
type RoleBindingBuilder struct {
	// Rolebinding definition. Used to create rolebinding object
	Definition *rbacv1.RoleBinding
	// Created rolebinding object
	Object *rbacv1.RoleBinding

	// Used in functions that define or mutate rolebinding definition. errorMsg is processed
	// before the rolebinding object is created
	errorMsg  string
	apiClient *clients.Settings
}

// RoleBindingAdditionalOptions additional options for RoleBinding object.
type RoleBindingAdditionalOptions func(builder *RoleBindingBuilder) (*RoleBindingBuilder, error)

// NewRoleBindingBuilder creates new instance of RoleBindingBuilder.
func NewRoleBindingBuilder(apiClient *clients.Settings,
	name, nsname, role string,
	subject rbacv1.Subject) *RoleBindingBuilder {
	klog.V(100).Infof(
		"Initializing new rolebinding structure with the following params: "+
			"name: %s, namespace: %s, role: %s, subject %v", name, nsname, role, subject)

	builder := &RoleBindingBuilder{
		apiClient: apiClient,
		Definition: &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Name:     role,
				Kind:     "Role",
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the rolebinding is empty")

		builder.errorMsg = "RoleBinding 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the rolebinding is empty")

		builder.errorMsg = "RoleBinding 'nsname' cannot be empty"

		return builder
	}

	builder.WithSubjects([]rbacv1.Subject{subject})

	return builder
}

// WithSubjects adds specified Subject to the RoleBinding.
func (builder *RoleBindingBuilder) WithSubjects(subjects []rbacv1.Subject) *RoleBindingBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Adding to the rolebinding %s these specified subjects: %v",
		builder.Definition.Name, subjects)

	if len(subjects) == 0 {
		klog.V(100).Info("The list of subjects is empty")

		builder.errorMsg = "cannot create rolebinding with empty subject"

		return builder
	}

	for _, subject := range subjects {
		if !slices.Contains(allowedSubjectKinds(), subject.Kind) {
			klog.V(100).Info("The rolebinding subject kind must be one of 'ServiceAccount', 'User', or 'Group'")

			builder.errorMsg = "rolebinding subject kind must be one of 'ServiceAccount', 'User', 'Group'"

			return builder
		}

		if subject.Name == "" {
			klog.V(100).Info("The rolebinding subject name cannot be empty")

			builder.errorMsg = "rolebinding subject name cannot be empty"

			return builder
		}
	}

	builder.Definition.Subjects = append(builder.Definition.Subjects, subjects...)

	return builder
}

// WithOptions creates RoleBinding with generic mutation options.
func (builder *RoleBindingBuilder) WithOptions(options ...RoleBindingAdditionalOptions) *RoleBindingBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting RoleBinding additional options")

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

// PullRoleBinding pulls existing rolebinding from cluster.
func PullRoleBinding(apiClient *clients.Settings, name, nsname string) (*RoleBindingBuilder, error) {
	klog.V(100).Infof("Pulling existing rolebinding name %s under namespace %s from cluster", name, nsname)

	builder := &RoleBindingBuilder{
		apiClient: apiClient,
		Definition: &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the rolebinding is empty")

		return builder, fmt.Errorf("rolebinding 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the rolebinding is empty")

		return builder, fmt.Errorf("rolebinding 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("rolebinding object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create generates a RoleBinding and stores the created object in struct.
func (builder *RoleBindingBuilder) Create() (*RoleBindingBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating rolebinding %s under namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		builder.Object, err = builder.apiClient.RoleBindings(builder.Definition.Namespace).Create(
			context.TODO(), builder.Definition, metav1.CreateOptions{})
	}

	return builder, err
}

// Delete removes a RoleBinding.
func (builder *RoleBindingBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Removing rolebinding %s under namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("RoleBinding object %s namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.RoleBindings(builder.Definition.Namespace).Delete(
		context.TODO(), builder.Definition.Name, metav1.DeleteOptions{})

	builder.Object = nil

	return err
}

// Update modifies an existing RoleBinding in the cluster.
func (builder *RoleBindingBuilder) Update() (*RoleBindingBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating rolebinding %s under namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.apiClient.RoleBindings(builder.Definition.Namespace).Update(
		context.TODO(), builder.Definition, metav1.UpdateOptions{})

	return builder, err
}

// Exists checks whether the given RoleBinding exists.
func (builder *RoleBindingBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if rolebinding %s exists under namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.apiClient.RoleBindings(builder.Definition.Namespace).Get(
		context.TODO(), builder.Definition.Name, metav1.GetOptions{})

	return err == nil || !k8serrors.IsNotFound(err)
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *RoleBindingBuilder) validate() (bool, error) {
	resourceCRD := "RoleBinding"

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
