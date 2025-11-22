package events

import (
	"context"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// List returns Events inventory in the given namespace.
func List(
	apiClient *clients.Settings, nsname string, options ...metaV1.ListOptions) ([]*Builder, error) {
	if nsname == "" {
		klog.V(100).Info("Events 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list Events, 'nsname' parameter is empty")
	}

	logMessage := fmt.Sprintf("Listing Events in the namespace %s", nsname)
	passedOptions := metaV1.ListOptions{}

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Infof("%v", logMessage)

	eventList, err := apiClient.Events(nsname).List(context.TODO(), passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list Events in the namespace %s due to %s", nsname, err.Error())

		return nil, err
	}

	var eventObjects []*Builder

	for _, event := range eventList.Items {
		copiedEvent := event
		stateBuilder := &Builder{
			apiClient: apiClient.Events(nsname),
			Object:    &copiedEvent}
		eventObjects = append(eventObjects, stateBuilder)
	}

	return eventObjects, nil
}
