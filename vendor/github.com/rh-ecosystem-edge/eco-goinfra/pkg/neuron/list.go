package neuron

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	neuronv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/neuron/v1alpha1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListDeviceConfigs returns a list of DeviceConfig builders in the given namespace.
func ListDeviceConfigs(
	apiClient *clients.Settings,
	nsname string,
	options ...goclient.ListOptions) ([]*Builder, error) {
	if apiClient == nil {
		klog.V(100).Infof("DeviceConfig 'apiClient' parameter can not be empty")

		return nil, fmt.Errorf("failed to list deviceConfigs, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(neuronv1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Infof("Failed to add neuron scheme to client schemes")

		return nil, err
	}

	logMessage := "Listing all DeviceConfig resources"
	if nsname != "" {
		logMessage = fmt.Sprintf("Listing DeviceConfig resources in namespace %s", nsname)
	}

	klog.V(100).Infof("%s", logMessage)

	deviceConfigList := &neuronv1alpha1.DeviceConfigList{}
	passedOptions := goclient.ListOptions{}

	if len(options) > 0 {
		passedOptions = options[0]
	}

	if nsname != "" {
		passedOptions.Namespace = nsname
	}

	err = apiClient.List(logging.DiscardContext(), deviceConfigList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list DeviceConfig objects due to %s", err.Error())

		return nil, err
	}

	deviceConfigBuilders := []*Builder{}

	for idx := range deviceConfigList.Items {
		copiedDeviceConfig := deviceConfigList.Items[idx]
		builder := &Builder{
			apiClient:  apiClient,
			Definition: &copiedDeviceConfig,
			Object:     &copiedDeviceConfig,
		}

		deviceConfigBuilders = append(deviceConfigBuilders, builder)
	}

	return deviceConfigBuilders, nil
}
