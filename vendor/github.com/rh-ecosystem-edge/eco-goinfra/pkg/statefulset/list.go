package statefulset

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// List returns statefulset inventory in the given namespace.
func List(apiClient *clients.Settings, nsname string, options ...metav1.ListOptions) ([]*Builder, error) {
	if nsname == "" {
		glog.V(100).Infof("statefulset 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list statefulsets, 'nsname' parameter is empty")
	}

	passedOptions := metav1.ListOptions{}
	logMessage := fmt.Sprintf("Listing statefulsets in the namespace %s", nsname)

	if len(options) > 1 {
		glog.V(100).Infof("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	glog.V(100).Infof(logMessage)

	statefulsetList, err := apiClient.StatefulSets(nsname).List(context.TODO(), passedOptions)
	if err != nil {
		glog.V(100).Infof("Failed to list statefulsets in the namespace %s due to %s", nsname, err.Error())

		return nil, err
	}

	var statefulsetObjects []*Builder

	for _, runningStatefulSet := range statefulsetList.Items {
		copiedStatefulSet := runningStatefulSet
		statefulsetBuilder := &Builder{
			apiClient:  apiClient,
			Object:     &copiedStatefulSet,
			Definition: &copiedStatefulSet,
		}

		statefulsetObjects = append(statefulsetObjects, statefulsetBuilder)
	}

	return statefulsetObjects, nil
}

// ListInAllNamespaces returns statefulset inventory in all namespaces.
func ListInAllNamespaces(apiClient *clients.Settings, options ...metav1.ListOptions) ([]*Builder, error) {
	passedOptions := metav1.ListOptions{}
	logMessage := "Listing statefulsets in all namespaces"

	if len(options) > 1 {
		glog.V(100).Infof("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	glog.V(100).Infof(logMessage)

	statefulsetList, err := apiClient.StatefulSets("").List(context.TODO(), passedOptions)
	if err != nil {
		glog.V(100).Infof("Failed to list statefulsets in all namespaces due to %s", err.Error())

		return nil, err
	}

	var statefulsetObjects []*Builder

	for _, runningStatefulSet := range statefulsetList.Items {
		copiedStatefulSet := runningStatefulSet
		statefulsetBuilder := &Builder{
			apiClient:  apiClient,
			Object:     &copiedStatefulSet,
			Definition: &copiedStatefulSet,
		}

		statefulsetObjects = append(statefulsetObjects, statefulsetBuilder)
	}

	glog.V(100).Infof("Found %d statefulsets across all namespaces", len(statefulsetObjects))

	return statefulsetObjects, nil
}
