package tests

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/client"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/clock"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

const holdoverTestTimeout = 10 * time.Minute

// holdoverParametersNoOutOfSpec sets MaxInSpecOffset above LocalMaxHoldoverOffSet, so holdover-out-of-spec
// is never entered; holdoverParametersWithOutOfSpec sets it below, so it always is.

var (
	holdoverParametersNoOutOfSpec = clock.HoldoverParameters{
		LocalHoldoverTimeout:   360,
		MaxInSpecOffset:        14401,
		LocalMaxHoldoverOffSet: 14400,
	}
	holdoverParametersWithOutOfSpec = clock.HoldoverParameters{
		LocalHoldoverTimeout:   360,
		MaxInSpecOffset:        1800,
		LocalMaxHoldoverOffSet: 14400,
	}
)

var _ = Describe("PTP Holdover", Label(tsparams.LabelTBCTSCHoldover), func() {
	var clusterSnapshot *client.PtpClusterSnapshot

	BeforeEach(func() {
		By("checking if PTP operator version supports holdover tests")

		inRange, err := cluster.PtpOperatorVersion("4.20.0-0", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to parse PTP operator version")

		if !inRange {
			Skip("Test is valid from version 4.20")
		}

		By("ensuring clocks are locked before testing")

		err = client.WaitForLockedClocks()
		Expect(err).ToNot(HaveOccurred(), "Failed to assert all clocks state are locked")
	})

	AfterEach(func() {
		if CurrentSpecReport().State == types.SpecStateSkipped {
			return
		}

		By("restoring cluster state after testing")

		err := client.Restore(clusterSnapshot)
		Expect(err).ToNot(HaveOccurred(), "Failed to restore cluster state")

		By("ensuring clocks are locked after testing")

		err = client.WaitForLockedClocks()
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")
	})

	Context("t-bc upstream clock loss & unassisted holdover", func() {
		var tbcHoClk *clock.Clock

		timeout := holdoverTestTimeout

		BeforeEach(func() {
			By("Holdover Capable T-BC")

			clks, err := client.DeriveAllClusterClocks()
			Expect(err).ToNot(HaveOccurred(), "Failed to derive PTP cluster Clocks")

			tbcClks, tbcOk := clks.OfType(clock.ClockTypeTBC)
			if !tbcOk {
				Skip("No T-BC configuration found for T-BC tests")
			}

			tbcHoClks, hoOk := tbcClks.HoldoverEnabled()
			if !hoOk {
				Skip("No T-BC which is holdover capable found for holdover tests")
			}

			tbcHoClk = tbcHoClks[0]

			clusterSnapshot, err = client.Snapshot()
			Expect(err).ToNot(HaveOccurred(), "Failed to snapshot cluster state")

			klog.V(tsparams.LogLevel).Infof("T-BC holdover test on node %s, upstream interfaces %v",
				tbcHoClk.NodeName, clock.InterfaceNames(tbcHoClk.TimeReceiverIfaces()))
		})

		// 83297 - Verifies t-bc transition from holdover-in-spec to locked when upstream clock recovers
		It("verifies t-bc transition from holdover-in-spec to locked when upstream clock recovers",
			reportxml.ID("83297"), func() {
				assertHoldoverInSpecToLocked(tbcHoClk, holdoverParametersNoOutOfSpec, timeout)
			})

		// 83298 - Verifies t-bc transition from holdover-in-spec to freerun when localmaxholdoveroffset reached
		It("verifies t-bc transition from holdover-in-spec to freerun when localmaxholdoveroffset reached",
			reportxml.ID("83298"), func() {
				assertHoldoverInSpecToFreerun(tbcHoClk, holdoverParametersNoOutOfSpec, timeout)
			})

		// 83299 - Verifies t-bc transition from holdover-in-spec to holdover-out-of-spec when maxinspecoffset reached
		It("verifies t-bc transition from holdover-in-spec to holdover-out-of-spec when maxinspecoffset reached",
			reportxml.ID("83299"), func() {
				assertHoldoverInSpecToOutOfSpec(tbcHoClk, holdoverParametersWithOutOfSpec, timeout)
			})

		// 83300 - Verifies t-bc transition from holdover-out-of-spec to freerun when localmaxholdoveroffset reached
		It("verifies t-bc transition from holdover-out-of-spec to freerun when localmaxholdoveroffset reached",
			reportxml.ID("83300"), func() {
				assertHoldoverOutOfSpecToFreerun(tbcHoClk, holdoverParametersWithOutOfSpec, timeout)
			})
	})

	Context("t-tsc upstream clock loss & unassisted holdover", func() {
		var tscHoClk *clock.Clock

		timeout := holdoverTestTimeout

		BeforeEach(func() {
			By("Holdover Capable T-TSC")

			clks, err := client.DeriveAllClusterClocks()
			Expect(err).ToNot(HaveOccurred(), "Failed to derive PTP cluster Clocks")

			ttscClocks, ttscClksOk := clks.OfType(clock.ClockTypeTTSC)
			if !ttscClksOk {
				Skip("No T-TSC configuration found for T-TSC tests")
			}

			tscHoClks, hoOk := ttscClocks.HoldoverEnabled()
			if !hoOk {
				Skip("No T-TSC which is holdover capable found for holdover tests")
			}

			tscHoClk = tscHoClks[0]

			clusterSnapshot, err = client.Snapshot()
			Expect(err).ToNot(HaveOccurred(), "Failed to snapshot cluster state")

			klog.V(tsparams.LogLevel).Infof("T-TSC holdover test on node %s, upstream interfaces %v",
				tscHoClk.NodeName, clock.InterfaceNames(tscHoClk.TimeReceiverIfaces()))
		})

		// 88274 - Verifies t-tsc transition from holdover-in-spec to locked when upstream clock recovers
		It("verifies t-tsc transition from holdover-in-spec to locked when upstream clock recovers",
			reportxml.ID("88274"), func() {
				assertHoldoverInSpecToLocked(tscHoClk, holdoverParametersNoOutOfSpec, timeout)
			})

		// 88275 - Verifies t-tsc transition from holdover-in-spec to freerun when localmaxholdoveroffset reached
		It("verifies t-tsc transition from holdover-in-spec to freerun when localmaxholdoveroffset reached",
			reportxml.ID("88275"), func() {
				assertHoldoverInSpecToFreerun(tscHoClk, holdoverParametersNoOutOfSpec, timeout)
			})

		// 88276 - Verifies t-tsc transition from holdover-in-spec to holdover-out-of-spec when maxinspecoffset reached
		It("verifies t-tsc transition from holdover-in-spec to holdover-out-of-spec when maxinspecoffset reached",
			reportxml.ID("88276"), func() {
				assertHoldoverInSpecToOutOfSpec(tscHoClk, holdoverParametersWithOutOfSpec, timeout)
			})

		// 88277 - Verifies t-tsc transition from holdover-out-of-spec to freerun when localmaxholdoveroffset reached
		It("verifies t-tsc transition from holdover-out-of-spec to freerun when localmaxholdoveroffset reached",
			reportxml.ID("88277"), func() {
				assertHoldoverOutOfSpecToFreerun(tscHoClk, holdoverParametersWithOutOfSpec, timeout)
			})
	})
})

// assertHoldoverInSpecToLocked validates that after upstream clock loss the clock enters holdover-in-spec,
// then recovers to locked when upstream is restored. No FREERUN events should be generated.
func assertHoldoverInSpecToLocked(
	clk *clock.Clock,
	holdoverParams clock.HoldoverParameters,
	timeout time.Duration,
) {
	GinkgoHelper()

	err := client.ChangeHoldoverParameters(clk, holdoverParams, timeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to change holdover settings")

	By("setting upstream clock interface down to enter holdover-in-spec")

	ifaceDownTime := time.Now()

	err = client.SetUpstreamInterfaces(clk, iface.InterfaceStateDown)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interface down")

	err = client.WaitForHoldover(clk, ifaceDownTime, timeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to assert holdover-in-spec")

	By("setting upstream clock interface up to return to locked")

	ifaceUpTime := time.Now()

	err = client.SetUpstreamInterfaces(clk, iface.InterfaceStateUp)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interface up")

	err = client.WaitForLockedFromHoldover(clk, ifaceUpTime, timeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to assert clean transition from holdover to locked")
}

// assertHoldoverInSpecToFreerun validates that after upstream clock loss the clock enters holdover-in-spec,
// transitions to freerun when LocalMaxHoldoverOffSet is reached, then recovers to locked when upstream is restored.
func assertHoldoverInSpecToFreerun(
	clk *clock.Clock,
	holdoverParams clock.HoldoverParameters,
	timeout time.Duration,
) {
	GinkgoHelper()

	err := client.ChangeHoldoverParameters(clk, holdoverParams, timeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to change holdover settings")

	By("setting upstream clock interface down to enter holdover-in-spec")

	ifaceDownTime := time.Now()

	err = client.SetUpstreamInterfaces(clk, iface.InterfaceStateDown)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interface down")

	err = client.WaitForHoldover(clk, ifaceDownTime, timeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to assert holdover-in-spec")

	err = client.WaitForFreerun(clk, ifaceDownTime, timeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to assert freerun")

	By("setting upstream clock interface up to return to locked")

	ifaceUpTime := time.Now()

	err = client.SetUpstreamInterfaces(clk, iface.InterfaceStateUp)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interface up")

	err = client.WaitForLocked(clk, ifaceUpTime, timeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to assert locked")
}

// assertHoldoverInSpecToOutOfSpec validates holdover-in-spec -> holdover-out-of-spec -> locked, after
// upstream clock loss and recovery. No FREERUN events should be generated.
func assertHoldoverInSpecToOutOfSpec(
	clk *clock.Clock,
	holdoverParams clock.HoldoverParameters,
	timeout time.Duration,
) {
	GinkgoHelper()

	err := client.ChangeHoldoverParameters(clk, holdoverParams, timeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to change holdover settings")

	By("setting upstream clock interface down to enter holdover-in-spec")

	ifaceDownTime := time.Now()

	err = client.SetUpstreamInterfaces(clk, iface.InterfaceStateDown)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interface down")

	err = client.WaitForHoldover(clk, ifaceDownTime, timeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to assert holdover-in-spec")

	err = client.WaitForHoldoverOutOfSpec(clk, ifaceDownTime, timeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to assert holdover-out-of-spec")

	By("setting upstream clock interface up to return to locked")

	ifaceUpTime := time.Now()

	err = client.SetUpstreamInterfaces(clk, iface.InterfaceStateUp)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interface up")

	err = client.WaitForLockedFromHoldover(clk, ifaceUpTime, timeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to assert clean transition from holdover to locked")
}

// assertHoldoverOutOfSpecToFreerun validates holdover-in-spec -> holdover-out-of-spec -> freerun -> locked,
// after upstream clock loss and recovery.
func assertHoldoverOutOfSpecToFreerun(
	clk *clock.Clock,
	holdoverParams clock.HoldoverParameters,
	timeout time.Duration,
) {
	GinkgoHelper()

	err := client.ChangeHoldoverParameters(clk, holdoverParams, timeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to change holdover settings")

	By("setting upstream clock interface down to enter holdover-in-spec")

	ifaceDownTime := time.Now()
	err = client.SetUpstreamInterfaces(clk, iface.InterfaceStateDown)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interface down")

	By("assert clock to be holdover-in-spec")

	err = client.WaitForHoldover(clk, ifaceDownTime, timeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to assert holdover-in-spec")

	By("assert clock to be holdover-out-of-spec")

	err = client.WaitForHoldoverOutOfSpec(clk, ifaceDownTime, timeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to assert holdover-out-of-spec")

	By("assert clock to be freerun")

	err = client.WaitForFreerun(clk, ifaceDownTime, timeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to assert freerun")

	By("setting upstream clock interface up to return to locked")

	ifaceUpTime := time.Now()
	err = client.SetUpstreamInterfaces(clk, iface.InterfaceStateUp)
	Expect(err).ToNot(HaveOccurred(), "Failed to set upstream clock interface up")

	err = client.WaitForLocked(clk, ifaceUpTime, timeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to assert locked")
}
