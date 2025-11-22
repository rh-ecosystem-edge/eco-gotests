package tests

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	eventptp "github.com/redhat-cne/sdk-go/pkg/event/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/consumer"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/events"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

var _ = Describe("PTP Events and Metrics", Label(tsparams.LabelEventsAndMetrics), func() {
	var prometheusAPI prometheusv1.API

	BeforeEach(func() {
		By("creating a Prometheus API client")
		var err error
		prometheusAPI, err = querier.CreatePrometheusAPIForCluster(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Prometheus API client")

		By("ensuring clocks are locked before testing")
		err = metrics.AssertQuery(context.TODO(), prometheusAPI, metrics.ClockStateQuery{}, metrics.ClockStateLocked,
			metrics.AssertWithStableDuration(10*time.Second),
			metrics.AssertWithTimeout(5*time.Minute))
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")
	})

	AfterEach(func() {
		By("ensuring clocks are locked after testing")
		err := metrics.AssertQuery(context.TODO(), prometheusAPI, metrics.ClockStateQuery{}, metrics.ClockStateLocked,
			metrics.AssertWithStableDuration(10*time.Second),
			metrics.AssertWithTimeout(5*time.Minute))
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")
	})

	// 82480 - Validating [LOCKED] clock state in PTP metrics
	It("verifies all clocks are LOCKED", reportxml.ID("82480"), func() {
		By("ensuring all clocks on all nodes are LOCKED")
		err := metrics.AssertQuery(context.TODO(), prometheusAPI, metrics.ClockStateQuery{}, metrics.ClockStateLocked,
			metrics.AssertWithStableDuration(10*time.Second),
			metrics.AssertWithTimeout(5*time.Minute))
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked after 5 minutes")
	})

	// 66848 - Validate stability of the system before and after system manipulations
	It("verifies phc2sys and ptp4l processes are UP", reportxml.ID("66848"), func() {
		By("ensuring all phc2sys and ptp4l processes are in 'UP' state")
		query := metrics.ProcessStatusQuery{Process: metrics.Includes(metrics.ProcessPHC2SYS, metrics.ProcessPTP4L)}
		err := metrics.AssertQuery(context.TODO(), prometheusAPI, query, metrics.ProcessStatusUp,
			metrics.AssertWithTimeout(5*time.Minute))
		Expect(err).ToNot(HaveOccurred(), "Failed to assert process status is 'UP' after 5 minutes")
	})

	// 49741 - Change Offset Thresholds to min, max
	It("verifies FREERUN event received after adding a PHC offset", reportxml.ID("49741"), func() {
		testRanAtLeastOnce := false

		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for _, nodeInfo := range nodeInfoMap {
			By("checking client interfaces on node " + nodeInfo.Name)
			clientInterfaces := nodeInfo.GetInterfacesByClockType(profiles.ClockTypeClient)
			if len(clientInterfaces) == 0 {
				klog.V(tsparams.LogLevel).Infof("No client interfaces found for node %s", nodeInfo.Name)

				continue
			}

			testRanAtLeastOnce = true
			ifaceGroups := iface.GroupInterfacesByNIC(profiles.GetInterfacesNames(clientInterfaces))

			By("getting the event pod for the node")
			eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeInfo.Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to get event pod for node %s", nodeInfo.Name)

			for nic, ifaces := range ifaceGroups {
				startTime := time.Now()

				By("adjusting the PHC by 5 ms for NIC " + string(nic))
				err := iface.AdjustPTPHardwareClock(RANConfig.Spoke1APIClient, nodeInfo.Name, ifaces[0], 0.005)
				Expect(err).ToNot(HaveOccurred(),
					"Failed to adjust PTP hardware clock for interface %s on node %s", ifaces[0], nodeInfo.Name)

				By("waiting to receive a FREERUN event")
				filter := events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(events.WithSyncState(eventptp.FREERUN)),
				)
				err = events.WaitForEvent(eventPod, startTime, 5*time.Minute, filter, events.WithoutCurrentState(true))
				Expect(err).ToNot(HaveOccurred(),
					"Failed to wait for free run event on interface %s on node %s", ifaces[0], nodeInfo.Name)

				startTime = time.Now()

				By("resetting the PTP hardware clock")
				err = iface.ResetPTPHardwareClock(RANConfig.Spoke1APIClient, nodeInfo.Name, ifaces[0])
				Expect(err).ToNot(HaveOccurred(),
					"Failed to reset PTP hardware clock for interface %s on node %s", ifaces[0], nodeInfo.Name)

				// The locked event should happen much sooner than in 15 minutes, except for GM
				// profiles. This is since the RDS's ts2phc settings for the servo do not allow the PHC
				// to be updated as quickly. The ptp4l settings allow it to be updated much faster.
				By("waiting to receive a locked event")
				filter = events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(events.WithSyncState(eventptp.LOCKED)),
				)
				err = events.WaitForEvent(eventPod, startTime, 15*time.Minute, filter, events.WithoutCurrentState(true))
				Expect(err).ToNot(HaveOccurred(),
					"Failed to wait for locked event on interface %s on node %s", ifaces[0], nodeInfo.Name)
			}
		}

		if !testRanAtLeastOnce {
			Skip("Could not find any node with at least one client interface")
		}
	})

	// 82302 - Validating 'phc2sys' and 'ptp4l' processes state is 'UP' after ptp config change
	It("verifies phc2sys and ptp4l processes are UP", reportxml.ID("82302"), func() {
		testRanAtLeastOnce := false
		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for _, nodeInfo := range nodeInfoMap {
			By("getting the first profile for the node " + nodeInfo.Name)
			profile, err := nodeInfo.GetProfileByConfigPath(RANConfig.Spoke1APIClient, nodeInfo.Name, "ptp4l.0.config")
			Expect(err).ToNot(HaveOccurred(), "Failed to get profile by config path for node %s", nodeInfo.Name)

			By("updating the holdover timeout")
			testRanAtLeastOnce = true

			oldHoldovers, err := profiles.SetHoldOverTimeouts(RANConfig.Spoke1APIClient, []*profiles.ProfileInfo{profile}, 60)
			Expect(err).ToNot(HaveOccurred(), "Failed to set holdover timeout for profile %s", profile.Reference.ProfileName)

			DeferCleanup(func() {
				// If the test succeeded, the PTP configs were restored by the test itself.
				if !CurrentSpecReport().Failed() {
					return
				}

				By("resetting the holdover timeout")
				err = profiles.ResetHoldOverTimeouts(RANConfig.Spoke1APIClient, oldHoldovers)
				Expect(err).ToNot(HaveOccurred(), "Failed to reset holdover timeout for profile %s", profile.Reference.ProfileName)

				By("waiting for the holdover timeout to be reset to original values")
				err = profiles.WaitForOldHoldOverTimeouts(prometheusAPI, nodeInfo.Name, oldHoldovers, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for holdover timeout to be reset to original values")
			})

			By("waiting for the new holdover timeout to show up in the metrics")
			err = profiles.WaitForHoldOverTimeouts(
				prometheusAPI, nodeInfo.Name, []*profiles.ProfileInfo{profile}, 60, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for holdover timeout to be set to 60 after 5 minutes")

			By("resetting the holdover timeout")
			err = profiles.ResetHoldOverTimeouts(RANConfig.Spoke1APIClient, oldHoldovers)
			Expect(err).ToNot(HaveOccurred(), "Failed to reset holdover timeout for profile %s", profile.Reference.ProfileName)

			By("waiting for the holdover timeout to be reset to original values")
			err = profiles.WaitForOldHoldOverTimeouts(prometheusAPI, nodeInfo.Name, oldHoldovers, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for holdover timeout to be reset to original values")

			By("ensuring the process status is UP for both phc2sys and ptp4l")
			processQuery := metrics.ProcessStatusQuery{
				Process: metrics.Includes(metrics.ProcessPHC2SYS, metrics.ProcessPTP4L),
				Node:    metrics.Equals(nodeInfo.Name),
				Config:  metrics.Equals("ptp4l.0.config"),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, processQuery, metrics.ProcessStatusUp,
				metrics.AssertWithTimeout(5*time.Minute))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert process status is UP after 5 minutes")
		}

		if !testRanAtLeastOnce {
			Skip("Could not find any node with at least one profile for this test")
		}
	})
})
