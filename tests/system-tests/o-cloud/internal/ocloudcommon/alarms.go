package ocloudcommon

import (
	"crypto/tls"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/siteconfig"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/shell"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudparams"
)

// Constants for alarm testing.
const (
	// Alarm testing constants.
	ACMPolicyViolationAlertName = "ACMPolicyViolationDetected"
	DefaultRetentionPeriod      = 1
	RetentionPeriodHours        = 23
	AlarmWaitTime               = 2 * time.Minute
	RetentionCheckInterval      = 1 * time.Hour
	FinalWaitTime               = 10 * time.Minute
	ExpectedAlarmCount          = 3

	// O2IMS API constants.
	TokenDuration = "24h"
)

// Shared variables for cluster deprovisioning.
var (
	sharedProvisioningRequest *oran.ProvisioningRequestBuilder
	sharedClusterInstance     *siteconfig.CIBuilder
)

// CreateO2IMSClient creates an O2IMS API client using token authentication and returns it.
func CreateO2IMSClient() *oranapi.AlarmsClient {
	By("creating the O2IMS API client")

	token, err := shell.ExecuteCmd(
		fmt.Sprintf("oc create token -n oran-o2ims test-client --duration=%s", TokenDuration))
	Expect(err).ToNot(HaveOccurred(), "Failed to create token for O2IMS API")

	clientBuilder := oranapi.NewClientBuilder(OCloudConfig.O2IMSBaseURL).
		WithToken(string(token)).
		WithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true})

	alarmsClient, err := clientBuilder.BuildAlarms()
	Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client")

	return alarmsClient
}

// FilterAlarmsByExtensions filters alarms by extension field values.
func FilterAlarmsByExtensions(
	client *oranapi.AlarmsClient, extensionFilters map[string]string) []oranapi.AlarmEventRecord {
	alarms, err := client.ListAlarms()
	Expect(err).ToNot(HaveOccurred(), "failed to list alarms")

	var filteredAlarms []oranapi.AlarmEventRecord

	for _, alarm := range alarms {
		if matchesExtensions(alarm.Extensions, extensionFilters) {
			filteredAlarms = append(filteredAlarms, alarm)
		}
	}

	return filteredAlarms
}

// matchesExtensions checks if alarm extensions match all the specified filter criteria.
func matchesExtensions(extensions map[string]string, filters map[string]string) bool {
	for field, expectedValue := range filters {
		if actualValue, exists := extensions[field]; !exists || actualValue != expectedValue {
			return false
		}
	}

	return true
}

// CreateAlarmSubscription creates a new alarm subscription and returns the subscription info.
func CreateAlarmSubscription(alarmsClient *oranapi.AlarmsClient) oranapi.AlarmSubscriptionInfo {
	By("creating a new subscription")

	subscriptionID := uuid.New()

	subscription, err := alarmsClient.CreateSubscription(oranapi.AlarmSubscriptionInfo{
		ConsumerSubscriptionId: &subscriptionID,
		Callback:               OCloudConfig.SubscriberURL + "/" + subscriptionID.String(),
	})
	Expect(err).ToNot(HaveOccurred(), "Failed to create a new subscription")

	return subscription
}

// ModifyPTPOperatorResources modifies PTP operator deployment resources to trigger or stop alarms.
func ModifyPTPOperatorResources(snoAPIClient *clients.Settings, triggerAlarm bool) {
	var cpuRequest, memoryRequest, cpuLimit, memoryLimit string

	if triggerAlarm {
		// Use values that will trigger policy violations
		cpuRequest = ocloudparams.PtpCPURequest
		memoryRequest = ocloudparams.PtpMemoryRequest
		cpuLimit = ocloudparams.PtpCPULimit
		memoryLimit = ocloudparams.PtpMemoryLimit
	} else {
		// Use values that will make policies compliant
		cpuLimit = ocloudparams.PtpCPULimit
		memoryLimit = ocloudparams.PtpMemoryLimit
		cpuRequest = ocloudparams.PtpCPURequest
		memoryRequest = ocloudparams.PtpMemoryRequest
	}

	modifyPTPDeploymentResources(
		snoAPIClient,
		cpuRequest,
		memoryRequest,
		cpuLimit,
		memoryLimit)
}

// GetACMPolicyViolationAlarms retrieves alarms filtered by ACMPolicyViolationDetected alertname.
func GetACMPolicyViolationAlarms(alarmsClient *oranapi.AlarmsClient) []oranapi.AlarmEventRecord {
	extensionFilters := map[string]string{
		"alertname": ACMPolicyViolationAlertName,
	}

	return FilterAlarmsByExtensions(alarmsClient, extensionFilters)
}

// VerifyAlarmCount verifies that the expected number of alarms exist.
func VerifyAlarmCount(alarms []oranapi.AlarmEventRecord, expectedCount int, message string) {
	Expect(len(alarms)).To(Equal(expectedCount), message, len(alarms))
}

// VerifyMinimumAlarmCount verifies that at least the minimum number of alarms exist.
func VerifyMinimumAlarmCount(alarms []oranapi.AlarmEventRecord, minCount int, message string) {
	Expect(len(alarms) >= minCount).To(BeTrue(), message, len(alarms))
}

// CleanupAlarmSubscription deletes the alarm subscription with proper error handling.
func CleanupAlarmSubscription(alarmsClient *oranapi.AlarmsClient, subscription oranapi.AlarmSubscriptionInfo) {
	By("deleting the test subscriptions")

	err := alarmsClient.DeleteSubscription(*subscription.AlarmSubscriptionId)
	Expect(err).ToNot(HaveOccurred(), "Failed to delete test subscription")
}

// VerifySuccessfulAlarmRetrieval verifies the test case of the successful retrieval of an alarm from the API.
func VerifySuccessfulAlarmRetrieval(ctx SpecContext) {
	By("verifying that the BMHs are available")

	VerifyBmhIsAvailable(OCloudConfig.BmhSpoke1, OCloudConfig.InventoryPoolNamespace)
	VerifyBmhIsAvailable(OCloudConfig.BmhSpoke2, OCloudConfig.InventoryPoolNamespace)

	By("provisioning a SNO cluster")

	provisioningRequest := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionAISuccess,
		OCloudConfig.NodeClusterName1,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters1)

	VerifyOcloudCRsExist(provisioningRequest)

	clusterInstance := VerifyClusterInstanceCompleted(provisioningRequest, ctx)
	nsname := provisioningRequest.Object.Status.Extensions.ClusterDetails.Name

	VerifyAllPoliciesInNamespaceAreCompliant(nsname, ctx, nil, nil)
	glog.V(ocloudparams.OCloudLogLevel).Infof("all the policies in namespace %s are compliant", nsname)

	VerifyProvisioningRequestIsFulfilled(provisioningRequest)
	glog.V(ocloudparams.OCloudLogLevel).Infof("provisioning request %s is fulfilled", provisioningRequest.Object.Name)

	alarmsClient := CreateO2IMSClient()
	subscription := CreateAlarmSubscription(alarmsClient)

	By("modifying the PTP operator deployment resources to trigger an alarm")

	snoAPIClient := CreateSnoAPIClient(OCloudConfig.ClusterName1)

	VerifyAllPodsRunningInNamespace(snoAPIClient, ocloudparams.PtpNamespace)

	ModifyPTPOperatorResources(snoAPIClient, true)

	VerifyPoliciesAreNotCompliant(OCloudConfig.ClusterName1, ctx, nil, nil)

	time.Sleep(AlarmWaitTime)

	By("filtering alarms by alertname")

	filteredAlarms := GetACMPolicyViolationAlarms(alarmsClient)
	VerifyMinimumAlarmCount(filteredAlarms, 1, "No alarms found with alertname: %s, found %d")

	for _, alarm := range filteredAlarms {
		By(fmt.Sprintf("verifying the retrieval of the alarm with the alarm event record id: %v", alarm.AlarmEventRecordId))
		_, err := alarmsClient.GetAlarm(alarm.AlarmEventRecordId)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to retrieve alarm with the alarm event record id: %v", alarm.AlarmEventRecordId))
	}

	By("modifying the PTP operator to stop triggering the alarm")
	ModifyPTPOperatorResources(snoAPIClient, false)

	VerifyAllPoliciesInNamespaceAreCompliant(OCloudConfig.ClusterName1, ctx, nil, nil)

	CleanupAlarmSubscription(alarmsClient, subscription)

	sharedProvisioningRequest = provisioningRequest
	sharedClusterInstance = clusterInstance
}

// VerifySuccessfulAlarmsCleanup verifies the test case where the alarms from the database are
// cleaned up after the retention period.
func VerifySuccessfulAlarmsCleanup(ctx SpecContext) {
	By("patching the alarm service configuration to set the retention period to 1 day")

	alarmsClient := CreateO2IMSClient()

	subscription := CreateAlarmSubscription(alarmsClient)

	snoAPIClient := CreateSnoAPIClient(OCloudConfig.ClusterName1)

	patchConfig := oranapi.AlarmServiceConfiguration{
		RetentionPeriod: DefaultRetentionPeriod,
	}
	patchedConfig, err := alarmsClient.PatchAlarmServiceConfiguration(patchConfig)
	Expect(err).ToNot(HaveOccurred(), "Failed to patch alarm service configuration")
	Expect(patchedConfig.RetentionPeriod).To(Equal(1), "Retention period should be 1")

	for iteration := 0; iteration < ExpectedAlarmCount; iteration++ {
		VerifyAllPodsRunningInNamespace(snoAPIClient, ocloudparams.PtpNamespace)

		By(fmt.Sprintf("modifying the PTP operator deployment resources to trigger an alarm iteration %d", iteration))
		ModifyPTPOperatorResources(snoAPIClient, true)

		VerifyPoliciesAreNotCompliant(OCloudConfig.ClusterName1, ctx, nil, nil)

		time.Sleep(AlarmWaitTime)

		By(fmt.Sprintf("modifying the PTP operator to stop triggering the alarms iteration %d", iteration))
		ModifyPTPOperatorResources(snoAPIClient, false)

		VerifyAllPoliciesInNamespaceAreCompliant(OCloudConfig.ClusterName1, ctx, nil, nil)

		time.Sleep(AlarmWaitTime)
	}

	By("filtering alarms by alertname to get the final number of alarms")

	filteredAlarms := GetACMPolicyViolationAlarms(alarmsClient)

	VerifyMinimumAlarmCount(
		filteredAlarms, ExpectedAlarmCount, "at least %d alarms should exist with alertname: %s, found %d")

	startTime := time.Now()
	retentionPeriod := RetentionPeriodHours * time.Hour

	for time.Since(startTime) < retentionPeriod {
		filteredAlarms := GetACMPolicyViolationAlarms(alarmsClient)
		VerifyMinimumAlarmCount(
			filteredAlarms, 1, "Alarms should still exist during retention period (elapsed: %v), found %d")

		time.Sleep(RetentionCheckInterval)
	}

	time.Sleep(FinalWaitTime)

	finalFilteredAlarms := GetACMPolicyViolationAlarms(alarmsClient)
	VerifyAlarmCount(
		finalFilteredAlarms, 0, "No alarms should be found with alertname %s after retention period, found %d")

	CleanupAlarmSubscription(alarmsClient, subscription)

	By("deprovisioning the SNO cluster")
	DeprovisionAiSnoCluster(sharedProvisioningRequest, sharedClusterInstance, ctx, nil)
}
