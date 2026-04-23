package helpers

import (
	"fmt"
	"strings"

	assistedv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
)

// ClusterInstanceInstallInput holds node fields extracted from a ClusterInstance for IBI preinstall.
type ClusterInstanceInstallInput struct {
	HostName             string
	BMCAddress           string
	BootMACAddress       string
	NetworkConfig        *assistedv1.NetConfig
	InstallationDisk     string
	CPUArchitecture      string
	IgnitionConfigOverride string
}

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

// ClusterInstanceInstallInputFrom extracts preinstall fields from the first node of a ClusterInstance.
func ClusterInstanceInstallInputFrom(clusterInst *siteconfigv1alpha1.ClusterInstance) (*ClusterInstanceInstallInput, error) {
	node, err := firstNode(clusterInst)
	if err != nil {
		return nil, err
	}

	if node.HostName == "" {
		return nil, fmt.Errorf("clusterinstance: nodes[0].hostName missing or empty")
	}

	if node.BmcAddress == "" {
		return nil, fmt.Errorf("clusterinstance: nodes[0].bmcAddress missing or empty")
	}

	if node.BootMACAddress == "" {
		return nil, fmt.Errorf("clusterinstance: nodes[0].bootMACAddress missing or empty")
	}

	netCfg, err := networkConfigFromNode(node)
	if err != nil {
		return nil, err
	}

	installDisk, err := installationDiskFromNode(node)
	if err != nil {
		return nil, err
	}

	arch, err := ibiCPUArchitectureFromClusterInstance(clusterInst)
	if err != nil {
		return nil, err
	}

	return &ClusterInstanceInstallInput{
		HostName:               node.HostName,
		BMCAddress:             node.BmcAddress,
		BootMACAddress:         node.BootMACAddress,
		NetworkConfig:          netCfg,
		InstallationDisk:       installDisk,
		CPUArchitecture:        arch,
		IgnitionConfigOverride: strings.TrimSpace(node.IgnitionConfigOverride),
	}, nil
}

func networkConfigFromNode(node *siteconfigv1alpha1.NodeSpec) (*assistedv1.NetConfig, error) {
	if node.NodeNetwork == nil {
		return nil, fmt.Errorf("clusterinstance: nodes[0].nodeNetwork missing")
	}

	raw := node.NodeNetwork.NetConfig.Raw
	if len(raw) == 0 {
		return nil, fmt.Errorf("clusterinstance: nodes[0].nodeNetwork.config missing")
	}

	return &assistedv1.NetConfig{Raw: append([]byte(nil), raw...)}, nil
}

func installationDiskFromNode(node *siteconfigv1alpha1.NodeSpec) (string, error) {
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

func ibiCPUArchitectureFromClusterInstance(clusterInst *siteconfigv1alpha1.ClusterInstance) (string, error) {
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
