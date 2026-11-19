package helper

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	mocksmo "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/oran-mock-smo"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NewProvisioningRequest loads the ProvisioningRequest template from the configured source
// (ECO_CNF_RAN_PROVISIONING_REQUEST_URL or _PATH), overrides metadata.name to TestPRName, and sets templateVersion
// to ClusterTemplateAffix-templateVersion. All templateParameters (clusterInstanceParameters, nodeClusterName,
// oCloudSiteId, policyTemplateParameters, etc.) come directly from the site-config YAML.
func NewProvisioningRequest(
	client runtimeclient.Client, templateVersion string) (*oran.ProvisioningRequestBuilder, error) {
	prTemplate, err := loadProvisioningRequestTemplate()
	if err != nil {
		return nil, fmt.Errorf("load ProvisioningRequest template: %w", err)
	}

	versionWithAffix := RANConfig.ClusterTemplateAffix + "-" + templateVersion

	prBuilder := oran.NewPRBuilder(client, tsparams.TestPRName, prTemplate.Spec.TemplateName, versionWithAffix)
	for key, value := range prTemplate.Spec.TemplateParameters {
		prBuilder = prBuilder.WithTemplateParameter(key, value)
	}

	return prBuilder, nil
}

// NewProvisioningRequestNamed loads the ProvisioningRequest template from the configured source, overrides
// metadata.name, and sets templateVersion to ClusterTemplateAffix-templateVersion.
func NewProvisioningRequestNamed(
	client runtimeclient.Client, name, templateVersion string) (*oran.ProvisioningRequestBuilder, error) {
	prTemplate, err := loadProvisioningRequestTemplate()
	if err != nil {
		return nil, fmt.Errorf("load ProvisioningRequest template: %w", err)
	}

	versionWithAffix := RANConfig.ClusterTemplateAffix + "-" + templateVersion

	prBuilder := oran.NewPRBuilder(client, name, prTemplate.Spec.TemplateName, versionWithAffix)
	for key, value := range prTemplate.Spec.TemplateParameters {
		prBuilder = prBuilder.WithTemplateParameter(key, value)
	}

	return prBuilder, nil
}

// WithClusterInstanceClusterName sets clusterInstanceParameters.clusterName on the ProvisioningRequest builder.
func WithClusterInstanceClusterName(
	prBuilder *oran.ProvisioningRequestBuilder, clusterName string) (*oran.ProvisioningRequestBuilder, error) {
	templateParameters, err := prBuilder.GetTemplateParameters()
	if err != nil {
		return prBuilder, fmt.Errorf("failed to get template parameters: %w", err)
	}

	clusterInstanceParams, ok := templateParameters[tsparams.ClusterInstanceParamsKey].(map[string]any)
	if !ok {
		return prBuilder, fmt.Errorf("template parameter %q is not a map[string]any",
			tsparams.ClusterInstanceParamsKey)
	}

	clusterInstanceParams["clusterName"] = clusterName

	return prBuilder.WithTemplateParameter(tsparams.ClusterInstanceParamsKey, clusterInstanceParams), nil
}

// NewSecondaryProvisioningRequest loads the ProvisioningRequest template and builds a secondary PR using TestPRName2
// with synthetic cluster/node names so the request does not fail on clusterName ownership before hardware allocation
// is attempted. Node count matches the primary topology so MNO pools remain fully claimed.
func NewSecondaryProvisioningRequest(
	client runtimeclient.Client, templateVersion string) (*oran.ProvisioningRequestBuilder, error) {
	prTemplate, err := loadProvisioningRequestTemplate()
	if err != nil {
		return nil, fmt.Errorf("load ProvisioningRequest template for secondary PR: %w", err)
	}

	secondaryCIParams, err := buildSecondaryCIParams(prTemplate)
	if err != nil {
		return nil, fmt.Errorf("build secondary clusterInstanceParameters: %w", err)
	}

	versionWithAffix := RANConfig.ClusterTemplateAffix + "-" + templateVersion

	prBuilder := oran.NewPRBuilder(client, tsparams.TestPRName2, prTemplate.Spec.TemplateName, versionWithAffix)
	for key, value := range prTemplate.Spec.TemplateParameters {
		prBuilder = prBuilder.WithTemplateParameter(key, value)
	}

	prBuilder = prBuilder.
		WithTemplateParameter("nodeClusterName", tsparams.TestName2).
		WithTemplateParameter(tsparams.ClusterInstanceParamsKey, secondaryCIParams)

	return prBuilder, nil
}

// WaitForNoncompliantImmutable waits up to timeout until one of the policies in namespace is NonCompliant and the
// message history shows it is due to an immutable field.
func WaitForNoncompliantImmutable(client *clients.Settings, namespace string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			policies, err := ocm.ListPoliciesInAllNamespaces(client, runtimeclient.ListOptions{Namespace: namespace})
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to list all policies in namespace %s: %v", namespace, err)

				return false, nil
			}

			for _, policy := range policies {
				if policy.Definition.Status.ComplianceState == policiesv1.NonCompliant {
					klog.V(tsparams.LogLevel).Infof("Policy %s in namespace %s is not compliant, checking history",
						policy.Definition.Name, policy.Definition.Namespace)

					details := policy.Definition.Status.Details
					if len(details) != 1 {
						continue
					}

					history := details[0].History
					if len(history) < 1 {
						continue
					}

					if strings.Contains(history[0].Message, tsparams.ImmutableMessage) {
						klog.V(tsparams.LogLevel).Infof("Policy %s in namespace %s is not compliant due to an immutable field",
							policy.Definition.Name, policy.Definition.Namespace)

						return true, nil
					}
				}
			}

			return false, nil
		})
}

// WaitForAlarmToExist waits up to timeout until an alarm with the matching extensions exists. This is done by listing
// all alarms and returning the first alarm where each key-value pair in matchingExtensions is a key-value pair in the
// alarm's extensions.
//
// The returned alarm is guaranteed to be non-nil if error is nil.
func WaitForAlarmToExist(
	alarmsClient *oranapi.AlarmsClient,
	matchingExtensions map[string]string,
	timeout time.Duration) (*oranapi.AlarmEventRecord, error) {
	return waitForAlarmToExist(func() ([]oranapi.AlarmEventRecord, error) {
		return alarmsClient.ListAlarms()
	}, func(alarm oranapi.AlarmEventRecord) map[string]any {
		return alarm.Extensions
	}, matchingExtensions, timeout)
}

// WaitForAlarmV2ToExist waits up to timeout for the first v2 alarm with all the matching extension key-value pairs.
// The returned alarm is guaranteed to be non-nil if error is nil.
func WaitForAlarmV2ToExist(
	alarmsClient *oranapi.AlarmsV2Client,
	matchingExtensions map[string]string,
	timeout time.Duration) (*oranapi.AlarmEventRecordV2, error) {
	return waitForAlarmToExist(func() ([]oranapi.AlarmEventRecordV2, error) {
		return alarmsClient.ListAlarms()
	}, func(alarm oranapi.AlarmEventRecordV2) map[string]any {
		return alarm.Extensions
	}, matchingExtensions, timeout)
}

// waitForAlarmToExist polls the supplied list function until it finds an alarm with matching extensions.
func waitForAlarmToExist[T any](
	list func() ([]T, error),
	extensions func(T) map[string]any,
	matchingExtensions map[string]string,
	timeout time.Duration,
) (*T, error) {
	var matchingAlarm *T

	err := wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			alarms, err := list()
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to list alarms: %v", err)

				return false, nil
			}

			for _, alarm := range alarms {
				if matchesExtensions(extensions(alarm), matchingExtensions) {
					matchingAlarm = &alarm

					return true, nil
				}
			}

			return false, nil
		})
	if err != nil {
		return nil, fmt.Errorf("failed to wait for alarm matching %v to exist: %w", matchingExtensions, err)
	}

	return matchingAlarm, nil
}

// matchesExtensions returns true if each key-value pair in matchingExtensions is a key-value pair in the extensions
// map. Keys not in matchingExtensions are ignored.
func matchesExtensions(extensions map[string]any, matchingExtensions map[string]string) bool {
	for key, value := range matchingExtensions {
		if extensionValue, ok := extensions[key].(string); !ok || extensionValue != value {
			return false
		}
	}

	return true
}

// WaitForAllNotifications waits up to timeout until all the expected trackers have been received as notifications by
// the mock SMO observer echo endpoint. The expectedTrackers map is modified in place as trackers are found; when all
// trackers are received, the map will be empty.
func WaitForAllNotifications(
	client *clients.Settings,
	namespace string,
	observerID string,
	startTime time.Time,
	expectedTrackers map[string]bool,
	timeout time.Duration) error {
	return waitForAllNotifications(client, namespace, observerID, startTime, expectedTrackers, timeout,
		func(notification *oranapi.AlarmEventNotification) map[string]any {
			return notification.Extensions
		})
}

// WaitForNewNodeClusterNotificationsV2 waits up to timeout for the mock SMO observer to receive a NEW NodeCluster
// notification for each expected tracker. It removes received trackers from expectedTrackers in place, leaving the
// map empty when all trackers have been received.
func WaitForNewNodeClusterNotificationsV2(
	client *clients.Settings,
	namespace string,
	observerID string,
	startTime time.Time,
	expectedTrackers map[string]bool,
	timeout time.Duration) error {
	return waitForAllNotifications(client, namespace, observerID, startTime, expectedTrackers, timeout,
		func(notification *oranapi.AlarmEventNotificationV2) map[string]any {
			if notification.NotificationEventType != oranapi.AlarmEventNotificationTypeV2NEW ||
				notification.ObjectClass != tsparams.NodeClusterObjectClass {
				return nil
			}

			return notification.Extensions
		})
}

// waitForAllNotifications polls the mock SMO observer and removes trackers found in the selected extensions.
func waitForAllNotifications[T any](
	client *clients.Settings,
	namespace string,
	observerID string,
	startTime time.Time,
	expectedTrackers map[string]bool,
	timeout time.Duration,
	extensions func(*T) map[string]any,
) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			// Set the new start time to right before we check so there is no risk of missing a
			// notification. Delete is a no-op if the tracker is not in the map, so we can tolerate
			// duplicates.
			newStartTime := time.Now()

			receivedNotifications, err := mocksmo.ListReceived[T](
				client, namespace, startTime, observerID)
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to list received notifications: %v", err)

				return false, nil
			}

			for _, notification := range receivedNotifications {
				if tracker, ok := extensions(notification)["tracker"].(string); ok {
					klog.V(tsparams.LogLevel).Infof("Deleting expected tracker %s", tracker)

					delete(expectedTrackers, tracker)
				}
			}

			startTime = newStartTime

			klog.V(tsparams.LogLevel).Infof("Waiting for %d more notifications", len(expectedTrackers))

			return len(expectedTrackers) == 0, nil
		})
}

// AlarmSubscriptionV2Exists checks whether the server still has the subscription with the given ID.
func AlarmSubscriptionV2Exists(client *oranapi.AlarmsV2Client, subscriptionID uuid.UUID) (bool, error) {
	response, err := client.GetSubscriptionWithResponse(context.TODO(), subscriptionID)
	if err != nil {
		return false, fmt.Errorf("failed to get alarm subscription %s: %w", subscriptionID, err)
	}

	switch response.StatusCode() {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status for alarm subscription %s: %d", subscriptionID, response.StatusCode())
	}
}
