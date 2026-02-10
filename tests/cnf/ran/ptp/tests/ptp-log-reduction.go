package tests

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var _ = Describe("PTP Log Reduction", Label(tsparams.LabelLogReduction), func() {
	var (
		prometheusAPI      prometheusv1.API
		savedPtpConfigs    []*ptp.PtpConfigBuilder
		testRanAtLeastOnce = false
		nodeInfoMap        map[string]*profiles.NodeInfo
	)

	BeforeEach(func() {
		var err error

		By("skipping if the PTP version is not supported")
		inRange, err := version.IsVersionStringInRange(RANConfig.Spoke1OperatorVersions[ranparam.PTP], "4.20", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to check PTP version range")

		if !inRange {
			Skip("log reduction is only supported for PTP version 4.20 and higher")
		}

		By("checking node info map")
		nodeInfoMap, err = profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		var gmCount uint
		for _, nodeInfo := range nodeInfoMap {
			gmCount += nodeInfo.Counts[profiles.ProfileTypeGM]
			gmCount += nodeInfo.Counts[profiles.ProfileTypeMultiNICGM]
		}

		if gmCount > 0 {
			Skip("Test does not support Grandmaster configurations")
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
		if CurrentSpecReport().State.String() == "skipped" {
			return
		}

		By("restoring PtpConfigs to original specs")
		startTime := time.Now()
		changedProfiles, err := profiles.RestorePtpConfigs(RANConfig.Spoke1APIClient, savedPtpConfigs)
		Expect(err).ToNot(HaveOccurred(), "Failed to restore PtpConfigs")

		if len(changedProfiles) > 0 {
			By("waiting for profile load on nodes")
			err := ptpdaemon.WaitForProfileLoadOnPTPNodes(RANConfig.Spoke1APIClient,
				ptpdaemon.WithStartTime(startTime),
				ptpdaemon.WithTimeout(5*time.Minute))
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to wait for profile load on PTP nodes: %v", err)
			}
		}

		By("ensuring clocks are locked after testing")
		err = metrics.EnsureClocksAreLocked(prometheusAPI)
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")

		testRanAtLeastOnce = false
	})

	// 83456 - Validates the thresholds summary is seen in the linuxptp-daemon logs after changing the ptpSetting
	// logReduce to "enhanced"
	It("validates the thresholds summary is seen in the linuxptp-daemon logs after changing the ptpSetting "+
		"logReduce to \"enhanced\"", reportxml.ID("83456"), func() {
		// master offset summary log pattern for 'enhanced' logReduction option
		// An example of a log with the pattern: master offset summary: cnt=2, min=-1, max=1, avg=0.00, SD=1.41
		pattern := `master\soffset\ssummary:\scnt=[0-9]+,\smin=-?[0-9]+,\smax=-?[0-9]+,\savg=-?` +
			`[0-9]+.?[0-9]*,\sSD=-?[0-9]+.?[0-9]*`

		for nodeName := range nodeInfoMap {
			testRanAtLeastOnce = true
			newLogReduceVal := "enhanced"

			By(fmt.Sprintf("updating the logReduce to \"enhanced\" for all profiles on node %s", nodeName))
			err := setLogReduceValForNode(RANConfig.Spoke1APIClient, nodeName, newLogReduceVal)
			Expect(err).ToNot(HaveOccurred(), "Failed to set logReduce value")

			startTime := time.Now()

			By(fmt.Sprintf("checking the logs for pattern on node %s", nodeName))
			klog.V(tsparams.LogLevel).Infof("Waiting for pattern: %s", pattern)

			regexPattern, err := regexp.Compile(pattern)
			Expect(err).ToNot(HaveOccurred(), "Failed to compile regex pattern")

			err = ptpdaemon.WaitForPodLog(RANConfig.Spoke1APIClient, nodeName,
				ptpdaemon.WithStartTime(startTime),
				ptpdaemon.WithTimeout(5*time.Minute),
				ptpdaemon.WithMatcher(ptpdaemon.RegexpMatcher(regexPattern)))
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

		interval := 30 * time.Second

		// log pattern when offset is outside range [-2, 2].
		// An example of a log with the pattern: master offset         -4
		// We expect not to catch:              master offset          2
		fullLogPattern := `master\soffset\s+(-?(?:[3-9]\d*|\d{2,}))`

		for nodeName := range nodeInfoMap {
			testRanAtLeastOnce = true
			// setting a small threshold to ensure the full 'master offset' log is displayed when the offset
			// exceeds the threshold.
			threshold := "2"

			newLogReduceVal := "enhanced " + interval.String() + " " + threshold

			By(fmt.Sprintf("updating the logReduce to %s on node %s", newLogReduceVal, nodeName))
			err := setLogReduceValForNode(RANConfig.Spoke1APIClient, nodeName, newLogReduceVal)
			Expect(err).ToNot(HaveOccurred(), "Failed to set logReduce value")

			startTime := time.Now()

			By(fmt.Sprintf("validating a full 'master offset' log appears only when offset exceeds threshold on"+
				" node %s", nodeName))
			regexPattern, err := regexp.Compile(fullLogPattern)
			Expect(err).ToNot(HaveOccurred(), "Failed to compile regex pattern")

			err = ptpdaemon.WaitForPodLog(RANConfig.Spoke1APIClient, nodeName,
				ptpdaemon.WithStartTime(startTime),
				ptpdaemon.WithTimeout(5*time.Minute),
				ptpdaemon.WithMatcher(ptpdaemon.RegexpMatcher(regexPattern)))
			Expect(err).ToNot(HaveOccurred(), "Failed to find pattern in logs")

			// log pattern when offset is within range [-2, 2].
			fullLogPatternInRange := `master\soffset\s+((?:-?[12])|0)\s`
			By(fmt.Sprintf("validating NO full 'master offset' log appears when offset is in [-2,2] range on node %s", nodeName))
			regexPatternInRange, err := regexp.Compile(fullLogPatternInRange)
			Expect(err).ToNot(HaveOccurred(), "Failed to compile regex pattern")

			err = ptpdaemon.WaitForPodLog(RANConfig.Spoke1APIClient, nodeName,
				ptpdaemon.WithStartTime(startTime),
				ptpdaemon.WithTimeout(1*time.Minute),
				ptpdaemon.WithMatcher(ptpdaemon.RegexpMatcher(regexPatternInRange)))
			Expect(err).To(HaveOccurred(), "Full 'master offset' log is displayed when threshold set to "+threshold)

			By(fmt.Sprintf("validating no full 'master offset' log appears when the threshold has high value on"+
				" node %s", nodeName))
			threshold = "999"

			klog.V(tsparams.LogLevel).Infof("Updating the logReduce to enhanced %s %s", interval.String(), threshold)
			newLogReduceVal = "enhanced " + interval.String() + " " + threshold
			err = setLogReduceValForNode(RANConfig.Spoke1APIClient, nodeName, newLogReduceVal)
			Expect(err).ToNot(HaveOccurred(), "Failed to set logReduce value")

			// Wait for PTP processes to stabilize with new configuration
			time.Sleep(10 * time.Second)

			startTime = time.Now() // reset the start time.

			fullLogPatternAny := `master\soffset\s+([+-]?\d+)`
			regexPatternAny, err := regexp.Compile(fullLogPatternAny)
			Expect(err).ToNot(HaveOccurred(), "Failed to compile regex pattern")

			err = ptpdaemon.WaitForPodLog(RANConfig.Spoke1APIClient, nodeName,
				ptpdaemon.WithStartTime(startTime),
				ptpdaemon.WithTimeout(1*time.Minute),
				ptpdaemon.WithMatcher(ptpdaemon.RegexpMatcher(regexPatternAny)))

			Expect(err).To(HaveOccurred(), "Full 'master offset' log is displayed when threshold set to "+threshold)

		}

		if !testRanAtLeastOnce {
			Skip("Could not find any node to run the test on")
		}
	})

	// 83591 - Validates a full offset log is shown when configure logReduce to "enhanced" with interval value only
	It("validates a full offset log is shown when configure logReduce to \"enhanced\" with "+
		"interval value only", reportxml.ID("83591"), func() {
		for nodeName := range nodeInfoMap {
			testRanAtLeastOnce = true

			By(fmt.Sprintf("getting the PTP daemon pod for node %s", nodeName))
			ptpDaemonPod, err := ptpdaemon.GetPtpDaemonPodOnNode(RANConfig.Spoke1APIClient, nodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon pod for node %s", nodeName)

			By(fmt.Sprintf("getting number of ptp4l and phc2sys processes on node %s", nodeName))
			cmd := []string{"/bin/bash", "-c", "ps -C ptp4l -C phc2sys -o pid= | wc -l"}
			numberOfProcessesBytes, err := ptpDaemonPod.ExecCommand(cmd, ranparam.PtpContainerName)
			Expect(err).ToNot(HaveOccurred(), "Failed to execute command on pod")

			numberOfProcesses, err := strconv.Atoi(strings.TrimSpace(numberOfProcessesBytes.String()))
			Expect(err).ToNot(HaveOccurred(), "Failed to convert number of processes to int")

			// Use a random interval between 1 and 31 seconds
			interval := time.Duration(GinkgoRandomSeed()%30+1) * time.Second
			newLogReduceVal := "enhanced " + interval.String()

			By(fmt.Sprintf("updating the logReduce to %s on node %s", newLogReduceVal, nodeName))
			err = setLogReduceValForNode(RANConfig.Spoke1APIClient, nodeName, newLogReduceVal)
			Expect(err).ToNot(HaveOccurred(), "Failed to set logReduce value")

			// Wait for PTP processes to stabilize with new configuration
			time.Sleep(10 * time.Second)

			By(fmt.Sprintf("validating the logs shows every %s on node %s", interval.String(), nodeName))
			validateLogWithInterval(RANConfig.Spoke1APIClient, nodeName, interval, numberOfProcesses)
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
func setLogReduceValForNode(client *clients.Settings, nodeName string, newLogReduceVal string) error {
	startTime := time.Now()

	By("getting all PtpConfigs")

	ptpConfigList, err := ptp.ListPtpConfigs(client)
	if err != nil {
		return fmt.Errorf("failed to list PtpConfigs: %w", err)
	}

	configsUpdated := false

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

			configsUpdated = true
		}
	}

	if configsUpdated {
		By(fmt.Sprintf("waiting for the config to be reloaded on node %s", nodeName))

		err = ptpdaemon.WaitForProfileLoad(client, nodeName,
			ptpdaemon.WithStartTime(startTime),
			ptpdaemon.WithTimeout(5*time.Minute))
		if err != nil {
			return fmt.Errorf("failed to wait for profile load on node %s: %w", nodeName, err)
		}
	}

	return nil
}

// validateLogWithInterval validates that the log reduction is working correctly with the specified interval.
// It waits for a 1-minute window, then counts log entries matching the pattern. The expected count is
// (windowDuration/interval * numberOfProcesses), with adaptive tolerance to account for timing variations.
// For short intervals (≤5s), uses higher tolerance as they're more sensitive to timing variations.
func validateLogWithInterval(client *clients.Settings, nodeName string, interval time.Duration, numberOfProcesses int) {
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

	pattern := `(phc|master)\soffset\ssummary:\scnt=[0-9]+,\smin=-?[0-9]+,\smax=-?[0-9]+,\savg=-?` +
		`[0-9]+.?[0-9]*,\sSD=-?[0-9]+.?[0-9]*`

	regexLog, err := regexp.Compile(pattern)
	Expect(err).ToNot(HaveOccurred(), "Failed to compile regex")

	numberOfLogs := len(regexLog.FindAllString(string(logs), -1))

	klog.V(tsparams.LogLevel).Infof("Found %d log entries matching pattern for node %s", numberOfLogs, nodeName)

	Expect(numberOfLogs).To(And(
		BeNumerically(">=", int(windowDuration/interval)*(numberOfProcesses-1)-numberOfProcesses),
		BeNumerically("<", int(windowDuration/interval)*(numberOfProcesses+1)+numberOfProcesses)),
		"Log count should be within expected range for interval %s", interval)
}
