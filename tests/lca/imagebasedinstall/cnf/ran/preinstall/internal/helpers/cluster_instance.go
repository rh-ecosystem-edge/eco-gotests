package helpers

import (
	"fmt"
	"strings"

	assistedv1 "github.com/openshift/assisted-service/api/v1beta1"
	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
)

func firstNode(clusterInst *siteconfigv1alpha1.ClusterInstance) (*siteconfigv1alpha1.NodeSpec, error) {
	if clusterInst == nil {
		return nil, fmt.Errorf("clusterinstance: nil")
	}

	nodes := clusterInst.Spec.Nodes
	if len(nodes) == 0 {
		return nil, fmt.Errorf("clusterinstance: spec.nodes empty")
	}

	return &nodes[0], nil
}

// ClusterNameFromClusterInstance returns spec.clusterName.
func ClusterNameFromClusterInstance(clusterInst *siteconfigv1alpha1.ClusterInstance) (string, error) {
	if clusterInst == nil {
		return "", fmt.Errorf("clusterinstance: nil")
	}

	if clusterInst.Spec.ClusterName == "" {
		return "", fmt.Errorf("clusterinstance: spec.clusterName missing or empty")
	}

	return clusterInst.Spec.ClusterName, nil
}

// NodeHostNameFromClusterInstance returns spec.nodes[0].hostName.
func NodeHostNameFromClusterInstance(clusterInst *siteconfigv1alpha1.ClusterInstance) (string, error) {
	node, err := firstNode(clusterInst)
	if err != nil {
		return "", err
	}

	if node.HostName == "" {
		return "", fmt.Errorf("clusterinstance: nodes[0].hostName missing or empty")
	}

	return node.HostName, nil
}

// BMCAddressFromClusterInstance returns spec.nodes[0].bmcAddress.
func BMCAddressFromClusterInstance(clusterInst *siteconfigv1alpha1.ClusterInstance) (string, error) {
	node, err := firstNode(clusterInst)
	if err != nil {
		return "", err
	}

	if node.BmcAddress == "" {
		return "", fmt.Errorf("clusterinstance: nodes[0].bmcAddress missing or empty")
	}

	return node.BmcAddress, nil
}

// BootMACAddressFromClusterInstance returns spec.nodes[0].bootMACAddress.
func BootMACAddressFromClusterInstance(clusterInst *siteconfigv1alpha1.ClusterInstance) (string, error) {
	node, err := firstNode(clusterInst)
	if err != nil {
		return "", err
	}

	if node.BootMACAddress == "" {
		return "", fmt.Errorf("clusterinstance: nodes[0].bootMACAddress missing or empty")
	}

	return node.BootMACAddress, nil
}

// IBICPUArchitectureFromClusterInstance returns spec.cpuArchitecture or spec.nodes[0].cpuArchitecture
// mapped for ImageBasedInstallationConfig (aarch64 -> arm64, x86_64 -> amd64), matching the Ansible playbook.
func IBICPUArchitectureFromClusterInstance(clusterInst *siteconfigv1alpha1.ClusterInstance) (string, error) {
	if clusterInst == nil {
		return "", fmt.Errorf("clusterinstance: nil")
	}

	var arch siteconfigv1alpha1.CPUArchitecture

	if clusterInst.Spec.CPUArchitecture != "" {
		arch = clusterInst.Spec.CPUArchitecture
	} else if node, err := firstNode(clusterInst); err == nil && node.CPUArchitecture != "" {
		arch = node.CPUArchitecture
	}

	if arch == "" {
		return "", nil
	}

	archString := string(arch)
	switch archString {
	case "aarch64":
		return "arm64", nil
	case "x86_64":
		return "amd64", nil
	default:
		return archString, nil
	}
}

// IgnitionConfigOverrideFromClusterInstance returns spec.nodes[0].ignitionConfigOverride when set.
func IgnitionConfigOverrideFromClusterInstance(clusterInst *siteconfigv1alpha1.ClusterInstance) (string, error) {
	node, err := firstNode(clusterInst)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(node.IgnitionConfigOverride), nil
}

// NetworkConfigForInstallation returns assisted-service NetConfig for openshift-install from the first node.
func NetworkConfigForInstallation(clusterInst *siteconfigv1alpha1.ClusterInstance) (*assistedv1.NetConfig, error) {
	node, err := firstNode(clusterInst)
	if err != nil {
		return nil, err
	}

	if node.NodeNetwork == nil {
		return nil, fmt.Errorf("clusterinstance: nodes[0].nodeNetwork missing")
	}

	raw := node.NodeNetwork.NetConfig.Raw
	if len(raw) == 0 {
		return nil, fmt.Errorf("clusterinstance: nodes[0].nodeNetwork.config missing")
	}

	return &assistedv1.NetConfig{Raw: append([]byte(nil), raw...)}, nil
}

// InstallationDiskFromClusterInstance derives the installation disk path from rootDeviceHints.
func InstallationDiskFromClusterInstance(clusterInst *siteconfigv1alpha1.ClusterInstance) (string, error) {
	node, err := firstNode(clusterInst)
	if err != nil {
		return "", err
	}

	hints := node.RootDeviceHints
	if hints == nil {
		return "", fmt.Errorf("clusterinstance: nodes[0].rootDeviceHints missing")
	}

	if hints.DeviceName != "" {
		return hints.DeviceName, nil
	}

	wwnStr := strings.TrimSpace(hints.WWN)
	if wwnStr != "" {
		if strings.HasPrefix(wwnStr, "eui.") {
			return fmt.Sprintf("/dev/disk/by-id/nvme-%s", wwnStr), nil
		}

		return fmt.Sprintf("/dev/disk/by-id/wwn-%s", wwnStr), nil
	}

	return "", fmt.Errorf("no supported rootDeviceHints found (deviceName or wwn)")
}
