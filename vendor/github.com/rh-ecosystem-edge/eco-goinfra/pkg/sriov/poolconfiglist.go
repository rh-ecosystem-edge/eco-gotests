package sriov

import (
	"fmt"

	srIovV1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ListPoolConfigs returns a sriovNetworkPoolConfig list in a given namespace.
func ListPoolConfigs(apiClient *clients.Settings, namespace string) ([]*PoolConfigBuilder, error) {
	sriovNetworkPoolConfigList := &srIovV1.SriovNetworkPoolConfigList{}

	if apiClient == nil {
		klog.V(100).Info("sriov network 'apiClient' parameter can not be empty")

		return nil, fmt.Errorf("failed to list sriov networks, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(srIovV1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add oplmV1alpha1 scheme to client schemes")

		return nil, err
	}

	if namespace == "" {
		klog.V(100).Info("sriovNetworkPoolConfigs 'namespace' parameter can not be empty")

		return nil, fmt.Errorf("failed to list sriovNetworkPoolConfigs, 'namespace' parameter is empty")
	}

	err = apiClient.List(logging.DiscardContext(), sriovNetworkPoolConfigList, &client.ListOptions{Namespace: namespace})
	if err != nil {
		klog.V(100).Infof("Failed to list SriovNetworkPoolConfigs in namespace: %s due to %s",
			namespace, err.Error())

		return nil, err
	}

	var poolConfigBuilderObjects []*PoolConfigBuilder

	for _, sriovNetworkPoolConfigObj := range sriovNetworkPoolConfigList.Items {
		sriovNetworkPoolConfig := sriovNetworkPoolConfigObj
		sriovNetworkPoolConfBuilder := &PoolConfigBuilder{
			apiClient:  apiClient.Client,
			Definition: &sriovNetworkPoolConfig,
			Object:     &sriovNetworkPoolConfig,
		}

		poolConfigBuilderObjects = append(poolConfigBuilderObjects, sriovNetworkPoolConfBuilder)
	}

	return poolConfigBuilderObjects, nil
}

// CleanAllPoolConfigs removes all sriovNetworkPoolConfigs.
func CleanAllPoolConfigs(
	apiClient *clients.Settings, operatornsname string) error {
	klog.V(100).Infof("Cleaning up SriovNetworkPoolConfigs in the %s namespace", operatornsname)

	if operatornsname == "" {
		klog.V(100).Info("'operatornsname' parameter can not be empty")

		return fmt.Errorf("failed to clean up SriovNetworkPoolConfigs, 'operatornsname' parameter is empty")
	}

	poolConfigs, err := ListPoolConfigs(apiClient, operatornsname)
	if err != nil {
		klog.V(100).Infof("Failed to list SriovNetworkPoolConfigs in namespace: %s", operatornsname)

		return err
	}

	for _, poolConfig := range poolConfigs {
		err = poolConfig.Delete()
		if err != nil {
			klog.V(100).Infof("Failed to delete SriovNetworkPoolConfigs: %s", poolConfig.Object.Name)

			return err
		}
	}

	return nil
}
