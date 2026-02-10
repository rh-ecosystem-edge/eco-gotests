package tests

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	eventptp "github.com/redhat-cne/sdk-go/pkg/event/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/consumer"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/eventmetric"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/events"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

var _ = Describe("PTP Event Consumer", Label(tsparams.LabelEventConsumer), func() {
	var (
		prometheusAPI   prometheusv1.API
		savedPtpConfigs []*ptp.PtpConfigBuilder
	)

	BeforeEach(func() {
		var err error

		By("creating a Prometheus API client")
		prometheusAPI, err = querier.CreatePrometheusAPIForCluster(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Prometheus API client")

		By("ensuring clocks are locked before testing")
		err = metrics.EnsureClocksAreLocked(prometheusAPI)
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")

		By("saving PtpConfigs before testing")
		savedPtpConfigs, err = profiles.SavePtpConfigs(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to save PtpConfigs")
	})

	AfterEach(func() {
		By("restoring PtpConfigs after testing")
		startTime := time.Now()
		changedProfiles, err := profiles.RestorePtpConfigs(RANConfig.Spoke1APIClient, savedPtpConfigs)
		Expect(err).ToNot(HaveOccurred(), "Failed to restore PtpConfigs")

		if len(changedProfiles) > 0 {
			By("waiting for profile load on nodes")
			err := ptpdaemon.WaitForProfileLoadOnPTPNodes(RANConfig.Spoke1APIClient,
				ptpdaemon.WithStartTime(startTime),
				ptpdaemon.WithTimeout(5*time.Minute))
			if err != nil {
				// Timeouts may occur if the profiles changed do not apply to all PTP nodes, so we make
				// this non-fatal. This only happens in certain scenarios in MNO clusters.
				klog.V(tsparams.LogLevel).Infof("Failed to wait for profile load on PTP nodes: %v", err)
			}
		}

		By("ensuring clocks are locked after testing")
		err = metrics.EnsureClocksAreLocked(prometheusAPI)
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")
	})

	// 64775 - Validate System is restored after POD restart/deletion
	It("should recover to stable state after delete PTP daemon pod", reportxml.ID("64775"), func() {
		testRanAtLeastOnce := false

		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for _, nodeInfo := range nodeInfoMap {
			testRanAtLeastOnce = true

			By("getting the PTP daemon pod for node " + nodeInfo.Name)
			ptpDaemonPod, err := ptpdaemon.GetPtpDaemonPodOnNode(RANConfig.Spoke1APIClient, nodeInfo.Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon pod for node %s", nodeInfo.Name)

			By("deleting the PTP daemon pod and waiting until it is deleted")
			startTime := time.Now()
			_, err = ptpDaemonPod.DeleteAndWait(5 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete PTP daemon pod for node %s", nodeInfo.Name)

			By("waiting for the PTP daemonset to be ready again")
			ptpDaemonset, err := daemonset.Pull(
				RANConfig.Spoke1APIClient, ranparam.LinuxPtpDaemonsetName, ranparam.PtpOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull PTP daemon set")

			ready := ptpDaemonset.IsReady(5 * time.Minute)
			Expect(ready).To(BeTrue(), "Failed to wait for PTP daemon set to be ready")

			By("waiting up to 10 minutes for metrics to be locked")
			query := metrics.ClockStateQuery{
				Process: metrics.DoesNotEqual(metrics.ProcessChronyd),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, query, metrics.ClockStateLocked,
				metrics.AssertWithStableDuration(10*time.Second),
				metrics.AssertWithTimeout(10*time.Minute))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked and stable after pod restart")

			By("waiting up to 10 minutes since startTime for the locked event and metric on the node")
			lockedFilter := events.All(
				events.IsType(eventptp.PtpStateChange),
				events.HasValue(events.WithSyncState(eventptp.LOCKED)),
			)
			err = eventmetric.NewAssertion(prometheusAPI,
				metrics.ClockStateQuery{Process: metrics.DoesNotEqual(metrics.ProcessChronyd)},
				metrics.ClockStateLocked, lockedFilter).
				ForNode(RANConfig.Spoke1APIClient, nodeInfo.Name).
				WithStartTime(startTime).
				WithTimeout(10 * time.Minute).
				WithMetricOptions(metrics.AssertWithStableDuration(10 * time.Second)).
				ExecuteAssertion(context.TODO())
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for locked event on node %s", nodeInfo.Name)
		}

		if !testRanAtLeastOnce {
			Skip("No nodes found to run the test on")
		}
	})

	// 82218 - Validates the consumer events after ptpoperatorconfig api version is modified
	It("validates the consumer events after ptpoperatorconfig api version is modified", reportxml.ID("82218"), func() {
		By("checking if events are enabled")
		eventsEnabled, err := consumer.AreEventsEnabled(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to check if events are enabled")

		if !eventsEnabled {
			Skip("Events are not enabled, skipping event consumer API version test")
		}

		By("checking if the PTP version is within the 4.16-4.18 range")
		inRange, err := version.IsVersionStringInRange(RANConfig.Spoke1OperatorVersions[ranparam.PTP], "4.16", "4.18")
		Expect(err).ToNot(HaveOccurred(), "Failed to check PTP version range")

		if !inRange {
			Skip("PTP version is not within the 4.16-4.18 range, skipping test")
		}

		By("cleaning up all consumers")
		err = consumer.CleanupConsumersOnNodes(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to cleanup consumers on nodes")

		By("retrieving the current API version from the PTP Operator Config")
		ptpOperatorConfig, err := ptp.PullPtpOperatorConfig(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull PTP operator config")

		originalAPIVersion := ptpOperatorConfig.Definition.Spec.EventConfig.ApiVersion

		DeferCleanup(func() {
			// If the test succeeded, this cleanup is not needed.
			if !CurrentSpecReport().Failed() {
				return
			}

			By("restoring the original PTP Operator Config after failure")
			ptpOperatorConfig.Definition.Spec.EventConfig.ApiVersion = originalAPIVersion
			_, err = ptpOperatorConfig.Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to restore original PTP operator config")

			By("redeploying all the consumers again after failure")
			err = consumer.DeployConsumersOnNodes(RANConfig.Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to redeploy consumers on nodes")
		})

		By("modifying the ptpEventConfig.apiVersion field in the PTP Operator Config")
		var newAPIVersion string
		if originalAPIVersion == "2.0" {
			newAPIVersion = "1.0"
		} else {
			newAPIVersion = "2.0"
		}

		ptpOperatorConfig.Definition.Spec.EventConfig.ApiVersion = newAPIVersion
		ptpOperatorConfig, err = ptpOperatorConfig.Update()
		Expect(err).ToNot(HaveOccurred(), "Failed to update PTP operator config with new API version")

		By("waiting for the changes to propagate")
		time.Sleep(1 * time.Minute)

		By("verifying that all PTP clocks are in a LOCKED state")
		err = metrics.AssertQuery(context.TODO(), prometheusAPI, metrics.ClockStateQuery{}, metrics.ClockStateLocked,
			metrics.AssertWithStableDuration(1*time.Minute),
			metrics.AssertWithTimeout(10*time.Minute))
		Expect(err).ToNot(HaveOccurred(), "Failed to assert all clocks are locked after API version change")

		By("redeploying all the consumers")
		redeploymentTime := time.Now()
		err = consumer.DeployConsumersOnNodes(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to deploy consumers on nodes")

		By("verifying that we see a PtpStateChange to LOCKED containing iface.Master")
		verifyPTPLockedEventOnNodes(RANConfig.Spoke1APIClient, redeploymentTime)

		By("cleaning up all consumers")
		err = consumer.CleanupConsumersOnNodes(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to cleanup consumers on nodes")

		By("restoring the original PTP Operator Config")
		ptpOperatorConfig.Definition.Spec.EventConfig.ApiVersion = originalAPIVersion
		_, err = ptpOperatorConfig.Update()
		Expect(err).ToNot(HaveOccurred(), "Failed to restore original PTP operator config")

		By("waiting for the PTP clocks to return to a LOCKED state")
		err = metrics.AssertQuery(context.TODO(), prometheusAPI, metrics.ClockStateQuery{}, metrics.ClockStateLocked,
			metrics.AssertWithStableDuration(1*time.Minute),
			metrics.AssertWithTimeout(10*time.Minute))
		Expect(err).ToNot(HaveOccurred(), "Failed to assert all clocks are locked after restoring original config")

		By("redeploying all the consumers again")
		redeploymentTime = time.Now()
		err = consumer.DeployConsumersOnNodes(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to redeploy consumers on nodes")

		By("verifying that we see a PtpStateChange to LOCKED containing iface.Master again")
		verifyPTPLockedEventOnNodes(RANConfig.Spoke1APIClient, redeploymentTime)
	})
})

func verifyPTPLockedEventOnNodes(client *clients.Settings, startTime time.Time) {
	consumerPods, err := consumer.ListConsumerPods(client)
	Expect(err).ToNot(HaveOccurred(), "Failed to list consumer pods")

	for _, consumerPod := range consumerPods {
		nodeName := consumerPod.Definition.Spec.NodeName

		By("waiting for ptp-state-change LOCKED event on node " + nodeName)

		filter := events.All(
			events.IsType(eventptp.PtpStateChange),
			events.HasValue(events.WithSyncState(eventptp.LOCKED), events.ContainingResource(string(iface.Master))),
		)
		err = events.WaitForEvent(consumerPod, startTime, 5*time.Minute, filter)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for locked event with iface.Master on node %s", nodeName)
	}
}
