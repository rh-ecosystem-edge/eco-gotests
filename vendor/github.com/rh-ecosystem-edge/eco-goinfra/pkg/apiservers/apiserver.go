package apiservers

import (
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const apiServerName = "cluster"

// APIServerBuilder provides a struct for the config.openshift.io/v1 APIServer object.
type APIServerBuilder struct {
	// APIServer definition. Used to define the APIServer object.
	Definition *configv1.APIServer
	// Created APIServer object.
	Object *configv1.APIServer
	// apiClient opens api connection to the cluster.
	apiClient goclient.Client
	// Used in functions that define or mutate APIServer definition. errorMsg is processed before the
	// APIServer object is created.
	errorMsg string
}

// PullAPIServer pulls the existing config.openshift.io/v1 APIServer singleton from the cluster.
func PullAPIServer(apiClient *clients.Settings) (*APIServerBuilder, error) {
	klog.V(100).Infof("Pulling existing APIServer name: %s", apiServerName)

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the APIServer is nil")

		return nil, fmt.Errorf("apiserver 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(configv1.Install)
	if err != nil {
		klog.V(100).Info("Failed to add config v1 scheme to client schemes")

		return nil, err
	}

	builder := APIServerBuilder{
		apiClient: apiClient.Client,
		Definition: &configv1.APIServer{
			ObjectMeta: metav1.ObjectMeta{
				Name: apiServerName,
			},
		},
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("apiserver object %s does not exist", apiServerName)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Get returns the APIServer object from the cluster if it exists.
func (builder *APIServerBuilder) Get() (*configv1.APIServer, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting APIServer object %s", builder.Definition.Name)

	apiServerObject := &configv1.APIServer{}

	err := builder.apiClient.Get(logging.DiscardContext(),
		goclient.ObjectKey{Name: builder.Definition.Name}, apiServerObject)
	if err != nil {
		klog.V(100).Infof("Failed to get APIServer %s: %s", builder.Definition.Name, err)

		return nil, err
	}

	return apiServerObject, nil
}

// Exists checks whether the given APIServer exists.
func (builder *APIServerBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if APIServer %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()
	if err != nil {
		klog.V(100).Infof("Failed to collect APIServer object due to %s", err.Error())
	}

	return err == nil
}

// Update renovates the existing APIServer object with the APIServer definition in builder.
func (builder *APIServerBuilder) Update() (*APIServerBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating APIServer %s", builder.Definition.Name)

	if !builder.Exists() {
		return nil, fmt.Errorf("apiserver object %s does not exist", builder.Definition.Name)
	}

	builder.Definition.ResourceVersion = builder.Object.ResourceVersion
	builder.Definition.CreationTimestamp = metav1.Time{}

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("Failed to update APIServer %s: %s", builder.Definition.Name, err)

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithTLSAdherence sets the TLS adherence policy on the APIServer definition.
func (builder *APIServerBuilder) WithTLSAdherence(
	policy configv1.TLSAdherencePolicy) *APIServerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting TLS adherence policy %q on APIServer %s", policy, builder.Definition.Name)

	builder.Definition.Spec.TLSAdherence = policy

	return builder
}

// WithTLSSecurityProfile sets the TLS security profile on the APIServer definition.
func (builder *APIServerBuilder) WithTLSSecurityProfile(
	profile *configv1.TLSSecurityProfile) *APIServerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting TLS security profile on APIServer %s", builder.Definition.Name)

	if profile == nil {
		builder.errorMsg = "apiserver TLS security profile cannot be nil"

		return builder
	}

	builder.Definition.Spec.TLSSecurityProfile = profile

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *APIServerBuilder) validate() (bool, error) {
	resourceCRD := "apiservers.config.openshift.io"

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
