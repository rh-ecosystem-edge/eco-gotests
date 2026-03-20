package ran_du_system_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuinittools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	ptpNamespace           = "openshift-ptp"
	ptpDaemonLabelSelector = "app=linuxptp-daemon"
	ptpContainerName       = "linuxptp-daemon-container"
)

// getUbloxProtocolVersion returns the u-blox protocol version for ubxtool based on PtpConfig profiles.
// E825/E830 use 29.25, E810 uses 29.20. Returns empty string if no GNSS-capable profile is found.
func getUbloxProtocolVersion() (string, error) {
	ptpConfigs, err := ptp.ListPtpConfigs(APIClient)
	if err != nil {
		return "", fmt.Errorf("failed to list PtpConfigs: %w", err)
	}
	for _, cfg := range ptpConfigs {
		for _, profile := range cfg.Definition.Spec.Profile {
			if profile.Plugins == nil {
				continue
			}
			if _, has := profile.Plugins["e825"]; has {
				return "29.25", nil
			}
			if _, has := profile.Plugins["e830"]; has {
				return "29.25", nil
			}
			if _, has := profile.Plugins["e810"]; has {
				return "29.20", nil
			}
		}
	}
	return "", fmt.Errorf("no PtpConfig profile with e810, e825, or e830 plugin found")
}

// simulateGNSSLoss simulates GNSS sync loss via ubxtool (sets required satellites to 50).
// Uses ubxtool -z CFG-NAVSPG-INFIL_NCNOTHRS,50,1 to require 50 satellites for a fix (unattainable).
func simulateGNSSLoss(nodeName, protocolVersion string) error {
	cmd := fmt.Sprintf("ubxtool -P %s -w 1 -v 3 -z CFG-NAVSPG-INFIL_NCNOTHRS,50,1", protocolVersion)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		daemonPod, err := getPtpDaemonPodOnNode(nodeName)
		if err != nil {
			lastErr = err
			time.Sleep(5 * time.Second)
			continue
		}
		buf, err := daemonPod.ExecCommand([]string{"sh", "-c", cmd}, ptpContainerName)
		if err != nil {
			lastErr = fmt.Errorf("ubxtool failed: %w, output: %s", err, buf.String())
			time.Sleep(5 * time.Second)
			continue
		}
		return nil
	}
	return lastErr
}

// simulateGNSSRecovery restores GNSS sync via ubxtool (resets required satellites to 0).
func simulateGNSSRecovery(nodeName, protocolVersion string) error {
	cmd := fmt.Sprintf("ubxtool -P %s -w 1 -v 3 -z CFG-NAVSPG-INFIL_NCNOTHRS,0,1", protocolVersion)
	daemonPod, err := getPtpDaemonPodOnNode(nodeName)
	if err != nil {
		return err
	}
	buf, err := daemonPod.ExecCommand([]string{"sh", "-c", cmd}, ptpContainerName)
	if err != nil {
		return fmt.Errorf("ubxtool recovery failed: %w, output: %s", err, buf.String())
	}
	return nil
}

// getPtpDaemonPodOnNode returns the PTP daemon pod running on the specified node.
func getPtpDaemonPodOnNode(nodeName string) (*pod.Builder, error) {
	daemonPods, err := pod.List(APIClient, ptpNamespace, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{"app": "linuxptp-daemon"}).String(),
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list PTP daemon pods on node %s: %w", nodeName, err)
	}
	if len(daemonPods) != 1 {
		return nil, fmt.Errorf("expected exactly one PTP daemon pod on node %s, found %d", nodeName, len(daemonPods))
	}
	return daemonPods[0], nil
}

const (
	// ptp3WpcExpectedClockClass is the expected Grandmaster ClockClass when synchronized.
	ptp3WpcExpectedClockClass = "6"
)

var _ = Describe(
	"PTP 3 WPC Validation",
	Ordered,
	Label("PTP3WPCValidation"),
	func() {
		var ptpNodes []string

		BeforeAll(func() {
			if !RanDuTestConfig.PtpEnabled {
				Skip("PTP is not enabled in RanDu configuration")
			}

			By("Listing PTP daemon pods")

			ptpPods, err := pod.List(APIClient, ptpNamespace, metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(labels.Set{"app": "linuxptp-daemon"}).String(),
			})
			Expect(err).ToNot(HaveOccurred(), "Failed to list PTP daemon pods")
			Expect(ptpPods).ToNot(BeEmpty(), "No PTP daemon pods found")

			nodeSet := make(map[string]struct{})
			for _, p := range ptpPods {
				if p.Object.Spec.NodeName != "" {
					nodeSet[p.Object.Spec.NodeName] = struct{}{}
				}
			}
			ptpNodes = make([]string, 0, len(nodeSet))
			for n := range nodeSet {
				ptpNodes = append(ptpNodes, n)
			}

			klog.V(randuparams.RanDuLogLevel).Infof("PTP 3 WPC validation will run on nodes: %v", ptpNodes)
		})

		// Case 01: To check GNSS Lock and NMEA Integrity
		// Test_Description: Verify the primary WPC card ens3f0 achieves a stable 1pps lock using PPS data.
		It("Case 01: To check GNSS Lock and NMEA Integrity", reportxml.ID("99991"), func() {
			for _, nodeName := range ptpNodes {
				By("Verify the primary WPC card ens3f0 achieves a stable 1pps lock using PPS data")

				daemonPod, err := getPtpDaemonPodOnNode(nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon pod on node %s", nodeName)

				logs, err := daemonPod.GetLogsWithOptions(&corev1.PodLogOptions{
					Container: ptpContainerName,
					TailLines: ptr(int64(500)),
				})
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon logs on node %s", nodeName)

				logStr := string(logs)

				// Look for gnss_status 5 (3D fix)
				Expect(logStr).To(ContainSubstring("gnss_status"),
					"Node %s: no gnss_status found in logs", nodeName)
				Expect(logStr).To(ContainSubstring("gnss_status 5"),
					"Node %s: expected gnss_status 5 (3D fix), check GNSS lock", nodeName)

				// Look for s2 (synchronized state)
				Expect(logStr).To(ContainSubstring(" s2"),
					"Node %s: expected sync state s2 in logs", nodeName)

				// Look for primary WPC card ens3f0 with s2 (stable 1pps lock)
				Expect(logStr).To(ContainSubstring("ens3f0"),
					"Node %s: expected primary WPC card ens3f0 in logs", nodeName)

				// Look for small offset (e.g., offset 2) - offset should be a small integer
				offsetRe := regexp.MustCompile(`offset\s+(-?\d+)`)
				matches := offsetRe.FindAllStringSubmatch(logStr, -1)
				Expect(matches).ToNot(BeEmpty(),
					"Node %s: no offset values found in logs", nodeName)

				// At least one offset should be a small integer (e.g., within ±100 ns)
				foundSmallOffset := false
				for _, m := range matches {
					if len(m) >= 2 {
						var offset int
						_, _ = fmt.Sscanf(m[1], "%d", &offset)
						if offset >= -100 && offset <= 100 {
							foundSmallOffset = true
							break
						}
					}
				}
				Expect(foundSmallOffset).To(BeTrue(),
					"Node %s: expected at least one small offset (e.g., ±100), check PTP sync", nodeName)
			}
		})

		// Case 02: To verify hardware sync between inter-card pps alignment
		// Test_Description: Verify the physical 1pps signal is distributed from the master card to secondary cards via sma cables.
		It("Case 02: To verify hardware sync between inter-card pps alignment", reportxml.ID("99992"), func() {
			interfaceRe := regexp.MustCompile(`(ens1f0|ens2f0|ens3f0).*s2`)

			for _, nodeName := range ptpNodes {
				By("Verify the physical 1pps signal is distributed from the master card to secondary cards via sma cables")

				daemonPod, err := getPtpDaemonPodOnNode(nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon pod on node %s", nodeName)

				logs, err := daemonPod.GetLogsWithOptions(&corev1.PodLogOptions{
					Container: ptpContainerName,
					TailLines: ptr(int64(1000)),
				})
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon logs on node %s", nodeName)

				logStr := string(logs)
				lines := strings.Split(logStr, "\n")

				foundInterfaces := make(map[string]bool)
				for _, line := range lines {
					if strings.Contains(line, "s2") &&
						(strings.Contains(line, "ens1f0") || strings.Contains(line, "ens2f0") || strings.Contains(line, "ens3f0")) {
						if interfaceRe.MatchString(line) {
							for _, iface := range []string{"ens1f0", "ens2f0", "ens3f0"} {
								if strings.Contains(line, iface) {
									foundInterfaces[iface] = true
									break
								}
							}
						}
					}
				}

				// Alternative: check for each interface with s2 in the same line
				for _, iface := range []string{"ens1f0", "ens2f0", "ens3f0"} {
					hasSync := false
					for _, line := range lines {
						if strings.Contains(line, iface) && strings.Contains(line, "s2") {
							hasSync = true
							break
						}
					}
					if hasSync {
						foundInterfaces[iface] = true
					}
				}

				for _, iface := range []string{"ens1f0", "ens2f0", "ens3f0"} {
					Expect(foundInterfaces[iface]).To(BeTrue(),
						"Node %s: interface %s must report s2 (sync state). "+
							"If s0 is shown, check physical SMA cable connection", nodeName, iface)
				}
			}
		})

		// Case 03: To verify dpll stability hardware dpll phase and freq lock
		// Test_Description: Verify the E810 dpll hardware state for long-term frequency stability.
		It("Case 03: To verify dpll stability hardware dpll phase and freq lock", reportxml.ID("99993"), func() {
			for _, nodeName := range ptpNodes {
				By("Verify the E810 dpll hardware state for long-term frequency stability")

				daemonPod, err := getPtpDaemonPodOnNode(nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon pod on node %s", nodeName)

				logs, err := daemonPod.GetLogsWithOptions(&corev1.PodLogOptions{
					Container: ptpContainerName,
					TailLines: ptr(int64(1000)),
				})
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon logs on node %s", nodeName)

				logStr := string(logs)

				Expect(logStr).To(ContainSubstring("dpll"),
					"Node %s: no dpll entries found in logs", nodeName)

				// Look for phase_status 3 and frequency_status 3
				Expect(logStr).To(ContainSubstring("phase_status 3"),
					"Node %s: expected phase_status 3 for high-precision hardware lock", nodeName)
				Expect(logStr).To(ContainSubstring("frequency_status 3"),
					"Node %s: expected frequency_status 3 for high-precision hardware lock", nodeName)
			}
		})

		// Case 04: To verify t-gm ptp announce messages validation from linuxptp pod
		// Test_Description: Verify the pod is reporting the correct ptp class and announce a message for the same as a grandmaster.
		It("Case 04: To verify t-gm ptp announce messages validation from linuxptp pod", reportxml.ID("99994"), func() {
			for _, nodeName := range ptpNodes {
				By("Verify the pod is reporting the correct ptp class and announce a message for the same as a grandmaster")

				daemonPod, err := getPtpDaemonPodOnNode(nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon pod on node %s", nodeName)

				buf, err := daemonPod.ExecCommand([]string{"sh", "-c",
					"pmc -u -b 0 'GET PARENT_DATASET' 2>/dev/null | grep 'gm.ClockClass'"}, ptpContainerName)
				Expect(err).ToNot(HaveOccurred(), "Failed to execute pmc on node %s", nodeName)
				output := buf.String()

				Expect(output).To(ContainSubstring("gm.ClockClass"),
					"Node %s: no gm.ClockClass in pmc output", nodeName)
				Expect(output).To(ContainSubstring("gm.ClockClass "+ptp3WpcExpectedClockClass),
					"Node %s: expected gm.ClockClass 6, got: %s. "+
						"ClockClass 7 or higher indicates node is not a synchronized Grandmaster", nodeName, output)
			}
		})

		// Case 05: To verify t-gm ptp sync locked back after GNSS signal loss and retrieved
		// Test_Description: Verify the clock status after GNSS signal loss, then verify it again once the signal is restored.
		It("Case 05: To verify t-gm ptp sync locked back after GNSS signal loss and retrieved", reportxml.ID("99995"), func() {
			protocolVersion, err := getUbloxProtocolVersion()
			if err != nil {
				Skip("GNSS simulation requires PtpConfig with e810/e825/e830 plugin: " + err.Error())
			}

			for _, nodeName := range ptpNodes {
				By("Verify the clock status after GNSS signal loss, then verify it again once the signal is restored")

				DeferCleanup(func() {
					By("Restoring GNSS sync")
					if restoreErr := simulateGNSSRecovery(nodeName, protocolVersion); restoreErr != nil {
						klog.Errorf("Failed to restore GNSS on node %s: %v", nodeName, restoreErr)
					}
				})

				By("Simulating GNSS signal loss")
				err = simulateGNSSLoss(nodeName, protocolVersion)
				Expect(err).ToNot(HaveOccurred(), "Failed to simulate GNSS loss on node %s", nodeName)

				gnssLossTime := time.Now()

				By("Verifying clock status after GNSS signal loss (holdover/freerun)")
				var foundHoldoverOrFreerun bool
				err = wait.PollUntilContextTimeout(
					context.TODO(), 10*time.Second, 5*time.Minute, true,
					func(ctx context.Context) (bool, error) {
						daemonPod, podErr := getPtpDaemonPodOnNode(nodeName)
						if podErr != nil {
							return false, nil
						}
						logs, logErr := daemonPod.GetLogsWithOptions(&corev1.PodLogOptions{
							Container: ptpContainerName,
							SinceTime: &metav1.Time{Time: gnssLossTime},
						})
						if logErr != nil {
							return false, nil
						}
						logStr := strings.ToLower(string(logs))
						foundHoldoverOrFreerun = strings.Contains(logStr, "holdover") || strings.Contains(logStr, "freerun")
						return foundHoldoverOrFreerun, nil
					})
				Expect(err).ToNot(HaveOccurred(), "Timeout waiting for holdover/freerun in logs on node %s", nodeName)
				Expect(foundHoldoverOrFreerun).To(BeTrue(),
					"Node %s: expected 'holdover' or 'freerun' in logs after GNSS loss simulation", nodeName)

				By("Restoring GNSS signal")
				err = simulateGNSSRecovery(nodeName, protocolVersion)
				Expect(err).ToNot(HaveOccurred(), "Failed to restore GNSS on node %s", nodeName)

				By("Verifying clock status after GNSS signal is restored (sync locked)")
				time.Sleep(30 * time.Second) // Allow time for sync to stabilize
				daemonPod, err := getPtpDaemonPodOnNode(nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon pod on node %s", nodeName)
				logs, err := daemonPod.GetLogsWithOptions(&corev1.PodLogOptions{
					Container: ptpContainerName,
					TailLines: ptr(int64(200)),
				})
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon logs on node %s", nodeName)
				logStr := string(logs)
				Expect(logStr).To(ContainSubstring(" s2"),
					"Node %s: expected sync state s2 after GNSS restore, clock should be locked", nodeName)
			}
		})
	})

func ptr[T any](v T) *T {
	return &v
}
