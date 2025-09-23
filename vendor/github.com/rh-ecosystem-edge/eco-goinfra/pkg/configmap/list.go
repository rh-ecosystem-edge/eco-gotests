package configmap

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// List returns configmap inventory in the given namespace.
func List(apiClient *clients.Settings, nsname string, options ...metav1.ListOptions) ([]*Builder, error) {
	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient cannot be nil")
	}

	if nsname == "" {
		glog.V(100).Infof("configmap 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list configmaps, 'nsname' parameter is empty")
	}

	passedOptions := metav1.ListOptions{}
	logMessage := fmt.Sprintf("Listing configmaps in the namespace %s", nsname)

	if len(options) > 1 {
		glog.V(100).Infof("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	glog.V(100).Infof(logMessage)

	configmapList, err := apiClient.ConfigMaps(nsname).List(context.TODO(), passedOptions)
	if err != nil {
		glog.V(100).Infof("Failed to list configmaps in the namespace %s due to %s", nsname, err.Error())

		return nil, err
	}

	var configmapObjects []*Builder

	for _, runningConfigmap := range configmapList.Items {
		copiedConfigmap := runningConfigmap
		configmapBuilder := &Builder{
			apiClient:  apiClient.CoreV1Interface,
			Object:     &copiedConfigmap,
			Definition: &copiedConfigmap,
		}

		configmapObjects = append(configmapObjects, configmapBuilder)
	}

	return configmapObjects, nil
}

// ListInAllNamespaces returns configmap inventory in the all the namespaces.
func ListInAllNamespaces(apiClient *clients.Settings, options ...metav1.ListOptions) ([]*Builder, error) {
	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient cannot be nil")
	}

	passedOptions := metav1.ListOptions{}
	logMessage := "Listing configmaps in all namespaces"

	if len(options) > 1 {
		glog.V(100).Infof("'options' parameter must be either empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	glog.V(100).Infof(logMessage)

	configmapList, err := apiClient.ConfigMaps("").List(context.TODO(), passedOptions)
	if err != nil {
		glog.V(100).Infof("Failed to list configmaps in all namespaces due to %s", err.Error())

		return nil, err
	}

	var configmapObjects []*Builder

	for _, runningConfigmap := range configmapList.Items {
		copiedConfigmap := runningConfigmap
		configmapBuilder := &Builder{
			apiClient:  apiClient.CoreV1Interface,
			Object:     &copiedConfigmap,
			Definition: &copiedConfigmap,
		}

		configmapObjects = append(configmapObjects, configmapBuilder)
	}

	return configmapObjects, nil
}
