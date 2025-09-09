package ran_du_system_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/platform"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/shell"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/stability"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuinittools"
)

var _ = Describe(
	"StabilityNoWorkload",
	Ordered,
	ContinueOnFailure,
	Label("StabilityNoWorkload"), func() {
		var (
			clusterName string
			err         error
		)
		BeforeAll(func() {

			if namespace.NewBuilder(APIClient, RanDuTestConfig.TestWorkload.Namespace).Exists() {
				By("Cleaning up test workload resources")
				_, err := shell.ExecuteCmd(RanDuTestConfig.TestWorkload.DeleteShellCmd)
				Expect(err).ToNot(HaveOccurred(), "Failed to delete workload")
			}

			By("Fetching Cluster name")
			clusterName, err = platform.GetOCPClusterName(APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to get cluster name")

		})
		It("StabilityNoWorkload", reportxml.ID("74522"), Label("StabilityNoWorkload"), func() {

			outputDir := RanDuTestConfig.StabilityOutputPath
			policiesOutputFile := fmt.Sprintf("%s/stability_no_workload_policies.log", outputDir)
			ptpOutputFile := fmt.Sprintf("%s/stability_no_workload_ptp.log", outputDir)
			tunedRestartsOutputFile := fmt.Sprintf("%s/stability_no_workload_tuned_restarts.log", outputDir)
			namespaces := []string{"openshift-etcd", "openshift-apiserver"}

			totalDuration := time.Duration(RanDuTestConfig.StabilityNoWorkloadDurMins) * time.Minute
			interval := time.Duration(RanDuTestConfig.StabilityNoWorkloadIntMins) * time.Minute
			startTime := time.Now()

			By(fmt.Sprintf("Collecting metrics during %d minutes", RanDuTestConfig.StabilityNoWorkloadDurMins))
			for time.Since(startTime) < totalDuration {

				if RanDuTestConfig.PtpEnabled {
					err = stability.SavePTPStatus(APIClient, ptpOutputFile, interval)
					if err != nil {
						fmt.Printf("Error, could not save PTP")
					}
				}

				if RanDuTestConfig.StabilityPoliciesCheck {
					err = stability.SavePolicyStatus(APIClient, clusterName, policiesOutputFile)
					if err != nil {
						fmt.Printf("Error, could not save policies status")
					}
				}

				for _, namespace := range namespaces {
					err = stability.SavePodsRestartsInNamespace(APIClient,
						namespace, fmt.Sprintf("%s/stability_no_workload_%s.log", outputDir, namespace))
					if err != nil {
						fmt.Printf("Error, could not save Pod restarts")
					}

				}

				err = stability.SaveTunedRestarts(APIClient, tunedRestartsOutputFile)
				if err != nil {
					fmt.Printf("Error, could not save tuned restarts")
				}

				time.Sleep(interval)
			}

			// Final check of all values
			By("Check all results")
			var stabilityErrors []string

			// Verify policies
			By("Check Policy changes")
			if RanDuTestConfig.StabilityPoliciesCheck {
				_, err := stability.VerifyStabilityStatusChange(policiesOutputFile)
				if err != nil {
					stabilityErrors = append(stabilityErrors, err.Error())
				}
			}

			// Verify podRestarts
			By("Check Pod restarts")
			for _, namespace := range namespaces {
				_, err := stability.VerifyStabilityStatusChange(fmt.Sprintf("%s/stability_no_workload_%s.log",
					outputDir,
					namespace))
				if err != nil {
					stabilityErrors = append(stabilityErrors, err.Error())
				}
			}

			// Verify PTP output
			By("Check PTP results")
			if RanDuTestConfig.PtpEnabled {
				_, err = stability.VerifyStabilityStatusChange(ptpOutputFile)
				if err != nil {
					stabilityErrors = append(stabilityErrors, err.Error())
				}
			}

			// Verify tuned restarts
			By("Check tuneds restarts")
			_, err = stability.VerifyStabilityStatusChange(tunedRestartsOutputFile)
			if err != nil {
				stabilityErrors = append(stabilityErrors, err.Error())
			}

			By("Check if there been any error")
			if len(stabilityErrors) > 0 {
				Expect(stabilityErrors).ToNot(HaveOccurred(), "One or more errors in stability tests:%s", stabilityErrors)
			}

		})
		AfterAll(func() {
		})
	})
