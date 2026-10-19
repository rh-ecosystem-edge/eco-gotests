package neuronhelpers

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

var (
	kmmModuleGVR = schema.GroupVersionResource{
		Group: "kmm.sigs.x-k8s.io", Version: "v1beta1", Resource: "modules",
	}
	nodeModulesConfigGVR = schema.GroupVersionResource{
		Group: "kmm.sigs.x-k8s.io", Version: "v1beta1", Resource: "nodemodulesconfigs",
	}
)

// CleanupVLLMWorkload removes the shared vLLM namespace and waits for all of
// its pods to terminate before a DeviceConfig lifecycle operation begins.
func CleanupVLLMWorkload(apiClient *clients.Settings, timeout time.Duration) error {
	nsBuilder, err := namespace.Pull(apiClient, params.VLLMTestNamespace)
	if apierrors.IsNotFound(err) {
		klog.V(params.NeuronLogLevel).Infof(
			"vLLM namespace %s is not present; no cleanup required", params.VLLMTestNamespace)

		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to check vLLM namespace %s: %w", params.VLLMTestNamespace, err)
	}

	klog.V(params.NeuronLogLevel).Infof(
		"Deleting vLLM namespace %s before DeviceConfig lifecycle operations", params.VLLMTestNamespace)

	if err := nsBuilder.DeleteAndWait(timeout); err != nil {
		LogDeviceConfigDeletionDiagnostics(apiClient, fmt.Sprintf(
			"vLLM namespace cleanup failed: %v", err))

		return fmt.Errorf("vLLM namespace %s was not fully deleted: %w", params.VLLMTestNamespace, err)
	}

	klog.V(params.NeuronLogLevel).Infof(
		"vLLM namespace %s and its workload consumers are fully deleted", params.VLLMTestNamespace)

	return nil
}

// WaitForKMMModuleReady waits until the KMM Module and device plugin have
// converged on all Neuron nodes before the DeviceConfig is replaced.
func WaitForKMMModuleReady(apiClient *clients.Settings, name, namespace string, timeout time.Duration) error {
	var lastState string

	err := wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			module, err := kmm.Pull(apiClient, name, namespace)
			if err != nil {
				lastState = fmt.Sprintf("module unavailable: %v", err)

				return false, nil
			}

			if module.Definition.DeletionTimestamp != nil {
				lastState = "module is being deleted"

				return false, nil
			}

			moduleLoader := module.Definition.Status.ModuleLoader

			devicePlugin := module.Definition.Status.DevicePlugin
			if moduleLoader.DesiredNumber == 0 ||
				moduleLoader.AvailableNumber != moduleLoader.DesiredNumber ||
				devicePlugin.DesiredNumber == 0 ||
				devicePlugin.AvailableNumber != devicePlugin.DesiredNumber {
				lastState = fmt.Sprintf(
					"module loader %d/%d, device plugin %d/%d",
					moduleLoader.AvailableNumber, moduleLoader.DesiredNumber,
					devicePlugin.AvailableNumber, devicePlugin.DesiredNumber)

				return false, nil
			}

			lastState = "ready"

			return true, nil
		})
	if err != nil {
		return fmt.Errorf("KMM Module %s/%s did not become ready: %s: %w",
			namespace, name, lastState, err)
	}

	klog.V(params.NeuronLogLevel).Infof("KMM Module %s/%s is ready", namespace, name)

	return nil
}

// WaitForDeviceConfigDeletion waits for the DeviceConfig, its KMM Module, and
// all NMC status entries for that Module to disappear before recreation.
func WaitForDeviceConfigDeletion(apiClient *clients.Settings, name, namespace string,
	timeout time.Duration) error {
	var lastState string

	err := wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			deviceConfigExists, err := resourceExists(
				ctx, apiClient.Resource(neuronparams.DeviceConfigGVR).Namespace(namespace), name)
			if err != nil {
				lastState = fmt.Sprintf("DeviceConfig lookup failed: %v", err)

				return false, nil
			}

			moduleExists, err := resourceExists(
				ctx, apiClient.Resource(kmmModuleGVR).Namespace(namespace), name)
			if err != nil {
				lastState = fmt.Sprintf("Module lookup failed: %v", err)

				return false, nil
			}

			consumers, err := listNMCModuleConsumers(ctx, apiClient, name, namespace)
			if err != nil {
				lastState = fmt.Sprintf("NMC lookup failed: %v", err)

				return false, nil
			}

			lastState = fmt.Sprintf("DeviceConfig=%t Module=%t NMC consumers=%v",
				deviceConfigExists, moduleExists, consumers)

			return !deviceConfigExists && !moduleExists && len(consumers) == 0, nil
		})
	if err != nil {
		LogDeviceConfigDeletionDiagnostics(apiClient, fmt.Sprintf(
			"DeviceConfig deletion did not converge: %s", lastState))

		return fmt.Errorf("DeviceConfig %s/%s and its KMM resources were not fully deleted: %w",
			namespace, name, err)
	}

	klog.V(params.NeuronLogLevel).Infof(
		"DeviceConfig %s/%s, KMM Module, and NMC consumers are fully deleted",
		namespace, name)

	return nil
}

// resourceExists distinguishes an absent resource from a failed API lookup.
func resourceExists(ctx context.Context, resource interface {
	Get(context.Context, string, metav1.GetOptions, ...string) (*unstructured.Unstructured, error)
}, name string) (bool, error) {
	_, err := resource.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

// listNMCModuleConsumers returns NMCs still reporting the named KMM Module.
func listNMCModuleConsumers(ctx context.Context, apiClient *clients.Settings,
	moduleName, moduleNamespace string) ([]string, error) {
	list, err := apiClient.Resource(nodeModulesConfigGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	consumers := make([]string, 0)

	for _, nmc := range list.Items {
		modules, found, err := unstructured.NestedSlice(nmc.Object, "status", "modules")
		if err != nil {
			return nil, err
		}

		if !found {
			continue
		}

		for _, rawModule := range modules {
			module, ok := rawModule.(map[string]interface{})
			if !ok {
				continue
			}

			name, _, _ := unstructured.NestedString(module, "name")

			namespace, _, _ := unstructured.NestedString(module, "namespace")
			if name == moduleName && (namespace == "" || namespace == moduleNamespace) {
				consumers = append(consumers, nmc.GetName())

				break
			}
		}
	}

	sort.Strings(consumers)

	return consumers, nil
}
