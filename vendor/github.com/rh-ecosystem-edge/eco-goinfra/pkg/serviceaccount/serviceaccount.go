package serviceaccount

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	corev1Typed "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/utils/ptr"
)

// Builder provides struct for serviceaccount object containing connection to the cluster and the
// serviceaccount definitions.
type Builder struct {
	// ServiceAccount definition. Used to create serviceaccount object.
	Definition *corev1.ServiceAccount
	// Created serviceaccount object.
	Object *corev1.ServiceAccount
	// Used in functions that defines or mutates configmap definition. errorMsg is processed before the configmap
	// object is created.
	errorMsg  string
	apiClient corev1Typed.ServiceAccountInterface
}

// AdditionalOptions additional options for ServiceAccount object.
type AdditionalOptions func(builder *Builder) (*Builder, error)

// NewBuilder creates a new instance of Builder.
func NewBuilder(apiClient *clients.Settings, name, nsname string) *Builder {
	glog.V(100).Infof("Initializing new serviceaccount structure with the following params: %s, %s", name, nsname)

	builder := &Builder{
		apiClient: apiClient.ServiceAccounts(nsname),
		Definition: &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the serviceaccount is empty")

		builder.errorMsg = "serviceaccount 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the serviceaccount is empty")

		builder.errorMsg = "serviceaccount 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// Pull loads an existing serviceaccount into Builder struct.
func Pull(apiClient *clients.Settings, name, nsname string) (*Builder, error) {
	glog.V(100).Infof("Pulling existing serviceaccount name: %s under namespace: %s", name, nsname)

	builder := &Builder{
		apiClient: apiClient.ServiceAccounts(nsname),
		Definition: &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		builder.errorMsg = "serviceaccount 'name' cannot be empty"

		return builder, fmt.Errorf("serviceaccount 'name' cannot be empty")
	}

	if nsname == "" {
		builder.errorMsg = "serviceaccount 'namespace' cannot be empty"

		return builder, fmt.Errorf("serviceaccount 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("serviceaccount object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create makes a serviceaccount in cluster and stores the created object in struct.
func (builder *Builder) Create() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof(
		"Creating serviceaccount %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		builder.Object, err = builder.apiClient.Create(
			context.TODO(), builder.Definition, metav1.CreateOptions{})
	}

	return builder, err
}

// CreateToken creates a new token for the builder's service account using the provided duration and audiences. The zero
// values of duration and audiences are both allowed. Note that the duration of the token returned is not guaranteed to
// match the requested duration. Its expiration will be logged, however.
func (builder *Builder) CreateToken(duration time.Duration, audiences ...string) (string, error) {
	if valid, err := builder.validate(); !valid {
		return "", err
	}

	glog.V(100).Infof("Creating token for serviceaccount %s in namespace %s with duration %s",
		builder.Definition.Name, builder.Definition.Namespace, duration)

	if !builder.Exists() {
		glog.V(100).Infof("Cannot create a token for serviceaccount %s in namespace %s because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		return "", fmt.Errorf("serviceaccount %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)
	}

	var durationSeconds *int64
	if floatDuration := duration.Round(time.Second).Seconds(); floatDuration > 0 {
		durationSeconds = ptr.To(int64(floatDuration))
	}

	tokenRequest := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences:         audiences,
			ExpirationSeconds: durationSeconds,
		},
	}

	tokenRequest, err := builder.apiClient.CreateToken(
		context.TODO(), builder.Definition.Name, tokenRequest, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}

	glog.V(100).Infof("Successfully created token for serviceaccount %s in namespace %s with expiration %s",
		builder.Definition.Name, builder.Definition.Namespace, tokenRequest.Status.ExpirationTimestamp.Format(time.RFC3339))

	return tokenRequest.Status.Token, nil
}

// Delete removes a serviceaccount.
func (builder *Builder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof(
		"Deleting serviceaccount %s from namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof("ServiceAccount %s namespace %s does not exist and cannot be deleted",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(
		context.TODO(), builder.Definition.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// Exists checks whether the given serviceaccount exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof(
		"Checking if serviceaccount %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.apiClient.Get(
		context.TODO(), builder.Definition.Name, metav1.GetOptions{})

	return err == nil || !k8serrors.IsNotFound(err)
}

// WithOptions creates serviceAccount with generic mutation options.
func (builder *Builder) WithOptions(options ...AdditionalOptions) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting serviceAccount additional options")

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

// GetGVR returns service's GroupVersionResource which could be used for Clean function.
func GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"}
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "ServiceAccount"

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
