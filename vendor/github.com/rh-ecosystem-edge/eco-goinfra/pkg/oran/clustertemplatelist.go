package oran

import (
	"fmt"

	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListClusterTemplates returns a list of ClusterTemplates in all namespaces, using the provided options.
func ListClusterTemplates(
	apiClient *clients.Settings, options ...runtimeclient.ListOptions) ([]*ClusterTemplateBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("ClusterTemplates 'apiClient' parameter cannot be nil")

		return nil, fmt.Errorf("failed to list clusterTemplates, 'apiClient' parameter is nil")
	}

	err := apiClient.AttachScheme(provisioningv1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add provisioning v1alpha1 scheme to client schemes")

		return nil, err
	}

	logMessage := "Listing ClusterTemplates in all namespaces"
	passedOptions := runtimeclient.ListOptions{}

	if len(options) > 1 {
		klog.V(100).Info("ClusterTemplates 'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Info(logMessage)

	clusterTemplateList := new(provisioningv1alpha1.ClusterTemplateList)

	err = apiClient.List(logging.DiscardContext(), clusterTemplateList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list ClusterTemplates in all namespaces due to %v", err)

		return nil, err
	}

	var clusterTemplateObjects []*ClusterTemplateBuilder

	for _, clusterTemplate := range clusterTemplateList.Items {
		copiedClusterTemplate := clusterTemplate
		clusterTemplateBuilder := &ClusterTemplateBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedClusterTemplate,
			Definition: &copiedClusterTemplate,
		}

		clusterTemplateObjects = append(clusterTemplateObjects, clusterTemplateBuilder)
	}

	return clusterTemplateObjects, nil
}
