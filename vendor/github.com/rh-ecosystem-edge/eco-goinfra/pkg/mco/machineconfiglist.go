package mco

import (
	"fmt"

	mcv1 "github.com/openshift/api/machineconfiguration/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListMC returns a list of builders for MachineConfigs.
func ListMC(apiClient *clients.Settings, options ...runtimeclient.ListOptions) ([]*MCBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("MachineConfig 'apiClient' can not be empty")

		return nil, fmt.Errorf("failed to list MachineConfigs, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(mcv1.Install)
	if err != nil {
		klog.V(100).Info("Failed to add machineconfig v1 scheme to client schemes")

		return nil, err
	}

	passedOptions := runtimeclient.ListOptions{}
	logMessage := "Listing all MC resources"

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Infof("%v", logMessage)

	mcList := new(mcv1.MachineConfigList)

	err = apiClient.List(logging.DiscardContext(), mcList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list MC objects due to %s", err.Error())

		return nil, err
	}

	var mcObjects []*MCBuilder

	for _, mc := range mcList.Items {
		copiedMc := mc
		mcBuilder := &MCBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedMc,
			Definition: &copiedMc,
		}

		mcObjects = append(mcObjects, mcBuilder)
	}

	return mcObjects, nil
}
