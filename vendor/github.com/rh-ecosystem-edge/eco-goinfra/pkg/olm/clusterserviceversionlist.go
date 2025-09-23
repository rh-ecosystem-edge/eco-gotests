package olm

import (
	"context"
	"fmt"
	"strings"

	oplmV1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/operators/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
)

// ListClusterServiceVersion returns clusterserviceversion inventory in the given namespace.
func ListClusterServiceVersion(
	apiClient *clients.Settings,
	nsname string,
	options ...client.ListOptions) ([]*ClusterServiceVersionBuilder, error) {
	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil, fmt.Errorf("clusterserviceversion 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(oplmV1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add oplmV1alpha1 scheme to client schemes")

		return nil, err
	}

	if nsname == "" {
		glog.V(100).Infof("clusterserviceversion 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list clusterserviceversion, 'nsname' parameter is empty")
	}

	passedOptions := client.ListOptions{}
	logMessage := fmt.Sprintf("Listing clusterserviceversion in the namespace %s", nsname)

	if len(options) > 1 {
		glog.V(100).Infof("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	glog.V(100).Infof(logMessage)

	csvList := new(oplmV1alpha1.ClusterServiceVersionList)

	err = apiClient.List(context.TODO(), csvList, &passedOptions)
	if err != nil {
		glog.V(100).Infof("Failed to list clusterserviceversion in the nsname %s due to %s", nsname, err.Error())

		return nil, err
	}

	var csvObjects []*ClusterServiceVersionBuilder

	for _, runningCSV := range csvList.Items {
		copiedCSV := runningCSV
		csvBuilder := &ClusterServiceVersionBuilder{
			apiClient:  apiClient,
			Object:     &copiedCSV,
			Definition: &copiedCSV,
		}

		csvObjects = append(csvObjects, csvBuilder)
	}

	return csvObjects, nil
}

// ListClusterServiceVersionWithNamePattern returns a cluster-wide clusterserviceversion inventory
// filtered by the name pattern.
func ListClusterServiceVersionWithNamePattern(
	apiClient *clients.Settings,
	namePattern string,
	nsname string,
	options ...client.ListOptions) ([]*ClusterServiceVersionBuilder, error) {
	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil, fmt.Errorf("clusterserviceversion 'apiClient' cannot be empty")
	}

	if namePattern == "" {
		glog.V(100).Info(
			"The namePattern field to filter out all relevant clusterserviceversion cannot be empty")

		return nil, fmt.Errorf(
			"the namePattern field to filter out all relevant clusterserviceversion cannot be empty")
	}

	glog.V(100).Infof("Listing clusterserviceversion filtered by the name pattern %s in %s namespace",
		namePattern, nsname)

	notFilteredCsvList, err := ListClusterServiceVersion(apiClient, nsname, options...)
	if err != nil {
		glog.V(100).Infof("Failed to list all clusterserviceversions in namespace %s due to %s",
			nsname, err.Error())

		return nil, err
	}

	var finalCsvList []*ClusterServiceVersionBuilder

	for _, foundCsv := range notFilteredCsvList {
		if strings.Contains(foundCsv.Definition.Name, namePattern) {
			finalCsvList = append(finalCsvList, foundCsv)
		}
	}

	return finalCsvList, nil
}

// ListClusterServiceVersionInAllNamespaces returns cluster-wide clusterserviceversion inventory.
func ListClusterServiceVersionInAllNamespaces(
	apiClient *clients.Settings,
	options ...client.ListOptions) ([]*ClusterServiceVersionBuilder, error) {
	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil, fmt.Errorf("clusterserviceversion 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(oplmV1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add oplmV1alpha1 scheme to client schemes")

		return nil, err
	}

	passedOptions := client.ListOptions{}
	logMessage := "Listing CSVs in all namespaces"

	if len(options) > 1 {
		glog.V(100).Infof("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	glog.V(100).Infof(logMessage)

	csvList := new(oplmV1alpha1.ClusterServiceVersionList)

	err = apiClient.List(context.TODO(), csvList, &passedOptions)
	if err != nil {
		glog.V(100).Infof("Failed to list CSVs in all namespaces due to %s", err.Error())

		return nil, err
	}

	var csvObjects []*ClusterServiceVersionBuilder

	for _, csvs := range csvList.Items {
		copiedCSV := csvs
		csvBuilder := &ClusterServiceVersionBuilder{
			apiClient:  apiClient,
			Object:     &copiedCSV,
			Definition: &copiedCSV,
		}

		csvObjects = append(csvObjects, csvBuilder)
	}

	return csvObjects, nil
}
