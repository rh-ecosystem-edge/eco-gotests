package cmd

import (
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"k8s.io/klog/v2"
)

// VerifySysctlKernelParametersConfiguredOnPodInterface checks that each sysctl key in
// sysctlPluginConfig matches on the pod interface. Keys may contain the IFNAME placeholder.
func VerifySysctlKernelParametersConfiguredOnPodInterface(
	podUnderTest *pod.Builder, sysctlPluginConfig map[string]string, interfaceName string) error {
	for key, value := range sysctlPluginConfig {
		sysctlKernelParam := strings.Replace(key, "IFNAME", interfaceName, 1)

		klog.V(90).Infof("Validate sysctl flag: %s has the right value in pod's interface: %s",
			sysctlKernelParam, interfaceName)

		cmdBuffer, err := podUnderTest.ExecCommand([]string{"sysctl", "-n", sysctlKernelParam})
		if err != nil {
			return fmt.Errorf("failed to execute sysctl command on the pod: %w", err)
		}

		actual := strings.TrimSpace(cmdBuffer.String())
		if actual != value {
			return fmt.Errorf("sysctl kernel param %s is not in expected state: got %q, want %q",
				sysctlKernelParam, actual, value)
		}
	}

	return nil
}
