package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"k8s.io/klog/v2"

	"github.com/onsi/ginkgo/v2/types"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/sma"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
)

var _ = Describe("PTP T-GM SMA Disconnect", Label(tsparams.LabelSMADisconnect), func() {
	var (
		prometheusAPI prometheusv1.API
	)

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
	})

	AfterEach(func() {
		if CurrentSpecReport().State == types.SpecStateSkipped {
			return
		}

		By("ensuring clocks are locked after testing")

		err := metrics.EnsureClocksAreLocked(prometheusAPI)
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")
	})

	// 81205 - checks sync loss is generated for dpll process for RX interface and FREERUN for GM process for TX
	// interface, and all clocks relock after reconnect
	It("checks sync loss is generated for dpll process for RX interface and FREERUN for GM process for TX interface,"+
		" and all clocks relock after reconnect",
		reportxml.ID("81205"), func() {
			testRanAtLeastOnce := false

			By("getting node info map")

			nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

			for nodeName, nodeInfo := range nodeInfoMap {
				gmProfilesInfo := nodeInfo.GetProfilesByTypes(profiles.ProfileTypeMultiNICGM)
				if len(gmProfilesInfo) == 0 {
					continue
				}

				gmProfileInfo := gmProfilesInfo[0]

				gmProfile, err := gmProfileInfo.PullProfile(RANConfig.Spoke1APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull multi-NIC GM profile for node %s", nodeName)

				By("checking if node " + nodeName + " has an e810 NIC with SMA connections")

				_, hasE810 := gmProfile.Plugins[string(ptp.PluginTypeE810)]
				if !hasE810 {
					klog.V(tsparams.LogLevel).Infof(
						"Skipping node %s: no e810 NIC found (SMA is only supported on e810)", nodeName)

					continue
				}

				rxInterfaces, err := profiles.GetRxInterfaces(gmProfile)
				Expect(err).ToNot(HaveOccurred(), "Failed to get RX interfaces for node %s", nodeName)

				if len(rxInterfaces) == 0 {
					klog.V(tsparams.LogLevel).Infof(
						"Skipping node %s: no RX interfaces found in e810 plugin", nodeName)

					continue
				}

				txInterface, err := profiles.GetGmInterfaceToGPS(gmProfile, gmProfileInfo.HardwareConfig)
				Expect(err).ToNot(HaveOccurred(), "Failed to get TX (GPS) interface for node %s", nodeName)

				klog.V(tsparams.LogLevel).Infof("Node %s: RX interfaces: %v", nodeName, rxInterfaces)
				klog.V(tsparams.LogLevel).Infof("Node %s: TX interface: %s", nodeName, txInterface)

				smaPinNames := make(map[string]string)
				smaConfigs := make(map[string]string)
				// The control mechanism is resolved per interface, not per node: the sysfs probe globs
				// /sys/class/net/<iface>/..., so two RX interfaces on the same node can in principle
				// resolve differently.
				smaCapabilities := make(map[string]sma.Capability)
				// Resolve every SMA pin and confirm it can actually be controlled before touching any of
				// them, so that a node where neither sysfs nor a dpll CLI is available is passed over
				// instead of being left with a half disconnected set of pins. Infrastructure errors are fatal.
				var discoveryError error

				nodeSupported := true

				for _, rxIface := range rxInterfaces {
					pinName, config, err := profiles.GetSmaPinFromProfile(gmProfile, rxIface)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to get SMA pin for interface %s on node %s", rxIface, nodeName)

					capability, err := sma.GetCapability(RANConfig.Spoke1APIClient, nodeName, rxIface, pinName)
					if err != nil {
						discoveryError = err
						nodeSupported = false

						break
					}

					if capability == sma.CapabilityNone {
						klog.V(tsparams.LogLevel).Infof(
							"Skipping node %s: cannot control SMA pin %s for interface %s",
							nodeName, pinName, rxIface)

						nodeSupported = false

						break
					}

					smaPinNames[string(rxIface)] = pinName
					smaConfigs[string(rxIface)] = config
					smaCapabilities[string(rxIface)] = capability

					klog.V(tsparams.LogLevel).Infof(
						"Node %s interface %s: pin %s, config %s, controlled via %s",
						nodeName, rxIface, pinName, config, capability)
				}

				if discoveryError != nil {
					Fail(fmt.Sprintf("Failed to discover SMA capability on node %s: %v", nodeName, discoveryError))
				}

				if !nodeSupported {
					continue
				}

				testRanAtLeastOnce = true

				for _, rxIface := range rxInterfaces {
					pinName := smaPinNames[string(rxIface)]
					smaConfig := smaConfigs[string(rxIface)]
					capability := smaCapabilities[string(rxIface)]

					DeferCleanup(func() {
						connected, err := sma.IsSmaConnectedWithCapability(
							RANConfig.Spoke1APIClient, nodeName, rxIface, pinName, capability)
						if err != nil {
							klog.V(tsparams.LogLevel).Infof(
								"Failed to check SMA connection for %s on node %s: %v", rxIface, nodeName, err)

							return
						}

						if !connected {
							err = sma.ReconnectSmaWithCapability(
								RANConfig.Spoke1APIClient, nodeName, rxIface, pinName, smaConfig, capability)
							if err != nil {
								klog.V(tsparams.LogLevel).Infof(
									"Failed to reconnect SMA for %s on node %s: %v", rxIface, nodeName, err)
							}
						}
					})

					By(fmt.Sprintf("disconnecting %s for interface %s on node %s", pinName, rxIface, nodeName))

					err = sma.DisconnectSmaWithCapability(
						RANConfig.Spoke1APIClient, nodeName, rxIface, pinName, capability)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to disconnect SMA %s for interface %s on node %s", pinName, rxIface, nodeName)

					connected, err := sma.IsSmaConnectedWithCapability(
						RANConfig.Spoke1APIClient, nodeName, rxIface, pinName, capability)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to check SMA connection status for %s on node %s", rxIface, nodeName)
					Expect(connected).To(BeFalse(),
						"Expected SMA %s for interface %s on node %s to be disconnected", pinName, rxIface, nodeName)

					klog.V(tsparams.LogLevel).Infof(
						"Node %s interface %s %s connected: %v, expected: false",
						nodeName, rxIface, pinName, connected)

					// The follower DPLL is only required to leave LOCKED. Whether it lands in HOLDOVER or
					// FREERUN depends on the hardware holdover timeout, which on the E810 is
					// LocalHoldoverTimeout 14400 (4 hours) for a recently locked interface, so requiring
					// FREERUN here would always time out. See CNF-29319 for details.
					By(fmt.Sprintf(
						"waiting for a not LOCKED clock state for dpll process on RX interface %s", rxIface))

					err = metrics.AssertQueryAnyOf(
						context.TODO(),
						prometheusAPI,
						metrics.ClockStateQuery{
							Interface: metrics.Equals(rxIface.GetAlias()),
							Process:   metrics.DoesNotEqual(metrics.ProcessTS2PHC),
						},
						[]metrics.PtpClockState{metrics.ClockStateHoldover, metrics.ClockStateFreerun},
						metrics.AssertWithTimeout(1*time.Minute),
						metrics.AssertWithPollInterval(5*time.Second),
					)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to wait for HOLDOVER or FREERUN clock state on RX interface %s for node %s",
						rxIface, nodeName)

					By(fmt.Sprintf(
						"waiting for FREERUN clock state for GM process on TX interface %s", txInterface))

					err = metrics.AssertQuery(
						context.TODO(),
						prometheusAPI,
						metrics.ClockStateQuery{
							Interface: metrics.Equals(txInterface.GetAlias()),
							Process: metrics.Excludes(
								metrics.ProcessDPLL, metrics.ProcessGNSS, metrics.ProcessTS2PHC),
						},
						metrics.ClockStateFreerun,
						metrics.AssertWithTimeout(1*time.Minute),
						metrics.AssertWithPollInterval(5*time.Second),
					)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to wait for FREERUN clock state on TX interface %s for node %s",
						txInterface, nodeName)

					By(fmt.Sprintf("reconnecting %s for RX interface %s on node %s", pinName, rxIface, nodeName))

					err = sma.ReconnectSmaWithCapability(
						RANConfig.Spoke1APIClient, nodeName, rxIface, pinName, smaConfig, capability)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to reconnect SMA %s for interface %s on node %s", pinName, rxIface, nodeName)

					connected, err = sma.IsSmaConnectedWithCapability(
						RANConfig.Spoke1APIClient, nodeName, rxIface, pinName, capability)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to check SMA connection status for %s on node %s", rxIface, nodeName)
					Expect(connected).To(BeTrue(),
						"Expected SMA %s for interface %s on node %s to be connected", pinName, rxIface, nodeName)

					klog.V(tsparams.LogLevel).Infof(
						"Node %s interface %s %s connected: %v, expected: true",
						nodeName, rxIface, pinName, connected)

					By("waiting for all clocks to return to LOCKED")

					err = metrics.EnsureClocksAreLocked(prometheusAPI)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to wait for clocks to return to LOCKED after reconnecting SMA %s for "+
							"interface %s on node %s", pinName, rxIface, nodeName)

					By("waiting for PPS status to return to available")

					err = metrics.AssertQuery(
						context.TODO(),
						prometheusAPI,
						metrics.PPSStatusQuery{},
						metrics.PPSStatusAvailable,
						metrics.AssertWithTimeout(1*time.Minute),
						metrics.AssertWithPollInterval(10*time.Second),
					)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to wait for PPS status to return to available on node %s", nodeName)
				}
			}

			if !testRanAtLeastOnce {
				Skip(fmt.Sprintf(
					"Could not find any multi-NIC GM node with an e810 NIC and controllable SMA connections: %v",
					sma.ErrSmaUnsupported))
			}
		})
})
