package metallb

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/metallb/mlbtypes"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ServiceBGPStatusBuilder provides struct for ServiceBGPStatus object which contains connection to cluster and
// ServiceBGPStatus definitions.
type ServiceBGPStatusBuilder struct {
	Definition *mlbtypes.ServiceBGPStatus
	Object     *mlbtypes.ServiceBGPStatus
	apiClient  runtimeClient.Client
	errorMsg   string
}

// PullServiceBGPStatus retrieves an existing ServiceBGPStatus object from the cluster.
func PullServiceBGPStatus(apiClient *clients.Settings, name string) (*ServiceBGPStatusBuilder, error) {
	klog.V(100).Infof("Pulling ServiceBGPStatus object name:%s", name)

	serviceBGPStatusBuilder := &ServiceBGPStatusBuilder{
		apiClient: apiClient.Client,
		Definition: &mlbtypes.ServiceBGPStatus{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the ServiceBGPStatus is empty")

		return nil, fmt.Errorf("serviceBGPStatus 'name' cannot be empty")
	}

	if !serviceBGPStatusBuilder.Exists() {
		return nil, fmt.Errorf("serviceBGPStatus object %s does not exist", name)
	}

	serviceBGPStatusBuilder.Definition = serviceBGPStatusBuilder.Object

	return serviceBGPStatusBuilder, nil
}

// Exists checks whether the given ServiceBGPStatus exists.
func (builder *ServiceBGPStatusBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if ServiceBGPStatus %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get returns ServiceBGPStatus object if found.
func (builder *ServiceBGPStatusBuilder) Get() (*mlbtypes.ServiceBGPStatus, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Collecting ServiceBGPStatus object %s", builder.Definition.Name)

	serviceBGPStatus := &mlbtypes.ServiceBGPStatus{}

	err := builder.apiClient.Get(logging.DiscardContext(), runtimeClient.ObjectKey{
		Name: builder.Definition.Name,
	}, serviceBGPStatus)
	if err != nil {
		klog.V(100).Infof("ServiceBGPStatus object %s does not exist", builder.Definition.Name)

		return nil, err
	}

	return serviceBGPStatus, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ServiceBGPStatusBuilder) validate() (bool, error) {
	resourceCRD := "serviceBGPStatus"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
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
