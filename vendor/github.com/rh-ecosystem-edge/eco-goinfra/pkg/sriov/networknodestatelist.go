package sriov

import (
	"context"
	"fmt"

	srIovV1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
)

// ListNetworkNodeState returns SriovNetworkNodeStates inventory in the given namespace.
func ListNetworkNodeState(
	apiClient *clients.Settings, nsname string, options ...client.ListOptions) ([]*NetworkNodeStateBuilder, error) {
	if apiClient == nil {
		glog.V(100).Infof("SriovNetworkNodeStates 'apiClient' parameter can not be empty")

		return nil, fmt.Errorf("failed to list SriovNetworkNodeStates, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(srIovV1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add srIovV1 scheme to client schemes")

		return nil, err
	}

	if nsname == "" {
		glog.V(100).Infof("SriovNetworkNodeStates 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list SriovNetworkNodeStates, 'nsname' parameter is empty")
	}

	logMessage := fmt.Sprintf("Listing SriovNetworkNodeStates in the namespace %s", nsname)
	passedOptions := client.ListOptions{}

	if len(options) > 1 {
		glog.V(100).Infof("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	glog.V(100).Infof(logMessage)

	networkNodeStateList := new(srIovV1.SriovNetworkNodeStateList)

	err = apiClient.List(context.TODO(), networkNodeStateList, &passedOptions)
	if err != nil {
		glog.V(100).Infof("Failed to list SriovNetworkNodeStates in the namespace %s due to %s", nsname, err.Error())

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
