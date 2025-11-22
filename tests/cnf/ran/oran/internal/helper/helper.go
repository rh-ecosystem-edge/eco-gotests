package helper

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/siteconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	subscriber "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/oran-subscriber"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NewProvisioningRequest creates a ProvisioningRequest builder with templateVersion, setting all the required
// parameters and using the affix from RANConfig.
func NewProvisioningRequest(client runtimeclient.Client, templateVersion string) *oran.ProvisioningRequestBuilder {
	versionWithAffix := RANConfig.ClusterTemplateAffix + "-" + templateVersion
	prBuilder := oran.NewPRBuilder(client, tsparams.TestPRName, tsparams.ClusterTemplateName, versionWithAffix).
		WithTemplateParameter("nodeClusterName", RANConfig.Spoke1Name).
		WithTemplateParameter("oCloudSiteId", tsparams.OCloudSiteID).
		WithTemplateParameter("policyTemplateParameters", map[string]any{}).
		WithTemplateParameter("clusterInstanceParameters", map[string]any{
			"clusterName": RANConfig.Spoke1Name,
			"nodes": []map[string]any{{
				"hostName": RANConfig.Spoke1Hostname,
			}},
		})

	return prBuilder
}

// NewNoTemplatePR creates a ProvisioningRequest builder with templateVersion, following the schema for no
// HardwareTemplate. All required parameters and the affix are set from RANConfig. The BMC and network data are
// incorrect so that a ClusterInstance is generated but will not actually provision.
func NewNoTemplatePR(client runtimeclient.Client, templateVersion string) *oran.ProvisioningRequestBuilder {
	versionWithAffix := RANConfig.ClusterTemplateAffix + "-" + templateVersion
	prBuilder := oran.NewPRBuilder(client, tsparams.TestPRName, tsparams.ClusterTemplateName, versionWithAffix).
		WithTemplateParameter("nodeClusterName", RANConfig.Spoke1Name).
		WithTemplateParameter("oCloudSiteId", tsparams.OCloudSiteID).
		WithTemplateParameter("policyTemplateParameters", map[string]any{}).
		WithTemplateParameter("clusterInstanceParameters", map[string]any{
			"clusterName": RANConfig.Spoke1Name,
			"nodes": []map[string]any{{
				"hostName": "fake.apps." + RANConfig.Spoke1Hostname,
				// 192.0.2.0 is a reserved test address so we never accidentally use a valid IP.
				"bmcAddress": "redfish-VirtualMedia://192.0.2.0/redfish/v1/Systems/System.Embedded.1",
				"bmcCredentialsDetails": map[string]any{
					"username": tsparams.TestBase64Credential,
					"password": tsparams.TestBase64Credential,
				},
				"bootMACAddress": "01:23:45:67:89:AB",
				"nodeNetwork": map[string]any{
					"interfaces": []map[string]any{{
						"macAddress": "01:23:45:67:89:AB",
					}},
				},
			}},
		})

	return prBuilder
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

// WaitForValidPRClusterInstance waits up to timeout until the ClusterInstance for the ProvisioningRequest has condition
// RenderedTemplatesApplied.
func WaitForValidPRClusterInstance(client *clients.Settings, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			clusterInstance, err := siteconfig.PullClusterInstance(client, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to pull ClusterInstance %s: %v", RANConfig.Spoke1Name, err)

				return false, nil
			}

			return slices.ContainsFunc(clusterInstance.Definition.Status.Conditions, func(condition metav1.Condition) bool {
				return condition.Type == string(siteconfigv1alpha1.RenderedTemplatesApplied) &&
					condition.Status == metav1.ConditionTrue
			}), nil
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
	var matchingAlarm *oranapi.AlarmEventRecord

	err := wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			alarms, err := alarmsClient.ListAlarms()
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to list alarms: %v", err)

				return false, nil
			}

			for _, alarm := range alarms {
				if matchesExtensions(alarm.Extensions, matchingExtensions) {
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
func matchesExtensions(extensions map[string]string, matchingExtensions map[string]string) bool {
	for key, value := range matchingExtensions {
		if extensions[key] != value {
			return false
		}
	}

	return true
}

// WaitForAllNotifications waits up to timeout until all the expected trackers have been received as notifications by
// the subscriber. The expectedTrackers map is modified in place as trackers are found; when all trackers are received,
// the map will be empty.
func WaitForAllNotifications(
	client *clients.Settings,
	namespace string,
	startTime time.Time,
	expectedTrackers map[string]bool,
	timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			// Set the new start time to right before we check so there is no risk of missing a
			// notification. Delete is a no-op if the tracker is not in the map, so we can tolerate
			// duplicates.
			newStartTime := time.Now()

			receivedNotifications, err := subscriber.ListReceivedNotifications(client, namespace, startTime)
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to list received notifications: %v", err)

				return false, nil
			}

			for _, notification := range receivedNotifications {
				if tracker, ok := notification.Extensions["tracker"]; ok {
					klog.V(tsparams.LogLevel).Infof("Deleting expected tracker %s", tracker)

					delete(expectedTrackers, tracker)
				}
			}

			startTime = newStartTime

			klog.V(tsparams.LogLevel).Infof("Waiting for %d more notifications", len(expectedTrackers))

			return len(expectedTrackers) == 0, nil
		})
}
