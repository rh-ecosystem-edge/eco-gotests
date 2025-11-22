package sriov

import (
	"fmt"

	srIovV1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"k8s.io/klog/v2"
)

// ListNetworkNodeState returns SriovNetworkNodeStates inventory in the given namespace.
func ListNetworkNodeState(
	apiClient *clients.Settings, nsname string, options ...client.ListOptions) ([]*NetworkNodeStateBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("SriovNetworkNodeStates 'apiClient' parameter can not be empty")

		return nil, fmt.Errorf("failed to list SriovNetworkNodeStates, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(srIovV1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add srIovV1 scheme to client schemes")

		return nil, err
	}

	if nsname == "" {
		klog.V(100).Info("SriovNetworkNodeStates 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list SriovNetworkNodeStates, 'nsname' parameter is empty")
	}

	logMessage := fmt.Sprintf("Listing SriovNetworkNodeStates in the namespace %s", nsname)
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

	networkNodeStateList := new(srIovV1.SriovNetworkNodeStateList)

	err = apiClient.List(logging.DiscardContext(), networkNodeStateList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list SriovNetworkNodeStates in the namespace %s due to %s", nsname, err.Error())

		return nil, err
	}

	var networkNodeStateObjects []*NetworkNodeStateBuilder

	for _, networkNodeState := range networkNodeStateList.Items {
		copiedNetworkNodeState := networkNodeState
		stateBuilder := &NetworkNodeStateBuilder{
			apiClient: apiClient.Client,
			Objects:   &copiedNetworkNodeState,
			nsName:    nsname,
			nodeName:  copiedNetworkNodeState.Name}

		networkNodeStateObjects = append(networkNodeStateObjects, stateBuilder)
	}

	return networkNodeStateObjects, nil
}
