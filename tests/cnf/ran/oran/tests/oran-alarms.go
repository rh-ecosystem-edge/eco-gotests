package tests

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	alertmanagerv2 "github.com/prometheus/alertmanager/api/v2/client"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/filter"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/alerter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/alert"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/auth"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	mocksmo "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/oran-mock-smo"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

// mockSMOBaseURL is the base URL of the mock SMO, including the scheme.
var mockSMOBaseURL = "https://" + RANConfig.GetAppsURL(RANConfig.MockSMOSubdomain)

var _ = Describe("ORAN Alarms Tests", Label(tsparams.LabelPostProvision, tsparams.LabelAlarms), func() {
	var (
		alarmsClient    *oranapi.AlarmsClient
		alarmsV2Client  *oranapi.AlarmsV2Client
		usingAlarmsV2   bool
		alertsClient    *alertmanagerv2.AlertmanagerAPI
		spoke1ClusterID string
	)

	BeforeEach(func() {
		var err error

		By("creating the O2IMS API client")

		clientBuilder, err := auth.NewClientBuilderForConfig(RANConfig)
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client builder")

		alarmsClient, err = clientBuilder.BuildAlarms()
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client")

		By("checking the O-Cloud Manager version")

		usingAlarmsV2, err = version.IsVersionStringInRange(
			RANConfig.HubOperatorVersions[ranparam.OCloudManager], "5.1.0-0", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to check O-Cloud Manager version")

		if usingAlarmsV2 {
			alarmsV2Client, err = clientBuilder.BuildAlarmsV2()
			Expect(err).ToNot(HaveOccurred(), "Failed to create v2 alarm API client")
		}

		By("creating the Alertmanager API client")

		alertsClient, err = alerter.CreateAlerterClientForCluster(HubAPIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create the Alertmanager API client")

		By("getting the spoke 1 cluster ID")

		spoke1ClusterID, err = rancluster.GetManagedClusterID(HubAPIClient, RANConfig.Spoke1Name)
		Expect(err).ToNot(HaveOccurred(), "Failed to get spoke 1 cluster ID")
	})

	// 91004 - Filter notifications by patching a subscription.
	It("filters alarm notifications after patching a subscription", reportxml.ID("91004"), func() {
		if !usingAlarmsV2 {
			Skip("Subscription PATCH requires the v2 alarm API")
		}

		By("creating a test subscription")

		consumerID := uuid.New().String()
		subscription, err := alarmsV2Client.CreateSubscription(oranapi.AlarmSubscriptionInfoV2{
			ConsumerSubscriptionId: &consumerID,
			Callback:               mocksmo.ObserverCallbackURL(mockSMOBaseURL, consumerID),
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create subscription")
		Expect(subscription.AlarmSubscriptionId).ToNot(BeNil(), "Missing subscription ID")
		DeferCleanup(func() {
			Expect(alarmsV2Client.DeleteSubscription(*subscription.AlarmSubscriptionId)).
				To(Succeed(), "Failed to delete subscription")
		})

		By("verifying the new subscription delivers all events")

		Expect(subscription.EventFilter).To(BeNil(), "New subscription should deliver all events")

		By("sending a test alert")

		start := time.Now()
		tracker, err := alert.SendToClient(alertsClient, alert.CreatePostable(alert.SeverityMajor, spoke1ClusterID))
		Expect(err).ToNot(HaveOccurred(), "Failed to send alert")

		By("waiting for the alarm to exist")

		alarm, err := helper.WaitForAlarmV2ToExist(alarmsV2Client, map[string]string{"tracker": tracker}, time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to find alarm")
		Expect(alarm.ObjectClass).To(Equal("oran.o2ims.cluster.NodeCluster"), "Unexpected alarm object class")

		By("waiting for the NEW notification")

		err = mocksmo.WaitFor[oranapi.AlarmEventNotificationV2](HubAPIClient, RANConfig.MockSMONamespace,
			mocksmo.WithStart[oranapi.AlarmEventNotificationV2](start),
			mocksmo.WithObserverID[oranapi.AlarmEventNotificationV2](consumerID),
			mocksmo.WithMatch(func(notification *oranapi.AlarmEventNotificationV2) bool {
				return notification.Extensions["tracker"] == tracker &&
					notification.NotificationEventType == oranapi.AlarmEventNotificationTypeV2NEW &&
					notification.ObjectClass == "oran.o2ims.cluster.NodeCluster" &&
					notification.ObjectRef[true] == "/O2ims_infrastructureMonitoring/v2/alarms/"+
						alarm.AlarmEventRecordId.String()
			}),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to receive NEW notification with the expected object reference")

		// The v2 eventFilter is exclusion-based: an event whose notificationEventType equals the filter is not
		// delivered, so setting it to NEW suppresses NEW notifications instead of selecting them.
		By("patching the subscription to exclude NEW notifications")

		patch := oranapi.AlarmSubscriptionUpdateV2{}
		patch.EventFilter.Set(oranapi.AlarmSubscriptionUpdateFilterV2NEW)
		patched, err := alarmsV2Client.PatchSubscription(*subscription.AlarmSubscriptionId, patch)
		Expect(err).ToNot(HaveOccurred(), "Failed to patch subscription")

		expectedFilter := nullable.NewNullableWithValue(oranapi.AlarmSubscriptionFilterV2NEW)
		Expect(patched.EventFilter).To(Equal(expectedFilter), "PATCH did not set eventFilter")

		By("verifying the patched event filter was persisted")

		retrieved, err := alarmsV2Client.GetSubscription(*subscription.AlarmSubscriptionId)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve subscription")
		Expect(retrieved.EventFilter).To(Equal(expectedFilter), "eventFilter was not persisted")

		By("sending a second test alert")

		start = time.Now()
		tracker, err = alert.SendToClient(alertsClient, alert.CreatePostable(alert.SeverityMajor, spoke1ClusterID))
		Expect(err).ToNot(HaveOccurred(), "Failed to send second alert")

		By("verifying the filtered alarm was persisted")

		alarm, err = helper.WaitForAlarmV2ToExist(alarmsV2Client, map[string]string{"tracker": tracker}, time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Filtered alarm was not persisted")
		Expect(alarm.ObjectClass).To(Equal("oran.o2ims.cluster.NodeCluster"), "Unexpected alarm object class")

		filteredAlarms, err := alarmsV2Client.ListAlarms(filter.Equals("objectId", spoke1ClusterID))
		Expect(err).ToNot(HaveOccurred(), "Failed to filter alarms by resource")
		Expect(slices.ContainsFunc(filteredAlarms, func(record oranapi.AlarmEventRecordV2) bool {
			return record.Extensions["tracker"] == tracker && record.ObjectClass == "oran.o2ims.cluster.NodeCluster"
		})).To(BeTrue(), "Filtered alarm was not persisted")

		By("verifying the filtered NEW notification was not delivered")

		Consistently(func(g Gomega) {
			notifications, listErr := mocksmo.ListReceived[oranapi.AlarmEventNotificationV2](
				HubAPIClient, RANConfig.MockSMONamespace, start, consumerID)
			g.Expect(listErr).ToNot(HaveOccurred(), "Failed to read notifications")
			g.Expect(slices.ContainsFunc(notifications, func(n *oranapi.AlarmEventNotificationV2) bool {
				return n.Extensions["tracker"] == tracker && n.NotificationEventType == oranapi.AlarmEventNotificationTypeV2NEW
			})).To(BeFalse(), "Filtered NEW notification was delivered")
		}, 30*time.Second, 3*time.Second).Should(Succeed())
	})

	// 83554 - Retrieve an alarm from the API
	It("retrieves an alarm from the API", reportxml.ID("83554"), func() {
		if usingAlarmsV2 {
			By("creating a test alarm")

			postableAlert := alert.CreatePostable(alert.SeverityMajor, spoke1ClusterID)
			tracker, err := alert.SendToClient(alertsClient, postableAlert)
			Expect(err).ToNot(HaveOccurred(), "Failed to send test alarm")

			By("waiting for the test alarm to exist")

			matchingAlarm, err := helper.WaitForAlarmV2ToExist(
				alarmsV2Client, map[string]string{"tracker": tracker}, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for test alarm to exist")

			alarmEventRecordID := matchingAlarm.AlarmEventRecordId

			By("retrieving the alarm from the API")

			alarm, err := alarmsV2Client.GetAlarm(alarmEventRecordID)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve alarm")
			Expect(alarm.Extensions["tracker"]).To(Equal(tracker), "Test alarm not found")
			Expect(alarm.ObjectClass).To(Equal("oran.o2ims.cluster.NodeCluster"), "Unexpected object class")

			return
		}

		By("creating a test alarm")

		postableAlert := alert.CreatePostable(alert.SeverityMajor, spoke1ClusterID)
		tracker, err := alert.SendToClient(alertsClient, postableAlert)
		Expect(err).ToNot(HaveOccurred(), "Failed to send test alarm")

		By("waiting for the test alarm to exist")

		matchingAlarm, err := helper.WaitForAlarmToExist(alarmsClient, map[string]string{"tracker": tracker}, time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for test alarm to exist")

		alarmEventRecordID := matchingAlarm.AlarmEventRecordId

		By("retrieving the alarm from the API")

		alarm, err := alarmsClient.GetAlarm(alarmEventRecordID)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve alarm")
		Expect(alarm.Extensions["tracker"]).To(Equal(tracker), "Test alarm not found")
	})

	// 83555 - Filter alarms from the API
	It("filters alarms from the API", reportxml.ID("83555"), func() {
		if usingAlarmsV2 {
			By("creating a test alarm with major severity")

			majorAlert := alert.CreatePostable(alert.SeverityMajor, spoke1ClusterID)
			majorTracker, err := alert.SendToClient(alertsClient, majorAlert)
			Expect(err).ToNot(HaveOccurred(), "Failed to send major test alarm")

			By("creating a test alarm with minor severity")

			minorAlert := alert.CreatePostable(alert.SeverityMinor, spoke1ClusterID)
			minorTracker, err := alert.SendToClient(alertsClient, minorAlert)
			Expect(err).ToNot(HaveOccurred(), "Failed to send minor test alarm")

			By("waiting for the minor and major alarms to exist")

			_, err = helper.WaitForAlarmV2ToExist(alarmsV2Client, map[string]string{"tracker": minorTracker}, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for minor test alarm to exist")
			_, err = helper.WaitForAlarmV2ToExist(alarmsV2Client, map[string]string{"tracker": majorTracker}, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for major test alarm to exist")

			By("filtering alarms by major severity and objectId")

			severityFilter := filter.Equals("perceivedSeverity", strconv.Itoa(int(oranapi.PerceivedSeverityV2MAJOR)))
			resourceFilter := filter.Equals("objectId", spoke1ClusterID)
			resourceAlarms, err := alarmsV2Client.ListAlarms(resourceFilter)
			Expect(err).ToNot(HaveOccurred(), "Failed to filter alarms by resource")
			Expect(slices.ContainsFunc(resourceAlarms, func(alarm oranapi.AlarmEventRecordV2) bool {
				return alarm.Extensions["tracker"] == majorTracker && alarm.ObjectClass == "oran.o2ims.cluster.NodeCluster"
			})).To(BeTrue(), "Major alarm missing or has unexpected object class")

			combinedFilter := filter.And(severityFilter, resourceFilter)

			filteredAlarms, err := alarmsV2Client.ListAlarms(combinedFilter)
			Expect(err).ToNot(HaveOccurred(), "Failed to filter alarms")

			By("verifying the filtered results contain the major alarm but not the minor alarm")

			containsMajorAlarm := slices.ContainsFunc(filteredAlarms, func(alarm oranapi.AlarmEventRecordV2) bool {
				return alarm.Extensions["tracker"] == majorTracker
			})
			containsMinorAlarm := slices.ContainsFunc(filteredAlarms, func(alarm oranapi.AlarmEventRecordV2) bool {
				return alarm.Extensions["tracker"] == minorTracker
			})

			Expect(containsMajorAlarm).To(BeTrue(), "Major alarm should be found in filtered results")
			Expect(containsMinorAlarm).To(BeFalse(), "Minor alarm should not be found in filtered results")

			return
		}

		By("creating a test alarm with major severity")

		majorAlert := alert.CreatePostable(alert.SeverityMajor, spoke1ClusterID)
		majorTracker, err := alert.SendToClient(alertsClient, majorAlert)
		Expect(err).ToNot(HaveOccurred(), "Failed to send major test alarm")

		By("creating a test alarm with minor severity")

		minorAlert := alert.CreatePostable(alert.SeverityMinor, spoke1ClusterID)
		minorTracker, err := alert.SendToClient(alertsClient, minorAlert)
		Expect(err).ToNot(HaveOccurred(), "Failed to send minor test alarm")

		By("waiting for the minor and major alarms to exist")

		_, err = helper.WaitForAlarmToExist(alarmsClient, map[string]string{"tracker": minorTracker}, time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for minor test alarm to exist")
		_, err = helper.WaitForAlarmToExist(alarmsClient, map[string]string{"tracker": majorTracker}, time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for major test alarm to exist")

		By("filtering alarms by major severity and resourceID")

		severityFilter := filter.Equals("perceivedSeverity", strconv.Itoa(int(oranapi.PerceivedSeverityMAJOR)))
		resourceFilter := filter.Equals("resourceID", spoke1ClusterID)
		combinedFilter := filter.And(severityFilter, resourceFilter)

		filteredAlarms, err := alarmsClient.ListAlarms(combinedFilter)
		Expect(err).ToNot(HaveOccurred(), "Failed to filter alarms")

		By("verifying the filtered results contain the major alarm but not the minor alarm")

		containsMajorAlarm := slices.ContainsFunc(filteredAlarms, func(alarm oranapi.AlarmEventRecord) bool {
			return alarm.Extensions["tracker"] == majorTracker
		})
		containsMinorAlarm := slices.ContainsFunc(filteredAlarms, func(alarm oranapi.AlarmEventRecord) bool {
			return alarm.Extensions["tracker"] == minorTracker
		})

		Expect(containsMajorAlarm).To(BeTrue(), "Major alarm should be found in filtered results")
		Expect(containsMinorAlarm).To(BeFalse(), "Minor alarm should not be found in filtered results")
	})

	// 83556 - Acknowledge an alarm and listen for notification
	It("acknowledges an alarm and listen for notification", reportxml.ID("83556"), func() {
		if usingAlarmsV2 {
			subscriptionID := uuid.New().String()
			subscription, err := alarmsV2Client.CreateSubscription(oranapi.AlarmSubscriptionInfoV2{
				ConsumerSubscriptionId: &subscriptionID,
				Callback:               mocksmo.ObserverCallbackURL(mockSMOBaseURL, subscriptionID),
			})
			Expect(err).ToNot(HaveOccurred(), "Failed to create subscription")
			Expect(subscription.AlarmSubscriptionId).ToNot(BeNil(), "Missing subscription ID")
			DeferCleanup(func() {
				Expect(alarmsV2Client.DeleteSubscription(*subscription.AlarmSubscriptionId)).
					To(Succeed(), "Failed to delete subscription")
			})

			start := time.Now()
			tracker, err := alert.SendToClient(alertsClient, alert.CreatePostable(alert.SeverityMajor, spoke1ClusterID))
			Expect(err).ToNot(HaveOccurred(), "Failed to send alert")
			alarm, err := helper.WaitForAlarmV2ToExist(alarmsV2Client, map[string]string{"tracker": tracker}, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to find alarm")

			waitNotification := func(eventType oranapi.AlarmEventNotificationTypeV2) {
				waitErr := mocksmo.WaitFor[oranapi.AlarmEventNotificationV2](HubAPIClient, RANConfig.MockSMONamespace,
					mocksmo.WithStart[oranapi.AlarmEventNotificationV2](start),
					mocksmo.WithObserverID[oranapi.AlarmEventNotificationV2](subscriptionID),
					mocksmo.WithMatch(func(notification *oranapi.AlarmEventNotificationV2) bool {
						return notification.Extensions["tracker"] == tracker &&
							notification.NotificationEventType == eventType &&
							notification.ObjectClass == "oran.o2ims.cluster.NodeCluster" &&
							notification.ObjectRef[true] == "/O2ims_infrastructureMonitoring/v2/alarms/"+
								alarm.AlarmEventRecordId.String()
					}),
				)
				Expect(waitErr).ToNot(HaveOccurred(), "Failed to receive expected notification")
			}
			waitNotification(oranapi.AlarmEventNotificationTypeV2NEW)

			_, err = alarmsV2Client.PatchAlarm(
				alarm.AlarmEventRecordId, oranapi.AlarmEventRecordModificationsV2{AlarmAcknowledged: new(true)})
			Expect(err).ToNot(HaveOccurred(), "Failed to acknowledge alarm")
			waitNotification(oranapi.AlarmEventNotificationTypeV2ACKNOWLEDGE)

			retrieved, err := alarmsV2Client.GetAlarm(alarm.AlarmEventRecordId)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve acknowledged alarm")
			Expect(retrieved.AlarmAcknowledged).To(BeTrue(), "Alarm was not acknowledged")
			Expect(retrieved.AlarmAcknowledgeTime).ToNot(BeNil(), "Acknowledgment time is missing")
			Expect(retrieved.ObjectClass).To(Equal("oran.o2ims.cluster.NodeCluster"), "Unexpected alarm object class")

			return
		}

		By("creating a test alarm")

		postableAlert := alert.CreatePostable(alert.SeverityMajor, spoke1ClusterID)
		tracker, err := alert.SendToClient(alertsClient, postableAlert)
		Expect(err).ToNot(HaveOccurred(), "Failed to send test alarm")

		By("waiting for the test alarm to exist")

		matchingAlarm, err := helper.WaitForAlarmToExist(alarmsClient, map[string]string{"tracker": tracker}, time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for test alarm to exist")

		alarmEventRecordID := matchingAlarm.AlarmEventRecordId

		By("creating a test subscription")

		subscriptionID := uuid.New()
		subscription, err := alarmsClient.CreateSubscription(oranapi.AlarmSubscriptionInfo{
			ConsumerSubscriptionId: &subscriptionID,
			Callback:               mocksmo.ObserverCallbackURL(mockSMOBaseURL, subscriptionID.String()),
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create test subscription")

		By("saving the time before acknowledging the alarm")

		timeBeforeAcknowledge := time.Now()

		By("acknowledging the alarm")

		_, err = alarmsClient.PatchAlarm(alarmEventRecordID, oranapi.AlarmEventRecordModifications{
			AlarmAcknowledged: ptr.To(true),
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to acknowledge alarm")

		By("waiting for the notification")

		err = mocksmo.WaitFor[oranapi.AlarmEventNotification](HubAPIClient, RANConfig.MockSMONamespace,
			mocksmo.WithStart[oranapi.AlarmEventNotification](timeBeforeAcknowledge),
			mocksmo.WithObserverID[oranapi.AlarmEventNotification](subscriptionID.String()),
			mocksmo.WithMatch(func(notification *oranapi.AlarmEventNotification) bool {
				return notification.Extensions["tracker"] == tracker &&
					notification.NotificationEventType == oranapi.AlarmEventNotificationTypeACKNOWLEDGE
			}),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to receive acknowledgment notification")

		By("deleting the test subscription")

		err = alarmsClient.DeleteSubscription(*subscription.AlarmSubscriptionId)
		Expect(err).ToNot(HaveOccurred(), "Failed to delete test subscription")
	})

	// 83557 - Filter alarm subscriptions from the API
	It("filters alarm subscriptions from the API", reportxml.ID("83557"), func() {
		if usingAlarmsV2 {
			By("creating a first test subscription")

			subscriptionID1 := uuid.New()
			subscription1, err := alarmsV2Client.CreateSubscription(oranapi.AlarmSubscriptionInfoV2{
				ConsumerSubscriptionId: new(subscriptionID1.String()),
				// Callback URLs must be unique, so we use the subscription ID as a suffix.
				Callback:    mocksmo.ObserverCallbackURL(mockSMOBaseURL, subscriptionID1.String()),
				EventFilter: nullable.NewNullableWithValue(oranapi.AlarmSubscriptionFilterV2NEW),
			})
			Expect(err).ToNot(HaveOccurred(), "Failed to create first test subscription")
			Expect(subscription1.AlarmSubscriptionId).ToNot(BeNil(), "Missing subscription ID")
			DeferCleanup(func() {
				Expect(alarmsV2Client.DeleteSubscription(*subscription1.AlarmSubscriptionId)).
					To(Succeed(), "Failed to delete subscription")
			})

			By("creating a second test subscription")

			subscriptionID2 := uuid.New()
			subscription2, err := alarmsV2Client.CreateSubscription(oranapi.AlarmSubscriptionInfoV2{
				ConsumerSubscriptionId: new(subscriptionID2.String()),
				// Callback URLs must be unique, so we use the subscription ID as a suffix.
				Callback:    mocksmo.ObserverCallbackURL(mockSMOBaseURL, subscriptionID2.String()),
				EventFilter: nullable.NewNullableWithValue(oranapi.AlarmSubscriptionFilterV2CHANGE),
			})
			Expect(err).ToNot(HaveOccurred(), "Failed to create second test subscription")
			Expect(subscription2.AlarmSubscriptionId).ToNot(BeNil(), "Missing subscription ID")
			DeferCleanup(func() {
				Expect(alarmsV2Client.DeleteSubscription(*subscription2.AlarmSubscriptionId)).
					To(Succeed(), "Failed to delete subscription")
			})

			By("filtering subscriptions by the first ConsumerSubscriptionId")

			consumerIDFilter := filter.Equals("consumerSubscriptionId", subscriptionID1.String())

			filteredSubscriptions, err := alarmsV2Client.ListSubscriptions(consumerIDFilter)
			Expect(err).ToNot(HaveOccurred(), "Failed to filter subscriptions")

			By("verifying the filtered results contain the first subscription but not the second")

			containsSubscription1 := slices.ContainsFunc(filteredSubscriptions,
				func(subscription oranapi.AlarmSubscriptionInfoV2) bool {
					return subscription.ConsumerSubscriptionId != nil &&
						*subscription.ConsumerSubscriptionId == subscriptionID1.String()
				})
			containsSubscription2 := slices.ContainsFunc(filteredSubscriptions,
				func(subscription oranapi.AlarmSubscriptionInfoV2) bool {
					return subscription.ConsumerSubscriptionId != nil &&
						*subscription.ConsumerSubscriptionId == subscriptionID2.String()
				})

			Expect(containsSubscription1).To(BeTrue(), "First subscription should be found in filtered results")
			Expect(containsSubscription2).To(BeFalse(), "Second subscription should not be found in filtered results")

			eventFilter := filter.Equals("eventFilter", "NEW")

			for _, query := range []filter.Filter{eventFilter, filter.And(eventFilter, consumerIDFilter)} {
				filtered, listErr := alarmsV2Client.ListSubscriptions(query)
				Expect(listErr).ToNot(HaveOccurred(), "Failed to filter subscriptions by eventFilter")
				Expect(slices.ContainsFunc(filtered, func(item oranapi.AlarmSubscriptionInfoV2) bool {
					return item.AlarmSubscriptionId != nil && *item.AlarmSubscriptionId == *subscription1.AlarmSubscriptionId
				})).To(BeTrue(), "Expected subscription missing from filtered results")

				for _, item := range filtered {
					Expect(item.EventFilter).
						To(Equal(nullable.NewNullableWithValue(oranapi.AlarmSubscriptionFilterV2NEW)), "Unexpected eventFilter")
				}
			}

			return
		}

		By("creating a first test subscription")

		subscriptionID1 := uuid.New()
		subscription1, err := alarmsClient.CreateSubscription(oranapi.AlarmSubscriptionInfo{
			ConsumerSubscriptionId: &subscriptionID1,
			// Callback URLs must be unique, so we use the subscription ID as a suffix.
			Callback: mocksmo.ObserverCallbackURL(mockSMOBaseURL, subscriptionID1.String()),
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create first test subscription")

		By("creating a second test subscription")

		subscriptionID2 := uuid.New()
		subscription2, err := alarmsClient.CreateSubscription(oranapi.AlarmSubscriptionInfo{
			ConsumerSubscriptionId: &subscriptionID2,
			// Callback URLs must be unique, so we use the subscription ID as a suffix.
			Callback: mocksmo.ObserverCallbackURL(mockSMOBaseURL, subscriptionID2.String()),
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create second test subscription")

		By("filtering subscriptions by the first ConsumerSubscriptionId")

		consumerIDFilter := filter.Equals("consumerSubscriptionId", subscriptionID1.String())

		filteredSubscriptions, err := alarmsClient.ListSubscriptions(consumerIDFilter)
		Expect(err).ToNot(HaveOccurred(), "Failed to filter subscriptions")

		By("verifying the filtered results contain the first subscription but not the second")

		containsSubscription1 := slices.ContainsFunc(filteredSubscriptions,
			func(subscription oranapi.AlarmSubscriptionInfo) bool {
				return subscription.ConsumerSubscriptionId.String() == subscriptionID1.String()
			})
		containsSubscription2 := slices.ContainsFunc(filteredSubscriptions,
			func(subscription oranapi.AlarmSubscriptionInfo) bool {
				return subscription.ConsumerSubscriptionId.String() == subscriptionID2.String()
			})

		Expect(containsSubscription1).To(BeTrue(), "First subscription should be found in filtered results")
		Expect(containsSubscription2).To(BeFalse(), "Second subscription should not be found in filtered results")

		By("deleting the test subscriptions")

		err = alarmsClient.DeleteSubscription(*subscription1.AlarmSubscriptionId)
		Expect(err).ToNot(HaveOccurred(), "Failed to delete first test subscription")

		err = alarmsClient.DeleteSubscription(*subscription2.AlarmSubscriptionId)
		Expect(err).ToNot(HaveOccurred(), "Failed to delete second test subscription")
	})

	// 83558 - Retrieve a subscription from the API
	It("retrieves a subscription from the API", reportxml.ID("83558"), func() {
		if usingAlarmsV2 {
			By("creating a test subscription")

			subscriptionID := uuid.New()
			subscription, err := alarmsV2Client.CreateSubscription(oranapi.AlarmSubscriptionInfoV2{
				ConsumerSubscriptionId: new(subscriptionID.String()),
				// Callback URLs must be unique, so we use the subscription ID as a suffix.
				Callback: mocksmo.ObserverCallbackURL(mockSMOBaseURL, subscriptionID.String()),
			})
			Expect(err).ToNot(HaveOccurred(), "Failed to create test subscription")

			By("retrieving the subscription from the API")

			retrievedSubscription, err := alarmsV2Client.GetSubscription(*subscription.AlarmSubscriptionId)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve test subscription")
			Expect(retrievedSubscription.ConsumerSubscriptionId).
				To(HaveValue(Equal(subscriptionID.String())), "Retrieved subscription should match the created one")

			By("listing all subscriptions")

			subscriptions, err := alarmsV2Client.ListSubscriptions()
			Expect(err).ToNot(HaveOccurred(), "Failed to list subscriptions")

			containsSubscription := slices.ContainsFunc(subscriptions, func(subscription oranapi.AlarmSubscriptionInfoV2) bool {
				return subscription.ConsumerSubscriptionId != nil && *subscription.ConsumerSubscriptionId == subscriptionID.String()
			})
			Expect(containsSubscription).To(BeTrue(), "Retrieved subscription should be in the list")

			By("deleting the test subscription")

			err = alarmsV2Client.DeleteSubscription(*subscription.AlarmSubscriptionId)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test subscription")
			deleted, err := alarmsV2Client.GetSubscriptionWithResponse(context.TODO(), *subscription.AlarmSubscriptionId)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve deleted subscription")
			Expect(deleted.StatusCode()).To(Equal(404), "Deleted subscription should not be found")

			return
		}

		By("creating a test subscription")

		subscriptionID := uuid.New()
		subscription, err := alarmsClient.CreateSubscription(oranapi.AlarmSubscriptionInfo{
			ConsumerSubscriptionId: &subscriptionID,
			// Callback URLs must be unique, so we use the subscription ID as a suffix.
			Callback: mocksmo.ObserverCallbackURL(mockSMOBaseURL, subscriptionID.String()),
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create test subscription")

		By("retrieving the subscription from the API")

		retrievedSubscription, err := alarmsClient.GetSubscription(*subscription.AlarmSubscriptionId)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve test subscription")
		Expect(retrievedSubscription.ConsumerSubscriptionId.String()).
			To(Equal(subscriptionID.String()), "Retrieved subscription should match the created one")

		By("listing all subscriptions")

		subscriptions, err := alarmsClient.ListSubscriptions()
		Expect(err).ToNot(HaveOccurred(), "Failed to list subscriptions")

		containsSubscription := slices.ContainsFunc(subscriptions, func(subscription oranapi.AlarmSubscriptionInfo) bool {
			return subscription.ConsumerSubscriptionId.String() == subscriptionID.String()
		})
		Expect(containsSubscription).To(BeTrue(), "Retrieved subscription should be in the list")

		By("deleting the test subscription")

		err = alarmsClient.DeleteSubscription(*subscription.AlarmSubscriptionId)
		Expect(err).ToNot(HaveOccurred(), "Failed to delete test subscription")
	})

	// 83559 - Update alarm service configuration
	It("updates alarm service configuration", reportxml.ID("83559"), func() {
		if usingAlarmsV2 {
			By("getting the current alarm service configuration")

			originalConfig, err := alarmsV2Client.GetServiceConfiguration()
			Expect(err).ToNot(HaveOccurred(), "Failed to get current alarm service configuration")

			Expect(originalConfig.Extensions).ToNot(BeNil(), "Configuration extensions should be an object")
			DeferCleanup(func() {
				_, restoreErr := alarmsV2Client.UpdateAlarmServiceConfiguration(originalConfig)
				Expect(restoreErr).ToNot(HaveOccurred(), "Failed to restore alarm service configuration")
			})

			originalRetentionPeriod := originalConfig.RetentionPeriod
			Expect(originalRetentionPeriod).To(BeNumerically(">=", 1), "Original retention period should be at least 1 day")

			By("patching to increment the retention period")

			patchConfig := oranapi.AlarmServiceConfigurationPatchV2{
				RetentionPeriod: ptr.To(originalRetentionPeriod + 1),
			}

			patchedConfig, err := alarmsV2Client.PatchAlarmServiceConfiguration(patchConfig)
			Expect(err).ToNot(HaveOccurred(), "Failed to patch alarm service configuration")

			By("verifying the retention period was incremented")
			Expect(patchedConfig.RetentionPeriod).To(Equal(originalRetentionPeriod+1),
				"Retention period should be incremented by 1")

			currentConfig, err := alarmsV2Client.GetServiceConfiguration()
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve patched configuration")
			Expect(currentConfig.RetentionPeriod).
				To(Equal(originalRetentionPeriod+1), "Patched retention period was not persisted")
			By("putting to decrement the retention period")

			putConfig := oranapi.AlarmServiceConfigurationV2{
				RetentionPeriod: originalRetentionPeriod,
				Extensions:      originalConfig.Extensions,
			}

			updatedConfig, err := alarmsV2Client.UpdateAlarmServiceConfiguration(putConfig)
			Expect(err).ToNot(HaveOccurred(), "Failed to update alarm service configuration")

			By("verifying the retention period matches the original")
			Expect(updatedConfig.RetentionPeriod).To(Equal(originalRetentionPeriod),
				"Retention period should match the original value")

			currentConfig, err = alarmsV2Client.GetServiceConfiguration()
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve restored configuration")
			Expect(currentConfig).To(Equal(originalConfig), "Restored configuration differs from original")

			return
		}

		By("getting the current alarm service configuration")

		originalConfig, err := alarmsClient.GetServiceConfiguration()
		Expect(err).ToNot(HaveOccurred(), "Failed to get current alarm service configuration")

		originalRetentionPeriod := originalConfig.RetentionPeriod
		Expect(originalRetentionPeriod).To(BeNumerically(">=", 1), "Original retention period should be at least 1 day")

		By("patching to increment the retention period")

		patchConfig := oranapi.AlarmServiceConfigurationPatch{
			RetentionPeriod: ptr.To(originalRetentionPeriod + 1),
		}

		patchedConfig, err := alarmsClient.PatchAlarmServiceConfiguration(patchConfig)
		Expect(err).ToNot(HaveOccurred(), "Failed to patch alarm service configuration")

		By("verifying the retention period was incremented")
		Expect(patchedConfig.RetentionPeriod).To(Equal(originalRetentionPeriod+1),
			"Retention period should be incremented by 1")

		By("putting to decrement the retention period")

		putConfig := oranapi.AlarmServiceConfiguration{
			RetentionPeriod: originalRetentionPeriod,
			Extensions:      originalConfig.Extensions,
		}

		updatedConfig, err := alarmsClient.UpdateAlarmServiceConfiguration(putConfig)
		Expect(err).ToNot(HaveOccurred(), "Failed to update alarm service configuration")

		By("verifying the retention period matches the original")
		Expect(updatedConfig.RetentionPeriod).To(Equal(originalRetentionPeriod),
			"Retention period should match the original value")
	})

	// 83561 - Ensure reliability of the alarms service
	It("ensures reliability of the alarms service", reportxml.ID("83561"), func() {
		if usingAlarmsV2 {
			By("creating a test subscription")

			subscriptionID := uuid.New()
			subscription, err := alarmsV2Client.CreateSubscription(oranapi.AlarmSubscriptionInfoV2{
				ConsumerSubscriptionId: new(subscriptionID.String()),
				Callback:               mocksmo.ObserverCallbackURL(mockSMOBaseURL, subscriptionID.String()),
			})
			Expect(err).ToNot(HaveOccurred(), "Failed to create test subscription")
			Expect(subscription.AlarmSubscriptionId).ToNot(BeNil(), "Missing subscription ID")
			DeferCleanup(func() {
				Expect(alarmsV2Client.DeleteSubscription(*subscription.AlarmSubscriptionId)).
					To(Succeed(), "Failed to delete subscription")
			})

			By("saving the time before sending alerts")

			timeBeforeSend := time.Now()

			By("sending alerts concurrently")

			sentAlerts := concurrentlySendAlerts(alertsClient, spoke1ClusterID, 100)
			expectedAlerts := maps.Clone(sentAlerts)

			By("waiting for all notifications to be received")
			// This process really can take a while, so the timeout is necessarily long.
			err = helper.WaitForAllNotificationsV2(
				HubAPIClient, RANConfig.MockSMONamespace, subscriptionID.String(), timeBeforeSend, sentAlerts, 20*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to receive all notifications")
			Expect(sentAlerts).To(BeEmpty(), "All alerts should have been received")

			persisted, err := alarmsV2Client.ListAlarms(filter.Equals("objectId", spoke1ClusterID))
			Expect(err).ToNot(HaveOccurred(), "Failed to list alarms for resource")

			for _, alarm := range persisted {
				if tracker, ok := alarm.Extensions["tracker"].(string); ok {
					delete(expectedAlerts, tracker)
				}
			}

			Expect(expectedAlerts).To(BeEmpty(), "Some sent alerts were not persisted")

			return
		}

		By("creating a test subscription")

		subscriptionID := uuid.New()
		subscription, err := alarmsClient.CreateSubscription(oranapi.AlarmSubscriptionInfo{
			ConsumerSubscriptionId: &subscriptionID,
			Callback:               mocksmo.ObserverCallbackURL(mockSMOBaseURL, subscriptionID.String()),
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create test subscription")

		By("saving the time before sending alerts")

		timeBeforeSend := time.Now()

		By("sending alerts concurrently")

		sentAlerts := concurrentlySendAlerts(alertsClient, spoke1ClusterID, 100)

		By("waiting for all notifications to be received")
		// This process really can take a while, so the timeout is necessarily long.
		err = helper.WaitForAllNotifications(
			HubAPIClient, RANConfig.MockSMONamespace, subscriptionID.String(), timeBeforeSend, sentAlerts, 20*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to receive all notifications")
		Expect(sentAlerts).To(BeEmpty(), "All alerts should have been received")

		By("deleting the test subscription")

		err = alarmsClient.DeleteSubscription(*subscription.AlarmSubscriptionId)
		Expect(err).ToNot(HaveOccurred(), "Failed to delete test subscription")
	})
})

// concurrentlySendAlerts sends a given number of alerts to the Alertmanager API for a given cluster ID concurrently.
// Each alert is sent in its own goroutine with retry logic.
//
// The function returns a map of tracker values to booleans, where the value is always true. It is guaranteed to be of
// length numAlerts, otherwise a Gomega assertion will fail.
func concurrentlySendAlerts(
	alertsClient *alertmanagerv2.AlertmanagerAPI, clusterID string, numAlerts uint) map[string]bool {
	// We create a buffered channel with space for every possible tracker so that we can read them back and create
	// the returned map synchronously.
	trackerChan := make(chan string, numAlerts)
	waitGroup := sync.WaitGroup{}

	for range numAlerts {
		waitGroup.Go(func() {
			// Allow up to 3 attempts to send the alert. This is to give some leeway in case of network
			// faults, although in ideal conditions there should be no retries.
			for attempt := range 3 {
				tracker, err := alert.SendToClient(alertsClient, alert.CreatePostable(alert.SeverityMajor, clusterID))
				if err == nil {
					trackerChan <- tracker

					return
				}

				klog.V(tsparams.LogLevel).Infof("Failed to send alert (attempt %d/3): %v", attempt+1, err)
			}

			klog.V(tsparams.LogLevel).Infof("Failed to send alert after 3 attempts")
		})
	}

	waitGroup.Wait()
	close(trackerChan)

	alerts := make(map[string]bool, numAlerts)
	for tracker := range trackerChan {
		alerts[tracker] = true
	}

	Expect(len(alerts)).To(Equal(int(numAlerts)), "Failed to send all alerts, retries exhausted")

	return alerts
}
