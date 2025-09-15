package ovn

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	ovnv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ovn/routeadvertisement/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListRouteAdvertisements returns RouteAdvertisement inventory (cluster-scoped).
func ListRouteAdvertisements(apiClient *clients.Settings, options ...metav1.ListOptions) ([]*RouteAdvertisementBuilder, error) {
	if apiClient == nil {
		glog.V(100).Infof("RouteAdvertisements 'apiClient' parameter can not be empty")

		return nil, fmt.Errorf("failed to list RouteAdvertisements, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(ovnv1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add ovn scheme to client schemes")

		return nil, err
	}

	passedOptions := metav1.ListOptions{}
	logMessage := "Listing all RouteAdvertisement resources"

	if len(options) > 1 {
		glog.V(100).Infof("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	glog.V(100).Infof(logMessage)

	routeAdvertisementList := &ovnv1.RouteAdvertisementsList{}

	// Convert metav1.ListOptions to controller-runtime ListOptions
	listOpts := []runtimeClient.ListOption{}
	if passedOptions.LabelSelector != "" {
		selector, parseErr := labels.Parse(passedOptions.LabelSelector)
		if parseErr == nil {
			listOpts = append(listOpts, runtimeClient.MatchingLabelsSelector{Selector: selector})
		}
	}
	if passedOptions.FieldSelector != "" {
		selector, parseErr := fields.ParseSelector(passedOptions.FieldSelector)
		if parseErr == nil {
			listOpts = append(listOpts, runtimeClient.MatchingFieldsSelector{Selector: selector})
		}
	}

	err = apiClient.Client.List(context.TODO(), routeAdvertisementList, listOpts...)

	if err != nil {
		glog.V(100).Infof("Failed to list RouteAdvertisements due to %s", err.Error())

		return nil, err
	}

	var routeAdvertisementObjects []*RouteAdvertisementBuilder

	for _, routeAdvertisement := range routeAdvertisementList.Items {
		copiedRouteAdvertisement := routeAdvertisement
		routeAdvertisementBuilder := &RouteAdvertisementBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedRouteAdvertisement,
			Definition: &copiedRouteAdvertisement,
		}

		routeAdvertisementObjects = append(routeAdvertisementObjects, routeAdvertisementBuilder)
	}

	return routeAdvertisementObjects, nil
}
