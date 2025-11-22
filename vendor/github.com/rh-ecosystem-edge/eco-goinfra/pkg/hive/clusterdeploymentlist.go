package hive

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	hiveV1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/hive/api/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListClusterDeploymentsInAllNamespaces returns a cluster-wide clusterdeployment inventory.
func ListClusterDeploymentsInAllNamespaces(
	apiClient *clients.Settings,
	options ...goclient.ListOptions) ([]*ClusterDeploymentBuilder, error) {
	passedOptions := goclient.ListOptions{}
	logMessage := "Listing all clusterdeployments"

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient cannot be nil")
	}

	err := apiClient.AttachScheme(hiveV1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add hive v1 scheme to client schemes")

		return nil, err
	}

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Infof("%v", logMessage)

	clusterDeployments := new(hiveV1.ClusterDeploymentList)

	err = apiClient.List(logging.DiscardContext(), clusterDeployments, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list all clusterDeployments due to %s", err.Error())

		return nil, err
	}

	var clusterDeploymentObjects []*ClusterDeploymentBuilder

	for _, clusterDeployment := range clusterDeployments.Items {
		copiedClusterDeployment := clusterDeployment
		clusterDeploymentBuilder := &ClusterDeploymentBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedClusterDeployment,
			Definition: &copiedClusterDeployment,
		}

		clusterDeploymentObjects = append(clusterDeploymentObjects, clusterDeploymentBuilder)
	}

	return clusterDeploymentObjects, nil
}
