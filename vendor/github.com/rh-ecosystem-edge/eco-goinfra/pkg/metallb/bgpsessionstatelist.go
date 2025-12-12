package metallb

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/metallb/frrtypes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ListBGPSessionState returns BGP session state inventory in the given cluster.
func ListBGPSessionState(
	apiClient *clients.Settings, options ...client.ListOptions) ([]*BGPSessionStateBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("BGPSessionStates 'apiClient' parameter can not be empty")

		return nil, fmt.Errorf("failed to list BGPSessionStates, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(frrtypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add frrk8 scheme to client schemes")

		return nil, err
	}

	logMessage := "Listing BGPSessionStates in cluster"
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

	bgpSessionStateList := new(frrtypes.BGPSessionStateList)

	err = apiClient.List(logging.DiscardContext(), bgpSessionStateList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list BGPSessionStates due to %s", err.Error())

		return nil, err
	}

	var bgpSessionStateObjects []*BGPSessionStateBuilder

	for _, bgpSessionState := range bgpSessionStateList.Items {
		copiedBgpSessionState := bgpSessionState
		stateBuilder := &BGPSessionStateBuilder{
			apiClient:  apiClient.Client,
			Definition: &copiedBgpSessionState,
			Object:     &copiedBgpSessionState,
			errorMsg:   "",
		}
		bgpSessionStateObjects = append(bgpSessionStateObjects, stateBuilder)
	}

	return bgpSessionStateObjects, nil
}
