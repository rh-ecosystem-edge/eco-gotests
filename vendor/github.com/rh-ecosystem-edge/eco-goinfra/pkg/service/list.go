package service

import (
	"context"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// List returns service inventory in the given namespace.
func List(apiClient *clients.Settings, nsname string, options ...metav1.ListOptions) ([]*Builder, error) {
	if nsname == "" {
		klog.V(100).Info("service 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list services, 'nsname' parameter is empty")
	}

	passedOptions := metav1.ListOptions{}
	logMessage := fmt.Sprintf("Listing services in the namespace %s", nsname)

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Infof("%v", logMessage)

	serviceList, err := apiClient.Services(nsname).List(context.TODO(), passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list services in the namespace %s due to %s", nsname, err.Error())

		return nil, err
	}

	var serviceObjects []*Builder

	for _, runningService := range serviceList.Items {
		copiedService := runningService
		serviceBuilder := &Builder{
			apiClient:  apiClient.CoreV1Interface,
			Object:     &copiedService,
			Definition: &copiedService,
		}

		serviceObjects = append(serviceObjects, serviceBuilder)
	}

	return serviceObjects, nil
}
