package nto //nolint:misspell

import (
	"context"
	"fmt"

	performanceprofilev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListProfiles returns a list of all installed PerformanceProfiles.
func ListProfiles(apiClient *clients.Settings, options ...goclient.ListOptions) ([]*Builder, error) {
	passedOptions := goclient.ListOptions{}
	logMessage := "Listing PerformanceProfiles on cluster"

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient cannot be nil")
	}

	err := apiClient.AttachScheme(performanceprofilev2.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add node-tuning-operator v2 scheme to client schemes")

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

	var performanceProfiles performanceprofilev2.PerformanceProfileList

	err = apiClient.List(context.TODO(), &performanceProfiles, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list PerformanceProfiles due to %s", err.Error())

		return nil, err
	}

	var perfProfilesObjects []*Builder

	for _, perfProfile := range performanceProfiles.Items {
		copiedPerfProfile := perfProfile
		perfProfileBuilder := &Builder{
			apiClient:  apiClient.Client,
			Object:     &copiedPerfProfile,
			Definition: &copiedPerfProfile,
		}

		perfProfilesObjects = append(perfProfilesObjects, perfProfileBuilder)
	}

	return perfProfilesObjects, nil
}

// CleanAllPerformanceProfiles removes all PerformanceProfiles installed on a cluster.
func CleanAllPerformanceProfiles(apiClient *clients.Settings, options ...goclient.ListOptions) error {
	klog.V(100).Info("Cleaning up PerformanceProfiles")

	policies, err := ListProfiles(apiClient, options...)
	if err != nil {
		klog.V(100).Info("Failed to list PerformanceProfiles")

		return err
	}

	for _, policy := range policies {
		_, err = policy.Delete()
		if err != nil {
			klog.V(100).Infof("Failed to delete PerformanceProfiles: %s", policy.Object.Name)

			return err
		}
	}

	return nil
}
