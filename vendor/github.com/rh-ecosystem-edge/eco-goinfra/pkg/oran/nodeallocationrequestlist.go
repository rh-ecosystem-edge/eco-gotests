package oran

import (
	"context"
	"fmt"

	pluginsv1alpha1 "github.com/openshift-kni/oran-o2ims/api/hardwaremanagement/plugins/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListNodeAllocationRequests returns a list of NodeAllocationRequests in all namespaces, using the provided options.
func ListNodeAllocationRequests(
	apiClient *clients.Settings, options ...runtimeclient.ListOptions) ([]*NARBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("NodeAllocationRequests 'apiClient' parameter cannot be nil")

		return nil, fmt.Errorf("failed to list nodeAllocationRequests, 'apiClient' parameter is nil")
	}

	err := apiClient.AttachScheme(pluginsv1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add plugins v1alpha1 scheme to client schemes")

		return nil, err
	}

	logMessage := "Listing NodeAllocationRequests in all namespaces"
	passedOptions := runtimeclient.ListOptions{}

	if len(options) > 1 {
		klog.V(100).Info("NodeAllocationRequests 'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Info(logMessage)

	nodeAllocationRequestList := new(pluginsv1alpha1.NodeAllocationRequestList)

	err = apiClient.List(context.TODO(), nodeAllocationRequestList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list NodeAllocationRequests in all namespaces due to %v", err)

		return nil, err
	}

	var nodeAllocationRequestObjects []*NARBuilder

	for _, nodeAllocationRequest := range nodeAllocationRequestList.Items {
		copiedNodeAllocationRequest := nodeAllocationRequest
		nodeAllocationRequestBuilder := &NARBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedNodeAllocationRequest,
			Definition: &copiedNodeAllocationRequest,
		}

		nodeAllocationRequestObjects = append(nodeAllocationRequestObjects, nodeAllocationRequestBuilder)
	}

	return nodeAllocationRequestObjects, nil
}
