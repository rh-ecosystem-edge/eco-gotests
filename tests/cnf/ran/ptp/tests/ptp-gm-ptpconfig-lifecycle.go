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

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/daemonlogs"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
)

const (
	gpsdDataPath         = "/gpsd/data"
	validateAttempts     = 5
	validateInterval     = 1 * time.Second
	profileReloadTimeout = 5 * time.Minute
	lifecycleCycles      = 10
)

var _ = Describe("PTP T-GM PtpConfig Lifecycle", Label(tsparams.LabelGMPtpConfigLifecycle), func() {
	var (
		prometheusAPI   prometheusv1.API
		savedPtpConfigs []*ptp.PtpConfigBuilder
	)

	BeforeEach(func() {
		By("skipping if the PTP version is not supported")

		inRange, err := version.IsVersionStringInRange(RANConfig.Spoke1OperatorVersions[ranparam.PTP], "4.16.0-0", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to check PTP version range")

		if !inRange {
			Skip("ntpfailover is only supported for PTP version 4.16 and higher")
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
		By("restoring PtpConfigs after testing")

		startTime := time.Now()

		changedProfiles, err := profiles.RestorePtpConfigs(RANConfig.Spoke1APIClient, savedPtpConfigs)
		Expect(err).ToNot(HaveOccurred(), "Failed to restore PtpConfigs")

		if len(changedProfiles) > 0 {
			By("waiting for profile load on nodes after restore")

			err = daemonlogs.WaitForProfileLoadOnPTPNodes(
				RANConfig.Spoke1APIClient,
				daemonlogs.WithStartTime(startTime),
				daemonlogs.WithTimeout(profileReloadTimeout),
			)
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to wait for profile load on PTP nodes: %v", err)
			}
		}

		By("ensuring clocks are locked after testing")

		err = metrics.EnsureClocksAreLocked(prometheusAPI)
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")
	})

	// 89564
	It("validates /gpsd/data does not grow after repeated GM ptpconfig delete and apply cycles",
		reportxml.ID("89564"), func() {
			testRanAtLeastOnce := false

			// Track configs that have already been cycled. When multiple GM nodes share
			// the same PtpConfig, cycling it once exercises all of them — running
			// additional cycles for the same config would be redundant.
			cycledConfigs := make(map[string]bool)

			nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

			for _, nodeInfo := range nodeInfoMap {
				if nodeInfo.Counts[profiles.ProfileTypeGM] == 0 &&
					nodeInfo.Counts[profiles.ProfileTypeMultiNICGM] == 0 &&
					nodeInfo.Counts[profiles.ProfileTypeNTPFallback] == 0 {
					klog.V(tsparams.LogLevel).Infof("Skipping node %s: no GM or NTPFallback profiles", nodeInfo.Name)

					continue
				}

				testRanAtLeastOnce = true

				gmProfileList := nodeInfo.GetProfilesByTypes(
					profiles.ProfileTypeGM, profiles.ProfileTypeMultiNICGM)
				Expect(gmProfileList).ToNot(BeEmpty(),
					"No GM profiles found on node %s", nodeInfo.Name)

				gmConfigName := gmProfileList[0].Reference.ConfigReference.Name

				if cycledConfigs[gmConfigName] {
					klog.V(tsparams.LogLevel).Infof(
						"Skipping node %s: ptpconfig %s already tested on another node",
						nodeInfo.Name, gmConfigName)

					continue
				}

				cycledConfigs[gmConfigName] = true

				// Reuse the already-saved builder throughout all cycles — Delete() refreshes
				// Object via Exists()/Get() on each call, so no fresh pull is needed.
				var gmSavedConfig *ptp.PtpConfigBuilder

				for _, cfg := range savedPtpConfigs {
					if cfg.Definition.Name == gmConfigName {
						gmSavedConfig = cfg

						break
					}
				}

				Expect(gmSavedConfig).ToNot(BeNil(),
					"GM ptpconfig %s not found in saved configs on node %s", gmConfigName, nodeInfo.Name)

				// Register ONE cleanup per config before cycling begins. If the test fails
				// between a delete and its paired re-apply, Ginkgo restores the config
				// before AfterEach runs.
				DeferCleanup(func() {
					By(fmt.Sprintf("ensuring GM ptpconfig %s is restored", gmConfigName))

					gmSavedConfig.Definition.ResourceVersion = ""
					gmSavedConfig.Definition.UID = ""

					_, err := gmSavedConfig.Create()
					Expect(err).ToNot(HaveOccurred(),
						"Failed to ensure GM ptpconfig %s is created", gmConfigName)
				})

				By(fmt.Sprintf("validating initial /gpsd/data state on node %s", nodeInfo.Name))

				validateGpsdFileEmpty(nodeInfo.Name)

				for cycle := range lifecycleCycles {
					cycleLabel := fmt.Sprintf("[%d/%d]", cycle+1, lifecycleCycles)

					By(fmt.Sprintf("%s deleting GM ptpconfig %s on node %s",
						cycleLabel, gmConfigName, nodeInfo.Name))

					deleteTime := time.Now()

					err = gmSavedConfig.Delete()
					Expect(err).ToNot(HaveOccurred(),
						"Failed to delete GM ptpconfig on cycle %d on node %s", cycle+1, nodeInfo.Name)

					// The daemon logs "load profiles" whenever it reloads its configuration.
					// Waiting for this message after deletion confirms the daemon processed the
					// removal before checking the /gpsd/data file state.
					By(fmt.Sprintf("%s waiting for daemon to reload after config deletion", cycleLabel))

					err = daemonlogs.WaitForProfileLoad(RANConfig.Spoke1APIClient, nodeInfo.Name,
						daemonlogs.WithStartTime(deleteTime),
						daemonlogs.WithTimeout(profileReloadTimeout))
					Expect(err).ToNot(HaveOccurred(),
						"Daemon did not reload profiles after config delete on cycle %d on node %s",
						cycle+1, nodeInfo.Name)

					By(fmt.Sprintf("%s validating /gpsd/data file is gone after config removal", cycleLabel))

					validateGpsdFileGone(nodeInfo.Name)

					By(fmt.Sprintf("%s re-applying GM ptpconfig %s on node %s",
						cycleLabel, gmConfigName, nodeInfo.Name))

					applyTime := time.Now()

					// Clear server-managed fields before re-creating the deleted object.
					gmSavedConfig.Definition.ResourceVersion = ""
					gmSavedConfig.Definition.UID = ""

					_, err = gmSavedConfig.Create()
					Expect(err).ToNot(HaveOccurred(),
						"Failed to re-create GM ptpconfig on cycle %d on node %s", cycle+1, nodeInfo.Name)

					// Same "load profiles" signal confirms the daemon picked up the re-applied
					// config before asserting the /gpsd/data file state.
					By(fmt.Sprintf("%s waiting for daemon to reload after config re-apply", cycleLabel))

					err = daemonlogs.WaitForProfileLoad(RANConfig.Spoke1APIClient, nodeInfo.Name,
						daemonlogs.WithStartTime(applyTime),
						daemonlogs.WithTimeout(profileReloadTimeout))
					Expect(err).ToNot(HaveOccurred(),
						"Daemon did not reload profiles after config re-apply on cycle %d on node %s",
						cycle+1, nodeInfo.Name)

					query := metrics.ClockStateQuery{
						Node:    metrics.Equals(nodeInfo.Name),
						Process: metrics.Equals(metrics.ProcessTS2PHC),
					}
					err := metrics.AssertQuery(context.TODO(), prometheusAPI, query, metrics.ClockStateLocked,
						metrics.AssertWithStableDuration(10*time.Second),
						metrics.AssertWithTimeout(5*time.Minute))
					Expect(err).ToNot(HaveOccurred(), "Failed to ensure clocks are locked on node %s", nodeInfo.Name)

					// Core regression assertion for OCPBUGS-58131: after re-applying the GM
					// ptpconfig, /gpsd/data must not accumulate data across cycles.
					By(fmt.Sprintf("%s validating /gpsd/data file is empty after config re-apply", cycleLabel))

					validateGpsdFileEmpty(nodeInfo.Name)

					klog.V(tsparams.LogLevel).Infof("Cycle %d/%d completed successfully on node %s",
						cycle+1, lifecycleCycles, nodeInfo.Name)
				}
			}

			if !testRanAtLeastOnce {
				Skip("No GM nodes found")
			}
		})
})

// validateGpsdFileEmpty asserts that /gpsd/data is empty (zero-size or absent) for the full
// validation window (validateAttempts × validateInterval). It fails as soon as any poll finds
// content in the file, indicating the growing-file bug from OCPBUGS-58131.
func validateGpsdFileEmpty(nodeName string) {
	GinkgoHelper()

	Consistently(func() error {
		_, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(
			RANConfig.Spoke1APIClient,
			nodeName,
			fmt.Sprintf("[ ! -s %s ]", gpsdDataPath),
		)

		return err
	}, validateAttempts*validateInterval, validateInterval).ShouldNot(HaveOccurred(),
		"/gpsd/data has content on node %s — file may be growing (OCPBUGS-58131)", nodeName)
}

// validateGpsdFileGone asserts that /gpsd/data no longer exists on the given node.
func validateGpsdFileGone(nodeName string) {
	GinkgoHelper()

	_, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(
		RANConfig.Spoke1APIClient,
		nodeName,
		fmt.Sprintf("[ ! -f %s ]", gpsdDataPath),
	)
	Expect(err).ToNot(HaveOccurred(), "%s still exists on node %s", gpsdDataPath, nodeName)
}
