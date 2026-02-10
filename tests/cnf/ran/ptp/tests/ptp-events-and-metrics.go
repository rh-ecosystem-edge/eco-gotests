package tests

import (
	"context"
	"maps"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	eventptp "github.com/redhat-cne/sdk-go/pkg/event/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/internal/nicinfo"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/eventmetric"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/events"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

var _ = Describe("PTP Events and Metrics", Label(tsparams.LabelEventsAndMetrics), func() {
	var (
		prometheusAPI   prometheusv1.API
		savedPtpConfigs []*ptp.PtpConfigBuilder
	)

	BeforeEach(func() {
		By("creating a Prometheus API client")
		var err error
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

	// 82480 - Validating [LOCKED] clock state in PTP metrics
	It("verifies all clocks are LOCKED", reportxml.ID("82480"), func() {
		By("ensuring all clocks on all nodes are LOCKED")
		err := metrics.EnsureClocksAreLocked(prometheusAPI)
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

			ifaceGroups := iface.GroupInterfacesByNIC(profiles.GetInterfacesNames(clientInterfaces))

			By("getting the egress interface for the node")
			egressInterface, err := iface.GetEgressInterfaceName(RANConfig.Spoke1APIClient, nodeInfo.Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to get egress interface name for node %s", nodeInfo.Name)

			for nic, ifaces := range ifaceGroups {
				if nic == egressInterface.GetNIC() {
					klog.V(tsparams.LogLevel).Infof("Skipping egress NIC %s", nic)

					continue
				}

				testRanAtLeastOnce = true

				// Include this interface in the interface information report for this suite.
				nicinfo.Node(nodeInfo.Name).MarkTested(string(ifaces[0]))

				startTime := time.Now()

				By("adjusting the PHC by 100 ms for NIC " + string(nic))
				err := iface.AdjustPTPHardwareClock(RANConfig.Spoke1APIClient, nodeInfo.Name, ifaces[0], 0.1)
				Expect(err).ToNot(HaveOccurred(),
					"Failed to adjust PTP hardware clock for interface %s on node %s", ifaces[0], nodeInfo.Name)

				By("waiting to receive a FREERUN event and metric")
				freerunFilter := events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(events.WithSyncState(eventptp.FREERUN), events.OnInterface(nic)),
				)
				ptp4lClockStateQuery := metrics.ClockStateQuery{
					Node:      metrics.Equals(nodeInfo.Name),
					Interface: metrics.Equals(nic),
					Process:   metrics.Equals(metrics.ProcessPTP4L),
				}
				err = eventmetric.NewAssertion(prometheusAPI, ptp4lClockStateQuery, metrics.ClockStateFreerun, freerunFilter).
					ForNode(RANConfig.Spoke1APIClient, nodeInfo.Name).
					WithStartTime(startTime).
					WithTimeout(5 * time.Minute).
					WithMetricOptions(metrics.AssertWithPollInterval(1 * time.Second)).
					ExecuteAssertion(context.TODO())
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
				By("waiting to receive a locked event and metric")
				lockedFilter := events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(events.WithSyncState(eventptp.LOCKED), events.OnInterface(nic)),
				)
				err = eventmetric.NewAssertion(prometheusAPI, ptp4lClockStateQuery, metrics.ClockStateLocked, lockedFilter).
					ForNode(RANConfig.Spoke1APIClient, nodeInfo.Name).
					WithStartTime(startTime).
					WithTimeout(15 * time.Minute).
					ExecuteAssertion(context.TODO())
				Expect(err).ToNot(HaveOccurred(),
					"Failed to wait for locked event on interface %s on node %s", ifaces[0], nodeInfo.Name)
			}
		}

		if !testRanAtLeastOnce {
			Skip("Could not find any node with at least one client interface")
		}
	})

	// 82302 - Validating 'phc2sys' and 'ptp4l' processes state is 'UP' after PtpConfig change
	It("verifies phc2sys and ptp4l processes are UP", reportxml.ID("82302"), func() {
		testRanAtLeastOnce := false

		By("getting node info map")
		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for _, nodeInfo := range nodeInfoMap {
			By("getting the first profile for the node " + nodeInfo.Name)
			profile, err := nodeInfo.GetProfileByConfigPath(RANConfig.Spoke1APIClient, nodeInfo.Name, "ptp4l.0.config")
			Expect(err).ToNot(HaveOccurred(), "Failed to get profile by config path for node %s", nodeInfo.Name)

			// Include all interfaces from the profile in the interface information report for this suite.
			nicinfo.Node(nodeInfo.Name).MarkSeqTested(iface.NamesToStringSeq(maps.Keys(profile.Interfaces)))

			By("updating the holdover timeout")
			testRanAtLeastOnce = true

			oldHoldovers, err := profiles.SetHoldOverTimeouts(RANConfig.Spoke1APIClient, []*profiles.ProfileInfo{profile}, 60)
			Expect(err).ToNot(HaveOccurred(), "Failed to set holdover timeout for profile %s", profile.Reference.ProfileName)

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
