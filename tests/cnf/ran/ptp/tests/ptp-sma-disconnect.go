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

	// 81205 - checks FREERUN status are generated for dpll process for RX interface and GM process for TX interface
	It("checks FREERUN status are generated for dpll process for RX interface and GM process for TX interface",
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

				testRanAtLeastOnce = true

				txInterface, err := profiles.GetGmInterfaceToGPS(gmProfile)
				Expect(err).ToNot(HaveOccurred(), "Failed to get TX (GPS) interface for node %s", nodeName)

				klog.V(tsparams.LogLevel).Infof("Node %s: RX interfaces: %v", nodeName, rxInterfaces)
				klog.V(tsparams.LogLevel).Infof("Node %s: TX interface: %s", nodeName, txInterface)

				smaPinNames := make(map[string]string)
				smaConfigs := make(map[string]string)

				for _, rxIface := range rxInterfaces {
					pinName, config, err := profiles.GetSmaPinFromProfile(gmProfile, rxIface)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to get SMA pin for interface %s on node %s", rxIface, nodeName)

					smaPinNames[string(rxIface)] = pinName
					smaConfigs[string(rxIface)] = config

					klog.V(tsparams.LogLevel).Infof(
						"Node %s interface %s: pin %s, config %s", nodeName, rxIface, pinName, config)
				}

				for _, rxIface := range rxInterfaces {
					pinName := smaPinNames[string(rxIface)]

					DeferCleanup(func() {
						connected, err := sma.IsSmaConnected(RANConfig.Spoke1APIClient, nodeName, rxIface, pinName)
						if err != nil {
							klog.V(tsparams.LogLevel).Infof(
								"Failed to check SMA connection for %s on node %s: %v", rxIface, nodeName, err)

							return
						}

						if !connected {
							err = sma.ReconnectSma(
								RANConfig.Spoke1APIClient, nodeName, rxIface, pinName, smaConfigs[string(rxIface)])
							if err != nil {
								klog.V(tsparams.LogLevel).Infof(
									"Failed to reconnect SMA for %s on node %s: %v", rxIface, nodeName, err)
							}
						}
					})

					By(fmt.Sprintf("disconnecting %s for interface %s on node %s", pinName, rxIface, nodeName))

					err = sma.DisconnectSma(RANConfig.Spoke1APIClient, nodeName, rxIface, pinName)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to disconnect SMA %s for interface %s on node %s", pinName, rxIface, nodeName)

					connected, err := sma.IsSmaConnected(RANConfig.Spoke1APIClient, nodeName, rxIface, pinName)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to check SMA connection status for %s on node %s", rxIface, nodeName)
					Expect(connected).To(BeFalse(),
						"Expected SMA %s for interface %s on node %s to be disconnected", pinName, rxIface, nodeName)

					klog.V(tsparams.LogLevel).Infof(
						"Node %s interface %s %s connected: %v, expected: false",
						nodeName, rxIface, pinName, connected)

					By(fmt.Sprintf(
						"waiting for FREERUN clock state for dpll process on RX interface %s", rxIface))

					err = metrics.AssertQuery(
						context.TODO(),
						prometheusAPI,
						metrics.ClockStateQuery{
							Interface: metrics.Equals(rxIface.GetNIC()),
							Process:   metrics.DoesNotEqual(metrics.ProcessTS2PHC),
						},
						metrics.ClockStateFreerun,
						metrics.AssertWithTimeout(1*time.Minute),
						metrics.AssertWithPollInterval(5*time.Second),
					)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to wait for FREERUN clock state on RX interface %s for node %s", rxIface, nodeName)

					By(fmt.Sprintf(
						"waiting for FREERUN clock state for GM process on TX interface %s", txInterface))

					err = metrics.AssertQuery(
						context.TODO(),
						prometheusAPI,
						metrics.ClockStateQuery{
							Interface: metrics.Equals(txInterface.GetNIC()),
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

					err = sma.ReconnectSma(
						RANConfig.Spoke1APIClient, nodeName, rxIface, pinName, smaConfigs[string(rxIface)])
					Expect(err).ToNot(HaveOccurred(),
						"Failed to reconnect SMA %s for interface %s on node %s", pinName, rxIface, nodeName)

					connected, err = sma.IsSmaConnected(RANConfig.Spoke1APIClient, nodeName, rxIface, pinName)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to check SMA connection status for %s on node %s", rxIface, nodeName)
					Expect(connected).To(BeTrue(),
						"Expected SMA %s for interface %s on node %s to be connected", pinName, rxIface, nodeName)

					klog.V(tsparams.LogLevel).Infof(
						"Node %s interface %s %s connected: %v, expected: true",
						nodeName, rxIface, pinName, connected)

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
				Skip("Could not find any multi-NIC GM node with e810 NIC and SMA connections")
			}
		})
})
