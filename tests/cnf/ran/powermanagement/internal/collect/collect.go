package collect

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nto"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/stats"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/powermanagement/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"
	"k8s.io/utils/cpuset"
)

// GetPowerState determines the power state from the workloadHints object of the PerformanceProfile.
func GetPowerState(perfProfile *nto.Builder) (string, error) {
	workloadHints := perfProfile.Definition.Spec.WorkloadHints

	if workloadHints == nil {
		// No workloadHints object -> default is Performance mode
		return tsparams.PerformanceMode, nil
	}

	realTime := *workloadHints.RealTime
	highPowerConsumption := *workloadHints.HighPowerConsumption
	perPodPowerManagement := *workloadHints.PerPodPowerManagement

	switch {
	case realTime && !highPowerConsumption && !perPodPowerManagement:
		return tsparams.PerformanceMode, nil
	case realTime && highPowerConsumption && !perPodPowerManagement:
		return tsparams.HighPerformanceMode, nil
	case realTime && !highPowerConsumption && perPodPowerManagement:
		return tsparams.PowerSavingMode, nil
	default:
		return "", fmt.Errorf("unknown workloadHints power state configuration")
	}
}

// CollectPowerMetricsWithNoWorkload collects metrics with no workload.
func CollectPowerMetricsWithNoWorkload(duration, interval time.Duration, tag string) (map[string]string, error) {
	klog.V(tsparams.LogLevel).Infof("Wait for %s for noworkload scenario", duration)

	return collectPowerUsageMetrics(duration, interval, "noworkload", tag)
}

// CollectPowerMetricsWithSteadyWorkload collects power metrics with steady workload scenario.
func CollectPowerMetricsWithSteadyWorkload(
	duration, interval time.Duration, tag string, perfProfile *nto.Builder, nodeName string) (map[string]string, error) {
	// stressNg cpu count is roughly 75% of total isolated cores.
	// 1 cpu will be used by other consumer pods, such as process-exporter, cnf-ran-gotests-priv.
	isolatedCPUSet, err := cpuset.Parse(string(*perfProfile.Object.Spec.CPU.Isolated))
	if err != nil {
		return nil, err
	}

	stressNgCPUCount := (isolatedCPUSet.Size() - 1) * 300 / 400
	stressngMaxPodCount := 50

	stressNgPods, err := deployStressNgPods(stressNgCPUCount, stressngMaxPodCount, nodeName)
	if err != nil {
		return nil, err
	}

	if len(stressNgPods) < 1 {
		return nil, fmt.Errorf("not enough stress-ng pods to run test")
	}

	klog.V(tsparams.LogLevel).Infof("Wait for %s for steadyworkload scenario", duration.String())
	result, collectErr := collectPowerUsageMetrics(duration, interval, "steadyworkload", tag)

	// Delete stress-ng pods regardless of whether collectPowerUsageMetrics failed.
	for _, stressPod := range stressNgPods {
		_, err = stressPod.DeleteAndWait(tsparams.PowerSaveTimeout)
		if err != nil {
			return nil, err
		}
	}

	// If deleting stress-ng pods was successful, still return error from collectPowerUsageMetrics.
	return result, collectErr
}

// collectPowerUsageMetrics collects power usage metrics.
func collectPowerUsageMetrics(duration, interval time.Duration, scenario, tag string) (map[string]string, error) {
	var powerMeasurements []float64

	endTime := time.Now().Add(duration)
	for time.Now().Before(endTime) {
		power, err := BMCClient.PowerUsage()
		if err != nil {
			klog.V(tsparams.LogLevel).Infof("error getting power usage: %v", err)

			continue
		}

		powerMeasurements = append(powerMeasurements, float64(power))

		time.Sleep(interval)
	}

	klog.V(tsparams.LogLevel).Info("Finished collecting power usage, waiting for results")

	if len(powerMeasurements) < 1 {
		return nil, fmt.Errorf("no power usage metrics were retrieved")
	}

	// Compute power metrics.
	return computePowerUsageStatistics(powerMeasurements, interval, scenario, tag)
}

// deployStressNgPods deploys the stress-ng workload pods.
func deployStressNgPods(stressNgCPUCount, stressngMaxPodCount int, nodeName string) ([]*pod.Builder, error) {
	// Determine cpu requests for stress-ng pods.
	stressngPodsCPUs := parsePodCountAndCpus(stressngMaxPodCount, stressNgCPUCount)

	// Create and wait for stress-ng pods to be Ready
	klog.V(tsparams.LogLevel).
		Infof("Creating up to %d stress-ng pods with total %d cpus", stressngMaxPodCount, stressNgCPUCount)

	stressngPods := []*pod.Builder{}

	for i, cpuReq := range stressngPodsCPUs {
		pod := defineStressPod(nodeName, cpuReq, false, fmt.Sprintf("ran-stressng-pod-%d", i))

		_, err := pod.Create()
		if err != nil {
			return nil, err
		}

		stressngPods = append(stressngPods, pod)
	}

	err := waitForPodsHealthy(stressngPods, 2*tsparams.PowerSaveTimeout)
	if err != nil {
		return nil, err
	}

	klog.V(tsparams.LogLevel).
		Infof("%d stress-ng pods with total %d cpus are created and running", len(stressngPods), stressNgCPUCount)

	return stressngPods, nil
}

// waitForPodsHealthy waits for given pods to appear and healthy.
func waitForPodsHealthy(pods []*pod.Builder, timeout time.Duration) error {
	for _, singlePod := range pods {
		tempPod, err := pod.Pull(Spoke1APIClient, singlePod.Definition.Name,
			singlePod.Object.Namespace)
		if err != nil {
			return err
		}

		err = tempPod.WaitUntilCondition(corev1.ContainersReady, timeout)
		if err != nil {
			return err
		}
	}

	return nil
}

// defineStressPod returns stress-ng pod definition.
func defineStressPod(nodeName string, cpus int, guaranteed bool, name string) *pod.Builder {
	stressngImage := RANConfig.StressngTestImage
	envVars := []corev1.EnvVar{{Name: "INITIAL_DELAY_SEC", Value: "60"}}
	cpuLimit := strconv.Itoa(cpus)
	memoryLimit := "100M"

	if !guaranteed {
		// Override CMDLINE for non-guaranteed pod to avoid specifying taskset
		envVars = append(envVars, corev1.EnvVar{Name: "CMDLINE", Value: fmt.Sprintf("--cpu %d --cpu-load 50", cpus)})
		cpuLimit = fmt.Sprintf("%dm", cpus*1200)
		memoryLimit = "200M"
	}

	stressPod := pod.NewBuilder(Spoke1APIClient, name, tsparams.TestingNamespace, stressngImage)
	stressPod = stressPod.DefineOnNode(nodeName)
	stressPod.RedefineDefaultContainer(corev1.Container{
		Name:            "stress-ng",
		Image:           stressngImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Env:             envVars,
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cpuLimit),
				corev1.ResourceMemory: resource.MustParse(memoryLimit),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(strconv.Itoa(cpus)),
				corev1.ResourceMemory: resource.MustParse("100M"),
			},
		},
	})

	return stressPod
}

func parsePodCountAndCpus(maxPodCount, cpuCount int) []int {
	podCount := int(math.Min(float64(cpuCount), float64(maxPodCount)))
	cpuPerPod := cpuCount / podCount
	cpus := []int{}

	for i := 1; i <= podCount-1; i++ {
		cpus = append(cpus, cpuPerPod)
	}

	cpus = append(cpus, cpuCount-cpuPerPod*(podCount-1))

	return cpus
}

// computePowerUsageStatistics computes the power usage summary statistics.
func computePowerUsageStatistics(
	instantPowerData []float64,
	samplingInterval time.Duration,
	scenario,
	tag string) (map[string]string, error) {
	klog.V(tsparams.LogLevel).Infof("Power usage measurements for %s: %v", scenario, instantPowerData)

	compMap := make(map[string]string)

	compMap[fmt.Sprintf("%s_%s_%s", tsparams.RanPowerMetricTotalSamples, scenario, tag)] =
		fmt.Sprintf("%d", len(instantPowerData))
	compMap[fmt.Sprintf("%s_%s_%s", tsparams.RanPowerMetricSamplingIntervalSeconds, scenario, tag)] =
		fmt.Sprintf("%.0f", samplingInterval.Seconds())

	minInstantaneousPower := slices.Min(instantPowerData)
	compMap[fmt.Sprintf("%s_%s_%s", tsparams.RanPowerMetricMinInstantPower, scenario, tag)] =
		fmt.Sprintf("%.7f", minInstantaneousPower)

	maxInstantaneousPower := slices.Max(instantPowerData)
	compMap[fmt.Sprintf("%s_%s_%s", tsparams.RanPowerMetricMaxInstantPower, scenario, tag)] =
		fmt.Sprintf("%.7f", maxInstantaneousPower)

	meanInstantaneousPower, err := stats.Mean(instantPowerData)
	if err != nil {
		return compMap, err
	}

	compMap[fmt.Sprintf("%s_%s_%s", tsparams.RanPowerMetricMeanInstantPower, scenario, tag)] =
		fmt.Sprintf("%.7f", meanInstantaneousPower)

	stdDevInstantaneousPower, err := stats.StdDev(instantPowerData)
	if err != nil {
		return compMap, err
	}

	compMap[fmt.Sprintf("%s_%s_%s", tsparams.RanPowerMetricStdDevInstantPower, scenario, tag)] =
		fmt.Sprintf("%.7f", stdDevInstantaneousPower)

	medianInstantaneousPower, err := stats.Median(instantPowerData)
	if err != nil {
		return compMap, err
	}

	compMap[fmt.Sprintf("%s_%s_%s", tsparams.RanPowerMetricMedianInstantPower, scenario, tag)] =
		fmt.Sprintf("%.7f", medianInstantaneousPower)

	return compMap, nil
}
