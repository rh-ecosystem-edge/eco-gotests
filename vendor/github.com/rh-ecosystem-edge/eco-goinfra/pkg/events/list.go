package events

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// List returns Events inventory in the given namespace.
func List(
	apiClient *clients.Settings, nsname string, options ...metaV1.ListOptions) ([]*Builder, error) {
	if nsname == "" {
		glog.V(100).Infof("Events 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list Events, 'nsname' parameter is empty")
	}

	logMessage := fmt.Sprintf("Listing Events in the namespace %s", nsname)
	passedOptions := metaV1.ListOptions{}

	if len(options) > 1 {
		glog.V(100).Infof("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	glog.V(100).Infof(logMessage)

	eventList, err := apiClient.Events(nsname).List(context.TODO(), passedOptions)
	if err != nil {
		glog.V(100).Infof("Failed to list Events in the namespace %s due to %s", nsname, err.Error())

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
