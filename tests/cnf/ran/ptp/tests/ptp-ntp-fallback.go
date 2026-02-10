package tests

import (
	"context"
	"maps"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	eventptp "github.com/redhat-cne/sdk-go/pkg/event/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/internal/nicinfo"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/consumer"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/events"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/gnss"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/processes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

var _ = Describe("PTP GNSS with NTP Fallback", Label(tsparams.LabelNTPFallback), func() {
	var (
		prometheusAPI   prometheusv1.API
		savedPtpConfigs []*ptp.PtpConfigBuilder
	)

	BeforeEach(func() {
		By("skipping if the PTP version is not supported")
		inRange, err := version.IsVersionStringInRange(RANConfig.Spoke1OperatorVersions[ranparam.PTP], "4.18", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to check PTP version range")

		if !inRange {
			Skip("ntpfailover is only supported for PTP version 4.18 and higher")
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
	})

	AfterEach(func() {
		if CurrentSpecReport().State == types.SpecStateSkipped {
			return
		}

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

		// Wait for 20 minutes instead of the usual 5 as a workaround for OCPBUGS-66352.
		By("ensuring clocks are locked after testing")
		query := metrics.ClockStateQuery{
			Process: metrics.DoesNotEqual(metrics.ProcessChronyd),
		}
		err = metrics.AssertQuery(context.TODO(), prometheusAPI, query, metrics.ClockStateLocked,
			metrics.AssertWithStableDuration(10*time.Second),
			metrics.AssertWithTimeout(20*time.Minute))
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")
	})

	// 85904 - Successful fallback to NTP when GNSS sync lost
	It("successfully falls back to NTP when GNSS sync lost", reportxml.ID("85904"), func() {
		testActuallyRan := false

		By("getting node info map")
		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for nodeName, nodeInfo := range nodeInfoMap {
			if nodeInfo.Counts[profiles.ProfileTypeNTPFallback] == 0 {
				continue
			}

			testActuallyRan = true

			By("getting the u-blox protocol version")
			ntpFallbackProfiles := nodeInfo.GetProfilesByTypes(profiles.ProfileTypeNTPFallback)
			Expect(ntpFallbackProfiles).ToNot(BeEmpty(), "No NTP fallback profile found for node %s", nodeName)

			ntpFallbackProfile, err := ntpFallbackProfiles[0].PullProfile(RANConfig.Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull NTP fallback profile for node %s", nodeName)

			protocolVersion, err := gnss.GetUbloxProtocolVersion(ntpFallbackProfile)
			Expect(err).ToNot(HaveOccurred(), "Failed to get u-blox protocol version for node %s", nodeName)

			// Include all interfaces from the profile in the interface information report for this suite.
			nicinfo.Node(nodeName).MarkSeqTested(iface.NamesToStringSeq(maps.Keys(ntpFallbackProfiles[0].Interfaces)))

			By("setting the ts2phc holdover to 10 seconds")
			updateTime := time.Now()
			oldProfile, err := profiles.UpdateTS2PHCHoldover(RANConfig.Spoke1APIClient, ntpFallbackProfiles[0], 10)
			Expect(err).ToNot(HaveOccurred(), "Failed to update ts2phc holdover for node %s", nodeName)

			waitForLoadAndTS2PHCLocked(prometheusAPI, nodeName, updateTime)

			By("using chronyc activity to verify chronyd is not syncing")
			chronycActivity, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(
				RANConfig.Spoke1APIClient, nodeName, "chronyc activity",
				ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnError(true), ptpdaemon.WithRetryOnEmptyOutput(true))
			Expect(err).ToNot(HaveOccurred(), "Failed to get chronyc activity for node %s", nodeName)
			Expect(chronycActivity).To(ContainSubstring("0 sources online"), "Chronyd has sources online on node %s", nodeName)

			By("simulating GNSS sync loss")
			DeferCleanup(cleanupGNSSSync(prometheusAPI, nodeName, protocolVersion))

			gnssLossTime := time.Now()
			err = gnss.SimulateSyncLoss(RANConfig.Spoke1APIClient, nodeName, protocolVersion)
			Expect(err).ToNot(HaveOccurred(), "Failed to simulate GNSS sync loss for node %s", nodeName)

			By("getting the event pod for the node")
			eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to get event pod for node %s", nodeName)

			By("waiting for os-clock-sync-state LOCKED event")
			osClockLockedFilter := events.All(
				events.IsType(eventptp.OsClockSyncStateChange),
				events.HasValue(events.WithSyncState(eventptp.LOCKED)),
			)
			err = events.WaitForEvent(eventPod, gnssLossTime, 5*time.Minute, osClockLockedFilter)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for os-clock-sync-state LOCKED event on node %s", nodeName)

			By("verifying phc2sys process is not running")
			err = processes.WaitForProcessRunning(RANConfig.Spoke1APIClient, nodeName, processes.Phc2sys, false, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for phc2sys process to be not running on node %s", nodeName)

			By("using chronyc activity to verify chronyd is syncing")
			// We make an asynchronous assertion since chronyd initially does a burst before it starts
			// showing sources online.
			Eventually(ptpdaemon.ExecuteCommandInPtpDaemonPod).
				WithArguments(RANConfig.Spoke1APIClient, nodeName, "chronyc activity").
				WithTimeout(time.Minute).WithPolling(10*time.Second).
				ShouldNot(ContainSubstring("0 sources online"), "Chronyd has 0 sources online on node %s", nodeName)

			By("restoring GNSS sync")
			gnssRecoveryTime := time.Now()
			err = gnss.SimulateSyncRecovery(RANConfig.Spoke1APIClient, nodeName, protocolVersion)
			Expect(err).ToNot(HaveOccurred(), "Failed to simulate GNSS sync recovery for node %s", nodeName)

			By("waiting for os-clock-sync-state LOCKED event")
			err = events.WaitForEvent(eventPod, gnssRecoveryTime, 5*time.Minute, osClockLockedFilter)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for os-clock-sync-state LOCKED event on node %s", nodeName)

			By("verifying phc2sys process is running")
			err = processes.WaitForProcessRunning(RANConfig.Spoke1APIClient, nodeName, processes.Phc2sys, true, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for phc2sys process to be running on node %s", nodeName)

			By("using chronyc activity to verify chronyd is not syncing")
			chronycActivity, err = ptpdaemon.ExecuteCommandInPtpDaemonPod(
				RANConfig.Spoke1APIClient, nodeName, "chronyc activity",
				ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnError(true), ptpdaemon.WithRetryOnEmptyOutput(true))
			Expect(err).ToNot(HaveOccurred(), "Failed to get chronyc activity for node %s", nodeName)
			Expect(chronycActivity).To(ContainSubstring("0 sources online"), "Chronyd has sources online on node %s", nodeName)

			By("restoring the ts2phc holdover")
			restoreTime := time.Now()
			changed, err := profiles.RestoreProfileToConfig(
				RANConfig.Spoke1APIClient, ntpFallbackProfiles[0].Reference, oldProfile)
			Expect(err).ToNot(HaveOccurred(), "Failed to restore NTP fallback profile for node %s", nodeName)

			if changed {
				waitForLoadAndTS2PHCLocked(prometheusAPI, nodeName, restoreTime)
			}
		}

		if !testActuallyRan {
			Skip("No receiver interfaces found for any node")
		}
	})

	// 86920 - Successful fallback to NTP when offset spike occurs
	It("successfully falls back to NTP when offset spike occurs", reportxml.ID("86920"), func() {
		// offsetSpikeAmount is the amount to adjust the PTP hardware clock by in seconds. This value
		// should be large enough to cause the servo state to transition from SERVO_LOCKED_STABLE (s3)
		// to SERVO_LOCKED (s2), which triggers the NTP fallback mechanism. The value of 0.001 seconds
		// (1 millisecond) is chosen to cause a significant offset that takes approximately 10 seconds
		// for ts2phc to correct.
		const offsetSpikeAmount = 0.001

		By("skipping if the PTP version is not supported")
		inRange, err := version.IsVersionStringInRange(RANConfig.Spoke1OperatorVersions[ranparam.PTP], "4.21", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to check PTP version range")

		if !inRange {
			Skip("ntpfailover offset spike is only supported for PTP version 4.21 and higher")
		}

		testActuallyRan := false

		By("getting node info map")
		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for nodeName, nodeInfo := range nodeInfoMap {
			if nodeInfo.Counts[profiles.ProfileTypeNTPFallback] == 0 {
				continue
			}

			testActuallyRan = true

			By("getting the NTP fallback profile and server interface")
			ntpFallbackProfiles := nodeInfo.GetProfilesByTypes(profiles.ProfileTypeNTPFallback)
			Expect(ntpFallbackProfiles).ToNot(BeEmpty(), "No NTP fallback profile found for node %s", nodeName)

			serverInterfaces := ntpFallbackProfiles[0].GetInterfacesByClockType(profiles.ClockTypeServer)
			Expect(serverInterfaces).ToNot(BeEmpty(), "No server interface found for NTP fallback profile on node %s", nodeName)

			serverInterface := serverInterfaces[0].Name

			// Include all interfaces from the profile in the interface information report for this suite.
			nicinfo.Node(nodeName).MarkSeqTested(iface.NamesToStringSeq(maps.Keys(ntpFallbackProfiles[0].Interfaces)))

			By("setting the ts2phc holdover to 10 seconds")
			updateTime := time.Now()
			oldProfile, err := profiles.UpdateTS2PHCHoldover(RANConfig.Spoke1APIClient, ntpFallbackProfiles[0], 10)
			Expect(err).ToNot(HaveOccurred(), "Failed to update ts2phc holdover for node %s", nodeName)

			waitForLoadAndTS2PHCLocked(prometheusAPI, nodeName, updateTime)

			By("using chronyc activity to verify chronyd is not syncing")
			chronycActivity, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(
				RANConfig.Spoke1APIClient, nodeName, "chronyc activity",
				ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnError(true), ptpdaemon.WithRetryOnEmptyOutput(true))
			Expect(err).ToNot(HaveOccurred(), "Failed to get chronyc activity for node %s", nodeName)
			Expect(chronycActivity).To(ContainSubstring("0 sources online"), "Chronyd has sources online on node %s", nodeName)

			By("injecting offset spike to trigger servo state transition")
			offsetSpikeTime := time.Now()
			err = iface.AdjustPTPHardwareClock(RANConfig.Spoke1APIClient, nodeName, serverInterface, offsetSpikeAmount)
			Expect(err).ToNot(HaveOccurred(), "Failed to inject offset spike for node %s", nodeName)

			By("getting the event pod for the node")
			eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to get event pod for node %s", nodeName)

			By("waiting for os-clock-sync-state LOCKED event from NTP fallback")
			osClockLockedFilter := events.All(
				events.IsType(eventptp.OsClockSyncStateChange),
				events.HasValue(events.WithSyncState(eventptp.LOCKED)),
			)
			err = events.WaitForEvent(eventPod, offsetSpikeTime, 5*time.Minute, osClockLockedFilter)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for os-clock-sync-state LOCKED event on node %s", nodeName)

			By("verifying phc2sys process is not running")
			err = processes.WaitForProcessRunning(RANConfig.Spoke1APIClient, nodeName, processes.Phc2sys, false, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for phc2sys process to be not running on node %s", nodeName)

			By("using chronyc activity to verify chronyd is syncing")
			// We make an asynchronous assertion since chronyd initially does a burst before it starts
			// showing sources online.
			Eventually(ptpdaemon.ExecuteCommandInPtpDaemonPod).
				WithArguments(RANConfig.Spoke1APIClient, nodeName, "chronyc activity").
				WithTimeout(time.Minute).WithPolling(10*time.Second).
				ShouldNot(ContainSubstring("0 sources online"), "Chronyd has 0 sources online on node %s", nodeName)

			By("waiting for ts2phc to correct the offset and restore PTP sync")
			// ts2phc will automatically work on correcting the offset spike. Once corrected, the system
			// will transition back to PTP sync and emit another os-clock-sync-state LOCKED event.
			recoveryTime := time.Now()
			err = events.WaitForEvent(eventPod, recoveryTime, 5*time.Minute, osClockLockedFilter)
			Expect(err).ToNot(HaveOccurred(),
				"Failed to wait for os-clock-sync-state LOCKED event after recovery on node %s", nodeName)

			By("verifying phc2sys process is running")
			err = processes.WaitForProcessRunning(RANConfig.Spoke1APIClient, nodeName, processes.Phc2sys, true, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for phc2sys process to be running on node %s", nodeName)

			By("using chronyc activity to verify chronyd is not syncing")
			chronycActivity, err = ptpdaemon.ExecuteCommandInPtpDaemonPod(
				RANConfig.Spoke1APIClient, nodeName, "chronyc activity",
				ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnError(true), ptpdaemon.WithRetryOnEmptyOutput(true))
			Expect(err).ToNot(HaveOccurred(), "Failed to get chronyc activity for node %s", nodeName)
			Expect(chronycActivity).To(ContainSubstring("0 sources online"), "Chronyd has sources online on node %s", nodeName)

			By("restoring the ts2phc holdover")
			restoreTime := time.Now()
			changed, err := profiles.RestoreProfileToConfig(
				RANConfig.Spoke1APIClient, ntpFallbackProfiles[0].Reference, oldProfile)
			Expect(err).ToNot(HaveOccurred(), "Failed to restore NTP fallback profile for node %s", nodeName)

			if changed {
				waitForLoadAndTS2PHCLocked(prometheusAPI, nodeName, restoreTime)
			}
		}

		if !testActuallyRan {
			Skip("No receiver interfaces found for any node")
		}
	})

	// 85905 - Failed fallback to NTP when GNSS sync lost
	It("fails to fall back to NTP when NTP server unreachable", reportxml.ID("85905"), func() {
		testActuallyRan := false

		By("getting node info map")
		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for nodeName, nodeInfo := range nodeInfoMap {
			if nodeInfo.Counts[profiles.ProfileTypeNTPFallback] == 0 {
				continue
			}

			testActuallyRan = true

			By("getting the u-blox protocol version")
			ntpFallbackProfiles := nodeInfo.GetProfilesByTypes(profiles.ProfileTypeNTPFallback)
			Expect(ntpFallbackProfiles).ToNot(BeEmpty(), "No NTP fallback profile found for node %s", nodeName)

			ntpFallbackProfile, err := ntpFallbackProfiles[0].PullProfile(RANConfig.Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull NTP fallback profile for node %s", nodeName)

			protocolVersion, err := gnss.GetUbloxProtocolVersion(ntpFallbackProfile)
			Expect(err).ToNot(HaveOccurred(), "Failed to get u-blox protocol version for node %s", nodeName)

			// Include all interfaces from the profile in the interface information report for this suite.
			nicinfo.Node(nodeName).MarkSeqTested(iface.NamesToStringSeq(maps.Keys(ntpFallbackProfiles[0].Interfaces)))

			By("setting the ts2phc holdover to 10 seconds")
			oldProfile, err := profiles.UpdateTS2PHCHoldover(RANConfig.Spoke1APIClient, ntpFallbackProfiles[0], 10)
			Expect(err).ToNot(HaveOccurred(), "Failed to update ts2phc holdover for node %s", nodeName)

			By("replacing chronyd servers with invalid server to simulate unreachable NTP server")
			invalidServerTime := time.Now()
			_, err = profiles.ReplaceChronydServers(
				RANConfig.Spoke1APIClient, ntpFallbackProfiles[0].Reference, ranparam.UnreachableIPv4Address)
			Expect(err).ToNot(HaveOccurred(), "Failed to replace chronyd servers for node %s", nodeName)

			waitForLoadAndTS2PHCLocked(prometheusAPI, nodeName, invalidServerTime)

			By("simulating GNSS sync loss")
			DeferCleanup(cleanupGNSSSync(prometheusAPI, nodeName, protocolVersion))

			gnssLossTime := time.Now()
			err = gnss.SimulateSyncLoss(RANConfig.Spoke1APIClient, nodeName, protocolVersion)
			Expect(err).ToNot(HaveOccurred(), "Failed to simulate GNSS sync loss for node %s", nodeName)

			By("getting the event pod for the node")
			eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to get event pod for node %s", nodeName)

			By("waiting for os-clock-sync-state FREERUN event")
			osClockFreerunFilter := events.All(
				events.IsType(eventptp.OsClockSyncStateChange),
				events.HasValue(events.WithSyncState(eventptp.FREERUN)),
			)
			err = events.WaitForEvent(eventPod, gnssLossTime, 5*time.Minute, osClockFreerunFilter)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for os-clock-sync-state FREERUN event on node %s", nodeName)

			By("ensuring no os-clock-sync-state LOCKED event is received")
			osClockLockedFilter := events.All(
				events.IsType(eventptp.OsClockSyncStateChange),
				events.HasValue(events.WithSyncState(eventptp.LOCKED)),
			)
			err = events.WaitForEvent(eventPod, time.Now(), time.Minute, osClockLockedFilter)
			Expect(err).To(HaveOccurred(), "Received unexpected os-clock-sync-state LOCKED event on node %s", nodeName)

			By("using chronyc activity to verify 0 sources online")
			chronycActivity, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(
				RANConfig.Spoke1APIClient, nodeName, "chronyc activity",
				ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnError(true), ptpdaemon.WithRetryOnEmptyOutput(true))
			Expect(err).ToNot(HaveOccurred(), "Failed to get chronyc activity for node %s", nodeName)
			Expect(chronycActivity).To(ContainSubstring("0 sources online"), "Chronyd has sources online on node %s", nodeName)

			By("restoring GNSS sync")
			gnssRecoveryTime := time.Now()
			err = gnss.SimulateSyncRecovery(RANConfig.Spoke1APIClient, nodeName, protocolVersion)
			Expect(err).ToNot(HaveOccurred(), "Failed to simulate GNSS sync recovery for node %s", nodeName)

			By("waiting for os-clock-sync-state LOCKED event")
			err = events.WaitForEvent(eventPod, gnssRecoveryTime, 5*time.Minute, osClockLockedFilter)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for os-clock-sync-state LOCKED event on node %s", nodeName)

			By("restoring the original profile configuration")
			restoreTime := time.Now()
			changed, err := profiles.RestoreProfileToConfig(
				RANConfig.Spoke1APIClient, ntpFallbackProfiles[0].Reference, oldProfile)
			Expect(err).ToNot(HaveOccurred(), "Failed to restore NTP fallback profile for node %s", nodeName)

			if changed {
				waitForLoadAndTS2PHCLocked(prometheusAPI, nodeName, restoreTime)
			}
		}

		if !testActuallyRan {
			Skip("No receiver interfaces found for any node")
		}
	})

	// 85906 - Ensure system clock is within 1.5 ms for entire holdover
	It("verifies system clock is within 1.5 ms for entire ts2phc holdover", reportxml.ID("85906"), func() {
		testActuallyRan := false

		By("getting node info map")
		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for nodeName, nodeInfo := range nodeInfoMap {
			if nodeInfo.Counts[profiles.ProfileTypeNTPFallback] == 0 || nodeInfo.Counts[profiles.ProfileTypeOC] == 0 {
				continue
			}

			testActuallyRan = true

			By("getting the u-blox protocol version")
			ntpFallbackProfiles := nodeInfo.GetProfilesByTypes(profiles.ProfileTypeNTPFallback)
			Expect(ntpFallbackProfiles).ToNot(BeEmpty(), "No NTP fallback profile found for node %s", nodeName)

			ntpFallbackProfile, err := ntpFallbackProfiles[0].PullProfile(RANConfig.Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull NTP fallback profile for node %s", nodeName)

			protocolVersion, err := gnss.GetUbloxProtocolVersion(ntpFallbackProfile)
			Expect(err).ToNot(HaveOccurred(), "Failed to get u-blox protocol version for node %s", nodeName)

			// Include all interfaces from the profile in the interface information report for this suite.
			nicinfo.Node(nodeName).MarkSeqTested(iface.NamesToStringSeq(maps.Keys(ntpFallbackProfiles[0].Interfaces)))

			By("getting the OC profile interface and PTP clock")
			ocProfiles := nodeInfo.GetProfilesByTypes(profiles.ProfileTypeOC)
			Expect(ocProfiles).ToNot(BeEmpty(), "No OC profile found for node %s", nodeName)

			ocInterfaces := ocProfiles[0].GetInterfacesByClockType(profiles.ClockTypeClient)
			Expect(ocInterfaces).ToNot(BeEmpty(), "No follower interface found for OC profile on node %s", nodeName)

			ocInterface := ocInterfaces[0].Name

			// Include the OC interface in the interface information report for this suite.
			nicinfo.Node(nodeName).MarkTested(string(ocInterface))

			ptpClockIndex, err := iface.GetPTPHardwareClock(RANConfig.Spoke1APIClient, nodeName, ocInterface)
			Expect(err).ToNot(HaveOccurred(),
				"Failed to get PTP hardware clock for interface %s on node %s", ocInterface, nodeName)

			// The feature requirement is to ensure that the system clock stays within 1.5 ms of true time
			// for 4 hours. The Intel documentation suggests 1.5 μs is feasible, so this should be easily
			// achieved.
			//
			// See documentation:
			// https://cdrdv2-public.intel.com/646265/646265_E810-XXVDA4T%20User%20Guide_Rev1.2.pdf.
			By("setting the ts2phc holdover to 4 hours")
			holdoverDurationSeconds := uint64(4 * 60 * 60)
			updateTime := time.Now()

			oldProfile, err := profiles.UpdateTS2PHCHoldover(
				RANConfig.Spoke1APIClient, ntpFallbackProfiles[0], holdoverDurationSeconds)
			Expect(err).ToNot(HaveOccurred(), "Failed to update ts2phc holdover for node %s", nodeName)

			waitForLoadAndTS2PHCLocked(prometheusAPI, nodeName, updateTime)

			By("simulating GNSS sync loss")
			DeferCleanup(cleanupGNSSSync(prometheusAPI, nodeName, protocolVersion))

			gnssLossTime := time.Now()
			err = gnss.SimulateSyncLoss(RANConfig.Spoke1APIClient, nodeName, protocolVersion)
			Expect(err).ToNot(HaveOccurred(), "Failed to simulate GNSS sync loss for node %s", nodeName)

			By("getting the event pod for the node")
			eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to get event pod for node %s", nodeName)

			By("waiting for sync-state HOLDOVER event")
			holdoverFilter := events.All(
				events.IsType(eventptp.PtpStateChange),
				events.HasValue(events.WithSyncState(eventptp.HOLDOVER)),
			)
			err = events.WaitForEvent(eventPod, gnssLossTime, 5*time.Minute, holdoverFilter)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for sync-state HOLDOVER event on node %s", nodeName)

			By("ensuring system clock is within 1.5 ms for entire holdover period")
			err = iface.PollPTPClockSystemTimeOffset(RANConfig.Spoke1APIClient, nodeName, ptpClockIndex,
				time.Duration(holdoverDurationSeconds)*time.Second, 1500*time.Microsecond)
			Expect(err).ToNot(HaveOccurred(), "Failed to poll PTP clock system time offset on node %s", nodeName)

			By("restoring GNSS sync")
			gnssRecoveryTime := time.Now()
			err = gnss.SimulateSyncRecovery(RANConfig.Spoke1APIClient, nodeName, protocolVersion)
			Expect(err).ToNot(HaveOccurred(), "Failed to simulate GNSS sync recovery for node %s", nodeName)

			By("waiting for os-clock-sync-state LOCKED event")
			osClockLockedFilter := events.All(
				events.IsType(eventptp.OsClockSyncStateChange),
				events.HasValue(events.WithSyncState(eventptp.LOCKED)),
			)
			err = events.WaitForEvent(eventPod, gnssRecoveryTime, 5*time.Minute, osClockLockedFilter)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for os-clock-sync-state LOCKED event on node %s", nodeName)

			By("restoring the ts2phc holdover")
			restoreTime := time.Now()
			changed, err := profiles.RestoreProfileToConfig(
				RANConfig.Spoke1APIClient, ntpFallbackProfiles[0].Reference, oldProfile)
			Expect(err).ToNot(HaveOccurred(), "Failed to restore NTP fallback profile for node %s", nodeName)

			if changed {
				waitForLoadAndTS2PHCLocked(prometheusAPI, nodeName, restoreTime)
			}

			// Only run once since this takes 4 hours to complete.
			break
		}

		if !testActuallyRan {
			Skip("No node found with both NTP fallback and OC profiles")
		}
	})
})

// cleanupGNSSSync restores GNSS sync and ensures the ts2phc process is locked on the given node. It returns a function
// that can be used as the argument to [DeferCleanup].
func cleanupGNSSSync(prometheusAPI prometheusv1.API, nodeName string, protocolVersion string) func() {
	return func() {
		if !CurrentSpecReport().Failed() {
			return
		}

		By("restoring GNSS sync")

		err := gnss.SimulateSyncRecovery(RANConfig.Spoke1APIClient, nodeName, protocolVersion)
		Expect(err).ToNot(HaveOccurred(), "Failed to simulate GNSS sync recovery for node %s", nodeName)

		By("ensuring ts2phc process is locked after restoring GNSS sync")
		ensureTS2PHCProcessLocked(prometheusAPI, nodeName)
	}
}

// waitForLoadAndTS2PHCLocked waits for the profile load and ensures the ts2phc process is locked on the given node.
func waitForLoadAndTS2PHCLocked(prometheusAPI prometheusv1.API, nodeName string, updateTime time.Time) {
	By("waiting for profile load after updating ts2phc holdover")

	profileLoadTime := time.Now()

	err := ptpdaemon.WaitForProfileLoadOnNodes(RANConfig.Spoke1APIClient, []string{nodeName},
		ptpdaemon.WithStartTime(updateTime),
		ptpdaemon.WithTimeout(5*time.Minute))
	Expect(err).ToNot(HaveOccurred(), "Failed to wait for profile load on node %s", nodeName)

	// If we do not wait for ts2phc to start properly after the profile load, we risk ending up in a situation where
	// holdover is not triggered properly.
	By("waiting for ts2phc to start after profile load")

	err = ptpdaemon.WaitForPodLog(
		RANConfig.Spoke1APIClient,
		nodeName,
		ptpdaemon.WithStartTime(profileLoadTime),
		ptpdaemon.WithTimeout(5*time.Minute),
		ptpdaemon.WithMatcher(ptpdaemon.ContainsMatcher("starting ts2phc")),
	)
	Expect(err).ToNot(HaveOccurred(), "Failed to find ts2phc start log on node %s", nodeName)

	By("ensuring ts2phc process is locked after profile load")
	ensureTS2PHCProcessLocked(prometheusAPI, nodeName)
}

// ensureTS2PHCProcessLocked ensures that the ts2phc process is locked on the given node.
func ensureTS2PHCProcessLocked(prometheusAPI prometheusv1.API, nodeName string) {
	query := metrics.ClockStateQuery{
		Node:    metrics.Equals(nodeName),
		Process: metrics.Equals(metrics.ProcessTS2PHC),
	}
	err := metrics.AssertQuery(context.TODO(), prometheusAPI, query, metrics.ClockStateLocked,
		metrics.AssertWithStableDuration(10*time.Second),
		metrics.AssertWithTimeout(5*time.Minute))
	Expect(err).ToNot(HaveOccurred(), "Failed to ensure ts2phc process is locked on node %s", nodeName)
}
