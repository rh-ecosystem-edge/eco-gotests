package sriovfec

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	sriovfectypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/fec/fectypes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// List returns SriovFecNodeConfigList from given namespace.
func List(apiClient *clients.Settings, nsname string, options ...client.ListOptions) ([]*NodeConfigBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("SriovFecNodeConfigList 'apiClient' parameter can not be empty")

		return nil, fmt.Errorf("failed to list SriovFecNodeConfig, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(sriovfectypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add sriov-fec scheme to client schemes")

		return nil, err
	}

	if nsname == "" {
		klog.V(100).Info("SriovFecNodeConfigList 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list SriovFecNodeConfig, 'nsname' parameter is empty")
	}

	passedOptions := client.ListOptions{}
	logMessage := fmt.Sprintf("Listing SriovFecNodeConfig in the namespace %s", nsname)

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Infof("%v", logMessage)

	sfncList := new(sriovfectypes.SriovFecNodeConfigList)

	err = apiClient.List(logging.DiscardContext(), sfncList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list SriovFecNodeConfigs in the namespace %s due to %s", nsname, err.Error())

		return nil, err
	}

	var sfncBuilderList []*NodeConfigBuilder

	for _, sfnc := range sfncList.Items {
		copiedObject := sfnc
		sfncBuilder := &NodeConfigBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedObject,
			Definition: &copiedObject,
		}

		sfncBuilderList = append(sfncBuilderList, sfncBuilder)
	}

	return sfncBuilderList, nil
}
