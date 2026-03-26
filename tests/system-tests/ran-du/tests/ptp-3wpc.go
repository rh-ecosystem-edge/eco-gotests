package ran_du_system_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	sysptp "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/ptp"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
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

			ptpPods, err := pod.List(APIClient, sysptp.Namespace, metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(labels.Set{
					sysptp.DaemonPodLabelKey: sysptp.DaemonPodLabelValueLinuxpt,
				}).String(),
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

		It("Case 01: To check GNSS Lock and NMEA Integrity", reportxml.ID("99991"), func() {
			for _, nodeName := range ptpNodes {
				By("Verify primary WPC PHC achieves a stable 1pps lock using PPS data")

				daemonPod, err := sysptp.GetLinuxptpDaemonPodOnNode(APIClient, nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon pod on node %s", nodeName)

				logs, err := daemonPod.GetLogsWithOptions(&corev1.PodLogOptions{
					Container: sysptp.DaemonContainerName,
					TailLines: ptr(int64(500)),
				})
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon logs on node %s", nodeName)

				logStr := string(logs)

				plan, err := sysptp.ResolveWPCInterfaces(
					RanDuTestConfig.PtpWpcSyncInterfaces,
					RanDuTestConfig.PtpWpcPrimaryInterface,
					logStr,
				)
				Expect(err).ToNot(HaveOccurred(), "Node %s: %v", nodeName, err)

				Expect(logStr).To(ContainSubstring("gnss_status"),
					"Node %s: no gnss_status found in logs", nodeName)
				Expect(logStr).To(ContainSubstring("gnss_status 5"),
					"Node %s: expected gnss_status 5 (3D fix), check GNSS lock", nodeName)

				Expect(logStr).To(ContainSubstring(sysptp.StateSubscribedLogMark),
					"Node %s: expected sync state s2 in logs", nodeName)

				Expect(logStr).To(ContainSubstring(plan.Primary),
					"Node %s: expected primary PHC %s in logs", nodeName, plan.Primary)

				offsetRe := regexp.MustCompile(`offset\s+(-?\d+)`)
				matches := offsetRe.FindAllStringSubmatch(logStr, -1)
				Expect(matches).ToNot(BeEmpty(),
					"Node %s: no offset values found in logs", nodeName)

				foundSmallOffset := false

				for _, m := range matches {
					if len(m) >= 2 {
						var offset int

						_, _ = fmt.Sscanf(m[1], "%d", &offset)
						if offset >= -sysptp.DefaultMaxAbsOffsetNS && offset <= sysptp.DefaultMaxAbsOffsetNS {
							foundSmallOffset = true

							break
						}
					}
				}

				Expect(foundSmallOffset).To(BeTrue(),
					"Node %s: expected at least one small offset within ±%d ns, check PTP sync",
					nodeName, sysptp.DefaultMaxAbsOffsetNS)
			}
		})

		// Case 02: To verify hardware sync between inter-card pps alignment
		// Test_Description: Verify the physical 1pps signal is distributed from the master card to
		// secondary cards via sma cables.
		It("Case 02: To verify hardware sync between inter-card pps alignment", reportxml.ID("99992"), func() {
			for _, nodeName := range ptpNodes {
				By("Verify physical 1pps is distributed to secondary cards via SMA cables")

				daemonPod, err := sysptp.GetLinuxptpDaemonPodOnNode(APIClient, nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon pod on node %s", nodeName)

				logs, err := daemonPod.GetLogsWithOptions(&corev1.PodLogOptions{
					Container: sysptp.DaemonContainerName,
					TailLines: ptr(int64(1000)),
				})
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon logs on node %s", nodeName)

				logStr := string(logs)
				plan, err := sysptp.ResolveWPCInterfaces(
					RanDuTestConfig.PtpWpcSyncInterfaces,
					RanDuTestConfig.PtpWpcPrimaryInterface,
					logStr,
				)
				Expect(err).ToNot(HaveOccurred(), "Node %s: %v", nodeName, err)

				ifaceRe, err := sysptp.IfaceSyncLineRegexp(plan.SyncAll)
				Expect(err).ToNot(HaveOccurred())

				lines := strings.Split(logStr, "\n")
				foundInterfaces := make(map[string]bool)

				for _, line := range lines {
					if !strings.Contains(line, "s2") {
						continue
					}

					for _, iface := range plan.SyncAll {
						if strings.Contains(line, iface) && ifaceRe.MatchString(line) {
							foundInterfaces[iface] = true
						}
					}
				}

				for _, iface := range plan.SyncAll {
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

				for _, iface := range plan.SyncAll {
					Expect(foundInterfaces[iface]).To(BeTrue(),
						"Node %s: interface %s must report s2 (sync state). "+
							"If s0 is shown, check physical SMA cable connection", nodeName, iface)
				}
			}
		})

		It("Case 03: To verify dpll stability hardware dpll phase and freq lock", reportxml.ID("99993"), func() {
			for _, nodeName := range ptpNodes {
				By("Verify the E810 dpll hardware state for long-term frequency stability")

				daemonPod, err := sysptp.GetLinuxptpDaemonPodOnNode(APIClient, nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon pod on node %s", nodeName)

				logs, err := daemonPod.GetLogsWithOptions(&corev1.PodLogOptions{
					Container: sysptp.DaemonContainerName,
					TailLines: ptr(int64(1000)),
				})
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon logs on node %s", nodeName)

				logStr := string(logs)

				Expect(logStr).To(ContainSubstring("dpll"),
					"Node %s: no dpll entries found in logs", nodeName)

				Expect(logStr).To(ContainSubstring("phase_status 3"),
					"Node %s: expected phase_status 3 for high-precision hardware lock", nodeName)
				Expect(logStr).To(ContainSubstring("frequency_status 3"),
					"Node %s: expected frequency_status 3 for high-precision hardware lock", nodeName)
			}
		})

		// Case 04: To verify t-gm ptp announce messages validation from linuxptp pod
		// Test_Description: Verify the pod is reporting the correct ptp class and announce a message
		// for the same as a grandmaster.
		It("Case 04: To verify t-gm ptp announce messages validation from linuxptp pod", reportxml.ID("99994"), func() {
			for _, nodeName := range ptpNodes {
				By("Verify correct PTP class and grandmaster announce from linuxptp pod")

				daemonPod, err := sysptp.GetLinuxptpDaemonPodOnNode(APIClient, nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon pod on node %s", nodeName)

				pmcParent := "pmc -u -b 0 'GET PARENT_DATASET' 2>/dev/null | grep 'gm.ClockClass'"
				buf, err := daemonPod.ExecCommand(
					[]string{"sh", "-c", pmcParent},
					sysptp.DaemonContainerName,
				)
				Expect(err).ToNot(HaveOccurred(), "Failed to execute pmc on node %s", nodeName)

				output := buf.String()

				Expect(output).To(ContainSubstring("gm.ClockClass"),
					"Node %s: no gm.ClockClass in pmc output", nodeName)
				Expect(output).To(ContainSubstring("gm.ClockClass "+ptp3WpcExpectedClockClass),
					"Node %s: expected gm.ClockClass 6, got: %s. "+
						"ClockClass 7 or higher indicates node is not a synchronized Grandmaster", nodeName, output)
			}
		})

		It("Case 05: To verify t-gm ptp sync locked back after GNSS signal loss and retrieved",
			reportxml.ID("99995"), func() {
				for _, nodeName := range ptpNodes {
					protocolVersion, err := sysptp.GetUbloxProtocolVersion(APIClient, nodeName)
					if err != nil {
						Skip("GNSS simulation requires PtpConfig with e810/e825/e830 for node " + nodeName + ": " + err.Error())
					}

					By("Verify clock status after GNSS signal loss, then after restore")

					DeferCleanup(func() {
						By("Restoring GNSS sync")

						if restoreErr := sysptp.SimulateGNSSRecovery(APIClient, nodeName, protocolVersion); restoreErr != nil {
							klog.Errorf("Failed to restore GNSS on node %s: %v", nodeName, restoreErr)
						}
					})

					By("Simulating GNSS signal loss")

					err = sysptp.SimulateGNSSLoss(APIClient, nodeName, protocolVersion)
					Expect(err).ToNot(HaveOccurred(), "Failed to simulate GNSS loss on node %s", nodeName)

					gnssLossTime := time.Now()

					By("Verifying clock status after GNSS signal loss (holdover/freerun)")

					var foundHoldoverOrFreerun bool

					err = wait.PollUntilContextTimeout(
						context.TODO(), 10*time.Second, 5*time.Minute, true,
						func(ctx context.Context) (bool, error) {
							daemonPod, podErr := sysptp.GetLinuxptpDaemonPodOnNode(APIClient, nodeName)
							if podErr != nil {
								return false, nil
							}

							logs, logErr := daemonPod.GetLogsWithOptions(&corev1.PodLogOptions{
								Container: sysptp.DaemonContainerName,
								SinceTime: &metav1.Time{Time: gnssLossTime},
							})
							if logErr != nil {
								return false, nil
							}

							logStr := strings.ToLower(string(logs))
							foundHoldoverOrFreerun = strings.Contains(logStr, sysptp.LogKeywordHoldover) ||
								strings.Contains(logStr, sysptp.LogKeywordFreerun)

							return foundHoldoverOrFreerun, nil
						})
					Expect(err).ToNot(HaveOccurred(), "Timeout waiting for holdover/freerun in logs on node %s", nodeName)
					Expect(foundHoldoverOrFreerun).To(BeTrue(),
						"Node %s: expected 'holdover' or 'freerun' in logs after GNSS loss simulation", nodeName)

					By("Restoring GNSS signal")

					err = sysptp.SimulateGNSSRecovery(APIClient, nodeName, protocolVersion)
					Expect(err).ToNot(HaveOccurred(), "Failed to restore GNSS on node %s", nodeName)

					By("Verifying clock status after GNSS signal is restored (sync locked)")

					restoreSince := time.Now()

					err = wait.PollUntilContextTimeout(
						context.TODO(), 5*time.Second, 5*time.Minute, true,
						func(ctx context.Context) (bool, error) {
							daemonPod, podErr := sysptp.GetLinuxptpDaemonPodOnNode(APIClient, nodeName)
							if podErr != nil {
								return false, nil
							}

							logs, logErr := daemonPod.GetLogsWithOptions(&corev1.PodLogOptions{
								Container: sysptp.DaemonContainerName,
								SinceTime: &metav1.Time{Time: restoreSince},
							})
							if logErr != nil {
								return false, nil
							}

							return strings.Contains(string(logs), sysptp.StateSubscribedLogMark), nil
						})
					Expect(err).ToNot(HaveOccurred(),
						"Timeout waiting for sync state s2 after GNSS restore on node %s", nodeName)
				}
			})

		It("Case 06: PTP Accuracy under high network throughput", reportxml.ID("99996"), func() {
			if strings.TrimSpace(RanDuTestConfig.PtpIperf3Server) == "" {
				Skip("Case 06 requires ptp_iperf3_server (ECO_RANDU_PTP_IPERF3_SERVER) pointing at an iperf3 server")
			}

			dur := RanDuTestConfig.PtpIperf3DurationSec
			if dur <= 0 {
				dur = 300
			}

			waitSec := RanDuTestConfig.PtpLockedStateWaitSec
			if waitSec > 0 {
				By(fmt.Sprintf("Waiting %d seconds for stable locked state before stress (test setup)", waitSec))
				time.Sleep(time.Duration(waitSec) * time.Second)
			}

			for _, nodeName := range ptpNodes {
				nodeName := nodeName

				By(fmt.Sprintf("Verify PTP offset stays within ±%dns under iperf3 load on node %s",
					sysptp.DefaultMaxAbsOffsetNS, nodeName))

				srv := strings.TrimSpace(RanDuTestConfig.PtpIperf3Server)

				iperfParts := []string{
					"setsid", "iperf3", "-c", sysptp.ShellQuoteArg(srv), "-t", strconv.Itoa(dur),
				}
				if b := strings.TrimSpace(RanDuTestConfig.PtpIperf3ClientBind); b != "" {
					iperfParts = append(iperfParts, "-B", sysptp.ShellQuoteArg(b))
				}

				startCmd := strings.Join(iperfParts, " ") +
					" </dev/null >/tmp/eco-ptp-iperf.log 2>&1 & echo $!"

				out, err := sysptp.ExecCmdOnNodeHost(APIClient, nodeName, startCmd)
				Expect(err).ToNot(HaveOccurred(), "failed to start iperf3 on node %s: %s", nodeName, out)

				pid := strings.TrimSpace(out)
				if idx := strings.LastIndex(pid, "\n"); idx >= 0 {
					pid = strings.TrimSpace(pid[idx+1:])
				}

				pidCopy := pid

				DeferCleanup(func() {
					killCmd := fmt.Sprintf("kill %s 2>/dev/null || true", sysptp.ShellQuoteArg(pidCopy))
					_, _ = sysptp.ExecCmdOnNodeHost(APIClient, nodeName, killCmd)
				})

				time.Sleep(3 * time.Second)

				iperfLog, err := sysptp.ExecCmdOnNodeHost(APIClient, nodeName,
					"cat /tmp/eco-ptp-iperf.log 2>/dev/null || true")
				Expect(err).ToNot(HaveOccurred(), "read iperf3 log on node %s", nodeName)

				psScript := fmt.Sprintf("ps -p %s -o pid= 2>/dev/null || true", sysptp.ShellQuoteArg(pidCopy))
				psOut, err := sysptp.ExecCmdOnNodeHost(APIClient, nodeName, psScript)
				Expect(err).ToNot(HaveOccurred(), "check iperf3 pid on node %s", nodeName)

				Expect(strings.TrimSpace(psOut)).NotTo(BeEmpty(),
					"iperf3 process not running on node %s (pid=%s); iperf log: %q",
					nodeName, pidCopy, iperfLog)
				Expect(strings.TrimSpace(iperfLog)).NotTo(BeEmpty(),
					"iperf3 log empty on node %s after start (pid=%s); check %s and connectivity",
					nodeName, pidCopy, srv)

				Expect(iperfLog).To(Or(
					ContainSubstring("Connecting to host"),
					ContainSubstring("connected"),
					ContainSubstring("iperf3"),
					ContainSubstring("Server listening"),
				), "iperf3 client did not produce expected output on node %s (pid=%s); log=%q",
					nodeName, pidCopy, iperfLog)

				sinceOffsets := time.Now()

				deadline := time.Now().Add(time.Duration(dur) * time.Second)
				for time.Now().Before(deadline) {
					time.Sleep(15 * time.Second)

					if time.Now().After(deadline) {
						break
					}

					daemonPod, podErr := sysptp.GetLinuxptpDaemonPodOnNode(APIClient, nodeName)
					Expect(podErr).ToNot(HaveOccurred())

					logs, logErr := daemonPod.GetLogsWithOptions(&corev1.PodLogOptions{
						Container: sysptp.DaemonContainerName,
						SinceTime: &metav1.Time{Time: sinceOffsets},
					})
					Expect(logErr).ToNot(HaveOccurred())

					ok, detail := sysptp.PTPOffsetsWithinSymmetricNS(string(logs), sysptp.DefaultMaxAbsOffsetNS)
					Expect(ok).To(BeTrue(), "node %s: %s", nodeName, detail)
				}

				killEnd := fmt.Sprintf("kill %s 2>/dev/null || true", sysptp.ShellQuoteArg(pidCopy))
				_, _ = sysptp.ExecCmdOnNodeHost(APIClient, nodeName, killEnd)
			}
		})

		It("Case 07: Robustness against PTP packet loss", reportxml.ID("99997"), func() {
			iface := strings.TrimSpace(RanDuTestConfig.PtpNetemInterface)
			if iface == "" {
				Skip("Case 07 requires ptp_netem_interface (ECO_RANDU_PTP_NETEM_INTERFACE)")
			}

			waitSec := RanDuTestConfig.PtpLockedStateWaitSec
			if waitSec > 0 {
				By(fmt.Sprintf("Waiting %d seconds for stable locked state before netem (test setup)", waitSec))
				time.Sleep(time.Duration(waitSec) * time.Second)
			}

			for _, nodeName := range ptpNodes {
				nodeName := nodeName

				By(fmt.Sprintf("Verify clock stays locked with 5%% loss on %s (node %s)", iface, nodeName))

				ifaceQ := sysptp.ShellQuoteArg(iface)

				DeferCleanup(func() {
					delCmd := fmt.Sprintf("tc qdisc del dev %s root 2>/dev/null || true", ifaceQ)
					_, _ = sysptp.ExecCmdOnNodeHost(APIClient, nodeName, delCmd)
				})

				netemSince := time.Now()
				tcAdd := fmt.Sprintf("tc qdisc add dev %s root netem loss 5%%", ifaceQ)
				_, err := sysptp.ExecCmdOnNodeHost(APIClient, nodeName, tcAdd)
				Expect(err).ToNot(HaveOccurred(), "failed to add netem loss on node %s", nodeName)

				time.Sleep(10 * time.Second)

				daemonPod, err := sysptp.GetLinuxptpDaemonPodOnNode(APIClient, nodeName)
				Expect(err).ToNot(HaveOccurred())

				pmcTimeStatus := "pmc -u -b 0 'GET TIME_STATUS_NP' 2>/dev/null"
				buf, err := daemonPod.ExecCommand(
					[]string{"sh", "-c", pmcTimeStatus},
					sysptp.DaemonContainerName,
				)
				Expect(err).ToNot(HaveOccurred(), "pmc GET TIME_STATUS_NP on node %s", nodeName)

				pmcOut := strings.ToLower(buf.String())
				Expect(pmcOut).ToNot(ContainSubstring(sysptp.LogKeywordFreerun),
					"node %s: TIME_STATUS_NP should not indicate freerun under 5%% loss", nodeName)

				logs, err := daemonPod.GetLogsWithOptions(&corev1.PodLogOptions{
					Container: sysptp.DaemonContainerName,
					SinceTime: &metav1.Time{Time: netemSince},
				})
				Expect(err).ToNot(HaveOccurred())

				logStr := strings.ToLower(string(logs))
				Expect(logStr).ToNot(ContainSubstring(sysptp.LogKeywordFreerun),
					"node %s: logs should not show freerun immediately after induced loss", nodeName)
				Expect(string(logs)).To(ContainSubstring(sysptp.StateSubscribedLogMark),
					"node %s: expected sync state s2 (locked) while loss is applied", nodeName)

				tcDel := fmt.Sprintf("tc qdisc del dev %s root", ifaceQ)
				_, err = sysptp.ExecCmdOnNodeHost(APIClient, nodeName, tcDel)
				Expect(err).ToNot(HaveOccurred(), "failed to remove netem qdisc on node %s", nodeName)
			}
		})
	})

func ptr[T any](v T) *T {
	return &v
}
