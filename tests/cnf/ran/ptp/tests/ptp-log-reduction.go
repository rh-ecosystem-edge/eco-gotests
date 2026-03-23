package tests

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/daemonlogs"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var (
	// master or phc offset summary log pattern for 'enhanced' logReduction option
	// An example of a log with the pattern: master offset summary: cnt=2, min=-1, max=1, avg=0.00, SD=1.41
	// used in tests 83456 and 83591.
	enhancedPattern = regexp.MustCompile(`(phc|master)\soffset\ssummary:\scnt=[0-9]+,\smin=-?[0-9]+,\s` +
		`max=-?[0-9]+,\savg=-?[0-9]+.?[0-9]*,\sSD=-?[0-9]+.?[0-9]*`)

	// log pattern when offset is outside range [-2, 2].
	// An example of a log with the pattern: master offset         -4
	// We expect not to catch:              master offset          2
	// used in test 83586.
	fullLogPatternOutsideRange = regexp.MustCompile(`master\soffset\s+(-?(?:[3-9]\d*|\d{2,}))`)

	// log pattern when offset is within range [-2, 2].
	// used in test 83586.
	fullLogPatternInRange = regexp.MustCompile(`master\soffset\s+((?:-?[12])|0)\s`)

	// log pattern when offset is any value.
	// used in test 83586.
	fullLogPatternAny = regexp.MustCompile(`master\soffset\s+([+-]?\d+)`)
)

var _ = Describe("PTP Log Reduction", Label(tsparams.LabelLogReduction), func() {
	var (
		prometheusAPI   prometheusv1.API
		savedPtpConfigs []*ptp.PtpConfigBuilder
	)

	BeforeEach(func() {
		var err error

		By("skipping if the PTP version is not supported")

		inRange, err := version.IsVersionStringInRange(RANConfig.Spoke1OperatorVersions[ranparam.PTP], "4.20", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to check PTP version range")

		if !inRange {
			Skip("log reduction is only supported for PTP version 4.20 and higher")
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

		By("restoring PtpConfigs to original specs")

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

	// 83456 - Validates the thresholds summary is seen in the linuxptp-daemon logs after changing the ptpSetting
	// logReduce to "enhanced"
	It("validates the thresholds summary is seen in the linuxptp-daemon logs after changing the ptpSetting "+
		"logReduce to \"enhanced\"", reportxml.ID("83456"), func() {
		testRanAtLeastOnce := false

		By("checking node info map")

		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for _, nodeInfo := range nodeInfoMap {
			if skipNodeForLogReduction(nodeInfo) {
				klog.V(tsparams.LogLevel).Infof(
					"Skipping test for node %s. Test does not support Grandmaster or NTP fallback profiles",
					nodeInfo.Name)

				continue
			}

			testRanAtLeastOnce = true
			newLogReduceVal := "enhanced"

			By(fmt.Sprintf("updating the logReduce to \"enhanced\" for all profiles on node %s", nodeInfo.Name))
			err := setLogReduceValForNode(prometheusAPI, RANConfig.Spoke1APIClient, nodeInfo.Name, newLogReduceVal)
			Expect(err).ToNot(HaveOccurred(), "Failed to set logReduce value")

			startTime := time.Now()

			By(fmt.Sprintf("checking the logs for pattern on node %s", nodeInfo.Name))
			klog.V(tsparams.LogLevel).Infof("Waiting for pattern: %s", enhancedPattern.String())

			err = daemonlogs.WaitForPodLog(RANConfig.Spoke1APIClient, nodeInfo.Name,
				daemonlogs.WithStartTime(startTime),
				daemonlogs.WithTimeout(5*time.Minute),
				daemonlogs.WithMatcher(daemonlogs.RegexpMatcher(enhancedPattern)))
			Expect(err).ToNot(HaveOccurred(), "Failed to find pattern in logs")
		}

		if !testRanAtLeastOnce {
			Skip("Could not find any node to run the test on")
		}
	})

	// 83586 - Validates the "enhanced" option can be set an interval value and a threshold value so a full log will be
	// displayed when the threshold is higher than that values
	It("validates the \"enhanced\" option can be set an interval value and a threshold value so a full log "+
		"will be displayed when the threshold is higher than that values", reportxml.ID("83586"), func() {
		testRanAtLeastOnce := false
		interval := 30 * time.Second

		By("checking node info map")

		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for _, nodeInfo := range nodeInfoMap {
			if skipNodeForLogReduction(nodeInfo) {
				klog.V(tsparams.LogLevel).Infof(
					"Skipping test for node %s. Test does not support Grandmaster or NTP fallback profiles",
					nodeInfo.Name)

				continue
			}

			testRanAtLeastOnce = true
			// setting a small threshold to ensure the full 'master offset' log is displayed when the offset
			// exceeds the threshold.
			threshold := "2"

			newLogReduceVal := "enhanced " + interval.String() + " " + threshold

			By(fmt.Sprintf("updating the logReduce to %s on node %s", newLogReduceVal, nodeInfo.Name))
			err := setLogReduceValForNode(prometheusAPI, RANConfig.Spoke1APIClient, nodeInfo.Name, newLogReduceVal)
			Expect(err).ToNot(HaveOccurred(), "Failed to set logReduce value")

			startTime := time.Now()

			By(fmt.Sprintf("validating a full 'master offset' log appears only when offset exceeds threshold on"+
				" node %s", nodeInfo.Name))

			err = daemonlogs.WaitForPodLog(RANConfig.Spoke1APIClient, nodeInfo.Name,
				daemonlogs.WithStartTime(startTime),
				daemonlogs.WithTimeout(5*time.Minute),
				daemonlogs.WithMatcher(daemonlogs.RegexpMatcher(fullLogPatternOutsideRange)))
			Expect(err).ToNot(HaveOccurred(), "Failed to find pattern in logs")

			By(fmt.Sprintf("validating NO full 'master offset' log appears when offset is in [-2,2] range on node %s",
				nodeInfo.Name))

			err = daemonlogs.WaitForPodLog(RANConfig.Spoke1APIClient, nodeInfo.Name,
				daemonlogs.WithStartTime(startTime),
				daemonlogs.WithTimeout(1*time.Minute),
				daemonlogs.WithMatcher(daemonlogs.RegexpMatcher(fullLogPatternInRange)))
			Expect(err).To(HaveOccurred(), "Full 'master offset' log is displayed when threshold set to "+threshold)

			// setting a high threshold to ensure no full 'master offset' log is displayed.
			threshold = "999"

			klog.V(tsparams.LogLevel).Infof("Updating the logReduce to enhanced %s %s", interval.String(), threshold)
			newLogReduceVal = "enhanced " + interval.String() + " " + threshold
			err = setLogReduceValForNode(prometheusAPI, RANConfig.Spoke1APIClient, nodeInfo.Name, newLogReduceVal)
			Expect(err).ToNot(HaveOccurred(), "Failed to set logReduce value")

			startTime = time.Now() // reset the start time.

			By(fmt.Sprintf("validating no full 'master offset' log appears when the threshold has high value on"+
				" node %s", nodeInfo.Name))

			err = daemonlogs.WaitForPodLog(RANConfig.Spoke1APIClient, nodeInfo.Name,
				daemonlogs.WithStartTime(startTime),
				daemonlogs.WithTimeout(1*time.Minute),
				daemonlogs.WithMatcher(daemonlogs.RegexpMatcher(fullLogPatternAny)))

			Expect(err).To(HaveOccurred(), "Full 'master offset' log is displayed when threshold set to "+threshold)
		}

		if !testRanAtLeastOnce {
			Skip("Could not find any node to run the test on")
		}
	})

	// 83591 - Validates a full offset log is shown when configure logReduce to "enhanced" with interval value only
	It("validates a full offset log is shown when configure logReduce to \"enhanced\" with "+
		"interval value only", reportxml.ID("83591"), func() {
		testRanAtLeastOnce := false

		By("checking node info map")

		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for _, nodeInfo := range nodeInfoMap {
			if skipNodeForLogReduction(nodeInfo) {
				klog.V(tsparams.LogLevel).Infof(
					"Skipping test for node %s. Test does not support Grandmaster or NTP fallback profiles",
					nodeInfo.Name)

				continue
			}

			testRanAtLeastOnce = true

			// Use a fixed interval of 10 seconds
			newLogReduceVal := "enhanced 10s"

			By(fmt.Sprintf("updating the logReduce to %s on node %s", newLogReduceVal, nodeInfo.Name))
			err = setLogReduceValForNode(prometheusAPI, RANConfig.Spoke1APIClient, nodeInfo.Name, newLogReduceVal)
			Expect(err).ToNot(HaveOccurred(), "Failed to set logReduce value")

			By(fmt.Sprintf("validating the logs shows every 10s on node %s", nodeInfo.Name))
			validateLogWithInterval(RANConfig.Spoke1APIClient, nodeInfo.Name)
		}

		if !testRanAtLeastOnce {
			Skip("Could not find any node to run the test on")
		}
	})
})

// setLogReduceValForNode sets the logReduce value for all PTP profiles in all PTP configs.
// It updates all profiles across all configs (not just those applying to the specified node).
// After updating configs, it waits for the profile to reload on the specified node. If no
// configs are updated (all already have the desired value), the wait is skipped.
func setLogReduceValForNode(prometheusAPI prometheusv1.API,
	client *clients.Settings,
	nodeName string,
	newLogReduceVal string) error {
	var startTime time.Time

	By("getting all PtpConfigs")

	ptpConfigList, err := ptp.ListPtpConfigs(client)
	if err != nil {
		return fmt.Errorf("failed to list PtpConfigs: %w", err)
	}

	for _, ptpConfig := range ptpConfigList {
		updated := false

		for i := range ptpConfig.Definition.Spec.Profile {
			profile := &ptpConfig.Definition.Spec.Profile[i]

			if profile.PtpSettings == nil {
				profile.PtpSettings = map[string]string{}
			}

			if profile.PtpSettings["logReduce"] == newLogReduceVal {
				continue
			}

			profile.PtpSettings["logReduce"] = newLogReduceVal
			updated = true
		}

		if updated {
			_, err = ptpConfig.Update()
			if err != nil {
				return fmt.Errorf("failed to update PtpConfig %s: %w", ptpConfig.Definition.Name, err)
			}

			startTime = time.Now()
		}
	}

	if !startTime.IsZero() {
		By(fmt.Sprintf("waiting for the config to be reloaded on node %s", nodeName))

		err = daemonlogs.WaitForProfileLoad(client, nodeName,
			daemonlogs.WithStartTime(startTime),
			daemonlogs.WithTimeout(5*time.Minute))
		if err != nil {
			return fmt.Errorf("failed to wait for profile load on node %s: %w", nodeName, err)
		}
	}

	By(fmt.Sprintf("ensuring clocks are locked after all profiles updated on node %s", nodeName))

	err = metrics.EnsureClocksAreLocked(prometheusAPI)
	if err != nil {
		return fmt.Errorf("failed to ensure clocks are locked after all profiles updated on node %s: %w",
			nodeName, err)
	}

	return nil
}

// validateLogWithInterval validates that the log reduction is working correctly with a fixed 10s interval.
// It waits for a 1-minute window, then counts log entries matching the pattern.
// The expected count is (6 * numberOfProcesses), with adaptive tolerance to account for timing variations.
func validateLogWithInterval(client *clients.Settings, nodeName string) {
	windowDuration := 1 * time.Minute

	By(fmt.Sprintf("getting the PTP daemon pod for node %s", nodeName))
	ptpDaemonPod, err := ptpdaemon.GetPtpDaemonPodOnNode(client, nodeName)
	Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon pod for node %s", nodeName)

	startTime := time.Now()
	time.Sleep(time.Until(startTime.Add(windowDuration))) // wait until end of window

	By(fmt.Sprintf("getting logs for node %s since %v", nodeName, startTime))
	logs, err := ptpDaemonPod.GetLogsWithOptions(&corev1.PodLogOptions{
		SinceTime: &metav1.Time{Time: startTime},
		Container: ranparam.PtpContainerName,
	})
	Expect(err).ToNot(HaveOccurred(), "Failed to get logs")

	numberOfLogs := len(enhancedPattern.FindAllString(string(logs), -1))

	klog.V(tsparams.LogLevel).Infof("Found %d log entries matching pattern for node %s", numberOfLogs, nodeName)

	By(fmt.Sprintf("getting number of ptp4l and phc2sys processes on node %s", nodeName))

	cmd := "ps -C ptp4l -C phc2sys -o pid= | wc -l"

	numberOfProcessesBytes, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client,
		nodeName,
		cmd,
		ptpdaemon.WithRetries(3),
		ptpdaemon.WithRetryOnError(true))
	Expect(err).ToNot(HaveOccurred(), "Failed to execute command on pod")

	numberOfProcesses, err := strconv.Atoi(strings.TrimSpace(numberOfProcessesBytes))
	Expect(err).ToNot(HaveOccurred(), "Failed to convert number of processes to int")

	Expect(numberOfLogs).To(And(
		BeNumerically(">=", 4*numberOfProcesses),
		BeNumerically("<=", 7*numberOfProcesses)),
		"Log count should be within expected range for interval 10s")
}

// skipNodeForLogReduction skips a node if it has Grandmaster or NTP fallback profiles.
func skipNodeForLogReduction(nodeInfo *profiles.NodeInfo) bool {
	profileCount := nodeInfo.Counts[profiles.ProfileTypeGM]
	profileCount += nodeInfo.Counts[profiles.ProfileTypeMultiNICGM]
	profileCount += nodeInfo.Counts[profiles.ProfileTypeNTPFallback]

	return profileCount > 0
}
