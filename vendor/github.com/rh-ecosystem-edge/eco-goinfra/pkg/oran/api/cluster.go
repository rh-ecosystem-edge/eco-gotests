package api

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/internal/cluster"
	"k8s.io/klog/v2"
)

// NodeClusterType is the type of the NodeClusterType resource returned by the API.
type NodeClusterType = cluster.NodeClusterType

// NodeCluster is the type of the NodeCluster resource returned by the API.
type NodeCluster = cluster.NodeCluster

// ClusterResourceType is the type of the ClusterResourceType resource returned by the API.
type ClusterResourceType = cluster.ClusterResourceType

// ClusterResource is the type of the ClusterResource resource returned by the API.
type ClusterResource = cluster.ClusterResource

// ClusterSubscription is the type of the Subscription resource returned by the API.
type ClusterSubscription = cluster.Subscription

// ClusterChangeNotification is the type of the ClusterChangeNotification resource returned by the API.
type ClusterChangeNotification = cluster.ClusterChangeNotification

// ClusterChangeNotificationEventType is the type of the notificationEventType field returned by the API.
type ClusterChangeNotificationEventType = cluster.ClusterChangeNotificationNotificationEventType

//nolint:revive // These are just re-exported constants no need for the linting.
const (
	ClusterChangeNotificationEventTypeCreate ClusterChangeNotificationEventType = cluster.N0
	ClusterChangeNotificationEventTypeModify ClusterChangeNotificationEventType = cluster.N1
	ClusterChangeNotificationEventTypeDelete ClusterChangeNotificationEventType = cluster.N2
)

// ClusterClient provides access to the O2IMS infrastructure cluster API. It is not a runtimeclient.Client since cluster
// resources do not correspond to CRs.
type ClusterClient struct {
	cluster.ClientWithResponsesInterface
}

// GetAllVersions returns the complete list of API versions implemented by the service.
func (client *ClusterClient) GetAllVersions() (APIVersions, error) {
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

// GetMinorVersions returns the list of minor API versions implemented for the current major version of the API.
func (client *ClusterClient) GetMinorVersions() (APIVersions, error) {
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

// ListNodeClusterTypes lists all node cluster types. Optional list options can be provided to filter the list or select
// fields. Use WithFilter to filter and WithFields to select fields. If multiple WithFilter or WithFields options are
// provided, only the last one is used. filter.And() can be used to combine multiple filters.
func (client *ClusterClient) ListNodeClusterTypes(opts ...ListOption) ([]NodeClusterType, error) {
	query := applyListOptions(opts...)

	if query.hasOptions() {
		klog.V(100).Infof("Listing node cluster types with query options %#v", query)
	} else {
		klog.V(100).Info("Listing node cluster types without query options")
	}

	resp, err := client.GetNodeClusterTypesWithResponse(context.TODO(), &cluster.GetNodeClusterTypesParams{
		AllFields:     query.allFields,
		ExcludeFields: query.excludeFields,
		Fields:        query.fields,
		Filter:        query.filter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list node cluster types: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list node cluster types: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetNodeClusterType gets a node cluster type by its ID which must be a valid UUID.
func (client *ClusterClient) GetNodeClusterType(id uuid.UUID) (NodeClusterType, error) {
	klog.V(100).Infof("Getting node cluster type with id %v", id)

	resp, err := client.GetNodeClusterTypeWithResponse(context.TODO(), id)
	if err != nil {
		return NodeClusterType{}, fmt.Errorf("failed to get node cluster type: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return NodeClusterType{}, fmt.Errorf("failed to get node cluster type: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetNodeClusterTypeAlarmDictionary gets the alarm dictionary for a node cluster type. The ID must be a valid UUID.
func (client *ClusterClient) GetNodeClusterTypeAlarmDictionary(id uuid.UUID) (AlarmDictionary, error) {
	klog.V(100).Infof("Getting alarm dictionary for node cluster type with id %v", id)

	resp, err := client.GetNodeClusterTypeAlarmDictionaryWithResponse(context.TODO(), id)
	if err != nil {
		return AlarmDictionary{}, fmt.Errorf("failed to get node cluster type alarm dictionary: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmDictionary{}, fmt.Errorf(
			"failed to get node cluster type alarm dictionary: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// ListNodeClusters lists all node clusters. Optional list options can be provided to filter the list or select fields.
// Use WithFilter to filter and WithFields to select fields. If multiple WithFilter or WithFields options are provided,
// only the last one is used. filter.And() can be used to combine multiple filters.
func (client *ClusterClient) ListNodeClusters(opts ...ListOption) ([]NodeCluster, error) {
	query := applyListOptions(opts...)

	if query.hasOptions() {
		klog.V(100).Infof("Listing node clusters with query options %#v", query)
	} else {
		klog.V(100).Info("Listing node clusters without query options")
	}

	resp, err := client.GetNodeClustersWithResponse(context.TODO(), &cluster.GetNodeClustersParams{
		AllFields:     query.allFields,
		ExcludeFields: query.excludeFields,
		Fields:        query.fields,
		Filter:        query.filter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list node clusters: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list node clusters: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetNodeCluster gets a node cluster by its ID which must be a valid UUID.
func (client *ClusterClient) GetNodeCluster(id uuid.UUID) (NodeCluster, error) {
	klog.V(100).Infof("Getting node cluster with id %v", id)

	resp, err := client.GetNodeClusterWithResponse(context.TODO(), id)
	if err != nil {
		return NodeCluster{}, fmt.Errorf("failed to get node cluster: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return NodeCluster{}, fmt.Errorf("failed to get node cluster: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// ListClusterResourceTypes lists all cluster resource types. Optional list options can be provided to filter the list or
// select fields. Use WithFilter to filter and WithFields to select fields. If multiple WithFilter or WithFields options
// are provided, only the last one is used. filter.And() can be used to combine multiple filters.
func (client *ClusterClient) ListClusterResourceTypes(opts ...ListOption) ([]ClusterResourceType, error) {
	query := applyListOptions(opts...)

	if query.hasOptions() {
		klog.V(100).Infof("Listing cluster resource types with query options %#v", query)
	} else {
		klog.V(100).Info("Listing cluster resource types without query options")
	}

	resp, err := client.GetClusterResourceTypesWithResponse(context.TODO(), &cluster.GetClusterResourceTypesParams{
		AllFields:     query.allFields,
		ExcludeFields: query.excludeFields,
		Fields:        query.fields,
		Filter:        query.filter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster resource types: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list cluster resource types: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetClusterResourceType gets a cluster resource type by its ID which must be a valid UUID.
func (client *ClusterClient) GetClusterResourceType(id uuid.UUID) (ClusterResourceType, error) {
	klog.V(100).Infof("Getting cluster resource type with id %v", id)

	resp, err := client.GetClusterResourceTypeWithResponse(context.TODO(), id)
	if err != nil {
		return ClusterResourceType{}, fmt.Errorf("failed to get cluster resource type: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return ClusterResourceType{}, fmt.Errorf("failed to get cluster resource type: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// ListClusterResources lists all cluster resources. Optional list options can be provided to filter the list or select
// fields. Use WithFilter to filter and WithFields to select fields. If multiple WithFilter or WithFields options are
// provided, only the last one is used. filter.And() can be used to combine multiple filters.
func (client *ClusterClient) ListClusterResources(opts ...ListOption) ([]ClusterResource, error) {
	query := applyListOptions(opts...)

	if query.hasOptions() {
		klog.V(100).Infof("Listing cluster resources with query options %#v", query)
	} else {
		klog.V(100).Info("Listing cluster resources without query options")
	}

	resp, err := client.GetClusterResourcesWithResponse(context.TODO(), &cluster.GetClusterResourcesParams{
		AllFields:     query.allFields,
		ExcludeFields: query.excludeFields,
		Fields:        query.fields,
		Filter:        query.filter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster resources: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list cluster resources: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetClusterResource gets a cluster resource by its ID which must be a valid UUID.
func (client *ClusterClient) GetClusterResource(id uuid.UUID) (ClusterResource, error) {
	klog.V(100).Infof("Getting cluster resource with id %v", id)

	resp, err := client.GetClusterResourceWithResponse(context.TODO(), id)
	if err != nil {
		return ClusterResource{}, fmt.Errorf("failed to get cluster resource: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return ClusterResource{}, fmt.Errorf("failed to get cluster resource: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// ListClusterSubscriptions lists all cluster subscriptions. Optional list options can be provided to filter the list or
// select fields. Use WithFilter to filter and WithFields to select fields. If multiple WithFilter or WithFields options
// are provided, only the last one is used. filter.And() can be used to combine multiple filters.
func (client *ClusterClient) ListClusterSubscriptions(opts ...ListOption) ([]ClusterSubscription, error) {
	query := applyListOptions(opts...)

	if query.hasOptions() {
		klog.V(100).Infof("Listing cluster subscriptions with query options %#v", query)
	} else {
		klog.V(100).Info("Listing cluster subscriptions without query options")
	}

	resp, err := client.GetSubscriptionsWithResponse(context.TODO(), &cluster.GetSubscriptionsParams{
		AllFields:     query.allFields,
		ExcludeFields: query.excludeFields,
		Fields:        query.fields,
		Filter:        query.filter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster subscriptions: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list cluster subscriptions: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// CreateClusterSubscription creates a new cluster subscription.
func (client *ClusterClient) CreateClusterSubscription(subscription ClusterSubscription) (ClusterSubscription, error) {
	klog.V(100).Infof("Creating cluster subscription %#v", subscription)

	resp, err := client.CreateSubscriptionWithResponse(context.TODO(), subscription)
	if err != nil {
		return ClusterSubscription{}, fmt.Errorf("failed to create cluster subscription: error contacting api: %w", err)
	}

	if resp.StatusCode() != 201 || resp.JSON201 == nil {
		return ClusterSubscription{},
			fmt.Errorf("failed to create cluster subscription: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON201, nil
}

// GetClusterSubscription retrieves exactly one cluster subscription by its ID which must be a valid UUID.
func (client *ClusterClient) GetClusterSubscription(id uuid.UUID) (ClusterSubscription, error) {
	klog.V(100).Infof("Getting cluster subscription with id %v", id)

	resp, err := client.GetSubscriptionWithResponse(context.TODO(), id)
	if err != nil {
		return ClusterSubscription{}, fmt.Errorf("failed to get cluster subscription: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return ClusterSubscription{},
			fmt.Errorf("failed to get cluster subscription: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// DeleteClusterSubscription deletes exactly one cluster subscription by its ID which must be a valid UUID.
func (client *ClusterClient) DeleteClusterSubscription(id uuid.UUID) error {
	klog.V(100).Infof("Deleting cluster subscription with id %v", id)

	resp, err := client.DeleteSubscriptionWithResponse(context.TODO(), id)
	if err != nil {
		return fmt.Errorf("failed to delete cluster subscription: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("failed to delete cluster subscription: received error from api: %w", apiErrorFromResponse(resp))
	}

	return nil
}

// ListAlarmDictionaries lists all alarm dictionaries. Optional list options can be provided to filter the list or
// select fields. Use WithFilter to filter and WithFields to select fields. If multiple WithFilter or WithFields options
// are provided, only the last one is used. filter.And() can be used to combine multiple filters.
func (client *ClusterClient) ListAlarmDictionaries(opts ...ListOption) ([]AlarmDictionary, error) {
	query := applyListOptions(opts...)

	if query.hasOptions() {
		klog.V(100).Infof("Listing alarm dictionaries with query options %#v", query)
	} else {
		klog.V(100).Info("Listing alarm dictionaries without query options")
	}

	resp, err := client.GetAlarmDictionariesWithResponse(context.TODO(), &cluster.GetAlarmDictionariesParams{
		AllFields:     query.allFields,
		ExcludeFields: query.excludeFields,
		Fields:        query.fields,
		Filter:        query.filter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list alarm dictionaries: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list alarm dictionaries: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetAlarmDictionary gets an alarm dictionary by its ID which must be a valid UUID.
func (client *ClusterClient) GetAlarmDictionary(id uuid.UUID) (AlarmDictionary, error) {
	klog.V(100).Infof("Getting alarm dictionary with id %v", id)

	resp, err := client.GetAlarmDictionaryWithResponse(context.TODO(), id)
	if err != nil {
		return AlarmDictionary{}, fmt.Errorf("failed to get alarm dictionary: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmDictionary{}, fmt.Errorf("failed to get alarm dictionary: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}
