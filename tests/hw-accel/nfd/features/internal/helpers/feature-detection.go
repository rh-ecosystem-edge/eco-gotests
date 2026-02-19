package helpers

import (
	"context"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// IsNFDTaintsEnabled checks if NFD master has --enable-taints flag set.
// Returns true if taints are enabled, false otherwise.
func IsNFDTaintsEnabled(apiClient *clients.Settings, nfdNamespace string) (bool, error) {
	// Get NFD master pods
	masterPods, err := pod.List(apiClient, nfdNamespace, metav1.ListOptions{
		LabelSelector: "app=nfd-master",
	})
	if err != nil {
		klog.V(nfdparams.LogLevel).Infof("Error listing NFD master pods: %v", err)

		return false, err
	}

	if len(masterPods) == 0 {
		klog.V(nfdparams.LogLevel).Info("No NFD master pods found")

		return false, nil
	}

	// Check the first master pod's containers for --enable-taints flag
	for _, masterPod := range masterPods {
		ctx := context.Background()

		podDetails, err := apiClient.CoreV1Interface.Pods(nfdNamespace).Get(
			ctx, masterPod.Object.Name, metav1.GetOptions{})
		if err != nil {
			continue
		}

		// Check all containers in the pod
		for _, container := range podDetails.Spec.Containers {
			for _, arg := range container.Args {
				if cmd == "--enable-taints" || cmd == "--enable-taints=true" {
					klog.V(nfdparams.LogLevel).Infof("Found --enable-taints in command of NFD master pod %s", masterPod.Object.Name)

					return true, nil
				}
			}
		}
	}

	klog.V(nfdparams.LogLevel).Info("--enable-taints flag not found in NFD master configuration")

	return false, nil
}

// GetNFDVersion attempts to determine the NFD version from the deployment.
// Returns version string or empty if unable to determine.
func GetNFDVersion(apiClient *clients.Settings, nfdNamespace string) string {
	// Try to get version from NFD master pod
	masterPods, err := pod.List(apiClient, nfdNamespace, metav1.ListOptions{
		LabelSelector: "app=nfd-master",
	})
	if err != nil || len(masterPods) == 0 {
		return ""
	}

	// Check image tag for version
	for _, masterPod := range masterPods {
		for _, container := range masterPod.Object.Spec.Containers {
			image := container.Image
			// Extract version from image tag (e.g., registry.io/nfd:v0.12.0)
			if parts := strings.Split(image, ":"); len(parts) > 1 {
				version := parts[len(parts)-1]
        
				klog.V(nfdparams.LogLevel).Infof("Detected NFD version: %s", version)

				return version
			}
		}
	}

	return ""
}

// CheckNFDFeatureSupport checks if a specific NFD feature is supported.
// Returns (supported bool, skipReason string, error).
func CheckNFDFeatureSupport(apiClient *clients.Settings, nfdNamespace string, feature string) (bool, string, error) {
	switch feature {
	case "taints":
		enabled, err := IsNFDTaintsEnabled(apiClient, nfdNamespace)
		if err != nil {
			return false, "", err
		}

		if !enabled {
			return false, "Node tainting requires --enable-taints flag on nfd-master. " +
				"Configure NodeFeatureDiscovery CR operand with: " +
				"spec.operand.servicePort with extraArgs: [\"--enable-taints\"] " +
				"or update nfd-master deployment directly.", nil
		}

		return true, "", nil

	case "backreferences":
		// Backreferences require NFD v0.11+
		// We'll test for support by checking if the feature works
		// Actual detection happens in the test itself
		version := GetNFDVersion(apiClient, nfdNamespace)
		if version != "" && strings.Contains(version, "v0.") {
			// Extract minor version
			if strings.HasPrefix(version, "v0.10") || strings.HasPrefix(version, "v0.9") ||
				strings.HasPrefix(version, "v0.8") {
				return false, "Backreferences require NFD v0.11+. Current version: " + version, nil
			}
		}
		// Version unknown or >= v0.11, let test check dynamically
		return true, "", nil

	default:
		return true, "", nil
	}
}

// WaitForFeatureDetection waits for a feature to be detected with a timeout.
// Returns true if feature is detected, false if timeout occurs.
func WaitForFeatureDetection(checkFunc func() bool, timeout time.Duration, pollInterval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if checkFunc() {
			return true
		}

		time.Sleep(pollInterval)
	}

	return false
}
