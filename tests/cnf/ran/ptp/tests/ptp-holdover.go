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
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/consumer"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/events"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

const holdoverTestTimeout = 7 * time.Minute

// TTSCClockClasses returns clock class values for T-TSC tests on 4.21+. Clock class does not change and
// remains 255 throughout all states.
func TTSCClockClasses() profiles.HoldoverExpectedClockClasses {
	return profiles.HoldoverExpectedClockClasses{
		Locked:            metrics.ClockClass255,
		HoldoverInSpec:    metrics.ClockClass255,
		HoldoverOutOfSpec: metrics.ClockClass255,
		Freerun:           metrics.ClockClass255,
	}
}

// backCompatTTSCClockClasses returns clock class values for T-TSC tests on 4.20, where T-TSC clock class
// values match T-BC (6, 135, 165, 248).
func backCompatTTSCClockClasses() profiles.HoldoverExpectedClockClasses {
	return profiles.TBCClockClasses()
}

var _ = Describe("PTP Holdover", Label(tsparams.LabelTBCTSCHoldover), func() {
	var prometheusAPI prometheusv1.API

	BeforeEach(func() {
		var err error

		By("creating a Prometheus API client")

		prometheusAPI, err = querier.CreatePrometheusAPIForCluster(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Prometheus API client")

		By("checking if PTP operator version supports holdover tests")

		inRange, err := version.IsVersionStringInRange(
			RANConfig.Spoke1OperatorVersions[ranparam.PTP], "4.20.0-0", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to parse PTP operator version")

		if !inRange {
			Skip("Test is valid from version 4.20")
		}
	})

	Context("t-bc upstream clock loss & unassisted holdover", func() {
		var testData profiles.HoldoverTestData

		timeout := holdoverTestTimeout

		BeforeEach(func() {
			By("getting node info map")

			discovered := discoverHoldoverTestData(prometheusAPI, profiles.ProfileTypeTBCReceiver)
			if discovered == nil {
				Skip("No T-BC configuration found for holdover tests")
			}

			testData = *discovered

			klog.V(tsparams.LogLevel).Infof(
				"T-BC holdover test on node %s, upstream interfaces %v", testData.NodeName, testData.UpstreamIfaces)

			assertDPLLPhaseLockedHoldoverAcquired(testData.PrometheusAPI, testData.NodeName, testData.UpstreamIfaces)
		})

		// 83297 - Verifies t-bc transition from holdover-in-spec to locked when upstream clock recovers
		It("verifies t-bc transition from holdover-in-spec to locked when upstream clock recovers",
			reportxml.ID("83297"), func() {
				assertHoldoverInSpecToLocked(testData, profiles.HoldoverPluginSettingsNoOutOfSpec,
					timeout, profiles.TBCClockClasses(), true)
			})

		// 83298 - Verifies t-bc transition from holdover-in-spec to freerun when localmaxholdoveroffset reached
		It("verifies t-bc transition from holdover-in-spec to freerun when localmaxholdoveroffset reached",
			reportxml.ID("83298"), func() {
				assertHoldoverInSpecToFreerun(testData, profiles.HoldoverPluginSettingsNoOutOfSpec,
					timeout, profiles.TBCClockClasses(), true)
			})

		// 83299 - Verifies t-bc transition from holdover-in-spec to holdover-out-of-spec when maxinspecoffset reached
		It("verifies t-bc transition from holdover-in-spec to holdover-out-of-spec when maxinspecoffset reached",
			reportxml.ID("83299"), func() {
				assertHoldoverInSpecToOutOfSpec(testData, profiles.HoldoverPluginSettingsWithOutOfSpec,
					timeout, profiles.TBCClockClasses(), true)
			})

		// 83300 - Verifies t-bc transition from holdover-out-of-spec to freerun when localmaxholdoveroffset reached
		It("verifies t-bc transition from holdover-out-of-spec to freerun when localmaxholdoveroffset reached",
			reportxml.ID("83300"), func() {
				assertHoldoverOutOfSpecToFreerun(testData, profiles.HoldoverPluginSettingsWithOutOfSpec,
					timeout, profiles.TBCClockClasses(), true)
			})
	})

	Context("t-tsc upstream clock loss & unassisted holdover", func() {
		var (
			testData             profiles.HoldoverTestData
			clockClassChanges    bool
			expectedClockClasses profiles.HoldoverExpectedClockClasses
		)

		timeout := holdoverTestTimeout

		BeforeEach(func() {
			is420, err := version.IsVersionStringInRange(
				RANConfig.Spoke1OperatorVersions[ranparam.PTP], "4.20.0-0", "4.21.0-0")
			Expect(err).ToNot(HaveOccurred(), "Failed to check PTP operator version range")

			if is420 {
				expectedClockClasses = backCompatTTSCClockClasses()
				clockClassChanges = true
			} else {
				expectedClockClasses = TTSCClockClasses()
				clockClassChanges = false
			}

			By("getting node info map")

			discovered := discoverHoldoverTestData(prometheusAPI, profiles.ProfileTypeTTSC)
			if discovered == nil {
				Skip("No T-TSC configuration found for holdover tests")
			}

			testData = *discovered

			klog.V(tsparams.LogLevel).Infof(
				"T-TSC holdover test on node %s, upstream interfaces %v", testData.NodeName, testData.UpstreamIfaces)

			assertDPLLPhaseLockedHoldoverAcquired(testData.PrometheusAPI, testData.NodeName, testData.UpstreamIfaces)
		})

		// 88274 - Verifies t-tsc transition from holdover-in-spec to locked when upstream clock recovers
		It("verifies t-tsc transition from holdover-in-spec to locked when upstream clock recovers",
			reportxml.ID("88274"), func() {
				assertHoldoverInSpecToLocked(testData, profiles.HoldoverPluginSettingsNoOutOfSpec,
					timeout, expectedClockClasses, clockClassChanges)
			})

		// 88275 - Verifies t-tsc transition from holdover-in-spec to freerun when localmaxholdoveroffset reached
		It("verifies t-tsc transition from holdover-in-spec to freerun when localmaxholdoveroffset reached",
			reportxml.ID("88275"), func() {
				assertHoldoverInSpecToFreerun(testData, profiles.HoldoverPluginSettingsNoOutOfSpec,
					timeout, expectedClockClasses, clockClassChanges)
			})

		// 88276 - Verifies t-tsc transition from holdover-in-spec to holdover-out-of-spec when maxinspecoffset reached
		It("verifies t-tsc transition from holdover-in-spec to holdover-out-of-spec when maxinspecoffset reached",
			reportxml.ID("88276"), func() {
				assertHoldoverInSpecToOutOfSpec(testData, profiles.HoldoverPluginSettingsWithOutOfSpec,
					timeout, expectedClockClasses, clockClassChanges)
			})

		// 88277 - Verifies t-tsc transition from holdover-out-of-spec to freerun when localmaxholdoveroffset reached
		It("verifies t-tsc transition from holdover-out-of-spec to freerun when localmaxholdoveroffset reached",
			reportxml.ID("88277"), func() {
				assertHoldoverOutOfSpecToFreerun(testData, profiles.HoldoverPluginSettingsWithOutOfSpec,
					timeout, expectedClockClasses, clockClassChanges)
			})
	})
})

// assertHoldoverInSpecToLocked validates that after upstream clock loss the clock enters holdover-in-spec,
// then recovers to locked when upstream is restored. No FREERUN events should be generated.
func assertHoldoverInSpecToLocked(
	testData profiles.HoldoverTestData,
	pluginSettings profiles.HoldoverPluginSettings,
	timeout time.Duration,
	expected profiles.HoldoverExpectedClockClasses,
	clockClassChanges bool,
) {
	GinkgoHelper()

	changeHoldoverSettings(testData, pluginSettings, expected.Locked, clockClassChanges, timeout)

	By("setting upstream clock interfaces down to enter holdover-in-spec")

	ifaceDownTime := time.Now()

	DeferCleanup(func() {
		restoreInterfacesAndWaitForRelock(testData.PrometheusAPI, testData.NodeName, testData.UpstreamIfaces)
	})

	err := iface.SetInterfacesStatus(RANConfig.Spoke1APIClient,
		testData.NodeName, testData.UpstreamIfaces, iface.InterfaceStateDown)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interfaces down")

	assertHoldoverState(testData.PrometheusAPI, testData.NodeName, ifaceDownTime,
		expected.HoldoverInSpec, clockClassChanges, timeout, true)

	By("setting upstream clock interfaces up to return to locked")

	ifaceUpTime := time.Now()

	err = iface.SetInterfacesStatus(RANConfig.Spoke1APIClient,
		testData.NodeName, testData.UpstreamIfaces, iface.InterfaceStateUp)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interfaces up")

	assertLockedState(testData.PrometheusAPI, testData.NodeName, ifaceUpTime,
		expected.Locked, clockClassChanges, timeout, testData.UpstreamIfaces)

	assertNoFreerunEvent(testData.NodeName, ifaceUpTime)
}

// assertHoldoverInSpecToFreerun validates that after upstream clock loss the clock enters holdover-in-spec,
// transitions to freerun when LocalMaxHoldoverOffSet is reached, then recovers to locked when upstream is restored.
func assertHoldoverInSpecToFreerun(
	testData profiles.HoldoverTestData,
	pluginSettings profiles.HoldoverPluginSettings,
	timeout time.Duration,
	expected profiles.HoldoverExpectedClockClasses,
	clockClassChanges bool,
) {
	GinkgoHelper()

	changeHoldoverSettings(testData, pluginSettings, expected.Locked, clockClassChanges, timeout)

	By("setting upstream clock interfaces down to enter holdover-in-spec")

	ifaceDownTime := time.Now()

	DeferCleanup(func() {
		restoreInterfacesAndWaitForRelock(testData.PrometheusAPI, testData.NodeName, testData.UpstreamIfaces)
	})

	err := iface.SetInterfacesStatus(RANConfig.Spoke1APIClient,
		testData.NodeName, testData.UpstreamIfaces, iface.InterfaceStateDown)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interfaces down")

	assertHoldoverState(testData.PrometheusAPI, testData.NodeName, ifaceDownTime,
		expected.HoldoverInSpec, clockClassChanges, timeout, true)

	assertFreerunState(testData.PrometheusAPI, testData.NodeName, ifaceDownTime,
		expected.Freerun, clockClassChanges, timeout)

	By("setting upstream clock interfaces up to return to locked")

	ifaceUpTime := time.Now()

	err = iface.SetInterfacesStatus(RANConfig.Spoke1APIClient,
		testData.NodeName, testData.UpstreamIfaces, iface.InterfaceStateUp)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interfaces up")

	assertLockedState(testData.PrometheusAPI, testData.NodeName, ifaceUpTime,
		expected.Locked, clockClassChanges, timeout, testData.UpstreamIfaces)
}

// assertHoldoverInSpecToOutOfSpec validates that after upstream clock loss the clock enters holdover-in-spec,
// transitions to holdover-out-of-spec when MaxInSpecOffset is reached, then recovers to locked when upstream
// is restored. No FREERUN events should be generated.
func assertHoldoverInSpecToOutOfSpec(
	testData profiles.HoldoverTestData,
	pluginSettings profiles.HoldoverPluginSettings,
	timeout time.Duration,
	expected profiles.HoldoverExpectedClockClasses,
	clockClassChanges bool,
) {
	GinkgoHelper()

	changeHoldoverSettings(testData, pluginSettings, expected.Locked, clockClassChanges, timeout)

	By("setting upstream clock interfaces down to enter holdover-in-spec")

	ifaceDownTime := time.Now()

	DeferCleanup(func() {
		restoreInterfacesAndWaitForRelock(testData.PrometheusAPI, testData.NodeName, testData.UpstreamIfaces)
	})

	err := iface.SetInterfacesStatus(RANConfig.Spoke1APIClient,
		testData.NodeName, testData.UpstreamIfaces, iface.InterfaceStateDown)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interfaces down")

	assertHoldoverState(testData.PrometheusAPI, testData.NodeName, ifaceDownTime,
		expected.HoldoverInSpec, clockClassChanges, timeout, true)

	assertHoldoverOutOfSpecClockClass(testData.PrometheusAPI, testData.NodeName, ifaceDownTime,
		expected.HoldoverOutOfSpec, clockClassChanges, timeout)

	By("setting upstream clock interfaces up to return to locked")

	ifaceUpTime := time.Now()

	err = iface.SetInterfacesStatus(RANConfig.Spoke1APIClient,
		testData.NodeName, testData.UpstreamIfaces, iface.InterfaceStateUp)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interfaces up")

	assertLockedState(testData.PrometheusAPI, testData.NodeName, ifaceUpTime,
		expected.Locked, clockClassChanges, timeout, testData.UpstreamIfaces)

	assertNoFreerunEvent(testData.NodeName, ifaceUpTime)
}

// assertHoldoverOutOfSpecToFreerun validates the full cascade: holdover-in-spec -> holdover-out-of-spec -> freerun,
// then recovery to locked when upstream is restored.
func assertHoldoverOutOfSpecToFreerun(
	testData profiles.HoldoverTestData,
	pluginSettings profiles.HoldoverPluginSettings,
	timeout time.Duration,
	expected profiles.HoldoverExpectedClockClasses,
	clockClassChanges bool,
) {
	GinkgoHelper()

	changeHoldoverSettings(testData, pluginSettings, expected.Locked, clockClassChanges, timeout)

	By("setting upstream clock interfaces down to enter holdover-in-spec")

	ifaceDownTime := time.Now()

	DeferCleanup(func() {
		restoreInterfacesAndWaitForRelock(testData.PrometheusAPI, testData.NodeName, testData.UpstreamIfaces)
	})

	err := iface.SetInterfacesStatus(RANConfig.Spoke1APIClient,
		testData.NodeName, testData.UpstreamIfaces, iface.InterfaceStateDown)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interfaces down")

	assertHoldoverState(testData.PrometheusAPI, testData.NodeName, ifaceDownTime,
		expected.HoldoverInSpec, clockClassChanges, timeout, true)

	assertHoldoverOutOfSpecClockClass(testData.PrometheusAPI, testData.NodeName, ifaceDownTime,
		expected.HoldoverOutOfSpec, clockClassChanges, timeout)

	assertFreerunState(testData.PrometheusAPI, testData.NodeName, ifaceDownTime,
		expected.Freerun, clockClassChanges, timeout)

	By("setting upstream clock interfaces up to return to locked")

	ifaceUpTime := time.Now()

	err = iface.SetInterfacesStatus(RANConfig.Spoke1APIClient,
		testData.NodeName, testData.UpstreamIfaces, iface.InterfaceStateUp)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interfaces up")

	assertLockedState(testData.PrometheusAPI, testData.NodeName, ifaceUpTime,
		expected.Locked, clockClassChanges, timeout, testData.UpstreamIfaces)
}

// assertHoldoverState waits for the HOLDOVER event and optional clock class change event, then validates
// the corresponding Prometheus metrics when checkMetrics is true. When checkMetrics is false the function
// returns after the event checks, skipping the Prometheus metric assertions. This is useful for scenarios
// such as dual T-BC failover where the clock may re-lock from a backup upstream port before the metric
// query runs.
func assertHoldoverState(
	prometheusAPI prometheusv1.API,
	nodeName string,
	sinceTime time.Time,
	expectedClockClass metrics.PtpClockClass,
	clockClassChanges bool,
	timeout time.Duration,
	checkMetrics bool,
) {
	GinkgoHelper()

	By("waiting for clock state HOLDOVER event")

	eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
	Expect(err).ToNot(HaveOccurred(), "Failed to get consumer pod for node")

	holdoverFilter := events.All(
		events.IsType(eventptp.PtpStateChange),
		events.HasValue(events.WithSyncState(eventptp.HOLDOVER)),
	)
	err = events.WaitForEvent(eventPod, sinceTime, timeout, holdoverFilter)
	Expect(err).ToNot(HaveOccurred(), "Failed to wait for HOLDOVER event")

	if clockClassChanges {
		By(fmt.Sprintf("waiting for clock class %d event", expectedClockClass))

		clockClassFilter := events.All(
			events.IsType(eventptp.PtpClockClassChange),
			events.HasValue(events.WithMetric(int64(expectedClockClass))),
		)
		err = events.WaitForEvent(eventPod, sinceTime, timeout, clockClassFilter)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for clock class %d event", expectedClockClass)
	}

	if !checkMetrics {
		return
	}

	By(fmt.Sprintf("validating metrics: clock class %d, clock state HOLDOVER", expectedClockClass))

	clockStateQuery := metrics.ClockStateQuery{
		Node:    metrics.Equals(nodeName),
		Process: metrics.Equals(metrics.ProcessTBC),
	}
	err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockStateQuery, metrics.ClockStateHoldover,
		metrics.AssertWithTimeout(1*time.Minute))
	Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state HOLDOVER in metrics")

	clockClassQuery := metrics.ClockClassQuery{
		Node:    metrics.Equals(nodeName),
		Process: metrics.Equals(metrics.ProcessPTP4L),
	}
	err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockClassQuery, expectedClockClass,
		metrics.AssertWithTimeout(1*time.Minute))
	Expect(err).ToNot(HaveOccurred(), "Failed to assert clock class %d in metrics", expectedClockClass)
}

// assertLockedState waits for the LOCKED event and optional clock class change event, then validates
// the corresponding Prometheus metrics.
func assertLockedState(
	prometheusAPI prometheusv1.API,
	nodeName string,
	sinceTime time.Time,
	expectedClockClass metrics.PtpClockClass,
	clockClassChanges bool,
	timeout time.Duration,
	upstreamIfaces []iface.Name,
) {
	GinkgoHelper()

	By("waiting for clock state LOCKED event")

	eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
	Expect(err).ToNot(HaveOccurred(), "Failed to get consumer pod for node")

	lockedFilter := events.All(
		events.IsType(eventptp.PtpStateChange),
		events.HasValue(events.WithSyncState(eventptp.LOCKED)),
	)
	err = events.WaitForEvent(eventPod, sinceTime, timeout, lockedFilter)
	Expect(err).ToNot(HaveOccurred(), "Failed to wait for LOCKED event")

	if clockClassChanges {
		By(fmt.Sprintf("waiting for clock class %d event", expectedClockClass))

		clockClassFilter := events.All(
			events.IsType(eventptp.PtpClockClassChange),
			events.HasValue(events.WithMetric(int64(expectedClockClass))),
		)
		err = events.WaitForEvent(eventPod, sinceTime, timeout, clockClassFilter)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for clock class %d event", expectedClockClass)
	}

	By(fmt.Sprintf("validating metrics: clock class %d, clock state LOCKED", expectedClockClass))

	clockStateQuery := metrics.ClockStateQuery{
		Node:    metrics.Equals(nodeName),
		Process: metrics.Equals(metrics.ProcessTBC),
	}
	err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockStateQuery, metrics.ClockStateLocked,
		metrics.AssertWithTimeout(1*time.Minute))
	Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state LOCKED in metrics")

	clockClassQuery := metrics.ClockClassQuery{
		Node:    metrics.Equals(nodeName),
		Process: metrics.Equals(metrics.ProcessPTP4L),
	}
	err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockClassQuery, expectedClockClass,
		metrics.AssertWithTimeout(1*time.Minute))
	Expect(err).ToNot(HaveOccurred(), "Failed to assert clock class %d in metrics", expectedClockClass)

	assertDPLLPhaseLockedHoldoverAcquired(prometheusAPI, nodeName, upstreamIfaces)
}

// assertFreerunState waits for the FREERUN event and optional clock class change event, then validates
// the corresponding Prometheus metrics.
func assertFreerunState(
	prometheusAPI prometheusv1.API,
	nodeName string,
	sinceTime time.Time,
	expectedClockClass metrics.PtpClockClass,
	clockClassChanges bool,
	timeout time.Duration,
) {
	GinkgoHelper()

	By("waiting for clock state FREERUN event")

	eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
	Expect(err).ToNot(HaveOccurred(), "Failed to get consumer pod for node")

	freerunFilter := events.All(
		events.IsType(eventptp.PtpStateChange),
		events.HasValue(events.WithSyncState(eventptp.FREERUN)),
	)
	err = events.WaitForEvent(eventPod, sinceTime, timeout, freerunFilter)
	Expect(err).ToNot(HaveOccurred(), "Failed to wait for FREERUN event")

	if clockClassChanges {
		By(fmt.Sprintf("waiting for clock class %d event", expectedClockClass))

		clockClassFilter := events.All(
			events.IsType(eventptp.PtpClockClassChange),
			events.HasValue(events.WithMetric(int64(expectedClockClass))),
		)
		err = events.WaitForEvent(eventPod, sinceTime, timeout, clockClassFilter)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for clock class %d event", expectedClockClass)
	}

	By("waiting for os-clock-sync-state FREERUN event for CLOCK_REALTIME")

	osClockFreerunFilter := events.All(
		events.IsType(eventptp.OsClockSyncStateChange),
		events.HasValue(events.WithSyncState(eventptp.FREERUN), events.OnInterface(iface.ClockRealtime)),
	)
	err = events.WaitForEvent(eventPod, sinceTime, timeout, osClockFreerunFilter)
	Expect(err).ToNot(HaveOccurred(),
		"Failed to wait for os-clock-sync-state FREERUN event for CLOCK_REALTIME on node %s", nodeName)

	By(fmt.Sprintf("validating metrics: clock class %d, clock state FREERUN", expectedClockClass))

	clockStateQuery := metrics.ClockStateQuery{
		Node:    metrics.Equals(nodeName),
		Process: metrics.Equals(metrics.ProcessTBC),
	}
	err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockStateQuery, metrics.ClockStateFreerun,
		metrics.AssertWithTimeout(1*time.Minute))
	Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state FREERUN in metrics")

	clockClassQuery := metrics.ClockClassQuery{
		Node:    metrics.Equals(nodeName),
		Process: metrics.Equals(metrics.ProcessPTP4L),
	}
	err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockClassQuery, expectedClockClass,
		metrics.AssertWithTimeout(1*time.Minute))
	Expect(err).ToNot(HaveOccurred(), "Failed to assert clock class %d in metrics", expectedClockClass)
}

// assertHoldoverOutOfSpecClockClass waits for the clock class to transition to holdover-out-of-spec and
// validates that the clock state remains HOLDOVER in metrics.
func assertHoldoverOutOfSpecClockClass(
	prometheusAPI prometheusv1.API,
	nodeName string,
	sinceTime time.Time,
	expectedClockClass metrics.PtpClockClass,
	clockClassChanges bool,
	timeout time.Duration,
) {
	GinkgoHelper()

	if clockClassChanges {
		By(fmt.Sprintf("waiting for clock class %d event (holdover-out-of-spec)", expectedClockClass))

		eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
		Expect(err).ToNot(HaveOccurred(), "Failed to get consumer pod for node")

		clockClassFilter := events.All(
			events.IsType(eventptp.PtpClockClassChange),
			events.HasValue(events.WithMetric(int64(expectedClockClass))),
		)
		err = events.WaitForEvent(eventPod, sinceTime, timeout, clockClassFilter)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for clock class %d event", expectedClockClass)
	}

	By(fmt.Sprintf("validating metrics: clock class %d, clock state HOLDOVER", expectedClockClass))

	clockStateQuery := metrics.ClockStateQuery{
		Node:    metrics.Equals(nodeName),
		Process: metrics.Equals(metrics.ProcessTBC),
	}
	err := metrics.AssertQuery(context.TODO(), prometheusAPI, clockStateQuery, metrics.ClockStateHoldover,
		metrics.AssertWithTimeout(1*time.Minute))
	Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state HOLDOVER in metrics")

	clockClassQuery := metrics.ClockClassQuery{
		Node:    metrics.Equals(nodeName),
		Process: metrics.Equals(metrics.ProcessPTP4L),
	}
	err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockClassQuery, expectedClockClass,
		metrics.AssertWithTimeout(1*time.Minute))
	Expect(err).ToNot(HaveOccurred(), "Failed to assert clock class %d in metrics", expectedClockClass)
}

// assertDPLLPhaseLockedHoldoverAcquired validates that the DPLL phase status is LOCKED_HO_ACQ on every upstream NIC.
func assertDPLLPhaseLockedHoldoverAcquired(
	prometheusAPI prometheusv1.API, nodeName string, upstreamIfaces []iface.Name,
) {
	GinkgoHelper()

	for nicName := range iface.GroupInterfacesByNIC(upstreamIfaces) {
		By(fmt.Sprintf("validating metrics: DPLL phase LOCKED_HO_ACQ on interface %s", nicName))

		phaseStatusQuery := metrics.PhaseStatusQuery{
			Node:      metrics.Equals(nodeName),
			Interface: metrics.Equals(nicName),
			From:      metrics.Equals(metrics.ProcessDPLL),
			Process:   metrics.Equals(metrics.ProcessDPLL),
		}
		err := metrics.AssertQuery(
			context.TODO(), prometheusAPI, phaseStatusQuery, metrics.PhaseStateLockedHoldoverAcquired,
			metrics.AssertWithTimeout(1*time.Minute))
		Expect(err).ToNot(HaveOccurred(), "Failed to assert DPLL phase state LOCKED_HO_ACQ on interface %s", nicName)
	}
}

// assertNoFreerunEvent validates that no FREERUN ptp-state-change event is generated within 30 seconds.
func assertNoFreerunEvent(nodeName string, sinceTime time.Time) {
	GinkgoHelper()

	By("validating no FREERUN event is generated")

	eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
	Expect(err).ToNot(HaveOccurred(), "Failed to get consumer pod for node")

	freerunFilter := events.All(
		events.IsType(eventptp.PtpStateChange),
		events.HasValue(events.WithSyncState(eventptp.FREERUN)),
	)
	err = events.WaitForEvent(eventPod, sinceTime, 30*time.Second, freerunFilter)
	Expect(err).To(HaveOccurred(), "Expected no FREERUN event, but one was received")
}

// changeHoldoverSettings updates holdover thresholds and waits for the daemon to re-lock.
// Routes through HardwareConfig CR on 4.22+/GNRD or PtpConfig plugin on pre-4.22.
// No-op when desired already matches current; restores originals via DeferCleanup.
func changeHoldoverSettings(
	testData profiles.HoldoverTestData,
	desired profiles.HoldoverPluginSettings,
	expectedLockedClass metrics.PtpClockClass,
	clockClassChanges bool,
	timeout time.Duration,
) {
	GinkgoHelper()

	current, err := profiles.GetHoldoverSettings(RANConfig.Spoke1APIClient, testData.ProfileInfo)
	Expect(err).ToNot(HaveOccurred(), "Failed to get current holdover settings")

	if desired == *current {
		return
	}

	original := *current

	By("setting test case holdover settings")

	err = profiles.ApplyHoldoverSettings(RANConfig.Spoke1APIClient, testData.ProfileInfo, desired)
	Expect(err).ToNot(HaveOccurred(), "Failed to apply holdover settings")

	DeferCleanup(func() {
		By("restoring original holdover settings")

		restoreErr := profiles.ApplyHoldoverSettings(RANConfig.Spoke1APIClient, testData.ProfileInfo, original)
		Expect(restoreErr).ToNot(HaveOccurred(), "Failed to restore holdover settings")

		restoreTime := time.Now()

		restoreErr = profiles.WaitForHoldoverSettingsApplied(
			RANConfig.Spoke1APIClient, testData.NodeName, testData.ProfileInfo, restoreTime, 5*time.Minute)
		Expect(restoreErr).ToNot(HaveOccurred(), "Daemon did not reload after restore")

		restoreTime = time.Now()

		assertLockedState(testData.PrometheusAPI, testData.NodeName, restoreTime,
			expectedLockedClass, clockClassChanges, timeout, testData.UpstreamIfaces)
	})

	setTime := time.Now()

	err = profiles.WaitForHoldoverSettingsApplied(
		RANConfig.Spoke1APIClient, testData.NodeName, testData.ProfileInfo, setTime, 3*time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Daemon did not reload after config change")

	setTime = time.Now()

	assertLockedState(testData.PrometheusAPI, testData.NodeName, setTime,
		expectedLockedClass, clockClassChanges, timeout, testData.UpstreamIfaces)
}

// discoverHoldoverTestData finds the first node with a matching profile type that supports holdover tests
// and returns the test context. Returns nil if no suitable profile is found.
// Without a HardwareConfig CR, only E810 supports holdover via the plugin path.
func discoverHoldoverTestData(
	prometheusAPI prometheusv1.API,
	profileType profiles.PtpProfileType,
) *profiles.HoldoverTestData {
	nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

	for name, nodeInfo := range nodeInfoMap {
		for _, profileInfo := range nodeInfo.GetProfilesByTypes(profileType) {
			ptpProfile, pullErr := profileInfo.PullProfile(RANConfig.Spoke1APIClient)
			Expect(pullErr).ToNot(HaveOccurred())

			if profileInfo.HardwareConfig == nil && !profiles.HasPlugin(ptpProfile, ptp.PluginTypeE810) {
				klog.V(tsparams.LogLevel).Infof(
					"Skipping profile %s on node %s: unsupported holdover path: %+v",
					profileInfo.Reference.ProfileName, name, ptpProfile.Plugins)

				continue
			}

			ports, portErr := profiles.GetUpstreamPortsForProfile(ptpProfile)
			if portErr != nil {
				klog.V(tsparams.LogLevel).Infof(
					"Skipping profile %s on node %s: cannot determine upstream ports: %v",
					profileInfo.Reference.ProfileName, name, portErr)

				continue
			}

			return &profiles.HoldoverTestData{
				PrometheusAPI:  prometheusAPI,
				NodeName:       name,
				ProfileInfo:    profileInfo,
				UpstreamIfaces: ports,
			}
		}
	}

	return nil
}

// restoreInterfacesAndWaitForRelock brings the upstream interfaces back up and waits for the T-BC clock to return
// to LOCKED state with a 5-second stable duration, matching the OC 2-port restore pattern. The process label
// is T-BC because the linuxptp-daemon uses the clockType from PtpSettings (set to "T-BC" for T-BC profiles)
// as the process label for clock state metrics. T-TSC profiles use the same shared helpers and the same process
// label because cnf-gotests uses ProcessTBC for both T-BC and T-TSC holdover metric assertions.
func restoreInterfacesAndWaitForRelock(
	prometheusAPI prometheusv1.API,
	nodeName string,
	upstreamIfaces []iface.Name,
) {
	GinkgoHelper()

	By("restoring upstream interfaces and waiting for relock")

	err := iface.SetInterfacesStatus(RANConfig.Spoke1APIClient, nodeName, upstreamIfaces, iface.InterfaceStateUp)
	Expect(err).ToNot(HaveOccurred(), "Failed to restore upstream clock interfaces")

	clockStateQuery := metrics.ClockStateQuery{
		Node:    metrics.Equals(nodeName),
		Process: metrics.Equals(metrics.ProcessTBC),
	}
	err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockStateQuery, metrics.ClockStateLocked,
		metrics.AssertWithStableDuration(5*time.Second),
		metrics.AssertWithTimeout(3*time.Minute))
	Expect(err).ToNot(HaveOccurred(), "Clock did not return to LOCKED after restoration")
}
