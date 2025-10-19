package ovn

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	ovnv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ovn/routeadvertisement/v1"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListRouteAdvertisements returns RouteAdvertisement inventory (cluster-scoped).
func ListRouteAdvertisements(apiClient *clients.Settings,
	options ...runtimeClient.ListOption) ([]*RouteAdvertisementBuilder, error) {
	if apiClient == nil {
		glog.V(100).Infof("RouteAdvertisements 'apiClient' parameter can not be empty")

		return nil, fmt.Errorf("failed to list RouteAdvertisements, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(ovnv1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add ovn scheme to client schemes")

		return nil, err
	}

	glog.V(100).Infof("Listing all RouteAdvertisement resources")

	routeAdvertisementList := &ovnv1.RouteAdvertisementsList{}

	err = apiClient.List(context.TODO(), routeAdvertisementList, options...)
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
