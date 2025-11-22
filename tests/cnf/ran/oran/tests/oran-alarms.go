package tests

import (
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	alertmanagerv2 "github.com/prometheus/alertmanager/api/v2/client"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/filter"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/alerter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/alert"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/auth"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	subscriber "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/oran-subscriber"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

// subscriberURL is the URL of the subscriber, including the scheme. It should not be modified.
var subscriberURL = "https://" + RANConfig.GetAppsURL(tsparams.SubscriberSubdomain)

var _ = Describe("ORAN Alarms Tests", Label(tsparams.LabelPostProvision, tsparams.LabelAlarms), func() {
	var (
		alarmsClient    *oranapi.AlarmsClient
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

		By("creating the Alertmanager API client")
		alertsClient, err = alerter.CreateAlerterClientForCluster(HubAPIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create the Alertmanager API client")

		By("getting the spoke 1 cluster ID")
		spoke1ClusterID, err = rancluster.GetManagedClusterID(HubAPIClient, RANConfig.Spoke1Name)
		Expect(err).ToNot(HaveOccurred(), "Failed to get spoke 1 cluster ID")
	})

	// 83554 - Retrieve an alarm from the API
	It("retrieves an alarm from the API", reportxml.ID("83554"), func() {
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
			Callback:               subscriberURL + "/" + subscriptionID.String(),
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
		err = subscriber.WaitForNotification(HubAPIClient, tsparams.SubscriberNamespace,
			subscriber.WithStart(timeBeforeAcknowledge),
			subscriber.WithMatchFunc(func(notification *oranapi.AlarmEventNotification) bool {
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
		By("creating a first test subscription")
		subscriptionID1 := uuid.New()
		subscription1, err := alarmsClient.CreateSubscription(oranapi.AlarmSubscriptionInfo{
			ConsumerSubscriptionId: &subscriptionID1,
			// Callback URLs must be unique, so we use the subscription ID as a suffix.
			Callback: subscriberURL + "/" + subscriptionID1.String(),
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create first test subscription")

		By("creating a second test subscription")
		subscriptionID2 := uuid.New()
		subscription2, err := alarmsClient.CreateSubscription(oranapi.AlarmSubscriptionInfo{
			ConsumerSubscriptionId: &subscriptionID2,
			// Callback URLs must be unique, so we use the subscription ID as a suffix.
			Callback: subscriberURL + "/" + subscriptionID2.String(),
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
		By("creating a test subscription")
		subscriptionID := uuid.New()
		subscription, err := alarmsClient.CreateSubscription(oranapi.AlarmSubscriptionInfo{
			ConsumerSubscriptionId: &subscriptionID,
			// Callback URLs must be unique, so we use the subscription ID as a suffix.
			Callback: subscriberURL + "/" + subscriptionID.String(),
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
		By("getting the current alarm service configuration")
		originalConfig, err := alarmsClient.GetServiceConfiguration()
		Expect(err).ToNot(HaveOccurred(), "Failed to get current alarm service configuration")

		originalRetentionPeriod := originalConfig.RetentionPeriod
		Expect(originalRetentionPeriod).To(BeNumerically(">=", 1), "Original retention period should be at least 1 day")

		By("patching to increment the retention period")
		patchConfig := oranapi.AlarmServiceConfiguration{
			RetentionPeriod: originalRetentionPeriod + 1,
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
		By("creating a test subscription")
		subscriptionID := uuid.New()
		subscription, err := alarmsClient.CreateSubscription(oranapi.AlarmSubscriptionInfo{
			ConsumerSubscriptionId: &subscriptionID,
			Callback:               subscriberURL + "/" + subscriptionID.String(),
			Filter:                 ptr.To(oranapi.AlarmSubscriptionFilterNEW),
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create test subscription")

		By("saving the time before sending alerts")
		timeBeforeSend := time.Now()

		By("sending alerts concurrently")
		sentAlerts := concurrentlySendAlerts(alertsClient, spoke1ClusterID, 100)

		By("waiting for all notifications to be received")
		// This process really can take a while, so the timeout is necessarily long.
		err = helper.WaitForAllNotifications(
			HubAPIClient, tsparams.SubscriberNamespace, timeBeforeSend, sentAlerts, 20*time.Minute)
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
