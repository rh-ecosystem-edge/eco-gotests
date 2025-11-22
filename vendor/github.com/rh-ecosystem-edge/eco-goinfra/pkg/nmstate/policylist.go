package nmstate

import (
	"context"
	"fmt"

	nmstateV1 "github.com/nmstate/kubernetes-nmstate/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListPolicy returns a list of NodeNetworkConfigurationPolicy.
func ListPolicy(apiClient *clients.Settings, options ...goclient.ListOptions) ([]*PolicyBuilder, error) {
	passedOptions := goclient.ListOptions{}
	logMessage := "Listing NodeNetworkConfigurationPolicy"

	if apiClient == nil {
		klog.V(100).Info("sriov network 'apiClient' parameter can not be empty")

		return nil, fmt.Errorf("failed to list sriov networks, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(nmstateV1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add nmstate v1 scheme to client schemes")

		return nil, err
	}

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Infof("%v", logMessage)

	policyList := &nmstateV1.NodeNetworkConfigurationPolicyList{}

	err = apiClient.List(context.TODO(), policyList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list NodeNetworkConfigurationPolicy due to %s", err.Error())

		return nil, err
	}

	var networkConfigurationPolicyObjects []*PolicyBuilder

	for _, policy := range policyList.Items {
		copiedPolicy := policy
		policyBuilder := &PolicyBuilder{
			apiClient:  apiClient.Client,
			Definition: &copiedPolicy,
			Object:     &copiedPolicy}

		networkConfigurationPolicyObjects = append(networkConfigurationPolicyObjects, policyBuilder)
	}

	return networkConfigurationPolicyObjects, nil
}

// CleanAllNMStatePolicies removes all NodeNetworkConfigurationPolicies.
func CleanAllNMStatePolicies(apiClient *clients.Settings, options ...goclient.ListOptions) error {
	klog.V(100).Info("Cleaning up NodeNetworkConfigurationPolicies")

	nncpList, err := ListPolicy(apiClient, options...)
	if err != nil {
		klog.V(100).Info("Failed to list NodeNetworkConfigurationPolicies")

		return err
	}

	for _, nncpPolicy := range nncpList {
		_, err = nncpPolicy.Delete()
		if err != nil {
			klog.V(100).Infof("Failed to delete NodeNetworkConfigurationPolicy: %s", nncpPolicy.Object.Name)

			return err
		}
	}

	return nil
}
