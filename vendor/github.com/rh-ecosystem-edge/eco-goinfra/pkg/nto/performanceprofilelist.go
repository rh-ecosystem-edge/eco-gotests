package nto //nolint:misspell

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	performanceprofilev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListProfiles returns a list of all installed PerformanceProfiles.
func ListProfiles(apiClient *clients.Settings, options ...goclient.ListOptions) ([]*Builder, error) {
	passedOptions := goclient.ListOptions{}
	logMessage := "Listing PerformanceProfiles on cluster"

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient cannot be nil")
	}

	err := apiClient.AttachScheme(performanceprofilev2.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add node-tuning-operator v2 scheme to client schemes")

		return nil, err
	}

	if len(options) > 1 {
		glog.V(100).Infof("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	glog.V(100).Infof(logMessage)

	var performanceProfiles performanceprofilev2.PerformanceProfileList

	err = apiClient.List(context.TODO(), &performanceProfiles, &passedOptions)
	if err != nil {
		glog.V(100).Infof("Failed to list PerformanceProfiles due to %s", err.Error())

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
	glog.V(100).Infof("Cleaning up PerformanceProfiles")

	policies, err := ListProfiles(apiClient, options...)
	if err != nil {
		glog.V(100).Infof("Failed to list PerformanceProfiles")

		return err
	}

	for _, policy := range policies {
		_, err = policy.Delete()
		if err != nil {
			glog.V(100).Infof("Failed to delete PerformanceProfiles: %s", policy.Object.Name)

			return err
		}
	}

	return nil
}
