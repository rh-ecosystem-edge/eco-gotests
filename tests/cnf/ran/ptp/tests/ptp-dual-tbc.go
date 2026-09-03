package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

var _ = Describe("PTP Dual T-BC", Label(tsparams.LabelDualTBC), func() {
	var prometheusAPI prometheusv1.API

	BeforeEach(func() {
		var err error

		By("creating a Prometheus API client")

		prometheusAPI, err = querier.CreatePrometheusAPIForCluster(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Prometheus API client")

		By("checking if PTP operator version supports dual T-BC tests")

		inRange, err := version.IsVersionStringInRange(
			RANConfig.Spoke1OperatorVersions[ranparam.PTP], "5.0.0-0", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to parse PTP operator version")

		if !inRange {
			Skip("Test is valid from version 5.0")
		}
	})

	Context("dual t-bc dual time receiver ports on the same NIC", func() {
		var testData profiles.HoldoverTestData

		timeout := holdoverTestTimeout

		BeforeEach(func() {
			By("getting node info map")

			discovered := discoverHoldoverTestData(prometheusAPI, profiles.ProfileTypeDualTBCReceiver)
			if discovered == nil || len(discovered.UpstreamIfaces) < 2 {
				Skip("No dual T-BC configuration found for dual time receiver tests")
			}

			testData = *discovered

			klog.V(tsparams.LogLevel).Infof(
				"Dual T-BC test on node %s, upstream interfaces %v", testData.NodeName, testData.UpstreamIfaces)
		})

		// 89744 - validates dual t-bc holdover to locked after switching to second upstream port
		// when the active port is disconnected. OCP-89745 is the BC (non T-BC) counterpart
		// and is not automated here.
		It("validates dual t-bc holdover to locked after switching to second upstream port",
			reportxml.ID("89744"), func() {
				assertDualTBCFailoverToBackup(testData, timeout)
			})

		// 89746 - validates dual t-bc holdover and freerun after both ports are disconnected
		It("validates dual t-bc holdover and freerun after both ports are disconnected",
			reportxml.ID("89746"), func() {
				assertDualTBCHoldoverInSpecToFreerun(testData, profiles.HoldoverPluginSettingsNoOutOfSpec,
					timeout, profiles.TBCClockClasses())
			})
	})
})

// assertDualTBCFailoverToBackup brings the active (SLAVE) time-receiver port down, waits for HOLDOVER
// (clock class 135), then LOCKED (clock class 6) once the backup port takes over. The original port
// stays down for the duration of the assertions and is restored in cleanup.
func assertDualTBCFailoverToBackup(testData profiles.HoldoverTestData, timeout time.Duration) {
	GinkgoHelper()

	activeIface, backupIface := dualTBCActiveBackupInterfaces(testData)

	By(fmt.Sprintf("setting active upstream interface %s down to trigger failover to %s",
		activeIface, backupIface))

	ifaceDownTime := time.Now()

	DeferCleanup(func() {
		restoreInterfacesAndWaitForRelock(testData.PrometheusAPI, testData.NodeName, testData.UpstreamIfaces)
	})

	err := iface.SetInterfaceStatus(RANConfig.Spoke1APIClient, testData.NodeName,
		activeIface, iface.InterfaceStateDown)
	Expect(err).ToNot(HaveOccurred(),
		"Failed to set active upstream interface %s down on node %s", activeIface, testData.NodeName)

	assertHoldoverState(testData.PrometheusAPI, testData.NodeName, ifaceDownTime,
		profiles.TBCClockClasses().HoldoverInSpec, true, timeout, false, testData.ConfigFile)

	assertLockedState(testData.PrometheusAPI, testData.NodeName, ifaceDownTime,
		profiles.TBCClockClasses().Locked, true, timeout, testData.UpstreamIfaces, testData.ConfigFile)

	By("validating interface roles after failover")

	err = metrics.AssertQuery(context.TODO(), testData.PrometheusAPI, metrics.InterfaceRoleQuery{
		Interface: metrics.Equals(activeIface),
		Node:      metrics.Equals(testData.NodeName),
		Process:   metrics.Equals(metrics.ProcessPTP4L),
	}, metrics.InterfaceRoleFaulty, metrics.AssertWithTimeout(1*time.Minute))
	Expect(err).ToNot(HaveOccurred(),
		"expected interface %s to be FAULTY on node %s", activeIface, testData.NodeName)

	err = metrics.AssertQuery(context.TODO(), testData.PrometheusAPI, metrics.InterfaceRoleQuery{
		Interface: metrics.Equals(backupIface),
		Node:      metrics.Equals(testData.NodeName),
		Process:   metrics.Equals(metrics.ProcessPTP4L),
	}, metrics.InterfaceRoleFollower, metrics.AssertWithTimeout(1*time.Minute))
	Expect(err).ToNot(HaveOccurred(),
		"expected interface %s to be FOLLOWER on node %s", backupIface, testData.NodeName)
}

// assertDualTBCHoldoverInSpecToFreerun brings both time-receiver ports down, waits for HOLDOVER then FREERUN,
// and asserts both ports are FAULTY. Cleanup restores ports via DeferCleanup.
func assertDualTBCHoldoverInSpecToFreerun(
	testData profiles.HoldoverTestData,
	pluginSettings profiles.HoldoverPluginSettings,
	timeout time.Duration,
	expected profiles.HoldoverExpectedClockClasses,
) {
	GinkgoHelper()

	changeHoldoverSettings(testData, pluginSettings, expected.Locked, true, timeout)

	By("setting both upstream clock interfaces down to enter holdover-in-spec")

	ifaceDownTime := time.Now()

	DeferCleanup(func() {
		restoreInterfacesAndWaitForRelock(testData.PrometheusAPI, testData.NodeName, testData.UpstreamIfaces)
	})

	err := iface.SetInterfacesStatus(RANConfig.Spoke1APIClient,
		testData.NodeName, testData.UpstreamIfaces, iface.InterfaceStateDown)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interfaces down")

	assertHoldoverState(testData.PrometheusAPI, testData.NodeName, ifaceDownTime,
		expected.HoldoverInSpec, true, timeout, true, testData.ConfigFile)

	assertFreerunState(testData.PrometheusAPI, testData.NodeName, ifaceDownTime,
		expected.Freerun, true, timeout, testData.ConfigFile)

	By("validating both upstream interfaces are FAULTY")

	for _, ifn := range testData.UpstreamIfaces {
		err := metrics.AssertQuery(context.TODO(), testData.PrometheusAPI, metrics.InterfaceRoleQuery{
			Interface: metrics.Equals(ifn),
			Node:      metrics.Equals(testData.NodeName),
			Process:   metrics.Equals(metrics.ProcessPTP4L),
		}, metrics.InterfaceRoleFaulty, metrics.AssertWithTimeout(1*time.Minute))
		Expect(err).ToNot(HaveOccurred(),
			"expected interface %s to be FAULTY on node %s", ifn, testData.NodeName)
	}
}

// dualTBCActiveBackupInterfaces returns the currently active (FOLLOWER/SLAVE) and backup (LISTENING)
// time-receiver interfaces for the dual T-BC profile.
func dualTBCActiveBackupInterfaces(testData profiles.HoldoverTestData) (iface.Name, iface.Name) {
	GinkgoHelper()

	clientIfaces := testData.ProfileInfo.GetInterfacesByClockType(profiles.ClockTypeClient)
	Expect(len(clientIfaces)).To(Equal(2),
		"Expected exactly 2 client interfaces for dual T-BC profile %s on node %s",
		testData.ProfileInfo.Reference.ProfileName, testData.NodeName)

	activeIface, backupIface, err := profiles.DetermineActivePassiveInterfaces(
		context.TODO(), testData.PrometheusAPI, testData.NodeName, clientIfaces)
	Expect(err).ToNot(HaveOccurred(),
		"Failed to determine active/backup interfaces for dual T-BC on node %s", testData.NodeName)

	By(fmt.Sprintf("identified active interface: %s, backup interface: %s", activeIface, backupIface))

	return activeIface, backupIface
}
