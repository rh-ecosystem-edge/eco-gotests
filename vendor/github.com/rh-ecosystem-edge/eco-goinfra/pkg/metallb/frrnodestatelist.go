package metallb

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/metallb/frrtypes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ListFrrNodeState returns frr node state inventory in the given cluster.
func ListFrrNodeState(
	apiClient *clients.Settings, options ...client.ListOptions) ([]*FrrNodeStateBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("FrrNodeStates 'apiClient' parameter can not be empty")

		return nil, fmt.Errorf("failed to list FrrNodeStates, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(frrtypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add frrk8 scheme to client schemes")

		return nil, err
	}

	logMessage := "Listing FrrNodeStates in cluster"
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

	frrNodeStateList := new(frrtypes.FRRNodeStateList)

	err = apiClient.List(logging.DiscardContext(), frrNodeStateList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list FrrNodeStates due to %s", err.Error())

		return nil, err
	}

	var frrNodeStateObjects []*FrrNodeStateBuilder

	for _, frrNodeState := range frrNodeStateList.Items {
		copiedNetworkNodeState := frrNodeState
		stateBuilder := &FrrNodeStateBuilder{
			apiClient:  apiClient.Client,
			Definition: &copiedNetworkNodeState,
			Object:     &copiedNetworkNodeState,
			errorMsg:   "",
		}
		frrNodeStateObjects = append(frrNodeStateObjects, stateBuilder)
	}

	return frrNodeStateObjects, nil
}
