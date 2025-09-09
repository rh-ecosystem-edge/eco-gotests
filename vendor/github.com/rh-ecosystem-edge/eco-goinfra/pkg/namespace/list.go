package namespace

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// List returns namespace inventory.
func List(apiClient *clients.Settings, options ...metav1.ListOptions) ([]*Builder, error) {
	logMessage := "Listing all namespace resources"
	passedOptions := metav1.ListOptions{}

	if len(options) > 1 {
		glog.V(100).Infof("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	glog.V(100).Infof(logMessage)

	namespacesList, err := apiClient.CoreV1Interface.Namespaces().List(context.TODO(), passedOptions)
	if err != nil {
		glog.V(100).Infof("Failed to list namespaces due to %s", err.Error())

		return nil, err
	}

	var namespaceObjects []*Builder

	for _, runningNamespace := range namespacesList.Items {
		copiedNamespace := runningNamespace
		namespaceBuilder := &Builder{
			apiClient:  apiClient,
			Object:     &copiedNamespace,
			Definition: &copiedNamespace,
		}

		namespaceObjects = append(namespaceObjects, namespaceBuilder)
	}

	return namespaceObjects, nil
}
