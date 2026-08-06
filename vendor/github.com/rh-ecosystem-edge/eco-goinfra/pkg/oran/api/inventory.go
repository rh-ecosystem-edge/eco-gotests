package api

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/internal/inventory"
	"k8s.io/klog/v2"
)

// DeploymentManager is the type of the DeploymentManager resource returned by the API.
type DeploymentManager = inventory.DeploymentManager

// InventorySubscription is the type of the Subscription resource returned by the API.
type InventorySubscription = inventory.Subscription

// OCloudInfo is the type of the OCloudInfo resource returned by the API.
type OCloudInfo = inventory.OCloudInfo

// ResourceType is the type of the ResourceType resource returned by the API.
type ResourceType = inventory.ResourceType

// ResourcePool is the type of the ResourcePool resource returned by the API.
type ResourcePool = inventory.ResourcePool

// Resource is the type of the Resource resource returned by the API.
type Resource = inventory.Resource

// LocationInfo is the type of the LocationInfo resource returned by the API.
type LocationInfo = inventory.LocationInfo

// OCloudSiteInfo is the type of the OCloudSiteInfo resource returned by the API.
type OCloudSiteInfo = inventory.OCloudSiteInfo

// InventoryChangeNotification is the type of the InventoryChangeNotification resource returned by the API.
type InventoryChangeNotification = inventory.InventoryChangeNotification

// InventoryChangeNotificationEventType is the type of the notificationEventType field returned by the API.
type InventoryChangeNotificationEventType = inventory.InventoryChangeNotificationNotificationEventType

//nolint:revive // These are just re-exported constants no need for the linting.
const (
	InventoryChangeNotificationEventTypeCreate InventoryChangeNotificationEventType = inventory.N0
	InventoryChangeNotificationEventTypeModify InventoryChangeNotificationEventType = inventory.N1
	InventoryChangeNotificationEventTypeDelete InventoryChangeNotificationEventType = inventory.N2
)

// InventoryClient provides access to the O2IMS infrastructure inventory API. It is not a runtimeclient.Client since
// inventory resources do not correspond to CRs.
type InventoryClient struct {
	inventory.ClientWithResponsesInterface
}

// GetAllVersions returns the complete list of API versions implemented by the service.
func (client *InventoryClient) GetAllVersions() (APIVersions, error) {
	klog.V(100).Info("Getting all API versions")

	resp, err := client.GetAllVersionsWithResponse(context.TODO())
	if err != nil {
		return APIVersions{}, fmt.Errorf("failed to get all API versions: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return APIVersions{}, fmt.Errorf("failed to get all API versions: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetCloudInfo returns the details of the O-Cloud instance. Optional list options can be provided to select fields.
// Use WithFields to select fields. If multiple WithFields options are provided, only the last one is used.
func (client *InventoryClient) GetCloudInfo(opts ...ListOption) (OCloudInfo, error) {
	query := applyListOptions(opts...)

	if query.hasOptions() {
		klog.V(100).Infof("Getting O-Cloud info with query options %#v", query)
	} else {
		klog.V(100).Info("Getting O-Cloud info without query options")
	}

	resp, err := client.GetCloudInfoWithResponse(context.TODO(), &inventory.GetCloudInfoParams{
		AllFields:     query.allFields,
		ExcludeFields: query.excludeFields,
		Fields:        query.fields,
	})
	if err != nil {
		return OCloudInfo{}, fmt.Errorf("failed to get O-Cloud info: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return OCloudInfo{}, fmt.Errorf("failed to get O-Cloud info: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetMinorVersions returns the list of minor API versions implemented for the current major version of the API.
func (client *InventoryClient) GetMinorVersions() (APIVersions, error) {
	klog.V(100).Info("Getting minor API versions")

	resp, err := client.GetMinorVersionsWithResponse(context.TODO())
	if err != nil {
		return APIVersions{}, fmt.Errorf("failed to get minor API versions: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return APIVersions{}, fmt.Errorf("failed to get minor API versions: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// ListDeploymentManagers lists all deployment managers. Optional list options can be provided to filter the list or
// select fields. Use WithFilter to filter and WithFields to select fields. If multiple WithFilter or WithFields options
// are provided, only the last one is used. filter.And() can be used to combine multiple filters.
func (client *InventoryClient) ListDeploymentManagers(opts ...ListOption) ([]DeploymentManager, error) {
	query := applyListOptions(opts...)

	if query.hasOptions() {
		klog.V(100).Infof("Listing deployment managers with query options %#v", query)
	} else {
		klog.V(100).Info("Listing deployment managers without query options")
	}

	resp, err := client.GetDeploymentManagersWithResponse(context.TODO(), &inventory.GetDeploymentManagersParams{
		AllFields:     query.allFields,
		ExcludeFields: query.excludeFields,
		Fields:        query.fields,
		Filter:        query.filter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployment managers: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list deployment managers: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetDeploymentManager gets a deployment manager by its ID which must be a valid UUID.
func (client *InventoryClient) GetDeploymentManager(id uuid.UUID) (DeploymentManager, error) {
	klog.V(100).Infof("Getting deployment manager with id %v", id)

	resp, err := client.GetDeploymentManagerWithResponse(context.TODO(), id)
	if err != nil {
		return DeploymentManager{}, fmt.Errorf("failed to get deployment manager: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return DeploymentManager{}, fmt.Errorf("failed to get deployment manager: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// ListInventorySubscriptions lists all inventory subscriptions. Optional list options can be provided to filter the
// list or select fields. Use WithFilter to filter and WithFields to select fields. If multiple WithFilter or WithFields
// options are provided, only the last one is used. filter.And() can be used to combine multiple filters.
func (client *InventoryClient) ListInventorySubscriptions(opts ...ListOption) ([]InventorySubscription, error) {
	query := applyListOptions(opts...)

	if query.hasOptions() {
		klog.V(100).Infof("Listing inventory subscriptions with query options %#v", query)
	} else {
		klog.V(100).Info("Listing inventory subscriptions without query options")
	}

	resp, err := client.GetSubscriptionsWithResponse(context.TODO(), &inventory.GetSubscriptionsParams{
		AllFields:     query.allFields,
		ExcludeFields: query.excludeFields,
		Fields:        query.fields,
		Filter:        query.filter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list inventory subscriptions: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list inventory subscriptions: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// CreateInventorySubscription creates a new inventory subscription.
func (client *InventoryClient) CreateInventorySubscription(subscription InventorySubscription) (InventorySubscription, error) {
	klog.V(100).Infof("Creating inventory subscription %#v", subscription)

	resp, err := client.CreateSubscriptionWithResponse(context.TODO(), subscription)
	if err != nil {
		return InventorySubscription{}, fmt.Errorf("failed to create inventory subscription: error contacting api: %w", err)
	}

	if resp.StatusCode() != 201 || resp.JSON201 == nil {
		return InventorySubscription{},
			fmt.Errorf("failed to create inventory subscription: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON201, nil
}

// GetInventorySubscription retrieves exactly one inventory subscription by its ID which must be a valid UUID.
func (client *InventoryClient) GetInventorySubscription(id uuid.UUID) (InventorySubscription, error) {
	klog.V(100).Infof("Getting inventory subscription with id %v", id)

	resp, err := client.GetSubscriptionWithResponse(context.TODO(), id)
	if err != nil {
		return InventorySubscription{}, fmt.Errorf("failed to get inventory subscription: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return InventorySubscription{},
			fmt.Errorf("failed to get inventory subscription: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// DeleteInventorySubscription deletes exactly one inventory subscription by its ID which must be a valid UUID.
func (client *InventoryClient) DeleteInventorySubscription(id uuid.UUID) error {
	klog.V(100).Infof("Deleting inventory subscription with id %v", id)

	resp, err := client.DeleteSubscriptionWithResponse(context.TODO(), id)
	if err != nil {
		return fmt.Errorf("failed to delete inventory subscription: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("failed to delete inventory subscription: received error from api: %w", apiErrorFromResponse(resp))
	}

	return nil
}

// ListResourceTypes lists all resource types. Optional list options can be provided to filter the list or select
// fields. Use WithFilter to filter and WithFields to select fields. If multiple WithFilter or WithFields options are
// provided, only the last one is used. filter.And() can be used to combine multiple filters.
func (client *InventoryClient) ListResourceTypes(opts ...ListOption) ([]ResourceType, error) {
	query := applyListOptions(opts...)

	if query.hasOptions() {
		klog.V(100).Infof("Listing resource types with query options %#v", query)
	} else {
		klog.V(100).Info("Listing resource types without query options")
	}

	resp, err := client.GetResourceTypesWithResponse(context.TODO(), &inventory.GetResourceTypesParams{
		AllFields:     query.allFields,
		ExcludeFields: query.excludeFields,
		Fields:        query.fields,
		Filter:        query.filter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list resource types: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list resource types: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetResourceType gets a resource type by its ID which must be a valid UUID.
func (client *InventoryClient) GetResourceType(id uuid.UUID) (ResourceType, error) {
	klog.V(100).Infof("Getting resource type with id %v", id)

	resp, err := client.GetResourceTypeWithResponse(context.TODO(), id)
	if err != nil {
		return ResourceType{}, fmt.Errorf("failed to get resource type: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return ResourceType{}, fmt.Errorf("failed to get resource type: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetResourceTypeAlarmDictionary gets the alarm dictionary for a resource type. The ID must be a valid UUID.
func (client *InventoryClient) GetResourceTypeAlarmDictionary(id uuid.UUID) (AlarmDictionary, error) {
	klog.V(100).Infof("Getting alarm dictionary for resource type with id %v", id)

	resp, err := client.GetResourceTypeAlarmDictionaryWithResponse(context.TODO(), id)
	if err != nil {
		return AlarmDictionary{}, fmt.Errorf("failed to get resource type alarm dictionary: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmDictionary{}, fmt.Errorf(
			"failed to get resource type alarm dictionary: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// ListResourcePools lists all resource pools. Optional list options can be provided to filter the list or select
// fields. Use WithFilter to filter and WithFields to select fields. If multiple WithFilter or WithFields options are
// provided, only the last one is used. filter.And() can be used to combine multiple filters.
func (client *InventoryClient) ListResourcePools(opts ...ListOption) ([]ResourcePool, error) {
	query := applyListOptions(opts...)

	if query.hasOptions() {
		klog.V(100).Infof("Listing resource pools with query options %#v", query)
	} else {
		klog.V(100).Info("Listing resource pools without query options")
	}

	resp, err := client.GetResourcePoolsWithResponse(context.TODO(), &inventory.GetResourcePoolsParams{
		AllFields:     query.allFields,
		ExcludeFields: query.excludeFields,
		Fields:        query.fields,
		Filter:        query.filter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list resource pools: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list resource pools: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetResourcePool gets a resource pool by its ID which must be a valid UUID.
func (client *InventoryClient) GetResourcePool(id uuid.UUID) (ResourcePool, error) {
	klog.V(100).Infof("Getting resource pool with id %v", id)

	resp, err := client.GetResourcePoolWithResponse(context.TODO(), id)
	if err != nil {
		return ResourcePool{}, fmt.Errorf("failed to get resource pool: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return ResourcePool{}, fmt.Errorf("failed to get resource pool: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// ListResources lists all resources in a resource pool. Optional list options can be provided to filter the list or
// select fields. Use WithFilter to filter and WithFields to select fields. If multiple WithFilter or WithFields options
// are provided, only the last one is used. filter.And() can be used to combine multiple filters.
func (client *InventoryClient) ListResources(resourcePoolID uuid.UUID, opts ...ListOption) ([]Resource, error) {
	query := applyListOptions(opts...)

	if query.hasOptions() {
		klog.V(100).Infof("Listing resources in pool %v with query options %#v", resourcePoolID, query)
	} else {
		klog.V(100).Infof("Listing resources in pool %v without query options", resourcePoolID)
	}

	resp, err := client.GetResourcesWithResponse(context.TODO(), resourcePoolID, &inventory.GetResourcesParams{
		AllFields:     query.allFields,
		ExcludeFields: query.excludeFields,
		Fields:        query.fields,
		Filter:        query.filter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list resources: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetResource gets a resource by its resource pool ID and resource ID, both of which must be valid UUIDs.
func (client *InventoryClient) GetResource(resourcePoolID, resourceID uuid.UUID) (Resource, error) {
	klog.V(100).Infof("Getting resource with id %v in pool %v", resourceID, resourcePoolID)

	resp, err := client.GetResourceWithResponse(context.TODO(), resourcePoolID, resourceID)
	if err != nil {
		return Resource{}, fmt.Errorf("failed to get resource: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return Resource{}, fmt.Errorf("failed to get resource: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// ListInventoryAlarmDictionaries lists all alarm dictionaries. Optional list options can be provided to filter the list
// or select fields. Use WithFilter to filter and WithFields to select fields. If multiple WithFilter or WithFields
// options are provided, only the last one is used. filter.And() can be used to combine multiple filters.
func (client *InventoryClient) ListInventoryAlarmDictionaries(opts ...ListOption) ([]AlarmDictionary, error) {
	query := applyListOptions(opts...)

	if query.hasOptions() {
		klog.V(100).Infof("Listing inventory alarm dictionaries with query options %#v", query)
	} else {
		klog.V(100).Info("Listing inventory alarm dictionaries without query options")
	}

	resp, err := client.GetAlarmDictionariesWithResponse(context.TODO(), &inventory.GetAlarmDictionariesParams{
		AllFields:     query.allFields,
		ExcludeFields: query.excludeFields,
		Fields:        query.fields,
		Filter:        query.filter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list inventory alarm dictionaries: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list inventory alarm dictionaries: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetInventoryAlarmDictionary gets an alarm dictionary by its ID which must be a valid UUID.
func (client *InventoryClient) GetInventoryAlarmDictionary(id uuid.UUID) (AlarmDictionary, error) {
	klog.V(100).Infof("Getting inventory alarm dictionary with id %v", id)

	resp, err := client.GetAlarmDictionaryWithResponse(context.TODO(), id)
	if err != nil {
		return AlarmDictionary{}, fmt.Errorf("failed to get inventory alarm dictionary: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmDictionary{}, fmt.Errorf("failed to get inventory alarm dictionary: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// ListLocations lists all locations. Optional list options can be provided to filter the list or select fields. Use
// WithFilter to filter and WithFields to select fields. If multiple WithFilter or WithFields options are provided, only
// the last one is used. filter.And() can be used to combine multiple filters.
func (client *InventoryClient) ListLocations(opts ...ListOption) ([]LocationInfo, error) {
	query := applyListOptions(opts...)

	if query.hasOptions() {
		klog.V(100).Infof("Listing locations with query options %#v", query)
	} else {
		klog.V(100).Info("Listing locations without query options")
	}

	resp, err := client.GetLocationsWithResponse(context.TODO(), &inventory.GetLocationsParams{
		AllFields:     query.allFields,
		ExcludeFields: query.excludeFields,
		Fields:        query.fields,
		Filter:        query.filter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list locations: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list locations: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetLocation gets a location by its global location ID.
func (client *InventoryClient) GetLocation(globalLocationID string) (LocationInfo, error) {
	klog.V(100).Infof("Getting location with id %v", globalLocationID)

	resp, err := client.GetLocationWithResponse(context.TODO(), globalLocationID)
	if err != nil {
		return LocationInfo{}, fmt.Errorf("failed to get location: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return LocationInfo{}, fmt.Errorf("failed to get location: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// ListOCloudSites lists all O-Cloud sites. Optional list options can be provided to filter the list or select fields.
// Use WithFilter to filter and WithFields to select fields. If multiple WithFilter or WithFields options are provided,
// only the last one is used. filter.And() can be used to combine multiple filters.
func (client *InventoryClient) ListOCloudSites(opts ...ListOption) ([]OCloudSiteInfo, error) {
	query := applyListOptions(opts...)

	if query.hasOptions() {
		klog.V(100).Infof("Listing O-Cloud sites with query options %#v", query)
	} else {
		klog.V(100).Info("Listing O-Cloud sites without query options")
	}

	resp, err := client.GetOCloudSitesWithResponse(context.TODO(), &inventory.GetOCloudSitesParams{
		AllFields:     query.allFields,
		ExcludeFields: query.excludeFields,
		Fields:        query.fields,
		Filter:        query.filter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list O-Cloud sites: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list O-Cloud sites: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetOCloudSite gets an O-Cloud site by its ID which must be a valid UUID.
func (client *InventoryClient) GetOCloudSite(id uuid.UUID) (OCloudSiteInfo, error) {
	klog.V(100).Infof("Getting O-Cloud site with id %v", id)

	resp, err := client.GetOCloudSiteWithResponse(context.TODO(), id)
	if err != nil {
		return OCloudSiteInfo{}, fmt.Errorf("failed to get O-Cloud site: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return OCloudSiteInfo{}, fmt.Errorf("failed to get O-Cloud site: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}
