package tests

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	mcov1 "github.com/openshift/api/machineconfiguration/v1"
	performancev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"github.com/openshift/cluster-node-tuning-operator/pkg/performanceprofile/controller/performanceprofile/components"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nto"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/powermanagement/internal/collect"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/powermanagement/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/powermanagement/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"
	"k8s.io/utils/cpuset"
)

var _ = Describe("Per-core runtime power states tuning", Label(tsparams.LabelPowerSaveTestCases), Ordered, func() {
	var (
		nodeList                []*nodes.Builder
		nodeName                string
		perfProfile             *nto.Builder
		originalPerfProfileSpec performancev2.PerformanceProfileSpec
		err                     error
	)

	BeforeAll(func() {
		nodeList, err = nodes.List(Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to list cluster nodes")

		if len(nodeList) != 1 {
			Skip("Power saving tests only support SNO clusters")
		}

		if nodeList[0].Object.Status.NodeInfo.Architecture != "amd64" {
			Skip("Power saving tests only support Intel nodes")
		}

		nodeName = nodeList[0].Object.Name
		perfProfile, err = helper.GetPerformanceProfileWithCPUSet()
		Expect(err).ToNot(HaveOccurred(), "Failed to get performance profile")

		originalPerfProfileSpec = perfProfile.Object.Spec
	})

	AfterAll(func() {
		if CurrentSpecReport().State.Is(types.SpecStateSkipped) {
			return
		}

		perfProfile, err = helper.GetPerformanceProfileWithCPUSet()
		Expect(err).ToNot(HaveOccurred(), "Failed to get performance profile")

		if reflect.DeepEqual(perfProfile.Object.Spec, originalPerfProfileSpec) {
			klog.V(tsparams.LogLevel).Info("Performance profile did not change, exiting")

			return
		}

		By("Restoring performance profile to original specs")
		perfProfile.Definition.Spec = originalPerfProfileSpec

		_, err = perfProfile.Update(true)
		Expect(err).ToNot(HaveOccurred())
		mcp, err := mco.Pull(Spoke1APIClient, "master")
		Expect(err).ToNot(HaveOccurred(), "Failed to get machineconfigpool")

		err = mcp.WaitToBeInCondition(mcov1.MachineConfigPoolUpdating, corev1.ConditionTrue, 2*tsparams.PowerSaveTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for machineconfigpool to be updating")

		err = mcp.WaitForUpdate(3 * tsparams.PowerSaveTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for machineconfigpool to be updated")
	})

	// 54571 - Install SNO node with standard DU profile that does not include WorkloadHints
	It("Verifies expected kernel parameters with no workload hints specified in PerformanceProfile",
		reportxml.ID("54571"), func() {
			workloadHints := perfProfile.Definition.Spec.WorkloadHints
			if workloadHints != nil {
				Skip("WorkloadHints already present in perfProfile.Spec")
			}

			By("Checking for expected kernel parameters")
			cmdline, err := cluster.ExecCommandOnSNOWithRetries(Spoke1APIClient,
				ranparam.RetryCount, ranparam.RetryInterval, "cat /proc/cmdline")
			Expect(err).ToNot(HaveOccurred(), "Failed to cat /proc/cmdline")

			// Expected default set of kernel parameters when no WorkloadHints are specified in PerformanceProfile
			requiredKernelParms := []string{
				"nohz_full=[0-9,-]+",
				"tsc=nowatchdog",
				"nosoftlockup",
				"nmi_watchdog=0",
				"mce=off",
				"skew_tick=1",
				"intel_pstate=disable",
			}
			for _, parameter := range requiredKernelParms {
				By(fmt.Sprintf("Checking /proc/cmdline for %s", parameter))
				rePattern := regexp.MustCompile(parameter)
				Expect(rePattern.FindStringIndex(cmdline)).
					ToNot(BeNil(), "Kernel parameter %s is missing from cmdline", parameter)
			}
		})

	// 54572 - Enable powersave at node level and then enable performance at node level
	It("Enables powersave at node level and then enable performance at node level", reportxml.ID("54572"), func() {
		By("Patching the performance profile with the workload hints")
		err := helper.SetPowerModeAndWaitForMcpUpdate(perfProfile, *nodeList[0], true, false, true)
		Expect(err).ToNot(HaveOccurred(), "Failed to set power mode")

		cmdline, err := cluster.ExecCommandOnSNOWithRetries(Spoke1APIClient,
			ranparam.RetryCount, ranparam.RetryInterval, "cat /proc/cmdline")
		Expect(err).ToNot(HaveOccurred(), "Failed to cat /proc/cmdline")
		Expect(cmdline).
			To(ContainSubstring("intel_pstate=passive"), "Kernel parameter intel_pstate=passive missing from /proc/cmdline")
		Expect(cmdline).
			ToNot(ContainSubstring("intel_pstate=disable"), "Kernel parameter intel_pstate=disable found on /proc/cmdline")
	})

	// 54574 - Enable powersave at node level and then enable high performance at node level, check power
	// consumption with no workload pods.
	It("Enable powersave, and then enable high performance at node level, check power consumption with no workload pods.",
		reportxml.ID("54574"), func() {
			testPodAnnotations := map[string]string{
				"cpu-load-balancing.crio.io": "disable",
				"cpu-quota.crio.io":          "disable",
				"irq-load-balancing.crio.io": "disable",
				"cpu-c-states.crio.io":       "disable",
				"cpu-freq-governor.crio.io":  "performance",
			}

			cpuLimit := resource.MustParse("2")
			memLimit := resource.MustParse("100Mi")

			By("Define test pod")
			testpod, err := helper.DefineQoSTestPod(
				tsparams.TestingNamespace, nodeName, cpuLimit.String(), cpuLimit.String(), memLimit.String(), memLimit.String())
			Expect(err).ToNot(HaveOccurred(), "Failed to define test pod")

			testpod.Definition.Annotations = testPodAnnotations
			runtimeClass := fmt.Sprintf("%s-%s", components.ComponentNamePrefix, perfProfile.Definition.Name)
			testpod.Definition.Spec.RuntimeClassName = &runtimeClass

			DeferCleanup(func() {
				// Delete the test pod if it's still around when the function returns, like in a test case failure.
				if testpod.Exists() {
					By("Delete pod in case of a failure")
					_, err = testpod.DeleteAndWait(tsparams.PowerSaveTimeout)
					Expect(err).ToNot(HaveOccurred(), "Failed to delete test pod in case of failure")
				}
			})

			By("Create test pod")
			testpod, err = testpod.CreateAndWaitUntilRunning(tsparams.PowerSaveTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to create pod")
			Expect(testpod.Object.Status.QOSClass).To(Equal(corev1.PodQOSGuaranteed),
				"Test pod does not have QoS class of Guaranteed")

			cpusetOutput, err := testpod.ExecCommand([]string{"sh", `-c`, "taskset -c -p $$ | cut -d: -f2"})
			Expect(err).ToNot(HaveOccurred(), "Failed to get cpuset")

			By("Verify powersetting of cpus used by the pod")
			trimmedOutput := strings.TrimSpace(cpusetOutput.String())
			cpusUsed, err := cpuset.Parse(trimmedOutput)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse cpuset output")

			targetCpus := cpusUsed.List()
			checkCPUGovernorsAndResumeLatency(targetCpus, "n/a", "performance")

			By("Verify the rest of cpus have default power setting")
			allCpus := nodeList[0].Object.Status.Capacity.Cpu()
			cpus, err := cpuset.Parse(fmt.Sprintf("0-%d", allCpus.Value()-1))
			Expect(err).ToNot(HaveOccurred(), "Failed to parse cpuset")

			otherCPUs := cpus.Difference(cpusUsed)
			// Verify cpus not assigned to the pod have default power settings.
			checkCPUGovernorsAndResumeLatency(otherCPUs.List(), "0", "performance")

			By("Delete the pod")
			_, err = testpod.DeleteAndWait(tsparams.PowerSaveTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test pod")

			By("Verify after pod was deleted cpus assigned to container have default powersave settings")
			checkCPUGovernorsAndResumeLatency(targetCpus, "0", "performance")
		})

	Context("Collect power usage metrics", Ordered, func() {
		var (
			samplingInterval time.Duration
			powerState       string
		)

		BeforeAll(func() {
			if BMCClient == nil {
				Skip("Collecting power usage metrics requires the BMC configuration be set.")
			}

			samplingInterval, err = time.ParseDuration(RANConfig.MetricSamplingInterval)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse metric sampling interval")

			// Determine power state to be used as a tag for the metric
			powerState, err = collect.GetPowerState(perfProfile)
			Expect(err).ToNot(HaveOccurred(), "Failed to get power state for the performance profile")
		})

		It("Checks power usage for 'noworkload' scenario", func() {
			duration, err := time.ParseDuration(RANConfig.NoWorkloadDuration)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse no workload duration")

			compMap, err := collect.CollectPowerMetricsWithNoWorkload(duration, samplingInterval, powerState)
			Expect(err).ToNot(HaveOccurred(), "Failed to collect power metrics with no workload")

			// Persist power usage metric to ginkgo report for further processing in pipeline.
			for metricName, metricValue := range compMap {
				GinkgoWriter.Printf("%s: %s\n", metricName, metricValue)
			}
		})

		It("Checks power usage for 'steadyworkload' scenario", func() {
			duration, err := time.ParseDuration(RANConfig.WorkloadDuration)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse steady workload duration")

			compMap, err := collect.CollectPowerMetricsWithSteadyWorkload(
				duration, samplingInterval, powerState, perfProfile, nodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to collect power metrics with steady workload")

			// Persist power usage metric to ginkgo report for further processing in pipeline.
			for metricName, metricValue := range compMap {
				GinkgoWriter.Printf("%s: %s\n", metricName, metricValue)
			}
		})
	})
})

// checkCPUGovernorsAndResumeLatency checks power and latency settings of the cpus.
func checkCPUGovernorsAndResumeLatency(cpus []int, pmQos, governor string) {
	for _, cpu := range cpus {
		command := fmt.Sprintf("cat /sys/devices/system/cpu/cpu%d/power/pm_qos_resume_latency_us", cpu)

		// Eventually allows for retries on malformed output, but we use StopTrying since the command failing is
		// a failure, not just a malformed output.
		Eventually(func() (string, error) {
			output, err := cluster.ExecCommandOnSNOWithRetries(Spoke1APIClient,
				ranparam.RetryCount, ranparam.RetryInterval, command)
			if err != nil {
				return "", StopTrying(fmt.Sprintf("Failed to check cpu %d resume latency", cpu)).Wrap(err)
			}

			return strings.TrimSpace(output), nil
		}, 10*time.Second, time.Second).Should(Equal(pmQos))

		command = fmt.Sprintf("cat /sys/devices/system/cpu/cpu%d/cpufreq/scaling_governor", cpu)

		Eventually(func() (string, error) {
			output, err := cluster.ExecCommandOnSNOWithRetries(Spoke1APIClient,
				ranparam.RetryCount, ranparam.RetryInterval, command)
			if err != nil {
				return "", StopTrying(fmt.Sprintf("Failed to check cpu %d scaling governor", cpu)).Wrap(err)
			}

			return strings.TrimSpace(output), nil
		}, 10*time.Second, time.Second).Should(Equal(governor))
	}
}
