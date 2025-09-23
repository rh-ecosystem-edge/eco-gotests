package rbac

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	"golang.org/x/exp/slices"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterRoleBindingBuilder provides struct for clusterrolebinding object
// containing connection to the cluster and the clusterrolebinding definitions.
type ClusterRoleBindingBuilder struct {
	// Clusterrolebinding definition. Used to create a clusterrolebinding object.
	Definition *rbacv1.ClusterRoleBinding
	// Created clusterrolebinding object
	Object *rbacv1.ClusterRoleBinding
	// Used in functions that define or mutate clusterrolebinding definition.
	// errorMsg is processed before the clusterrolebinding object is created.
	errorMsg  string
	apiClient *clients.Settings
}

// ClusterRoleBindingAdditionalOptions additional options for ClusterRoleBinding object.
type ClusterRoleBindingAdditionalOptions func(builder *ClusterRoleBindingBuilder) (*ClusterRoleBindingBuilder, error)

// NewClusterRoleBindingBuilder creates a new instance of ClusterRoleBindingBuilder.
func NewClusterRoleBindingBuilder(
	apiClient *clients.Settings, name, clusterRole string, subject rbacv1.Subject) *ClusterRoleBindingBuilder {
	glog.V(100).Infof(
		"Initializing new clusterrolebinding structure with the following params: "+
			"name: %s, clusterrole: %s, subject %v",
		name, clusterRole, subject)

	builder := &ClusterRoleBindingBuilder{
		apiClient: apiClient,
		Definition: &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Name:     clusterRole,
				Kind:     "ClusterRole",
			},
		},
	}

	builder.WithSubjects([]rbacv1.Subject{subject})

	if name == "" {
		glog.V(100).Infof("The name of the clusterrolebinding is empty")

		builder.errorMsg = "clusterrolebinding 'name' cannot be empty"

		return builder
	}

	return builder
}

// WithSubjects appends additional subjects to clusterrolebinding definition.
func (builder *ClusterRoleBindingBuilder) WithSubjects(subjects []rbacv1.Subject) *ClusterRoleBindingBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Appending to the definition of clusterrolebinding %s these additional subjects %v",
		builder.Definition.Name, subjects)

	if len(subjects) == 0 {
		glog.V(100).Infof("The list of subjects is empty")

		builder.errorMsg = "cannot accept nil or empty slice as subjects"

		return builder
	}

	for _, subject := range subjects {
		if !slices.Contains(allowedSubjectKinds(), subject.Kind) {
			glog.V(100).Infof("The clusterrolebinding subject kind must be one of 'ServiceAccount', 'User', or 'Group'")

			builder.errorMsg = "clusterrolebinding subject kind must be one of 'ServiceAccount', 'User', or 'Group'"

			return builder
		}

		if subject.Name == "" {
			glog.V(100).Infof("The clusterrolebinding subject name cannot be empty")

			builder.errorMsg = "clusterrolebinding subject name cannot be empty"

			return builder
		}
	}

	builder.Definition.Subjects = append(builder.Definition.Subjects, subjects...)

	return builder
}

// WithOptions creates ClusterRoleBinding with generic mutation options.
func (builder *ClusterRoleBindingBuilder) WithOptions(
	options ...ClusterRoleBindingAdditionalOptions) *ClusterRoleBindingBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting ClusterRoleBinding additional options")

	for _, option := range options {
		if option != nil {
			builder, err := option(builder)
			if err != nil {
				glog.V(100).Infof("Error occurred in mutation function")

				builder.errorMsg = err.Error()

				return builder
			}
		}
	}

	return builder
}

// PullClusterRoleBinding pulls existing clusterrolebinding from cluster.
func PullClusterRoleBinding(apiClient *clients.Settings, name string) (*ClusterRoleBindingBuilder, error) {
	glog.V(100).Infof("Pulling existing clusterrolebinding name %s from cluster", name)

	builder := ClusterRoleBindingBuilder{
		apiClient: apiClient,
		Definition: &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the clusterrolebinding is empty")

		return nil, fmt.Errorf("clusterrolebinding 'name' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("clusterrolebinding object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Create generates a clusterrolebinding in the cluster and stores the created object in struct.
func (builder *ClusterRoleBindingBuilder) Create() (*ClusterRoleBindingBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating clusterrolebinding %s",
		builder.Definition.Name)

	var err error
	if !builder.Exists() {
		builder.Object, err = builder.apiClient.ClusterRoleBindings().Create(
			context.TODO(), builder.Definition, metav1.CreateOptions{})
	}

	return builder, err
}

// Delete removes a clusterrolebinding from the cluster.
func (builder *ClusterRoleBindingBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Removing clusterrolebinding %s",
		builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("ClusterRoleBinding object %s does not exist",
			builder.Definition.Name)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.ClusterRoleBindings().Delete(
		context.TODO(), builder.Definition.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// Update modifies a clusterrolebinding object in the cluster.
func (builder *ClusterRoleBindingBuilder) Update() (*ClusterRoleBindingBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating clusterrolebinding %s",
		builder.Definition.Name)

	var err error

	builder.Object, err = builder.apiClient.ClusterRoleBindings().Update(
		context.TODO(), builder.Definition, metav1.UpdateOptions{})

	return builder, err
}

// Exists checks if clusterrolebinding exists in the cluster.
func (builder *ClusterRoleBindingBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if clusterrolebinding %s exists",
		builder.Definition.Name)

	var err error

	builder.Object, err = builder.apiClient.ClusterRoleBindings().Get(
		context.TODO(), builder.Definition.Name, metav1.GetOptions{})

	return err == nil || !k8serrors.IsNotFound(err)
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ClusterRoleBindingBuilder) validate() (bool, error) {
	resourceCRD := "ClusterRoleBinding"

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
