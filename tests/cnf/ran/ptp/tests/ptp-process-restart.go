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
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/processes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

var _ = Describe("PTP Process Restart", Label(tsparams.LabelProcessRestart), func() {
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
			Expect(err).ToNot(HaveOccurred(), "Failed to kill phc2sys process for node %s", nodeInfo.Name)

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
			Skip("No nodes to run the test on")
		}
	})
})
