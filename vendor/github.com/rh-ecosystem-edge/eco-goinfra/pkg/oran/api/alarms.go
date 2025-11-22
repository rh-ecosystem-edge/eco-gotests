package api

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/filter"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/internal/alarms"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/internal/common"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

// AlarmEventRecord is the type of the AlarmEventRecord resource returned by the API.
type AlarmEventRecord = alarms.AlarmEventRecord

// AlarmEventRecordModifications is the type of the AlarmEventRecordModifications resource returned by the API.
type AlarmEventRecordModifications = alarms.AlarmEventRecordModifications

// AlarmServiceConfiguration is the type of the AlarmServiceConfiguration resource returned by the API.
type AlarmServiceConfiguration = alarms.AlarmServiceConfiguration

// AlarmSubscriptionInfo is the type of the AlarmSubscriptionInfo resource returned by the API.
type AlarmSubscriptionInfo = alarms.AlarmSubscriptionInfo

// AlarmEventNotificationType is the type of the AlarmEventNotificationType field returned by the API.
type AlarmEventNotificationType = alarms.AlarmEventNotificationNotificationEventType

// AlarmEventNotification is the type of the AlarmEventNotification resource returned by the API.
type AlarmEventNotification = alarms.AlarmEventNotification

//nolint:revive // These are just re-exported constants no need for the linting.
const (
	AlarmEventNotificationTypeACKNOWLEDGE AlarmEventNotificationType = alarms.AlarmEventNotificationNotificationEventTypeACKNOWLEDGE
	AlarmEventNotificationTypeCHANGE      AlarmEventNotificationType = alarms.AlarmEventNotificationNotificationEventTypeCHANGE
	AlarmEventNotificationTypeCLEAR       AlarmEventNotificationType = alarms.AlarmEventNotificationNotificationEventTypeCLEAR
	AlarmEventNotificationTypeNEW         AlarmEventNotificationType = alarms.AlarmEventNotificationNotificationEventTypeNEW
)

// AlarmSubscriptionFilter is the type of the AlarmSubscriptionInfoFilter field returned by the API.
type AlarmSubscriptionFilter = alarms.AlarmSubscriptionInfoFilter

//nolint:revive // These are just re-exported constants no need for the linting.
const (
	AlarmSubscriptionFilterACKNOWLEDGE AlarmSubscriptionFilter = alarms.AlarmSubscriptionInfoFilterACKNOWLEDGE
	AlarmSubscriptionFilterCHANGE      AlarmSubscriptionFilter = alarms.AlarmSubscriptionInfoFilterCHANGE
	AlarmSubscriptionFilterCLEAR       AlarmSubscriptionFilter = alarms.AlarmSubscriptionInfoFilterCLEAR
	AlarmSubscriptionFilterNEW         AlarmSubscriptionFilter = alarms.AlarmSubscriptionInfoFilterNEW
)

// PerceivedSeverity is the type of the PerceivedSeverity field returned by the API.
type PerceivedSeverity = alarms.PerceivedSeverity

//nolint:revive // These are just re-exported constants no need for the linting.
const (
	PerceivedSeverityCLEARED       PerceivedSeverity = alarms.CLEARED
	PerceivedSeverityCRITICAL      PerceivedSeverity = alarms.CRITICAL
	PerceivedSeverityINDETERMINATE PerceivedSeverity = alarms.INDETERMINATE
	PerceivedSeverityMAJOR         PerceivedSeverity = alarms.MAJOR
	PerceivedSeverityMINOR         PerceivedSeverity = alarms.MINOR
	PerceivedSeverityWARNING       PerceivedSeverity = alarms.WARNING
)

// AlarmsClient provides access to the O2IMS infrastructure monitoring API. It is not a runtimeclient.Client since
// AlarmEventRecords do not correspond to CRs.
type AlarmsClient struct {
	alarms.ClientWithResponsesInterface
}

// ListAlarms lists all alarms. Optionally, a filter can be provided to filter the list of alarms. If more than one
// filter is provided, only the first one is used. filter.And() can be used to combine multiple filters.
func (client *AlarmsClient) ListAlarms(filter ...filter.Filter) ([]AlarmEventRecord, error) {
	var filterString *common.Filter

	if len(filter) > 0 {
		filterString = ptr.To(filter[0].Filter())

		klog.V(100).Infof("Listing alarms with filter %q", *filterString)
	} else {
		klog.V(100).Infof("Listing alarms without filter")
	}

	resp, err := client.GetAlarmsWithResponse(context.TODO(), &alarms.GetAlarmsParams{Filter: filterString})
	if err != nil {
		return nil, fmt.Errorf("failed to list alarms: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list alarms: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetAlarm gets an alarm by its ID which must be a valid UUID.
func (client *AlarmsClient) GetAlarm(id uuid.UUID) (AlarmEventRecord, error) {
	klog.V(100).Infof("Getting alarm with id %v", id)

	resp, err := client.GetAlarmWithResponse(context.TODO(), id)
	if err != nil {
		return AlarmEventRecord{}, fmt.Errorf("failed to get alarm: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmEventRecord{}, fmt.Errorf("failed to get alarm: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// PatchAlarm patches an alarm. Only the non-nil fields of the patch will be applied. The ID must be a valid UUID.
func (client *AlarmsClient) PatchAlarm(
	id uuid.UUID, patch AlarmEventRecordModifications) (AlarmEventRecordModifications, error) {
	klog.V(100).Infof("Patching alarm with id %v with patch %#v", id, patch)

	resp, err := client.PatchAlarmWithApplicationMergePatchPlusJSONBodyWithResponse(context.TODO(), id, patch)
	if err != nil {
		return AlarmEventRecordModifications{}, fmt.Errorf("failed to patch alarm: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmEventRecordModifications{},
			fmt.Errorf("failed to patch alarm: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetServiceConfiguration retrieves the alarm service configuration.
func (client *AlarmsClient) GetServiceConfiguration() (AlarmServiceConfiguration, error) {
	klog.V(100).Info("Getting service configuration")

	resp, err := client.GetServiceConfigurationWithResponse(context.TODO())
	if err != nil {
		return AlarmServiceConfiguration{}, fmt.Errorf("failed to get service configuration: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmServiceConfiguration{},
			fmt.Errorf("failed to get service configuration: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// UpdateAlarmServiceConfiguration modifies all fields of the Alarm Service Configuration.
func (client *AlarmsClient) UpdateAlarmServiceConfiguration(
	config AlarmServiceConfiguration) (AlarmServiceConfiguration, error) {
	klog.V(100).Infof("Updating service configuration with config %#v", config)

	resp, err := client.UpdateAlarmServiceConfigurationWithResponse(context.TODO(), config)
	if err != nil {
		return AlarmServiceConfiguration{},
			fmt.Errorf("failed to update service configuration: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmServiceConfiguration{},
			fmt.Errorf("failed to update service configuration: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// PatchAlarmServiceConfiguration modifies individual fields of the Alarm Service Configuration.
func (client *AlarmsClient) PatchAlarmServiceConfiguration(
	config AlarmServiceConfiguration) (AlarmServiceConfiguration, error) {
	klog.V(100).Infof("Patching service configuration with config %#v", config)

	// Using generated method names has its downsides.
	resp, err := client.PatchAlarmServiceConfigurationWithApplicationMergePatchPlusJSONBodyWithResponse(
		context.TODO(), config)
	if err != nil {
		return AlarmServiceConfiguration{}, fmt.Errorf("failed to patch service configuration: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmServiceConfiguration{},
			fmt.Errorf("failed to patch service configuration: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// ListSubscriptions retrieves the list of alarm subscriptions. Optionally, a filter can be provided to filter the list.
// If more than one filter is provided, only the first one is used. filter.And() can be used to combine multiple
// filters.
func (client *AlarmsClient) ListSubscriptions(filter ...filter.Filter) ([]AlarmSubscriptionInfo, error) {
	var filterString *common.Filter

	if len(filter) > 0 {
		filterString = ptr.To(filter[0].Filter())

		klog.V(100).Infof("Listing subscriptions with filter %q", *filterString)
	} else {
		klog.V(100).Infof("Listing subscriptions without filter")
	}

	resp, err := client.GetSubscriptionsWithResponse(context.TODO(), &alarms.GetSubscriptionsParams{Filter: filterString})
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list subscriptions: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// CreateSubscription creates a new alarm subscription.
func (client *AlarmsClient) CreateSubscription(subscription AlarmSubscriptionInfo) (AlarmSubscriptionInfo, error) {
	klog.V(100).Infof("Creating subscription %#v", subscription)

	resp, err := client.CreateSubscriptionWithResponse(context.TODO(), subscription)
	if err != nil {
		return AlarmSubscriptionInfo{}, fmt.Errorf("failed to create subscription: error contacting api: %w", err)
	}

	if resp.StatusCode() != 201 || resp.JSON201 == nil {
		return AlarmSubscriptionInfo{},
			fmt.Errorf("failed to create subscription: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON201, nil
}

// GetSubscription retrieves exactly one subscription by its ID which must be a valid UUID.
func (client *AlarmsClient) GetSubscription(id uuid.UUID) (AlarmSubscriptionInfo, error) {
	klog.V(100).Infof("Getting subscription with id %v", id)

	resp, err := client.GetSubscriptionWithResponse(context.TODO(), id)
	if err != nil {
		return AlarmSubscriptionInfo{}, fmt.Errorf("failed to get subscription: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmSubscriptionInfo{},
			fmt.Errorf("failed to get subscription: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// DeleteSubscription deletes exactly one subscription by its ID which must be a valid UUID.
func (client *AlarmsClient) DeleteSubscription(id uuid.UUID) error {
	klog.V(100).Infof("Deleting subscription with id %v", id)

	resp, err := client.DeleteSubscriptionWithResponse(context.TODO(), id)
	if err != nil {
		return fmt.Errorf("failed to delete subscription: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("failed to delete subscription: received error from api: %w", apiErrorFromResponse(resp))
	}

	return nil
}
