package ran_du_system_test

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/ptp"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/shell"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuparams"
)

var _ = Describe(
	"LaunchWorkloadMultipleIterations",
	Ordered,
	ContinueOnFailure,
	Label("LaunchWorkloadMultipleIterations"), func() {
		It("Launch workload multiple times", reportxml.ID("45698"), Label("LaunchWorkloadMultipleIterations"), func() {

			By("Launch workload")
			for iter := 0; iter < RanDuTestConfig.LaunchWorkloadIterations; iter++ {
				fmt.Printf("Launch workload iteration no. %d\n", iter)

				if namespace.NewBuilder(APIClient, RanDuTestConfig.TestWorkload.Namespace).Exists() {
					By("Deleting workload using shell method")
					_, err := shell.ExecuteCmd(RanDuTestConfig.TestWorkload.DeleteShellCmd)
					Expect(err).ToNot(HaveOccurred(), "Failed to delete workload")
				}

				if RanDuTestConfig.TestWorkload.CreateMethod == randuparams.TestWorkloadShellLaunchMethod {
					By("Launching workload using shell method")
					_, err := shell.ExecuteCmd(RanDuTestConfig.TestWorkload.CreateShellCmd)
					Expect(err).ToNot(HaveOccurred(), "Failed to launch workload")
				}

				By("Waiting for deployment replicas to become ready")
				_, err := await.WaitUntilAllDeploymentsReady(APIClient, RanDuTestConfig.TestWorkload.Namespace,
					randuparams.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "error while waiting for deployment to become ready")

				By("Waiting for statefulset replicas to become ready")
				_, err = await.WaitUntilAllStatefulSetsReady(APIClient, RanDuTestConfig.TestWorkload.Namespace,
					randuparams.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "error while waiting for statefulsets to become ready")

				By("Waiting for all pods to become ready")
				_, err = await.WaitUntilAllPodsReady(APIClient, RanDuTestConfig.TestWorkload.Namespace, randuparams.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "pod not ready: %s", err)

				if RanDuTestConfig.PtpEnabled {
					timeInterval := 3 * time.Minute
					time.Sleep(timeInterval)

					By("Check PTP status for the last 3 minutes after workload deployment")
					ptpOnSync, err := ptp.ValidatePTPStatus(APIClient, timeInterval)
					Expect(err).ToNot(HaveOccurred(), "PTP Error: %s", err)
					Expect(ptpOnSync).To(Equal(true))
				}

			}

			By("Observe node load average while workload is running")
			cmdToExec := "cat /proc/loadavg"

			for n := 0; n < 30; n++ {
				cmdResult, err := cluster.ExecCmdWithStdout(APIClient, cmdToExec)
				Expect(err).ToNot(HaveOccurred(), "could not execute command: %s", err)

				for _, stdout := range cmdResult {
					if len(strings.Fields(stdout)) > 0 {
						buf := strings.Fields(stdout)[0]
						floatBuf, err := strconv.ParseFloat(strings.ReplaceAll(strings.ReplaceAll(buf, "\r", ""), "\n", ""), 32)
						if err != nil {
							fmt.Println(err)
						}
						Expect(floatBuf).To(BeNumerically("<", randuparams.TestMultipleLaunchWorkloadLoadAvg),
							"error: node load average detected above 100: %f", floatBuf)
					}
				}

				time.Sleep(10 * time.Second)
			}

		})
		AfterAll(func() {
			By("Cleaning up test workload resources")
			_, err := shell.ExecuteCmd(RanDuTestConfig.TestWorkload.DeleteShellCmd)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete workload")
		})
	})
