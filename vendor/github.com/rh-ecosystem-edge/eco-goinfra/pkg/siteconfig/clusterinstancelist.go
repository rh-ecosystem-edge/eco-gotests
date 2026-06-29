package siteconfig

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListClusterInstances returns a list of ClusterInstances in all namespaces, using the provided options.
func ListClusterInstances(
	apiClient *clients.Settings, options ...runtimeclient.ListOptions) ([]*CIBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("ClusterInstances 'apiClient' parameter cannot be nil")

		return nil, fmt.Errorf("failed to list ClusterInstances, 'apiClient' parameter is nil")
	}

	err := apiClient.AttachScheme(siteconfigv1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add siteconfig v1alpha1 scheme to client schemes")

		return nil, err
	}

	logMessage := "Listing ClusterInstances in all namespaces"
	passedOptions := runtimeclient.ListOptions{}

	if len(options) > 1 {
		klog.V(100).Info("ClusterInstances 'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Info(logMessage)

	clusterInstanceList := new(siteconfigv1alpha1.ClusterInstanceList)

	err = apiClient.List(logging.DiscardContext(), clusterInstanceList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list ClusterInstances in all namespaces due to %v", err)

		return nil, err
	}

	var clusterInstanceObjects []*CIBuilder

	for _, clusterInstance := range clusterInstanceList.Items {
		copiedClusterInstance := clusterInstance
		clusterInstanceBuilder := &CIBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedClusterInstance,
			Definition: &copiedClusterInstance,
		}

		clusterInstanceObjects = append(clusterInstanceObjects, clusterInstanceBuilder)
	}

	return clusterInstanceObjects, nil
}
