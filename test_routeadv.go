package main

import (
	"fmt"
	"log"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	ovn "github.com/rh-ecosystem-edge/eco-goinfra/pkg/ovn"
	ovnv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ovn/routeadvertisement/v1"
	types "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ovn/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	fmt.Println("Testing RouteAdvertisement creation...")

	// Create API client
	apiClient := clients.New("")
	if apiClient == nil {
		log.Fatal("Failed to create API client")
	}

	fmt.Println("API client created successfully")

	// Define RouteAdvertisement parameters
	advertisements := []ovnv1.AdvertisementType{
		ovnv1.PodNetwork,
	}
	nodeSelector := metav1.LabelSelector{}
	frrConfigurationSelector := metav1.LabelSelector{}
	networkSelectors := types.NetworkSelectors{
		{
			NetworkSelectionType: types.DefaultNetwork,
		},
	}

	fmt.Println("Creating RouteAdvertisement builder...")

	// Create RouteAdvertisement builder
	routeAdv := ovn.NewRouteAdvertisementBuilder(
		apiClient.Client,
		"test-routeadv",
		advertisements,
		nodeSelector,
		frrConfigurationSelector,
		networkSelectors)

	if routeAdv == nil {
		log.Fatal("Failed to create RouteAdvertisement builder")
	}

	fmt.Println("RouteAdvertisement builder created successfully")

	// Attempt to create the resource
	fmt.Println("Attempting to create RouteAdvertisement resource...")
	createdRouteAdv, err := routeAdv.Create()
	if err != nil {
		fmt.Printf("RouteAdvertisement creation failed with error: %v\n", err)
		log.Fatal(err)
	}

	if createdRouteAdv == nil {
		log.Fatal("Created RouteAdvertisement is nil")
	}

	fmt.Printf("RouteAdvertisement created successfully: %s\n", createdRouteAdv.Definition.Name)
	fmt.Printf("Namespace: %s\n", createdRouteAdv.Definition.Namespace)
	fmt.Printf("Advertisements: %v\n", createdRouteAdv.Definition.Spec.Advertisements)

	// Clean up - delete the test resource
	fmt.Println("Cleaning up test resource...")
	err = createdRouteAdv.Delete()
	if err != nil {
		fmt.Printf("Failed to delete test RouteAdvertisement: %v\n", err)
	} else {
		fmt.Println("Test RouteAdvertisement deleted successfully")
	}

	fmt.Println("Test completed successfully!")
}
