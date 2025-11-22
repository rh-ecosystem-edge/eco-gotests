package sriov

import (
	"context"
	"fmt"

	srIovV1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// List returns sriov networks in the given namespace.
func List(apiClient *clients.Settings, nsname string, options ...client.ListOptions) ([]*NetworkBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("sriov network 'apiClient' parameter can not be empty")

		return nil, fmt.Errorf("failed to list sriov networks, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(srIovV1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add oplmV1alpha1 scheme to client schemes")

		return nil, err
	}

	if nsname == "" {
		klog.V(100).Info("sriov network 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list sriov networks, 'nsname' parameter is empty")
	}

	passedOptions := client.ListOptions{}
	logMessage := fmt.Sprintf("Listing sriov networks in the namespace %s", nsname)

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Infof("%v", logMessage)

	networkList := new(srIovV1.SriovNetworkList)

	err = apiClient.List(context.TODO(), networkList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list sriov networks in the namespace %s due to %s", nsname, err.Error())

		return nil, err
	}

	var networkObjects []*NetworkBuilder

	for _, runningNetwork := range networkList.Items {
		copiedNetwork := runningNetwork
		networkBuilder := &NetworkBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedNetwork,
			Definition: &copiedNetwork,
		}

		networkObjects = append(networkObjects, networkBuilder)
	}

	return networkObjects, nil
}

// CleanAllNetworksByTargetNamespace deletes all networks matched by their NetworkNamespace spec.
func CleanAllNetworksByTargetNamespace(
	apiClient *clients.Settings,
	operatornsname string,
	targetnsname string,
	options ...client.ListOptions) error {
	klog.V(100).Infof("Cleaning up sriov networks in the %s namespace with %s NetworkNamespace spec",
		operatornsname, targetnsname)

	if operatornsname == "" {
		klog.V(100).Info("'operatornsname' parameter can not be empty")

		return fmt.Errorf("failed to clean up sriov networks, 'operatornsname' parameter is empty")
	}

	if targetnsname == "" {
		klog.V(100).Info("'targetnsname' parameter can not be empty")

		return fmt.Errorf("failed to clean up sriov networks, 'targetnsname' parameter is empty")
	}

	networks, err := List(apiClient, operatornsname, options...)
	if err != nil {
		klog.V(100).Infof("Failed to list sriov networks in namespace: %s", operatornsname)

		return err
	}

	for _, network := range networks {
		if network.Object.Spec.NetworkNamespace == targetnsname {
			err = network.Delete()
			if err != nil {
				klog.V(100).Infof("Failed to delete sriov networks: %s", network.Object.Name)

				return err
			}
		}
	}

	return nil
}
