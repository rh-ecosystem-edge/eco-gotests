package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	eventptp "github.com/redhat-cne/sdk-go/pkg/event/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/consumer"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/daemonlogs"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/events"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/gnss"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/processes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
)

var _ = Describe("PTP T-GM GNSS Loss", Label(tsparams.LabelGNSSLoss), func() {
	var (
		prometheusAPI   prometheusv1.API
		savedPtpConfigs []*ptp.PtpConfigBuilder
		configSupported bool
	)

	eventTimeout := 5 * time.Minute

	BeforeEach(func() {
		By("skipping if PTP version is below 4.18")

		inRange, err := version.IsVersionStringInRange(
			RANConfig.Spoke1OperatorVersions[ranparam.PTP], "4.18.0-0", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to check PTP version range")

		if !inRange {
			Skip("Test is valid from PTP version 4.18 and higher")
		}

		By("creating a Prometheus API client")

		prometheusAPI, err = querier.CreatePrometheusAPIForCluster(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Prometheus API client")

		By("ensuring clocks are locked before testing")

		err = metrics.EnsureClocksAreLocked(prometheusAPI)
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")

		By("saving PtpConfigs before testing")

		savedPtpConfigs, err = profiles.SavePtpConfigs(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to save PtpConfigs")

		configSupported, err = version.IsVersionStringInRange(
			RANConfig.Spoke1OperatorVersions[ranparam.PTP], "4.20.0-0", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to check PTP config label version range")
	})

	AfterEach(func() {
		By("restoring PtpConfigs after testing")

		startTime := time.Now()
		changedProfiles, err := profiles.RestorePtpConfigs(RANConfig.Spoke1APIClient, savedPtpConfigs)
		Expect(err).ToNot(HaveOccurred(), "Failed to restore PtpConfigs")

		if len(changedProfiles) > 0 {
			By("waiting for profile load on nodes")

			err := daemonlogs.WaitForProfileLoadOnPTPNodes(RANConfig.Spoke1APIClient,
				daemonlogs.WithStartTime(startTime),
				daemonlogs.WithTimeout(5*time.Minute))
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to wait for profile load on PTP nodes: %v", err)
			}
		}

		By("ensuring clocks are locked after testing")

		err = metrics.EnsureClocksAreLocked(prometheusAPI)
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")
	})

	// 78463 - verifies t-gm transition from holdover to locked due to gnss recovery
	It("verifies t-gm transition from holdover to locked due to gnss recovery",
		reportxml.ID("78463"), func() {
			testActuallyRan := false

			By("getting node info map")

			nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

			for nodeName, nodeInfo := range nodeInfoMap {
				gmProfilesInfo := nodeInfo.GetProfilesByTypes(profiles.ProfileTypeGM, profiles.ProfileTypeMultiNICGM)
				if len(gmProfilesInfo) == 0 {
					continue
				}

				testActuallyRan = true
				gmProfileInfo := gmProfilesInfo[0]

				gmProfile, err := gmProfileInfo.PullProfile(RANConfig.Spoke1APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull GM profile for node %s", nodeName)

				By("getting the ublox protocol version for node " + nodeName)

				protocolVersion, err := gnss.GetUbloxProtocolVersion(gmProfile)
				Expect(err).ToNot(HaveOccurred(), "Failed to get ublox protocol version for node %s", nodeName)

				ptpConfig, err := gmProfileInfo.Reference.PullPtpConfig(RANConfig.Spoke1APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull PtpConfig for node %s", nodeName)

				profileToUpdate := &ptpConfig.Definition.Spec.Profile[gmProfileInfo.Reference.ProfileIndex]

				desiredSettings := profiles.HoldoverPluginSettings{
					LocalHoldoverTimeout:   14400,
					LocalMaxHoldoverOffSet: 1500,
					MaxInSpecOffset:        1000,
				}

				currentSettings, err := profiles.GetHoldoverPluginSettings(profileToUpdate)
				Expect(err).ToNot(HaveOccurred(), "Failed to get current holdover plugin settings")

				if *currentSettings != desiredSettings {
					By("setting plugin DPLL settings for extended holdover on node " + nodeName)

					err = profiles.SetHoldoverPluginSettings(profileToUpdate, desiredSettings)
					Expect(err).ToNot(HaveOccurred(), "Failed to set holdover plugin settings on node %s", nodeName)

					configChangeTime := time.Now()

					_, updateErr := ptpConfig.Update()
					Expect(updateErr).ToNot(HaveOccurred(), "Failed to update PtpConfig for node %s", nodeName)

					By("waiting for profile load after config change")

					err = daemonlogs.WaitForProfileLoadOnPTPNodes(RANConfig.Spoke1APIClient,
						daemonlogs.WithStartTime(configChangeTime),
						daemonlogs.WithTimeout(eventTimeout))
					Expect(err).ToNot(HaveOccurred(), "Failed to wait for profile load after config change")

					By("ensuring clocks are locked and holdover plugin stabilizes")

					err = metrics.EnsureClocksAreStable(prometheusAPI, 1*time.Minute)
					Expect(err).ToNot(HaveOccurred(), "Failed to ensure clocks are stable after config change")
				}

				By("checking NMEA status metric is available before GNSS loss on node " + nodeName)

				assertNMEAStatusAvailable(prometheusAPI, nodeName)

				By("simulating GNSS loss on node " + nodeName)

				DeferCleanup(cleanupGNSSSync(prometheusAPI, nodeName, protocolVersion))

				gpsLossTime := time.Now()

				err = gnss.SimulateSyncLoss(RANConfig.Spoke1APIClient, nodeName, protocolVersion)
				Expect(err).ToNot(HaveOccurred(), "Failed to simulate GNSS loss on node %s", nodeName)

				By("getting the event consumer pod for node " + nodeName)

				eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get event pod for node %s", nodeName)

				By("waiting for FAILURE-NOFIX GNSS event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.GnssStateChange),
					events.HasValue(events.WithSyncState(eventptp.FAILURE_NOFIX)),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive FAILURE-NOFIX GNSS event on node %s", nodeName)

				By("waiting for HOLDOVER state event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(
						events.WithSyncState(eventptp.HOLDOVER),
						events.ContainingResource(string(iface.Master)),
					),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive HOLDOVER state event on node %s", nodeName)

				By("waiting for clock class 7 event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.PtpClockClassChange),
					events.HasValue(events.WithMetric(int64(metrics.ClockClass7))),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive clock class 7 event on node %s", nodeName)

				By("restoring GNSS sync on node " + nodeName)

				err = gnss.SimulateSyncRecovery(RANConfig.Spoke1APIClient, nodeName, protocolVersion)
				Expect(err).ToNot(HaveOccurred(), "Failed to restore GNSS sync on node %s", nodeName)

				By("waiting for SYNCHRONIZED GNSS event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.GnssStateChange),
					events.HasValue(events.WithSyncState(eventptp.SYNCHRONIZED)),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive SYNCHRONIZED GNSS event on node %s", nodeName)

				By("waiting for LOCKED state event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(
						events.WithSyncState(eventptp.LOCKED),
						events.ContainingResource(string(iface.Master)),
					),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive LOCKED state event on node %s", nodeName)

				By("waiting for clock class 6 event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.PtpClockClassChange),
					events.HasValue(events.WithMetric(int64(metrics.ClockClass6))),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive clock class 6 event on node %s", nodeName)

				By("verifying NMEA status is available after recovery on node " + nodeName)

				assertNMEAStatusAvailable(prometheusAPI, nodeName)

				By("verifying clock class 6 in metrics")

				assertClockClass6InMetrics(prometheusAPI, nodeName, configSupported)

				By("verifying clock state LOCKED in metrics")

				clockStateQuery := metrics.ClockStateQuery{
					Process: metrics.DoesNotEqual(metrics.ProcessChronyd),
					Node:    metrics.Equals(nodeName),
				}
				err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockStateQuery,
					metrics.ClockStateLocked, metrics.AssertWithTimeout(1*time.Minute))
				Expect(err).ToNot(HaveOccurred(),
					"Failed to assert clock state is LOCKED in metrics on node %s", nodeName)

				By("validating no FREERUN events were received")

				err = events.WaitForEvent(eventPod, gpsLossTime, 30*time.Second, events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(
						events.WithSyncState(eventptp.FREERUN),
						events.ContainingResource(string(iface.Master)),
					),
				))
				Expect(err).To(HaveOccurred(),
					"Expected no FREERUN event on node %s, but one was received", nodeName)
			}

			if !testActuallyRan {
				Skip("Test requires Grandmaster configuration")
			}
		})

	// 78464 - verifies t-gm transition from holdover to freerun due to timeout
	It("verifies t-gm transition from holdover to freerun due to timeout",
		reportxml.ID("78464"), func() {
			const holdoverTimeoutSeconds = 3

			testActuallyRan := false

			By("getting node info map")

			nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

			for nodeName, nodeInfo := range nodeInfoMap {
				gmProfilesInfo := nodeInfo.GetProfilesByTypes(profiles.ProfileTypeGM, profiles.ProfileTypeMultiNICGM)
				if len(gmProfilesInfo) == 0 {
					continue
				}

				testActuallyRan = true
				gmProfileInfo := gmProfilesInfo[0]

				By("getting the ublox protocol version for node " + nodeName)

				gmProfile, err := gmProfileInfo.PullProfile(RANConfig.Spoke1APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull GM profile for node %s", nodeName)

				protocolVersion, err := gnss.GetUbloxProtocolVersion(gmProfile)
				Expect(err).ToNot(HaveOccurred(), "Failed to get ublox protocol version for node %s", nodeName)

				ptpConfig, err := gmProfileInfo.Reference.PullPtpConfig(RANConfig.Spoke1APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull PtpConfig for node %s", nodeName)

				profileToUpdate := &ptpConfig.Definition.Spec.Profile[gmProfileInfo.Reference.ProfileIndex]

				desiredSettings := profiles.HoldoverPluginSettings{
					LocalHoldoverTimeout:   holdoverTimeoutSeconds,
					LocalMaxHoldoverOffSet: 300,
					MaxInSpecOffset:        600,
				}

				currentSettings, err := profiles.GetHoldoverPluginSettings(profileToUpdate)
				Expect(err).ToNot(HaveOccurred(), "Failed to get current holdover plugin settings")

				if *currentSettings != desiredSettings {
					By("setting plugin DPLL settings for short holdover timeout on node " + nodeName)

					err = profiles.SetHoldoverPluginSettings(profileToUpdate, desiredSettings)
					Expect(err).ToNot(HaveOccurred(), "Failed to set holdover plugin settings on node %s", nodeName)

					configChangeTime := time.Now()

					_, updateErr := ptpConfig.Update()
					Expect(updateErr).ToNot(HaveOccurred(), "Failed to update PtpConfig for node %s", nodeName)

					By("waiting for profile load after config change")

					err = daemonlogs.WaitForProfileLoadOnPTPNodes(RANConfig.Spoke1APIClient,
						daemonlogs.WithStartTime(configChangeTime),
						daemonlogs.WithTimeout(eventTimeout))
					Expect(err).ToNot(HaveOccurred(), "Failed to wait for profile load after config change")

					By("ensuring clocks are locked and holdover plugin stabilizes")

					err = metrics.EnsureClocksAreStable(prometheusAPI, 1*time.Minute)
					Expect(err).ToNot(HaveOccurred(), "Failed to ensure clocks are stable after config change")
				}

				By("checking NMEA status metric is available before GNSS loss on node " + nodeName)

				assertNMEAStatusAvailable(prometheusAPI, nodeName)

				By("simulating GNSS loss on node " + nodeName)

				DeferCleanup(cleanupGNSSSync(prometheusAPI, nodeName, protocolVersion))

				gpsLossTime := time.Now()

				err = gnss.SimulateSyncLoss(RANConfig.Spoke1APIClient, nodeName, protocolVersion)
				Expect(err).ToNot(HaveOccurred(), "Failed to simulate GNSS loss on node %s", nodeName)

				By("waiting for the holdover timeout to expire")

				time.Sleep(holdoverTimeoutSeconds * time.Second)

				By("restoring GNSS sync on node " + nodeName)

				err = gnss.SimulateSyncRecovery(RANConfig.Spoke1APIClient, nodeName, protocolVersion)
				Expect(err).ToNot(HaveOccurred(), "Failed to restore GNSS sync on node %s", nodeName)

				By("getting the event consumer pod for node " + nodeName)

				eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get event pod for node %s", nodeName)

				By("waiting for FAILURE-NOFIX GNSS event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.GnssStateChange),
					events.HasValue(events.WithSyncState(eventptp.FAILURE_NOFIX)),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive FAILURE-NOFIX GNSS event on node %s", nodeName)

				By("waiting for HOLDOVER state event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(
						events.WithSyncState(eventptp.HOLDOVER),
						events.ContainingResource(string(iface.Master)),
					),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive HOLDOVER state event on node %s", nodeName)

				By("waiting for clock class 7 event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.PtpClockClassChange),
					events.HasValue(events.WithMetric(int64(metrics.ClockClass7))),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive clock class 7 event on node %s", nodeName)

				By("checking for holdover timer expiration in daemon logs")

				err = daemonlogs.WaitForPodLog(RANConfig.Spoke1APIClient, nodeName,
					daemonlogs.WithStartTime(gpsLossTime),
					daemonlogs.WithTimeout(eventTimeout),
					daemonlogs.WithMatcher(daemonlogs.ContainsMatcher("holdover timer")))
				Expect(err).ToNot(HaveOccurred(),
					"Failed to find holdover timer message in daemon logs on node %s", nodeName)

				By("waiting for FREERUN state event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(
						events.WithSyncState(eventptp.FREERUN),
						events.ContainingResource(string(iface.Master)),
					),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive FREERUN state event on node %s", nodeName)

				By("waiting for clock class 248 event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.PtpClockClassChange),
					events.HasValue(events.WithMetric(int64(metrics.ClockClass248))),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive clock class 248 event on node %s", nodeName)

				By("waiting for os-clock-sync-state FREERUN event for CLOCK_REALTIME")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.OsClockSyncStateChange),
					events.HasValue(events.WithSyncState(eventptp.FREERUN), events.OnInterface(iface.ClockRealtime)),
				))
				Expect(err).ToNot(HaveOccurred(),
					"Failed to receive os-clock-sync-state FREERUN event on node %s", nodeName)

				By("waiting for SYNCHRONIZED GNSS event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.GnssStateChange),
					events.HasValue(events.WithSyncState(eventptp.SYNCHRONIZED)),
				))
				Expect(err).ToNot(HaveOccurred(),
					"Failed to receive SYNCHRONIZED GNSS event on node %s", nodeName)

				By("waiting for LOCKED state event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(
						events.WithSyncState(eventptp.LOCKED),
						events.ContainingResource(string(iface.Master)),
					),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive LOCKED state event on node %s", nodeName)

				By("waiting for clock class 6 event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.PtpClockClassChange),
					events.HasValue(events.WithMetric(int64(metrics.ClockClass6))),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive clock class 6 event on node %s", nodeName)

				By("waiting for os-clock-sync-state LOCKED event for CLOCK_REALTIME")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.OsClockSyncStateChange),
					events.HasValue(events.WithSyncState(eventptp.LOCKED), events.OnInterface(iface.ClockRealtime)),
				))
				Expect(err).ToNot(HaveOccurred(),
					"Failed to receive os-clock-sync-state LOCKED event on node %s", nodeName)

				By("verifying NMEA status is available after recovery on node " + nodeName)

				assertNMEAStatusAvailable(prometheusAPI, nodeName)

				By("verifying clock class 6 in metrics")

				assertClockClass6InMetrics(prometheusAPI, nodeName, configSupported)
			}

			if !testActuallyRan {
				Skip("Test requires Grandmaster configuration")
			}
		})

	// 78465 - verifies t-gm transition from holdover to freerun due to offset
	It("verifies t-gm transition from holdover to freerun due to offset",
		reportxml.ID("78465"), func() {
			const (
				holdoverTimeoutSeconds = 600
				localMaxHoldoverOffset = 60000
				maxInSpecOffset        = 300
				timeToReachMaxInSpec   = (maxInSpecOffset / (localMaxHoldoverOffset / holdoverTimeoutSeconds)) * time.Second
			)

			testActuallyRan := false

			By("getting node info map")

			nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

			for nodeName, nodeInfo := range nodeInfoMap {
				gmProfilesInfo := nodeInfo.GetProfilesByTypes(profiles.ProfileTypeGM, profiles.ProfileTypeMultiNICGM)
				if len(gmProfilesInfo) == 0 {
					continue
				}

				testActuallyRan = true
				gmProfileInfo := gmProfilesInfo[0]

				By("getting the ublox protocol version for node " + nodeName)

				gmProfile, err := gmProfileInfo.PullProfile(RANConfig.Spoke1APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull GM profile for node %s", nodeName)

				protocolVersion, err := gnss.GetUbloxProtocolVersion(gmProfile)
				Expect(err).ToNot(HaveOccurred(), "Failed to get ublox protocol version for node %s", nodeName)

				ptpConfig, err := gmProfileInfo.Reference.PullPtpConfig(RANConfig.Spoke1APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull PtpConfig for node %s", nodeName)

				profileToUpdate := &ptpConfig.Definition.Spec.Profile[gmProfileInfo.Reference.ProfileIndex]

				desiredSettings := profiles.HoldoverPluginSettings{
					LocalHoldoverTimeout:   holdoverTimeoutSeconds,
					LocalMaxHoldoverOffSet: localMaxHoldoverOffset,
					MaxInSpecOffset:        maxInSpecOffset,
				}

				currentSettings, err := profiles.GetHoldoverPluginSettings(profileToUpdate)
				Expect(err).ToNot(HaveOccurred(), "Failed to get current holdover plugin settings")

				if *currentSettings != desiredSettings {
					By("setting plugin DPLL settings for offset-triggered freerun on node " + nodeName)

					err = profiles.SetHoldoverPluginSettings(profileToUpdate, desiredSettings)
					Expect(err).ToNot(HaveOccurred(), "Failed to set holdover plugin settings on node %s", nodeName)

					configChangeTime := time.Now()

					_, updateErr := ptpConfig.Update()
					Expect(updateErr).ToNot(HaveOccurred(), "Failed to update PtpConfig for node %s", nodeName)

					By("waiting for profile load after config change")

					err = daemonlogs.WaitForProfileLoadOnPTPNodes(RANConfig.Spoke1APIClient,
						daemonlogs.WithStartTime(configChangeTime),
						daemonlogs.WithTimeout(eventTimeout))
					Expect(err).ToNot(HaveOccurred(), "Failed to wait for profile load after config change")

					By("ensuring clocks are locked and holdover plugin stabilizes")

					err = metrics.EnsureClocksAreStable(prometheusAPI, 1*time.Minute)
					Expect(err).ToNot(HaveOccurred(), "Failed to ensure clocks are stable after config change")
				}

				By("checking NMEA status metric is available before GNSS loss on node " + nodeName)

				assertNMEAStatusAvailable(prometheusAPI, nodeName)

				By(fmt.Sprintf("simulating GNSS loss on node %s for %v to reach max in-spec offset",
					nodeName, timeToReachMaxInSpec))

				DeferCleanup(cleanupGNSSSync(prometheusAPI, nodeName, protocolVersion))

				gpsLossTime := time.Now()

				err = gnss.SimulateSyncLoss(RANConfig.Spoke1APIClient, nodeName, protocolVersion)
				Expect(err).ToNot(HaveOccurred(), "Failed to simulate GNSS loss on node %s", nodeName)

				By("waiting for the offset to exceed the max in-spec threshold")

				time.Sleep(timeToReachMaxInSpec)

				By("restoring GNSS sync on node " + nodeName)

				err = gnss.SimulateSyncRecovery(RANConfig.Spoke1APIClient, nodeName, protocolVersion)
				Expect(err).ToNot(HaveOccurred(), "Failed to restore GNSS sync on node %s", nodeName)

				By("getting the event consumer pod for node " + nodeName)

				eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get event pod for node %s", nodeName)

				By("waiting for FAILURE-NOFIX GNSS event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.GnssStateChange),
					events.HasValue(events.WithSyncState(eventptp.FAILURE_NOFIX)),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive FAILURE-NOFIX GNSS event on node %s", nodeName)

				By("waiting for HOLDOVER state event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(
						events.WithSyncState(eventptp.HOLDOVER),
						events.ContainingResource(string(iface.Master)),
					),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive HOLDOVER state event on node %s", nodeName)

				By("waiting for clock class 7 event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.PtpClockClassChange),
					events.HasValue(events.WithMetric(int64(metrics.ClockClass7))),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive clock class 7 event on node %s", nodeName)

				By("checking for offset out of range in daemon logs")

				err = daemonlogs.WaitForPodLog(RANConfig.Spoke1APIClient, nodeName,
					daemonlogs.WithStartTime(gpsLossTime),
					daemonlogs.WithTimeout(eventTimeout),
					daemonlogs.WithMatcher(daemonlogs.ContainsMatcher("dpll inspec offset is out of range")))
				Expect(err).ToNot(HaveOccurred(),
					"Failed to find 'dpll inspec offset is out of range' message in daemon logs on node %s", nodeName)

				By("waiting for FREERUN state event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(
						events.WithSyncState(eventptp.FREERUN),
						events.ContainingResource(string(iface.Master)),
					),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive FREERUN state event on node %s", nodeName)

				By("waiting for clock class 248 event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.PtpClockClassChange),
					events.HasValue(events.WithMetric(int64(metrics.ClockClass248))),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive clock class 248 event on node %s", nodeName)

				By("waiting for os-clock-sync-state FREERUN event for CLOCK_REALTIME")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.OsClockSyncStateChange),
					events.HasValue(events.WithSyncState(eventptp.FREERUN), events.OnInterface(iface.ClockRealtime)),
				))
				Expect(err).ToNot(HaveOccurred(),
					"Failed to receive os-clock-sync-state FREERUN event on node %s", nodeName)

				By("waiting for SYNCHRONIZED GNSS event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.GnssStateChange),
					events.HasValue(events.WithSyncState(eventptp.SYNCHRONIZED)),
				))
				Expect(err).ToNot(HaveOccurred(),
					"Failed to receive SYNCHRONIZED GNSS event on node %s", nodeName)

				By("waiting for LOCKED state event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(
						events.WithSyncState(eventptp.LOCKED),
						events.ContainingResource(string(iface.Master)),
					),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive LOCKED state event on node %s", nodeName)

				By("waiting for clock class 6 event")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.PtpClockClassChange),
					events.HasValue(events.WithMetric(int64(metrics.ClockClass6))),
				))
				Expect(err).ToNot(HaveOccurred(), "Failed to receive clock class 6 event on node %s", nodeName)

				By("waiting for os-clock-sync-state LOCKED event for CLOCK_REALTIME")

				err = events.WaitForEvent(eventPod, gpsLossTime, eventTimeout, events.All(
					events.IsType(eventptp.OsClockSyncStateChange),
					events.HasValue(events.WithSyncState(eventptp.LOCKED), events.OnInterface(iface.ClockRealtime)),
				))
				Expect(err).ToNot(HaveOccurred(),
					"Failed to receive os-clock-sync-state LOCKED event on node %s", nodeName)

				By("verifying NMEA status is available after recovery on node " + nodeName)

				assertNMEAStatusAvailable(prometheusAPI, nodeName)

				By("verifying clock class 6 in metrics")

				assertClockClass6InMetrics(prometheusAPI, nodeName, configSupported)
			}

			if !testActuallyRan {
				Skip("Test requires Grandmaster configuration")
			}
		})
})

func assertNMEAStatusAvailable(
	prometheusAPI prometheusv1.API,
	nodeName string,
) {
	GinkgoHelper()

	nmeaQuery := metrics.NMEAStatusQuery{
		Process: metrics.Equals(metrics.ProcessTS2PHC),
		Node:    metrics.Equals(nodeName),
	}
	err := metrics.AssertQuery(context.TODO(), prometheusAPI, nmeaQuery,
		metrics.NMEAStatusAvailable, metrics.AssertWithTimeout(1*time.Minute))
	Expect(err).ToNot(HaveOccurred(),
		"Failed to assert NMEA status is available on node %s", nodeName)
}

func assertClockClass6InMetrics(
	prometheusAPI prometheusv1.API,
	nodeName string,
	configSupported bool,
) {
	GinkgoHelper()

	var configFile string

	var err error
	if configSupported {
		configFile, err = processes.GetPtp4lConfigByRelatedProcess(
			RANConfig.Spoke1APIClient, nodeName, processes.Ts2phc)
		Expect(err).ToNot(HaveOccurred(), "Failed to determine ptp4l config for node %s", nodeName)
	}

	clockClassQuery := metrics.ClockClassQuery{
		Process: metrics.Equals(metrics.ProcessPTP4L),
		Node:    metrics.Equals(nodeName),
		Config:  metrics.Equals(configFile),
	}
	err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockClassQuery,
		metrics.ClockClass6, metrics.AssertWithTimeout(1*time.Minute))
	Expect(err).ToNot(HaveOccurred(),
		"Failed to assert clock class is 6 in metrics on node %s", nodeName)
}
