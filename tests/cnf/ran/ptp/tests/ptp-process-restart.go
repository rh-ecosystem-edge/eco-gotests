package tests

import (
	"context"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	eventptp "github.com/redhat-cne/sdk-go/pkg/event/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/consumer"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/daemonlogs"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/events"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/processes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

var _ = Describe("PTP Process Restart", Label(tsparams.LabelProcessRestart), func() {
	var (
		prometheusAPI        prometheusv1.API
		savedPtpConfigs      []*ptp.PtpConfigBuilder
		configSupported      bool
		clockClass7Supported bool
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

		configSupported, err = version.IsVersionStringInRange(
			RANConfig.Spoke1OperatorVersions[ranparam.PTP], "4.21", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to check PTP version range")
		clockClass7Supported, err = version.IsVersionStringInRange(
			RANConfig.Spoke1OperatorVersions[ranparam.PTP], "4.18", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to check clock class 7 supported")
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
				// Timeouts may occur if the profiles changed do not apply to all PTP nodes, so we make
				// this non-fatal. This only happens in certain scenarios in MNO clusters.
				klog.V(tsparams.LogLevel).Infof("Failed to wait for profile load on PTP nodes: %v", err)
			}
		}

		By("ensuring clocks are locked after testing")
		err = metrics.EnsureClocksAreLocked(prometheusAPI)
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")
	})

	// 59862 - validate phc2sys process restart after killing that process
	It("should recover the phc2sys process after killing it", reportxml.ID("59862"), func() {
		testRanAtLeastOnce := false

		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for _, nodeInfo := range nodeInfoMap {
			testRanAtLeastOnce = true

			By("getting the event pod for the node " + nodeInfo.Name)
			eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeInfo.Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to get event pod for node %s", nodeInfo.Name)

			By("getting the phc2sys PID")
			oldPhc2sysPID, err := processes.GetPID(RANConfig.Spoke1APIClient, nodeInfo.Name, processes.Phc2sys)
			Expect(err).ToNot(HaveOccurred(), "Failed to get phc2sys PID for node %s", nodeInfo.Name)

			startTime := time.Now()

			By("killing the phc2sys process twice")
			err = processes.KillPtpProcessMultipleTimes(RANConfig.Spoke1APIClient, nodeInfo.Name, processes.Phc2sys, 2)
			Expect(err).ToNot(HaveOccurred(), "Failed to kill phc2sys process for node %s", nodeInfo.Name)

			By("waiting for the FREERUN event to be received for CLOCK_REALTIME")
			filter := events.All(
				events.IsType(eventptp.OsClockSyncStateChange),
				events.HasValue(events.WithSyncState(eventptp.FREERUN), events.OnInterface(iface.ClockRealtime)),
			)
			err = events.WaitForEvent(eventPod, startTime, 5*time.Minute, filter)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for free run event on node %s", nodeInfo.Name)

			By("waiting for the LOCKED event to be received for CLOCK_REALTIME")
			filter = events.All(
				events.IsType(eventptp.OsClockSyncStateChange),
				events.HasValue(events.WithSyncState(eventptp.LOCKED), events.OnInterface(iface.ClockRealtime)),
			)
			err = events.WaitForEvent(eventPod, startTime, 5*time.Minute, filter)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for locked event on node %s", nodeInfo.Name)

			By("getting the new phc2sys PID")
			newPhc2sysPID, err := processes.GetPID(RANConfig.Spoke1APIClient, nodeInfo.Name, processes.Phc2sys)
			Expect(err).ToNot(HaveOccurred(), "Failed to get phc2sys PID for node %s", nodeInfo.Name)
			Expect(newPhc2sysPID).NotTo(Equal(oldPhc2sysPID), "phc2sys PID did not change: "+oldPhc2sysPID)
		}

		if !testRanAtLeastOnce {
			Skip("No nodes to run the test on")
		}
	})

	// 57197 - Ptp4l restart - single process - Dual Nic
	It("ensures ptp4l is restarted after killing ptp4l unrelated to phc2sys", reportxml.ID("57197"), func() {
		testRanAtLeastOnce := false

		By("getting node info map")
		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for _, nodeInfo := range nodeInfoMap {
			By("checking if there are at least 2 profiles on node " + nodeInfo.Name)
			if len(nodeInfo.Profiles) < 2 {
				klog.V(tsparams.LogLevel).Infof("Skipping node %s because it has less than 2 profiles", nodeInfo.Name)

				continue
			}

			testRanAtLeastOnce = true

			By("updating the holdover timeout for all profiles on the node")
			oldHoldovers, err := profiles.SetHoldOverTimeouts(RANConfig.Spoke1APIClient, nodeInfo.Profiles, 180)
			Expect(err).ToNot(HaveOccurred(), "Failed to set holdover timeout for profiles on node %s", nodeInfo.Name)

			DeferCleanup(func() {
				By("resetting the holdover timeout for all profiles on the node")
				err = profiles.ResetHoldOverTimeouts(RANConfig.Spoke1APIClient, oldHoldovers)
				Expect(err).ToNot(HaveOccurred(), "Failed to reset holdover timeout for profiles on node %s", nodeInfo.Name)

				By("waiting for the holdover timeout to be reset to original values")
				err = profiles.WaitForOldHoldOverTimeouts(prometheusAPI, nodeInfo.Name, oldHoldovers, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for holdover timeout to be reset to original values")
			})

			By("waiting for the new holdover timeout to show up in the metrics")
			err = profiles.WaitForHoldOverTimeouts(
				prometheusAPI, nodeInfo.Name, nodeInfo.Profiles, 180, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for holdover timeout to be set to 180 after 5 minutes")

			By("getting the event pod for the node")
			eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeInfo.Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to get event pod for node %s", nodeInfo.Name)

			By("getting the original ptp4l and phc2sys PIDs")
			oldPtp4lPIDs, err := processes.GetPtp4lPIDsByRelatedProcess(
				RANConfig.Spoke1APIClient, nodeInfo.Name, processes.Phc2sys, false)
			Expect(err).ToNot(HaveOccurred(), "Failed to get ptp4l PIDs by related process for node %s", nodeInfo.Name)
			Expect(oldPtp4lPIDs).ToNot(BeEmpty(), "No ptp4l PIDs found for node %s", nodeInfo.Name)

			ptp4lPIDToKill := oldPtp4lPIDs[0]

			oldPhc2sysPID, err := processes.GetPID(RANConfig.Spoke1APIClient, nodeInfo.Name, processes.Phc2sys)
			Expect(err).ToNot(HaveOccurred(), "Failed to get phc2sys PID for node %s", nodeInfo.Name)

			startTime := time.Now()

			By("killing the first ptp4l process unrelated to the phc2sys process")
			err = processes.KillProcessByPID(RANConfig.Spoke1APIClient, nodeInfo.Name, ptp4lPIDToKill)
			Expect(err).ToNot(HaveOccurred(), "Failed to kill ptp4l process for node %s", nodeInfo.Name)

			By("waiting for the FREERUN event to be received after killing the ptp4l process for 4.19-")
			filter := events.All(
				events.IsType(eventptp.PtpStateChange),
				events.HasValue(events.WithSyncState(eventptp.FREERUN), events.ContainingResource(string(iface.Master))),
			)
			err = events.WaitForEvent(
				eventPod, startTime, 3*time.Minute, filter, events.WithoutCurrentState(true))
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for free run event on node %s", nodeInfo.Name)

			By("waiting for the LOCKED event to be received after killing the ptp4l process")
			filter = events.All(
				events.IsType(eventptp.PtpStateChange),
				events.HasValue(events.WithSyncState(eventptp.LOCKED), events.ContainingResource(string(iface.Master))),
			)
			err = events.WaitForEvent(
				eventPod, startTime, 3*time.Minute, filter, events.WithoutCurrentState(true))
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for locked event on node %s", nodeInfo.Name)

			By("ensuring the phc2sys process is not affected by killing the ptp4l process")
			newPhc2sysPID, err := processes.GetPID(RANConfig.Spoke1APIClient, nodeInfo.Name, processes.Phc2sys)
			Expect(err).ToNot(HaveOccurred(), "Failed to get phc2sys PID for node %s", nodeInfo.Name)
			Expect(newPhc2sysPID).To(Equal(oldPhc2sysPID), "phc2sys PID did not change: "+oldPhc2sysPID)

			By("ensuring a new ptp4l process is started")
			newPtp4lPIDs, err := processes.GetPtp4lPIDsByRelatedProcess(
				RANConfig.Spoke1APIClient, nodeInfo.Name, processes.Phc2sys, false)
			Expect(err).ToNot(HaveOccurred(), "Failed to get ptp4l PIDs by related process for node %s", nodeInfo.Name)
			Expect(newPtp4lPIDs).ToNot(BeEmpty(), "No new ptp4l PIDs found for node %s", nodeInfo.Name)
			Expect(newPtp4lPIDs).ToNot(ContainElement(ptp4lPIDToKill),
				"New ptp4l PIDs contain the PID that was killed: %s", ptp4lPIDToKill)

			By("resetting the holdover timeout for all profiles on the node")
			err = profiles.ResetHoldOverTimeouts(RANConfig.Spoke1APIClient, oldHoldovers)
			Expect(err).ToNot(HaveOccurred(), "Failed to reset holdover timeout for profiles on node %s", nodeInfo.Name)

			By("waiting for the old holdover timeout to show up in the metrics")
			err = profiles.WaitForOldHoldOverTimeouts(prometheusAPI, nodeInfo.Name, oldHoldovers, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for old holdover timeout to be set after 5 minutes")
		}

		if !testRanAtLeastOnce {
			Skip("No nodes with at least 2 profiles to run the test on")
		}
	})

	Context("GM process restart", func() {
		It("validates ts2phc process recover after a ts2phc process restart",
			reportxml.ID("59863"), func() {
				testRanAtLeastOnce := false

				nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

				for _, nodeInfo := range nodeInfoMap {
					// ts2phc runs only on T-GM nodes
					if nodeInfo.Counts[profiles.ProfileTypeMultiNICGM] == 0 &&
						nodeInfo.Counts[profiles.ProfileTypeGM] == 0 {
						continue
					}

					testRanAtLeastOnce = true

					By("getting the ts2phc PID")
					oldTS2phcPID, err := processes.GetPID(RANConfig.Spoke1APIClient, nodeInfo.Name, processes.Ts2phc)
					Expect(err).ToNot(HaveOccurred(), "Failed to get ts2phc PID for node %s", nodeInfo.Name)

					startTime := time.Now()

					By("killing the ts2phc process twice")
					err = processes.KillPtpProcessMultipleTimes(RANConfig.Spoke1APIClient,
						nodeInfo.Name, processes.Ts2phc, 2)
					Expect(err).ToNot(HaveOccurred(), "Failed to kill ts2phc process for node %s", nodeInfo.Name)

					By("getting the event pod for the node " + nodeInfo.Name)
					eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeInfo.Name)
					Expect(err).ToNot(HaveOccurred(), "Failed to get event pod for node %s", nodeInfo.Name)

					By("waiting for the FREERUN event to be received")
					filter := events.All(
						events.IsType(eventptp.PtpStateChange),
						events.HasValue(events.WithSyncState(eventptp.FREERUN), events.ContainingResource(string(iface.Master))),
					)
					err = events.WaitForEvent(
						eventPod, startTime, 3*time.Minute, filter,
						events.WithoutCurrentState(true))
					Expect(err).ToNot(HaveOccurred(), "Failed to wait for consumer free run event on node %s", nodeInfo.Name)

					clockClassFilter := events.All(
						events.IsType(eventptp.PtpClockClassChange),
						events.HasValue(
							events.WithMetric(248),
							events.ContainingResource(string(iface.Master)),
						),
					)

					By("waiting for a event.sync.ptp-status.ptp-clock-class-change to 248")
					err = events.WaitForEvent(
						eventPod, startTime, 3*time.Minute, clockClassFilter, events.WithoutCurrentState(true))
					Expect(err).ToNot(HaveOccurred(), "Failed to wait for consumer clock class change event on node %s", nodeInfo.Name)

					By("waiting for the ts2phc clock state to transition back to LOCKED")
					lockedQuery := metrics.ClockStateQuery{
						Node:    metrics.Equals(nodeInfo.Name),
						Process: metrics.Equals(metrics.ProcessTS2PHC),
					}
					err = metrics.AssertQuery(
						context.TODO(),
						prometheusAPI,
						lockedQuery,
						metrics.ClockStateLocked,
						metrics.AssertWithStableDuration(10*time.Second),
						metrics.AssertWithTimeout(5*time.Minute),
						metrics.AssertWithStartTime(startTime),
					)
					Expect(err).ToNot(HaveOccurred(), "Failed to assert ts2phc LOCKED state on node %s", nodeInfo.Name)

					By("verifying the ts2phc PID has changed")
					newTS2phcPID, err := processes.GetPID(RANConfig.Spoke1APIClient, nodeInfo.Name, processes.Ts2phc)
					Expect(err).ToNot(HaveOccurred(), "Failed to get ts2phc PID for node %s", nodeInfo.Name)
					Expect(newTS2phcPID).NotTo(Equal(oldTS2phcPID), "ts2phc PID did not change: "+oldTS2phcPID)

					filter = events.All(
						events.IsType(eventptp.PtpStateChange),
						events.HasValue(events.WithSyncState(eventptp.LOCKED), events.ContainingResource(string(iface.Master))),
					)

					By("waiting for the LOCKED event to be received")
					err = events.WaitForEvent(
						eventPod, startTime, 3*time.Minute, filter, events.WithoutCurrentState(true))
					Expect(err).ToNot(HaveOccurred(), "Failed to wait for consumer LOCKED event on node %s", nodeInfo.Name)

					clockClassFilter = events.All(
						events.IsType(eventptp.PtpClockClassChange),
						events.HasValue(
							events.WithMetric(6),
							events.ContainingResource(string(iface.Master)),
						),
					)
					By("waiting for a event.sync.ptp-status.ptp-clock-class-change to 6 on the consumer pod")
					err = events.WaitForEvent(
						eventPod, startTime, 3*time.Minute, clockClassFilter, events.WithoutCurrentState(true))
					Expect(err).ToNot(HaveOccurred(), "Failed to wait for consumer clock class change event on node %s", nodeInfo.Name)

					var configFile string
					if configSupported {
						configFile, err = processes.GetPtp4lConfigByRelatedProcess(
							RANConfig.Spoke1APIClient, nodeInfo.Name, processes.Ts2phc)
						Expect(err).ToNot(HaveOccurred(), "Failed to determine ptp4l config for node %s", nodeInfo.Name)
					}

					By("waiting for the ptp4l clock class to transition to 6 on metrics")
					clockClassQuery := metrics.ClockClassQuery{
						Node:    metrics.Equals(nodeInfo.Name),
						Process: metrics.Equals(metrics.ProcessPTP4L),
						Config:  metrics.Equals(configFile),
					}
					err = metrics.AssertQuery(
						context.TODO(),
						prometheusAPI,
						clockClassQuery,
						metrics.ClockClass6,
						metrics.AssertWithStableDuration(10*time.Second),
						metrics.AssertWithTimeout(5*time.Minute),
						metrics.AssertWithStartTime(startTime),
					)
					Expect(err).ToNot(HaveOccurred(), "Failed to assert clock class 6 state on node %s", nodeInfo.Name)
				}

				if !testRanAtLeastOnce {
					Skip("No GM nodes found")
				}

			})
	})
	It("Validates T-GM config ptp4l process recover after restart that process", reportxml.ID("59864"), func() {
		testRanAtLeastOnce := false
		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for _, nodeInfo := range nodeInfoMap {
			// Checks only ptp4l process related to ts2phc on T-GM nodes
			if nodeInfo.Counts[profiles.ProfileTypeMultiNICGM] == 0 &&
				nodeInfo.Counts[profiles.ProfileTypeGM] == 0 {
				continue
			}
			testRanAtLeastOnce = true

			By("getting the ptp4l PIDs by related to T-GM config")
			oldPtp4lPIDs, err := processes.GetPtp4lPIDsByRelatedProcess(
				RANConfig.Spoke1APIClient, nodeInfo.Name, processes.Ts2phc, true)
			Expect(err).ToNot(HaveOccurred(), "Failed to get ptp4l PIDs by related process for node %s", nodeInfo.Name)
			Expect(oldPtp4lPIDs).ToNot(BeEmpty(), "No ptp4l PIDs found for node %s", nodeInfo.Name)
			Expect(len(oldPtp4lPIDs)).To(Equal(1), "Expected 1 ptp4l PID related to ts2phc for node %s", nodeInfo.Name)

			ptp4lPIDToKill := oldPtp4lPIDs[0]
			By("killing ptp4l process related to T-GM config")
			err = processes.KillProcessByPID(RANConfig.Spoke1APIClient, nodeInfo.Name, ptp4lPIDToKill)
			Expect(err).ToNot(HaveOccurred(), "Failed to kill ptp4l process for node %s", nodeInfo.Name)

			By("waiting for the all clocks state to transition back to LOCKED")
			err = metrics.EnsureClocksAreLocked(prometheusAPI)
			Expect(err).ToNot(HaveOccurred(), "Failed to ensure clocks are LOCKED on node %s", nodeInfo.Name)

			By("validate a new ptp4l process is started")
			newPtp4lPIDs, err := processes.GetPtp4lPIDsByRelatedProcess(
				RANConfig.Spoke1APIClient, nodeInfo.Name, processes.Ts2phc, true)
			Expect(err).ToNot(HaveOccurred(), "Failed to get ptp4l PIDs by related process for node %s", nodeInfo.Name)
			Expect(newPtp4lPIDs).ToNot(BeEmpty(), "No ptp4l PIDs found for node %s", nodeInfo.Name)
			Expect(len(newPtp4lPIDs)).To(Equal(1), "Expected 1 ptp4l PID related to ts2phc for node %s", nodeInfo.Name)
			Expect(newPtp4lPIDs).NotTo(Equal(oldPtp4lPIDs), "ptp4l PID did not change: "+oldPtp4lPIDs[0])
		}

		if !testRanAtLeastOnce {
			Skip("No GM nodes found")
		}
	})

	It("Validates gpsd process recovery after restart that process", reportxml.ID("64777"), func() {
		testRanAtLeastOnce := false

		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for _, nodeInfo := range nodeInfoMap {
			// checks only gpsd process on GM nodes
			if nodeInfo.Counts[profiles.ProfileTypeMultiNICGM] == 0 &&
				nodeInfo.Counts[profiles.ProfileTypeGM] == 0 {
				continue
			}

			testRanAtLeastOnce = true

			By("getting the gpsd PID")
			oldGpsdPID, err := processes.GetPID(RANConfig.Spoke1APIClient, nodeInfo.Name, processes.Gpsd)
			Expect(err).ToNot(HaveOccurred(), "Failed to get gpsd PID for node %s", nodeInfo.Name)

			startTime := time.Now()

			By("killing the gpsd process twice")
			err = processes.KillPtpProcessMultipleTimes(RANConfig.Spoke1APIClient,
				nodeInfo.Name, processes.Gpsd, 2)
			Expect(err).ToNot(HaveOccurred(), "Failed to kill gpsd process for node %s", nodeInfo.Name)

			var clockClassFilter events.EventFilter
			clockClassValue := int64(7)
			if !clockClass7Supported {
				clockClassValue = int64(248)
			}
			clockClassFilter = events.All(
				events.IsType(eventptp.PtpClockClassChange),
				events.HasValue(
					events.WithMetric(clockClassValue),
					events.ContainingResource(string(iface.Master)),
				),
			)

			By("waiting for a event.sync.ptp-status.ptp-clock-class-change to " +
				strconv.FormatInt(clockClassValue, 10) + " on the consumer pod")
			eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeInfo.Name)

			Expect(err).ToNot(HaveOccurred(), "Failed to get event pod for node %s", nodeInfo.Name)
			err = events.WaitForEvent(
				eventPod, startTime, 3*time.Minute, clockClassFilter, events.WithoutCurrentState(true))
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for consumer clock class change event on node %s", nodeInfo.Name)

			By("waiting for the all clocks state to transition back to LOCKED")
			lockedQuery := metrics.ClockStateQuery{
				Node: metrics.Equals(nodeInfo.Name),
			}
			err = metrics.AssertQuery(
				context.TODO(),
				prometheusAPI,
				lockedQuery,
				metrics.ClockStateLocked,
				metrics.AssertWithStableDuration(10*time.Second),
				metrics.AssertWithTimeout(5*time.Minute),
				metrics.AssertWithStartTime(startTime),
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to assert all clocks are LOCKED on node %s", nodeInfo.Name)

			By("getting the new gpsd PID")
			newGpsdPID, err := processes.GetPID(RANConfig.Spoke1APIClient, nodeInfo.Name, processes.Gpsd)
			Expect(err).ToNot(HaveOccurred(), "Failed to get gpsd PID for node %s", nodeInfo.Name)
			Expect(newGpsdPID).NotTo(Equal(oldGpsdPID), "gpsd PID did not change: "+oldGpsdPID)

			clockClassFilter = events.All(
				events.IsType(eventptp.PtpClockClassChange),
				events.HasValue(
					events.WithMetric(6),
					events.ContainingResource(string(iface.Master)),
				),
			)

			By("waiting for event.sync.ptp-status.ptp-clock-class-change to 6")
			err = events.WaitForEvent(
				eventPod, startTime, 3*time.Minute, clockClassFilter, events.WithoutCurrentState(true))
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for consumer clock class change event on node %s", nodeInfo.Name)

			var configFile string
			if configSupported {
				configFile, err = processes.GetPtp4lConfigByRelatedProcess(
					RANConfig.Spoke1APIClient, nodeInfo.Name, processes.Ts2phc)
				Expect(err).ToNot(HaveOccurred(), "Failed to determine ptp4l config for node %s", nodeInfo.Name)
			}

			By("waiting for the ptp4l clock class to transition to 6 on metrics")
			clockClassQuery := metrics.ClockClassQuery{
				Node:    metrics.Equals(nodeInfo.Name),
				Process: metrics.Equals(metrics.ProcessPTP4L),
				Config:  metrics.Equals(configFile),
			}
			err = metrics.AssertQuery(
				context.TODO(),
				prometheusAPI,
				clockClassQuery,
				metrics.ClockClass6,
				metrics.AssertWithStableDuration(10*time.Second),
				metrics.AssertWithTimeout(5*time.Minute),
				metrics.AssertWithStartTime(startTime),
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to assert clock class 6 state on node %s", nodeInfo.Name)
		}

		if !testRanAtLeastOnce {
			Skip("No GM nodes found")
		}
	})
})
