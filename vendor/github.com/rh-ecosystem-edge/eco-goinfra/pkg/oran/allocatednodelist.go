package oran

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	pluginsv1alpha1 "github.com/openshift-kni/oran-o2ims/api/hardwaremanagement/plugins/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListAllocatedNodes returns a list of AllocatedNodes in all namespaces, using the provided options.
func ListAllocatedNodes(
	apiClient *clients.Settings, options ...runtimeclient.ListOptions) ([]*AllocatedNodeBuilder, error) {
	if apiClient == nil {
		glog.V(100).Info("AllocatedNodes 'apiClient' parameter cannot be nil")

		return nil, fmt.Errorf("failed to list allocatedNodes, 'apiClient' parameter is nil")
	}

	err := apiClient.AttachScheme(pluginsv1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Info("Failed to add plugins v1alpha1 scheme to client schemes")

		return nil, err
	}

	logMessage := "Listing AllocatedNodes in all namespaces"
	passedOptions := runtimeclient.ListOptions{}

	if len(options) > 1 {
		glog.V(100).Info("AllocatedNodes 'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	glog.V(100).Info(logMessage)

	nodeList := new(pluginsv1alpha1.AllocatedNodeList)

	err = apiClient.List(context.TODO(), nodeList, &passedOptions)
	if err != nil {
		glog.V(100).Infof("Failed to list AllocatedNodes in all namespaces due to %v", err)

		return nil, err
	}

	var nodeObjects []*AllocatedNodeBuilder

	for _, node := range nodeList.Items {
		copiedNode := node
		nodeBuilder := &AllocatedNodeBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedNode,
			Definition: &copiedNode,
		}

		nodeObjects = append(nodeObjects, nodeBuilder)
	}

	return nodeObjects, nil
}
