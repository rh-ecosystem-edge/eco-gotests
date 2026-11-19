package api

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/filter"
	alarmsv2 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/internal/alarms/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/internal/common"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

// AlarmEventRecordV2 is the type of the AlarmEventRecord resource returned by the v2 API.
type AlarmEventRecordV2 = alarmsv2.AlarmEventRecord

// AlarmEventRecordModificationsV2 is the type of the AlarmEventRecordModifications resource returned by the v2 API.
type AlarmEventRecordModificationsV2 = alarmsv2.AlarmEventRecordModifications

// AlarmServiceConfigurationV2 is the type of the AlarmServiceConfiguration resource returned by the v2 API.
type AlarmServiceConfigurationV2 = alarmsv2.AlarmServiceConfiguration

// AlarmServiceConfigurationPatchV2 is the type of the AlarmServiceConfigurationPatch resource sent to the v2 API.
type AlarmServiceConfigurationPatchV2 = alarmsv2.AlarmServiceConfigurationPatch

// AlarmSubscriptionInfoV2 is the type of the AlarmSubscriptionInfo resource returned by the v2 API.
type AlarmSubscriptionInfoV2 = alarmsv2.AlarmSubscriptionInfo

// AlarmSubscriptionUpdateV2 is the type of the AlarmSubscriptionUpdate resource sent to the v2 API.
type AlarmSubscriptionUpdateV2 = alarmsv2.AlarmSubscriptionUpdate

// AlarmEventNotificationTypeV2 is the type of the AlarmEventNotificationType field returned by the v2 API.
type AlarmEventNotificationTypeV2 = alarmsv2.AlarmEventNotificationNotificationEventType

// AlarmEventNotificationV2 is the type of the AlarmEventNotification resource returned by the v2 API.
type AlarmEventNotificationV2 = alarmsv2.AlarmEventNotification

//nolint:revive // These are just re-exported constants no need for the linting.
const (
	AlarmEventNotificationTypeV2ACKNOWLEDGE AlarmEventNotificationTypeV2 = alarmsv2.AlarmEventNotificationNotificationEventTypeACKNOWLEDGE
	AlarmEventNotificationTypeV2CHANGE      AlarmEventNotificationTypeV2 = alarmsv2.AlarmEventNotificationNotificationEventTypeCHANGE
	AlarmEventNotificationTypeV2CLEAR       AlarmEventNotificationTypeV2 = alarmsv2.AlarmEventNotificationNotificationEventTypeCLEAR
	AlarmEventNotificationTypeV2NEW         AlarmEventNotificationTypeV2 = alarmsv2.AlarmEventNotificationNotificationEventTypeNEW
)

// AlarmSubscriptionFilterV2 is the type of the AlarmSubscriptionInfoEventFilter field returned by the v2 API.
type AlarmSubscriptionFilterV2 = alarmsv2.AlarmSubscriptionInfoEventFilter

//nolint:revive // These are just re-exported constants no need for the linting.
const (
	AlarmSubscriptionFilterV2ACKNOWLEDGE AlarmSubscriptionFilterV2 = alarmsv2.AlarmSubscriptionInfoEventFilterACKNOWLEDGE
	AlarmSubscriptionFilterV2CHANGE      AlarmSubscriptionFilterV2 = alarmsv2.AlarmSubscriptionInfoEventFilterCHANGE
	AlarmSubscriptionFilterV2CLEAR       AlarmSubscriptionFilterV2 = alarmsv2.AlarmSubscriptionInfoEventFilterCLEAR
	AlarmSubscriptionFilterV2NEW         AlarmSubscriptionFilterV2 = alarmsv2.AlarmSubscriptionInfoEventFilterNEW
)

// AlarmSubscriptionUpdateFilterV2 is the type of the AlarmSubscriptionUpdateEventFilter field sent to the v2 API.
type AlarmSubscriptionUpdateFilterV2 = alarmsv2.AlarmSubscriptionUpdateEventFilter

//nolint:revive // These are just re-exported constants no need for the linting.
const (
	AlarmSubscriptionUpdateFilterV2ACKNOWLEDGE AlarmSubscriptionUpdateFilterV2 = alarmsv2.AlarmSubscriptionUpdateEventFilterACKNOWLEDGE
	AlarmSubscriptionUpdateFilterV2CHANGE      AlarmSubscriptionUpdateFilterV2 = alarmsv2.AlarmSubscriptionUpdateEventFilterCHANGE
	AlarmSubscriptionUpdateFilterV2CLEAR       AlarmSubscriptionUpdateFilterV2 = alarmsv2.AlarmSubscriptionUpdateEventFilterCLEAR
	AlarmSubscriptionUpdateFilterV2NEW         AlarmSubscriptionUpdateFilterV2 = alarmsv2.AlarmSubscriptionUpdateEventFilterNEW
)

// PerceivedSeverityV2 is the type of the PerceivedSeverity field returned by the v2 API.
type PerceivedSeverityV2 = alarmsv2.PerceivedSeverity

//nolint:revive // These are just re-exported constants no need for the linting.
const (
	PerceivedSeverityV2CLEARED       PerceivedSeverityV2 = alarmsv2.CLEARED
	PerceivedSeverityV2CRITICAL      PerceivedSeverityV2 = alarmsv2.CRITICAL
	PerceivedSeverityV2INDETERMINATE PerceivedSeverityV2 = alarmsv2.INDETERMINATE
	PerceivedSeverityV2MAJOR         PerceivedSeverityV2 = alarmsv2.MAJOR
	PerceivedSeverityV2MINOR         PerceivedSeverityV2 = alarmsv2.MINOR
	PerceivedSeverityV2WARNING       PerceivedSeverityV2 = alarmsv2.WARNING
)

// AlarmsV2Client provides access to version 2 of the O2IMS infrastructure monitoring API. It is not a
// runtimeclient.Client since AlarmEventRecords do not correspond to CRs.
type AlarmsV2Client struct {
	alarmsv2.ClientWithResponsesInterface
}

// ListAlarms lists all alarms. Optionally, a filter can be provided to filter the list of alarms. If more than one
// filter is provided, only the first one is used. filter.And() can be used to combine multiple filters.
func (client *AlarmsV2Client) ListAlarms(filter ...filter.Filter) ([]AlarmEventRecordV2, error) {
	var filterString *common.Filter

	if len(filter) > 0 {
		filterString = ptr.To(filter[0].Filter())

		klog.V(100).Infof("Listing alarms with filter %q", *filterString)
	} else {
		klog.V(100).Infof("Listing alarms without filter")
	}

	resp, err := client.GetAlarmsWithResponse(context.TODO(), &alarmsv2.GetAlarmsParams{Filter: filterString})
	if err != nil {
		return nil, fmt.Errorf("failed to list alarms: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list alarms: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetAlarm gets an alarm by its ID which must be a valid UUID.
func (client *AlarmsV2Client) GetAlarm(id uuid.UUID) (AlarmEventRecordV2, error) {
	klog.V(100).Infof("Getting alarm with id %v", id)

	resp, err := client.GetAlarmWithResponse(context.TODO(), id)
	if err != nil {
		return AlarmEventRecordV2{}, fmt.Errorf("failed to get alarm: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmEventRecordV2{}, fmt.Errorf("failed to get alarm: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// PatchAlarm patches an alarm. Only the non-nil fields of the patch will be applied. The ID must be a valid UUID.
func (client *AlarmsV2Client) PatchAlarm(
	id uuid.UUID, patch AlarmEventRecordModificationsV2) (AlarmEventRecordModificationsV2, error) {
	klog.V(100).Infof("Patching alarm with id %v with patch %#v", id, patch)

	resp, err := client.PatchAlarmWithApplicationMergePatchPlusJSONBodyWithResponse(context.TODO(), id, patch)
	if err != nil {
		return AlarmEventRecordModificationsV2{}, fmt.Errorf("failed to patch alarm: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmEventRecordModificationsV2{},
			fmt.Errorf("failed to patch alarm: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetServiceConfiguration retrieves the alarm service configuration.
func (client *AlarmsV2Client) GetServiceConfiguration() (AlarmServiceConfigurationV2, error) {
	klog.V(100).Info("Getting service configuration")

	resp, err := client.GetServiceConfigurationWithResponse(context.TODO())
	if err != nil {
		return AlarmServiceConfigurationV2{}, fmt.Errorf("failed to get service configuration: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmServiceConfigurationV2{},
			fmt.Errorf("failed to get service configuration: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// UpdateAlarmServiceConfiguration modifies all fields of the Alarm Service Configuration.
func (client *AlarmsV2Client) UpdateAlarmServiceConfiguration(
	config AlarmServiceConfigurationV2) (AlarmServiceConfigurationV2, error) {
	klog.V(100).Infof("Updating service configuration with config %#v", config)

	resp, err := client.UpdateAlarmServiceConfigurationWithResponse(context.TODO(), config)
	if err != nil {
		return AlarmServiceConfigurationV2{},
			fmt.Errorf("failed to update service configuration: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmServiceConfigurationV2{},
			fmt.Errorf("failed to update service configuration: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// PatchAlarmServiceConfiguration modifies individual fields of the Alarm Service Configuration.
func (client *AlarmsV2Client) PatchAlarmServiceConfiguration(
	config AlarmServiceConfigurationPatchV2) (AlarmServiceConfigurationV2, error) {
	klog.V(100).Infof("Patching service configuration with config %#v", config)

	resp, err := client.PatchAlarmServiceConfigurationWithApplicationMergePatchPlusJSONBodyWithResponse(
		context.TODO(), config)
	if err != nil {
		return AlarmServiceConfigurationV2{}, fmt.Errorf("failed to patch service configuration: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmServiceConfigurationV2{},
			fmt.Errorf("failed to patch service configuration: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// ListSubscriptions retrieves the list of alarm subscriptions. Optionally, a filter can be provided to filter the list.
// If more than one filter is provided, only the first one is used. filter.And() can be used to combine multiple
// filters.
func (client *AlarmsV2Client) ListSubscriptions(filter ...filter.Filter) ([]AlarmSubscriptionInfoV2, error) {
	var filterString *common.Filter

	if len(filter) > 0 {
		filterString = ptr.To(filter[0].Filter())

		klog.V(100).Infof("Listing subscriptions with filter %q", *filterString)
	} else {
		klog.V(100).Infof("Listing subscriptions without filter")
	}

	resp, err := client.GetSubscriptionsWithResponse(context.TODO(), &alarmsv2.GetSubscriptionsParams{Filter: filterString})
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list subscriptions: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// CreateSubscription creates a new alarm subscription.
func (client *AlarmsV2Client) CreateSubscription(
	subscription AlarmSubscriptionInfoV2) (AlarmSubscriptionInfoV2, error) {
	klog.V(100).Infof("Creating subscription %#v", subscription)

	resp, err := client.CreateSubscriptionWithResponse(context.TODO(), subscription)
	if err != nil {
		return AlarmSubscriptionInfoV2{}, fmt.Errorf("failed to create subscription: error contacting api: %w", err)
	}

	if resp.StatusCode() != 201 || resp.JSON201 == nil {
		return AlarmSubscriptionInfoV2{},
			fmt.Errorf("failed to create subscription: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON201, nil
}

// GetSubscription retrieves exactly one subscription by its ID which must be a valid UUID.
func (client *AlarmsV2Client) GetSubscription(id uuid.UUID) (AlarmSubscriptionInfoV2, error) {
	klog.V(100).Infof("Getting subscription with id %v", id)

	resp, err := client.GetSubscriptionWithResponse(context.TODO(), id)
	if err != nil {
		return AlarmSubscriptionInfoV2{}, fmt.Errorf("failed to get subscription: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmSubscriptionInfoV2{},
			fmt.Errorf("failed to get subscription: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// DeleteSubscription deletes exactly one subscription by its ID which must be a valid UUID.
func (client *AlarmsV2Client) DeleteSubscription(id uuid.UUID) error {
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

// PatchSubscription modifies individual fields of an alarm subscription. Only the non-nil fields of the patch will be
// applied. The ID must be a valid UUID.
func (client *AlarmsV2Client) PatchSubscription(
	id uuid.UUID, patch AlarmSubscriptionUpdateV2) (AlarmSubscriptionInfoV2, error) {
	klog.V(100).Infof("Patching subscription with id %v with patch %#v", id, patch)

	resp, err := client.PatchSubscriptionWithApplicationMergePatchPlusJSONBodyWithResponse(context.TODO(), id, patch)
	if err != nil {
		return AlarmSubscriptionInfoV2{}, fmt.Errorf("failed to patch subscription: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return AlarmSubscriptionInfoV2{},
			fmt.Errorf("failed to patch subscription: received error from api: %w", apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}
