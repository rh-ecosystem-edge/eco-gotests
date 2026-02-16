package validation

import (
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/get"
	"k8s.io/klog/v2"
)

// HasPCIDevice checks if any node in the cluster has a PCI device with the specified vendor and device ID.
func HasPCIDevice(apiClient *clients.Settings, vendorID, deviceID string) (bool, error) {
	klog.V(100).Infof("Checking for PCI device with vendor %s and device %s", vendorID, deviceID)

	// Look for PCI device labels
	labelPattern := fmt.Sprintf("pci-%s_%s", vendorID, deviceID)
	nodes, err := get.NodesWithLabel(apiClient, labelPattern)
	if err != nil {
		return false, fmt.Errorf("failed to check for PCI device: %w", err)
	}

	return len(nodes) > 0, nil
}

// HasUSBDevice checks if any node in the cluster has a USB device with the specified vendor and device ID.
func HasUSBDevice(apiClient *clients.Settings, vendorID, deviceID string) (bool, error) {
	klog.V(100).Infof("Checking for USB device with vendor %s and device %s", vendorID, deviceID)

	// Look for USB device labels
	labelPattern := fmt.Sprintf("usb-%s_%s", vendorID, deviceID)
	nodes, err := get.NodesWithLabel(apiClient, labelPattern)
	if err != nil {
		return false, fmt.Errorf("failed to check for USB device: %w", err)
	}

	return len(nodes) > 0, nil
}

// HasAnyPCIDevice checks if any node has PCI device labels.
func HasAnyPCIDevice(apiClient *clients.Settings) (bool, error) {
	klog.V(100).Info("Checking if any PCI devices are present")

	nodes, err := get.NodesWithLabel(apiClient, "feature.node.kubernetes.io/pci-")
	if err != nil {
		return false, fmt.Errorf("failed to check for PCI devices: %w", err)
	}

	return len(nodes) > 0, nil
}

// HasAnyUSBDevice checks if any node has USB device labels.
func HasAnyUSBDevice(apiClient *clients.Settings) (bool, error) {
	klog.V(100).Info("Checking if any USB devices are present")

	nodes, err := get.NodesWithLabel(apiClient, "feature.node.kubernetes.io/usb-")
	if err != nil {
		return false, fmt.Errorf("failed to check for USB devices: %w", err)
	}

	return len(nodes) > 0, nil
}

// HasSRIOVCapability checks if any node has SR-IOV capability.
func HasSRIOVCapability(apiClient *clients.Settings) (bool, error) {
	klog.V(100).Info("Checking for SR-IOV capability")

	nodes, err := get.NodesWithLabel(apiClient, "feature.node.kubernetes.io/network-sriov.capable")
	if err != nil {
		return false, fmt.Errorf("failed to check for SR-IOV capability: %w", err)
	}

	return len(nodes) > 0, nil
}

// HasStorageFeature checks if any node has specific storage features (SSD, NVMe, etc.).
func HasStorageFeature(apiClient *clients.Settings, featureName string) (bool, error) {
	klog.V(100).Infof("Checking for storage feature: %s", featureName)

	labelPattern := fmt.Sprintf("feature.node.kubernetes.io/storage-%s", strings.ToLower(featureName))
	nodes, err := get.NodesWithLabel(apiClient, labelPattern)
	if err != nil {
		return false, fmt.Errorf("failed to check for storage feature %s: %w", featureName, err)
	}

	return len(nodes) > 0, nil
}
