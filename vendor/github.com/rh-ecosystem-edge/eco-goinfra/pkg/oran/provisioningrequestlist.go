package oran

import (
	"fmt"

	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListProvisioningRequests returns a list of ProvisioningRequests, using the provided options.
func ListProvisioningRequests(
	apiClient *clients.Settings, options ...runtimeclient.ListOptions) ([]*ProvisioningRequestBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("ProvisioningRequests 'apiClient' parameter cannot be nil")

		return nil, fmt.Errorf("failed to list provisioningRequests, 'apiClient' parameter is nil")
	}

	err := apiClient.AttachScheme(provisioningv1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add provisioning v1alpha1 scheme to client schemes")

		return nil, err
	}

	logMessage := "Listing ProvisioningRequests"
	passedOptions := runtimeclient.ListOptions{}

	if len(options) > 1 {
		klog.V(100).Info("ProvisioningRequests 'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Info(logMessage)

	provisioningRequestList := new(provisioningv1alpha1.ProvisioningRequestList)

	err = apiClient.List(logging.DiscardContext(), provisioningRequestList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list ProvisioningRequests due to %v", err)

		return nil, err
	}

	var provisioningRequestObjects []*ProvisioningRequestBuilder

	for _, provisioningRequest := range provisioningRequestList.Items {
		copiedProvisioningRequest := provisioningRequest
		provisioningRequestBuilder := &ProvisioningRequestBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedProvisioningRequest,
			Definition: &copiedProvisioningRequest,
		}

		provisioningRequestObjects = append(provisioningRequestObjects, provisioningRequestBuilder)
	}

	return provisioningRequestObjects, nil
}
