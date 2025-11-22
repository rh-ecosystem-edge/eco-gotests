package machine

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// ListWorkerMachineSets returns a slice of SetBuilder objects in a namespace on a cluster.
func ListWorkerMachineSets(
	apiClient *clients.Settings,
	namespace string,
	workerLabel string,
	options ...metav1.ListOptions) ([]*SetBuilder, error) {
	if namespace == "" {
		klog.V(100).Info("machineSet 'namespace' parameter can not be empty")

		return nil, fmt.Errorf("failed to list MachineSets, 'namespace' parameter is empty")
	}

	if workerLabel == "" {
		klog.V(100).Info("machineSet 'workerLabel' parameter can not be empty")

		return nil, fmt.Errorf("failed to list MachineSets, 'workerLabel' parameter is empty")
	}

	logMessage := fmt.Sprintf("Listing all workerMachinesSets in the namespace %s", namespace)
	passedOptions := metav1.ListOptions{}

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Infof("%v", logMessage)

	machineSetList, err := apiClient.MachineSets(namespace).List(logging.DiscardContext(), passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list MachineSets in the namespace %s due to %s",
			namespace, err.Error())

		return nil, err
	}

	var machineSetObjects []*SetBuilder

	for _, runningMachineSet := range machineSetList.Items {
		copiedMachineSet := runningMachineSet
		SetBuilder := &SetBuilder{
			apiClient:  apiClient,
			Object:     &copiedMachineSet,
			Definition: &copiedMachineSet,
		}

		if val, ok := SetBuilder.Definition.Spec.Template.Labels[workerLabel]; ok && val == "worker" {
			machineSetObjects = append(machineSetObjects, SetBuilder)
		}
	}

	return machineSetObjects, nil
}
