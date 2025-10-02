package tests

import (
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/golang/glog"
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
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	subscriber "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/oran-subscriber"
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

		By("waiting 5 seconds to ensure the alarm propagates")
		time.Sleep(5 * time.Second)

		By("listing alarms to find the test alarm")
		alarms, err := alarmsClient.ListAlarms()
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve alarms")

		alarmIndex := slices.IndexFunc(alarms, func(alarm oranapi.AlarmEventRecord) bool {
			return alarm.Extensions["tracker"] == tracker
		})
		Expect(alarmIndex).ToNot(Equal(-1), "Failed to find test alarm")

		alarmEventRecordID := alarms[alarmIndex].AlarmEventRecordId

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

		By("waiting 5 seconds to ensure the alarm propagates")
		time.Sleep(5 * time.Second)

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

		By("waiting 5 seconds to ensure the alarm propagates")
		time.Sleep(5 * time.Second)

		By("listing alarms to find the test alarm")
		alarms, err := alarmsClient.ListAlarms()
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve alarms")

		alarmIndex := slices.IndexFunc(alarms, func(alarm oranapi.AlarmEventRecord) bool {
			return alarm.Extensions["tracker"] == tracker
		})
		Expect(alarmIndex).ToNot(Equal(-1), "Failed to find test alarm")

		alarmEventRecordID := alarms[alarmIndex].AlarmEventRecordId

		By("creating a test subscription")
		subscriptionID := uuid.New()
		subscription, err := alarmsClient.CreateSubscription(oranapi.AlarmSubscriptionInfo{
			ConsumerSubscriptionId: &subscriptionID,
			Callback:               subscriberURL + "/" + subscriptionID.String(),
			Filter:                 ptr.To(oranapi.AlarmSubscriptionFilterACKNOWLEDGE),
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
		// 16 alerts per sender times 8 senders equals 128 alerts.
		sentAlerts := concurrentlySendAlerts(alertsClient, spoke1ClusterID, 16)

		By("waiting 30 seconds to allow alerts to be received")
		time.Sleep(30 * time.Second)

		By("getting notifications from the subscriber")
		receivedNotifications, err := subscriber.ListReceivedNotifications(
			HubAPIClient, tsparams.SubscriberNamespace, timeBeforeSend)
		Expect(err).ToNot(HaveOccurred(), "Failed to get subscriber notifications")

		By("verifying the subscriber received all alerts")
		for _, notification := range receivedNotifications {
			if tracker, ok := notification.Extensions["tracker"]; ok {
				delete(sentAlerts, tracker)
			}
		}

		Expect(sentAlerts).To(BeEmpty(), "Not all sent alerts were received")

		By("deleting the test subscription")
		err = alarmsClient.DeleteSubscription(*subscription.AlarmSubscriptionId)
		Expect(err).ToNot(HaveOccurred(), "Failed to delete test subscription")
	})
})

// concurrentlySendAlerts sends a given number of alerts to the Alertmanager API for a given cluster ID concurrently. It
// utilises 8 different goroutines to send the alerts, so numPerSender is the number of alerts to send per sender. The
// total alerts sent is numPerSender * 8.
//
// The function returns a map of tracker values to booleans, where the value is always true. It is guaranteed to be of
// length numPerSender * 8, otherwise a Gomega assertion will fail.
func concurrentlySendAlerts(
	alertsClient *alertmanagerv2.AlertmanagerAPI, clusterID string, numPerSender uint) map[string]bool {
	trackerChan := make(chan string, 8*int(numPerSender))
	waitGroup := sync.WaitGroup{}

	for range 8 {
		waitGroup.Go(func() {
			// We allow up to 8 retries per sender. This is to give some leeway in case of network faults,
			// although in ideal conditions there should be no retries. If the retries are exhausted, the
			// function returns. This causes too few alerts to be sent, which is checked by an assertion.
			retriesLeft := 8

			for sent := 0; sent < int(numPerSender); {
				tracker, err := alert.SendToClient(alertsClient, alert.CreatePostable(alert.SeverityMajor, clusterID))
				if err != nil {
					glog.V(tsparams.LogLevel).Infof("Failed to send alert: %v", err)

					retriesLeft--
					if retriesLeft <= 0 {
						return
					}

					continue
				}

				sent++

				trackerChan <- tracker
			}
		})
	}

	waitGroup.Wait()
	close(trackerChan)

	alerts := make(map[string]bool)
	for tracker := range trackerChan {
		alerts[tracker] = true
	}

	Expect(len(alerts)).To(Equal(8*int(numPerSender)), "Failed to send all alerts, retries exhausted")

	return alerts
}
