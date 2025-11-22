package helper

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	mcov1 "github.com/openshift/api/machineconfiguration/v1"
	performancev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nto"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/powermanagement/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/cpuset"
	"k8s.io/utils/ptr"
)

// GetPerformanceProfileWithCPUSet returns the first performance profile found with reserved and isolated cpuset.
func GetPerformanceProfileWithCPUSet() (*nto.Builder, error) {
	profileBuilders, err := nto.ListProfiles(Spoke1APIClient)
	if err != nil {
		return nil, err
	}

	for _, profileBuilder := range profileBuilders {
		if profileBuilder.Object.Spec.CPU != nil &&
			profileBuilder.Object.Spec.CPU.Reserved != nil &&
			profileBuilder.Object.Spec.CPU.Isolated != nil {
			return profileBuilder, nil
		}
	}

	return nil, fmt.Errorf("failed to find performance profile with reserved and isolated CPU set")
}

// SetPowerModeAndWaitForMcpUpdate updates the performance profile with the given workload hints,
// and waits for the mcp update.
func SetPowerModeAndWaitForMcpUpdate(
	perfProfile *nto.Builder, node nodes.Builder, perPodPowerManagement, highPowerConsumption, realTime bool) error {
	klog.V(tsparams.LogLevel).Infof("Set powersave mode on performance profile")

	perfProfile.Definition.Spec.WorkloadHints = &performancev2.WorkloadHints{
		PerPodPowerManagement: ptr.To(perPodPowerManagement),
		HighPowerConsumption:  ptr.To(highPowerConsumption),
		RealTime:              ptr.To(realTime),
	}

	_, err := perfProfile.Update(true)
	if err != nil {
		return err
	}

	mcp, err := mco.Pull(Spoke1APIClient, "master")
	if err != nil {
		return err
	}

	err = mcp.WaitToBeInCondition(mcov1.MachineConfigPoolUpdating, corev1.ConditionTrue, 2*tsparams.PowerSaveTimeout)
	if err != nil {
		return err
	}

	err = mcp.WaitForUpdate(3 * tsparams.PowerSaveTimeout)
	if err != nil {
		return err
	}

	err = node.WaitUntilReady(tsparams.PowerSaveTimeout)

	return err
}

// SetCPUFreq updates the performance profile with the given isolated and reserved
// core frequencies and verifies that the frequencies have been updated on the spoke cluster.
func SetCPUFreq(
	perfProfile *nto.Builder,
	desiredIsolatedCoreFreq *performancev2.CPUfrequency,
	desiredReservedCoreFreq *performancev2.CPUfrequency) error {
	klog.V(tsparams.LogLevel).Infof("Set Reserved and Isolated CPU Frequency on performance profile")

	// Exists will ensure that perfProfile.Object is up to date.
	if !perfProfile.Exists() {
		klog.V(tsparams.LogLevel).Infof(
			"Performance profile %s provided to SetCPUFreq does not exist", perfProfile.Definition.Name)

		return fmt.Errorf("performance profile %s does not exist", perfProfile.Definition.Name)
	}

	// Replace the resource version with the latest value to prevent conflicts.
	perfProfile.Definition.ResourceVersion = perfProfile.Object.ResourceVersion

	// Update PerfProfile with new CPU Frequencies
	perfProfile.Definition.Spec.HardwareTuning = &performancev2.HardwareTuning{
		IsolatedCpuFreq: desiredIsolatedCoreFreq,
		ReservedCpuFreq: desiredReservedCoreFreq,
	}

	_, err := perfProfile.Update(false)
	if err != nil {
		return err
	}

	// Determine isolated CPU number from IsolatedCPUset
	isolatedCPUSet, err := cpuset.Parse(string(*perfProfile.Object.Spec.CPU.Isolated))
	if err != nil {
		return err
	}

	// Determine reserved CPU number from reservedCPUset
	reservedCPUSet, err := cpuset.Parse(string(*perfProfile.Object.Spec.CPU.Reserved))
	if err != nil {
		return err
	}

	isolatedCPUNumber := isolatedCPUSet.List()[0]
	reservedCCPUNumber := reservedCPUSet.List()[0]

	spokeCommandIsolatedCPUs := fmt.Sprintf("cat /sys/devices/system/cpu/cpufreq/policy%v/scaling_max_freq",
		isolatedCPUNumber)

	spokeCommandReservedCPUs := fmt.Sprintf("cat /sys/devices/system/cpu/cpufreq/policy%v/scaling_max_freq",
		reservedCCPUNumber)

	// Wait for Isolated CPU Frequency to be updated.
	err = wait.PollUntilContextTimeout(
		context.TODO(), 2*time.Second, 1*time.Minute, true, func(ctx context.Context) (bool, error) {
			// Get current isolated core frequency from spoke cluster and compare to desired frequency
			cmdOut, err := cluster.ExecCommandOnSNOWithRetries(Spoke1APIClient, ranparam.RetryCount,
				ranparam.RetryInterval, spokeCommandIsolatedCPUs)
			if err != nil {
				return false, fmt.Errorf("command failed: %s", cmdOut)
			}

			cmdOut = strings.TrimSpace(cmdOut)

			currIsolatedCoreFreq, err := strconv.Atoi(cmdOut)
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("converting cpu frequency %s to an int failed: %v", cmdOut, err)

				return false, nil
			}

			if currIsolatedCoreFreq != int(*desiredIsolatedCoreFreq) {
				return false, nil
			}

			// Get current isolated core frequency from spoke cluster and compare to desired frequency
			cmdOut, err = cluster.ExecCommandOnSNOWithRetries(Spoke1APIClient, ranparam.RetryCount,
				ranparam.RetryInterval, spokeCommandReservedCPUs)
			if err != nil {
				return false, fmt.Errorf("command failed: %s", cmdOut)
			}

			cmdOut = strings.TrimSpace(cmdOut)

			currReservedFreq, err := strconv.Atoi(cmdOut)
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("converting cpu frequency %s to an int failed: %v", cmdOut, err)

				return false, nil
			}

			if currReservedFreq != int(*desiredReservedCoreFreq) {
				return false, nil
			}

			return true, nil
		})

	return err
}

// DefineQoSTestPod defines test pod with given cpu and memory resources.
func DefineQoSTestPod(namespace, nodeName, cpuReq, cpuLimit, memReq, memLimit string) (*pod.Builder, error) {
	var err error

	pod := pod.NewBuilder(Spoke1APIClient, "qos-test-pod", namespace, RANConfig.CnfTestImage).DefineOnNode(nodeName)
	pod, err = redefineContainerResources(pod, cpuReq, cpuLimit, memReq, memLimit)

	return pod, err
}

// redefineContainerResources redefines a pod builder with CPU and Memory resources in first container
// Use empty string to skip a resource. e.g., cpuLimit="".
func redefineContainerResources(
	pod *pod.Builder, cpuRequest string, cpuLimit string, memoryRequest string, memoryLimit string) (*pod.Builder, error) {
	if len(pod.Definition.Spec.Containers) < 1 {
		return pod, fmt.Errorf("pod has no containers to redefine resources for")
	}

	pod.Definition.Spec.Containers[0].Resources.Requests = corev1.ResourceList{}
	pod.Definition.Spec.Containers[0].Resources.Limits = corev1.ResourceList{}

	if cpuLimit != "" {
		pod.Definition.Spec.Containers[0].Resources.Limits["cpu"] = resource.MustParse(cpuLimit)
	}

	if cpuRequest != "" {
		pod.Definition.Spec.Containers[0].Resources.Requests["cpu"] = resource.MustParse(cpuRequest)
	}

	if memoryLimit != "" {
		pod.Definition.Spec.Containers[0].Resources.Limits["memory"] = resource.MustParse(memoryLimit)
	}

	if memoryRequest != "" {
		pod.Definition.Spec.Containers[0].Resources.Requests["memory"] = resource.MustParse(memoryRequest)
	}

	return pod, nil
}
