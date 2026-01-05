package amdgpuhelpers

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	amdgpuparams "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/params"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// DeleteBlacklistMachineConfig deletes the amdgpu-module-blacklist MachineConfig.
func DeleteBlacklistMachineConfig(apiClient *clients.Settings) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Info("Deleting amdgpu blacklist MachineConfig")

	mcName := "amdgpu-module-blacklist"

	mcBuilder, err := mco.PullMachineConfig(apiClient, mcName)
	if err != nil {
		klog.V(amdgpuparams.AMDGPULogLevel).Infof("MachineConfig %s not found: %v", mcName, err)

		return nil
	}

	if mcBuilder == nil || !mcBuilder.Exists() {
		klog.V(amdgpuparams.AMDGPULogLevel).Info("MachineConfig does not exist")

		return nil
	}

	err = mcBuilder.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete MachineConfig %s: %w", mcName, err)
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Info("MachineConfig deleted successfully")

	return nil
}

// CleanupAMDGPUNodeLabels removes AMD GPU related labels from all nodes.
func CleanupAMDGPUNodeLabels(apiClient *clients.Settings) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Info("Cleaning up AMD GPU node labels")

	allNodes, err := nodes.List(apiClient, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	labelsToRemove := []string{
		"feature.node.kubernetes.io/amd-gpu",
		"amd.com/gpu",
		"amd.com/gpu.count",
		"amd.com/gpu.device",
		"amd.com/gpu.family",
		"amd.com/gpu.product",
		"amd.com/gpu.vram",
		"amd.com/gpu.driver-version",
	}

	for _, nodeBuilder := range allNodes {
		for _, label := range labelsToRemove {
			if _, exists := nodeBuilder.Object.Labels[label]; exists {
				nodeBuilder.WithNewLabel(label, "")
				delete(nodeBuilder.Definition.Labels, label)
			}
		}

		_, err := nodeBuilder.Update()
		if err != nil {
			klog.V(amdgpuparams.AMDGPULogLevel).Infof(
				"Failed to remove labels from node %s: %v", nodeBuilder.Object.Name, err)
		}
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Info("Node labels cleanup completed")

	return nil
}

// DeleteOperatorNamespaces deletes the AMD GPU and KMM operator namespaces.
func DeleteOperatorNamespaces(apiClient *clients.Settings) error {
	klog.V(amdgpuparams.AMDGPULogLevel).Info("Deleting operator namespaces")

	namespacesToDelete := []string{
		amdgpuparams.AMDGPUNamespace,
		"openshift-kmm",
	}

	for _, nsName := range namespacesToDelete {
		nsBuilder, err := namespace.Pull(apiClient, nsName)
		if err != nil {
			klog.V(amdgpuparams.AMDGPULogLevel).Infof("Namespace %s not found: %v", nsName, err)

			continue
		}

		if nsBuilder == nil || !nsBuilder.Exists() {
			continue
		}

		err = nsBuilder.DeleteAndWait(5 * time.Minute)
		if err != nil {
			klog.V(amdgpuparams.AMDGPULogLevel).Infof("Failed to delete namespace %s: %v", nsName, err)

			// Try force deletion
			err = forceDeleteNamespace(apiClient, nsName)
			if err != nil {
				klog.V(amdgpuparams.AMDGPULogLevel).Infof("Force delete also failed for %s: %v", nsName, err)
			}
		}
	}

	klog.V(amdgpuparams.AMDGPULogLevel).Info("Namespace deletion completed")

	return nil
}

// forceDeleteNamespace attempts to force delete a stuck namespace by removing finalizers.
func forceDeleteNamespace(apiClient *clients.Settings, nsName string) error {
	ctx := context.Background()

	namespaceObj, err := apiClient.CoreV1Interface.Namespaces().Get(ctx, nsName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Remove finalizers
	namespaceObj.Spec.Finalizers = nil

	_, err = apiClient.CoreV1Interface.Namespaces().Finalize(ctx, namespaceObj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to remove finalizers: %w", err)
	}

	return nil
}

