package amdgpuregistry

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"

	amdgpuparams "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/params"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// isTransientConnectionError checks if the error is a temporary connection issue.
func isTransientConnectionError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	return strings.Contains(errStr, "connection reset by peer") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "context deadline exceeded") ||
		strings.Contains(errStr, "i/o timeout") ||
		strings.Contains(errStr, "unable to retrieve the complete list of server APIs") ||
		strings.Contains(errStr, "the server was unable to return a response")
}

// VerifyAndConfigureInternalRegistry checks and configures the internal image registry for the AMD GPU operator.
// Includes retry logic to handle cluster reboots in SNO environments.
func VerifyAndConfigureInternalRegistry(apiClient *clients.Settings) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Info("Verifying internal image registry configuration for AMD GPU operator")

	// Use shorter timeout - 5 minutes for initial API check
	apiCheckTimeout := 5 * time.Minute
	retryInterval := 10 * time.Second

	var (
		imageRegistryConfig *unstructured.Unstructured
		lastErr             error
	)

	ctx, cancel := context.WithTimeout(context.Background(), apiCheckTimeout)
	defer cancel()

	klog.V(amdgpuparams.AMDGPULogLevel).Infof(
		"Waiting for cluster API to be available (timeout: %s)", apiCheckTimeout)

	err := wait.PollUntilContextTimeout(ctx, retryInterval, apiCheckTimeout, true,
		func(ctx context.Context) (bool, error) {
			var getErr error

			imageRegistryConfig, getErr = getImageRegistryConfig(apiClient)
			if getErr != nil {
				if isTransientConnectionError(getErr) {
					klog.V(amdgpuparams.AMDGPULogLevel).Infof(
						"Cluster API not ready (possibly rebooting), retrying: %v", getErr)
					lastErr = getErr

					return false, nil // Continue polling
				}

				// Non-transient error.
				return false, getErr
			}

			return true, nil
		})
	if err != nil {
		if lastErr != nil {
			return fmt.Errorf("failed to get image registry config after retries: %w (last: %w)", err, lastErr)
		}

		return fmt.Errorf("failed to get image registry config: %w", err)
	}

	managementState, found, err := getRegistryManagementState(imageRegistryConfig)
	if err != nil {
		return err
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Infof("Current image registry management state: %s", managementState)

	if !found || managementState != "Managed" {
		return configureRegistryAsManaged(apiClient, imageRegistryConfig)
	}

	return verifyRegistryAvailability(apiClient)
}

// getImageRegistryConfig retrieves the image registry configuration.
func getImageRegistryConfig(apiClient *clients.Settings) (*unstructured.Unstructured, error) {
	ctx := context.Background()

	imageRegistryGVK := schema.GroupVersionKind{
		Group:   "imageregistry.operator.openshift.io",
		Version: "v1",
		Kind:    "Config",
	}

	imageRegistryConfig := &unstructured.Unstructured{}
	imageRegistryConfig.SetGroupVersionKind(imageRegistryGVK)

	err := apiClient.Client.Get(ctx, client.ObjectKey{Name: "cluster"}, imageRegistryConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get image registry configuration: %w", err)
	}

	return imageRegistryConfig, nil
}

// getRegistryManagementState extracts the management state from registry config.
func getRegistryManagementState(config *unstructured.Unstructured) (string, bool, error) {
	managementState, found, err := unstructured.NestedString(config.Object, "spec", "managementState")
	if err != nil {
		return "", false, fmt.Errorf("failed to get image registry management state: %w", err)
	}

	return managementState, found, nil
}

// configureRegistryAsManaged configures the registry to be managed and sets up storage.
func configureRegistryAsManaged(apiClient *clients.Settings, config *unstructured.Unstructured) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Info("Internal registry is not managed - configuring it for AMD GPU operator")

	err := setRegistryManagementState(config)
	if err != nil {
		return err
	}

	err = ensureRegistryStorage(config)
	if err != nil {
		return err
	}

	err = updateRegistryConfig(apiClient, config)
	if err != nil {
		return err
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Info("Updated image registry to Managed state")

	// Wait max 10 minutes for registry to become available after configuration
	return waitForImageRegistryAvailable(apiClient, 10*time.Minute)
}

// setRegistryManagementState sets the registry management state to "Managed".
func setRegistryManagementState(config *unstructured.Unstructured) error {
	err := unstructured.SetNestedField(config.Object, "Managed", "spec", "managementState")
	if err != nil {
		return fmt.Errorf("failed to set image registry management state: %w", err)
	}

	return nil
}

// ensureRegistryStorage ensures registry has storage configuration.
func ensureRegistryStorage(config *unstructured.Unstructured) error {
	storageConfig, storageFound, err := unstructured.NestedMap(config.Object, "spec", "storage")
	if err != nil {
		return fmt.Errorf("failed to check image registry storage configuration: %w", err)
	}

	if !storageFound || storageConfig == nil || len(storageConfig) == 0 {
		return setEmptyDirStorage(config)
	}

	return nil
}

// setEmptyDirStorage sets emptyDir storage for the registry.
func setEmptyDirStorage(config *unstructured.Unstructured) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Info(
		"No storage configured for image registry - adding emptyDir storage for testing")

	newStorageConfig := map[string]interface{}{
		"emptyDir": map[string]interface{}{},
	}

	err := unstructured.SetNestedMap(config.Object, newStorageConfig, "spec", "storage")
	if err != nil {
		return fmt.Errorf("failed to set image registry storage: %w", err)
	}

	return nil
}

// updateRegistryConfig updates the registry configuration in the cluster.
func updateRegistryConfig(apiClient *clients.Settings, config *unstructured.Unstructured) error {
	ctx := context.Background()

	err := apiClient.Client.Update(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to update image registry configuration: %w", err)
	}

	return nil
}

// verifyRegistryAvailability verifies that the registry is available.
func verifyRegistryAvailability(apiClient *clients.Settings) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Info("Internal registry is already managed - verifying availability")

	err := waitForImageRegistryAvailable(apiClient, amdgpuparams.DefaultTimeout)
	if err != nil {
		return fmt.Errorf("image registry is not available: %w", err)
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Info("Internal image registry is properly configured and available")

	return nil
}

// verifyRegistryService verifies image registry service.
func verifyRegistryService(apiClient *clients.Settings) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Info("Verifying image registry service")

	ctx := context.Background()

	service, err := apiClient.CoreV1Interface.Services("openshift-image-registry").Get(
		ctx, "image-registry", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("image registry service not found: %w", err)
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Infof("Image registry service found: %s (ClusterIP: %s)",
		service.Name, service.Spec.ClusterIP)

	routes := &unstructured.UnstructuredList{}
	routes.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "route.openshift.io",
		Version: "v1",
		Kind:    "RouteList",
	})

	err = apiClient.Client.List(ctx, routes, client.InNamespace("openshift-image-registry"))
	if err != nil {
		klog.V(amdgpuparams.AMDGPULogLevel).Infof("Could not check registry routes: %v", err)
	} else if len(routes.Items) > 0 {
		for _, route := range routes.Items {
			if routeName := route.GetName(); routeName == "default-route" {
				if host, found, _ := unstructured.NestedString(route.Object, "spec", "host"); found {
					klog.V(amdgpuparams.AMDGPULogLevel).Infof("Image registry route available: %s", host)
				}
			}
		}
	}

	return nil
}

// waitForImageRegistryAvailable waits for internal image registry to become available.
func waitForImageRegistryAvailable(apiClient *clients.Settings, timeout time.Duration) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Info("Waiting for internal image registry to become available")

	podIsRunning, err := pod.WaitForAllPodsInNamespaceRunning(
		apiClient,
		"openshift-image-registry",
		timeout, metav1.ListOptions{
			LabelSelector: "docker-registry=default",
		})
	if err != nil {
		return fmt.Errorf("timeout waiting for image registry availability: %w", err)
	}

	err = verifyRegistryService(apiClient)
	if err != nil {
		klog.V(amdgpuparams.AMDGPULogLevel).Infof("Registry service verification warning: %v", err)
	}

	if !podIsRunning {
		klog.V(amdgpuparams.AMDGPULogLevel).Infof("Image registry pods are not running")

		return fmt.Errorf("image registry pods are not running")
	}

	return nil
}

// ResetRegistryToRemoved resets the internal image registry to "Removed" state.
func ResetRegistryToRemoved(apiClient *clients.Settings) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Info("Resetting internal image registry to Removed state")

	imageRegistryConfig, err := getImageRegistryConfig(apiClient)
	if err != nil {
		klog.V(amdgpuparams.AMDGPULogLevel).Infof("Could not get image registry config: %v", err)

		return nil
	}

	err = unstructured.SetNestedField(imageRegistryConfig.Object, "Removed", "spec", "managementState")
	if err != nil {
		return fmt.Errorf("failed to set registry management state to Removed: %w", err)
	}

	err = updateRegistryConfig(apiClient, imageRegistryConfig)
	if err != nil {
		return fmt.Errorf("failed to update registry config: %w", err)
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Info("Image registry set to Removed state")

	return nil
}
