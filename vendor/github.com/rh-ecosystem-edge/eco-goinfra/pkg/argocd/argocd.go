package argocd

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	argocdoperator "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/argocd/argocdoperator"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Builder provides struct for the argocd object containing connection to
// the cluster and the argocd definitions.
type Builder struct {
	// argocd Definition, used to create the argocd object.
	Definition *argocdoperator.ArgoCD
	// created argocd object.
	Object *argocdoperator.ArgoCD
	// api client to interact with the cluster.
	apiClient runtimeclient.Client
	// used to store latest error message upon defining the argocd definition.
	errorMsg string
}

// NewBuilder creates a new instance of Builder.
func NewBuilder(apiClient *clients.Settings, name, nsname string) *Builder {
	klog.V(100).Infof("Initializing new ArgoCD structure with the following params: name: %s, nsname: %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil
	}

	err := apiClient.AttachScheme(argocdoperator.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add ArgoCD scheme to client schemes")

		return nil
	}

	builder := &Builder{
		apiClient: apiClient.Client,
		Definition: &argocdoperator.ArgoCD{
			Spec: argocdoperator.ArgoCDSpec{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the argocd is empty")

		builder.errorMsg = "argocd 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the argocd is empty")

		builder.errorMsg = "argocd 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// Pull pulls existing argocd from cluster.
func Pull(apiClient *clients.Settings, name, nsname string) (*Builder, error) {
	klog.V(100).Infof("Pulling existing argocd name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("argocd 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(argocdoperator.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add ArgoCD scheme to client schemes")

		return nil, err
	}

	builder := Builder{
		apiClient: apiClient.Client,
		Definition: &argocdoperator.ArgoCD{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the argocd is empty")

		return nil, fmt.Errorf("argocd 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the argocd is empty")

		return nil, fmt.Errorf("argocd 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("argocd object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Exists checks whether the given argocd exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if argocd %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get returns argocd object if found.
func (builder *Builder) Get() (*argocdoperator.ArgoCD, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting argocd %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	argocd := &argocdoperator.ArgoCD{}

	err := builder.apiClient.Get(logging.DiscardContext(), runtimeclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, argocd)
	if err != nil {
		return nil, err
	}

	return argocd, err
}

// Create makes an argocd in the cluster and stores the created object in struct.
func (builder *Builder) Create() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the argocd %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Delete removes argocd from a cluster.
func (builder *Builder) Delete() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting the argocd %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		builder.Object = nil

		klog.V(100).Infof("argocd %s in namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete argocd: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Update renovates the existing argocd object with the argocd definition in builder.
func (builder *Builder) Update(force bool) (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating the argocd object %s", builder.Definition.Name)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%v", msg.FailToUpdateNotification("argocd", builder.Definition.Name))

			builder, err := builder.Delete()
			if err != nil {
				klog.V(100).Infof("%v", msg.FailToUpdateError("argocd", builder.Definition.Name))

				return nil, err
			}

			return builder.Create()
		}
	}

	builder.Object = builder.Definition

	return builder, err
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "argocds"

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

		return false, fmt.Errorf("error: received nil %s builder apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
