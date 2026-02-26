package metallb

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/metallb/mlbtypes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ListServiceBGPStatus returns service bgp status inventory in the given cluster.
func ListServiceBGPStatus(
	apiClient *clients.Settings, options ...client.ListOptions) ([]*ServiceBGPStatusBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("ServiceBGPStatuses 'apiClient' parameter can not be empty")

		return nil, fmt.Errorf("failed to list ServiceBGPStatuses, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(mlbtypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add metallb scheme to client schemes")

		return nil, err
	}

	logMessage := "Listing ServiceBGPStatuses in cluster"
	passedOptions := client.ListOptions{}

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Infof("%v", logMessage)

	serviceBGPStatusList := new(mlbtypes.ServiceBGPStatusList)

	err = apiClient.List(logging.DiscardContext(), serviceBGPStatusList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list ServiceBGPStatuses due to %s", err.Error())

		return nil, err
	}

	var serviceBGPStatusObjects []*ServiceBGPStatusBuilder

	for _, serviceBGPStatus := range serviceBGPStatusList.Items {
		copiedServiceBGPStatus := serviceBGPStatus
		statusBuilder := &ServiceBGPStatusBuilder{
			apiClient:  apiClient.Client,
			Definition: &copiedServiceBGPStatus,
			Object:     &copiedServiceBGPStatus,
			errorMsg:   "",
		}
		serviceBGPStatusObjects = append(serviceBGPStatusObjects, statusBuilder)
	}

	return serviceBGPStatusObjects, nil
}
