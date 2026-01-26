package neuronhelpers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/neuron"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nfd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	// NeuronNFDRuleName is the name of the Neuron NodeFeatureRule.
	NeuronNFDRuleName = "neuron-nfd-rule"
	// NFDInstanceName is the name of the NodeFeatureDiscovery instance.
	NFDInstanceName = "nfd-instance"
	// NFDInstanceWaitTimeout is the timeout for waiting for NFD workers to be ready.
	NFDInstanceWaitTimeout = 5 * time.Minute
)

// CreateNFDInstance creates the NodeFeatureDiscovery instance to deploy NFD workers.
func CreateNFDInstance(apiClient *clients.Settings) error {
	klog.V(params.NeuronLogLevel).Info("Creating NFD instance to deploy NFD workers")

	nfdBuilder := nfd.NewBuilderFromObjectString(apiClient, getNFDInstanceYAML())
	if nfdBuilder == nil {
		return fmt.Errorf("failed to create NFD instance builder")
	}

	if nfdBuilder.Exists() {
		klog.V(params.NeuronLogLevel).Info("NFD instance already exists")

		return nil
	}

	_, err := nfdBuilder.Create()
	if err != nil {
		return fmt.Errorf("failed to create NFD instance: %w", err)
	}

	klog.V(params.NeuronLogLevel).Info("Successfully created NFD instance, waiting for workers")

	// Wait for NFD workers to start collecting node features
	err = waitForNFDWorkersReady(apiClient)
	if err != nil {
		return fmt.Errorf("NFD workers failed to become ready: %w", err)
	}

	klog.V(params.NeuronLogLevel).Info("NFD workers are ready")

	return nil
}

// getNFDInstanceYAML returns the YAML configuration for NodeFeatureDiscovery instance.
func getNFDInstanceYAML() string {
	// Worker config for PCI device discovery
	workerConfigData := "sources:\\n  pci:\\n    deviceClassWhitelist:\\n" +
		"      - \\\"0300\\\"\\n      - \\\"0302\\\"\\n      - \\\"0c80\\\"\\n" +
		"    deviceLabelFields:\\n      - vendor\\n      - device\\n"

	// Note: We don't specify operand.image - the NFD operator will use
	// the correct default image that matches the installed OCP version.
	return fmt.Sprintf(`
[
    {
        "apiVersion": "nfd.openshift.io/v1",
        "kind": "NodeFeatureDiscovery",
        "metadata": {
            "name": "%s",
            "namespace": "%s"
        },
        "spec": {
            "workerConfig": {
                "configData": "%s"
            }
        }
    }
]`, NFDInstanceName, params.NFDNamespace, workerConfigData)
}

// waitForNFDWorkersReady waits for NFD worker pods to be running.
func waitForNFDWorkersReady(apiClient *clients.Settings) error {
	klog.V(params.NeuronLogLevel).Info("Waiting for NFD workers to be ready")

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, NFDInstanceWaitTimeout, true,
		func(ctx context.Context) (bool, error) {
			// List all daemonsets in NFD namespace and find one with "worker" in name
			dsList, err := apiClient.K8sClient.AppsV1().DaemonSets(params.NFDNamespace).List(
				ctx, metav1.ListOptions{})
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Error listing daemonsets: %v", err)

				return false, nil
			}

			// Find worker daemonset by name pattern
			for _, workerDS := range dsList.Items {
				if strings.Contains(workerDS.Name, "worker") {
					ready := workerDS.Status.NumberReady
					desired := workerDS.Status.DesiredNumberScheduled

					klog.V(params.NeuronLogLevel).Infof("Found NFD worker daemonset %s: %d/%d ready",
						workerDS.Name, ready, desired)

					if ready > 0 && ready == desired {
						klog.V(params.NeuronLogLevel).Infof("NFD workers ready: %d/%d", ready, desired)

						return true, nil
					}

					return false, nil
				}
			}

			klog.V(params.NeuronLogLevel).Infof("NFD worker daemonset not found yet (found %d daemonsets)",
				len(dsList.Items))

			return false, nil
		})
}

// NFDInstanceExists checks if the NFD instance exists.
func NFDInstanceExists(apiClient *clients.Settings) bool {
	_, err := nfd.Pull(apiClient, NFDInstanceName, params.NFDNamespace)

	return err == nil
}

// DeleteNFDInstance deletes the NodeFeatureDiscovery instance.
func DeleteNFDInstance(apiClient *clients.Settings) error {
	klog.V(params.NeuronLogLevel).Info("Deleting NFD instance")

	nfdBuilder, err := nfd.Pull(apiClient, NFDInstanceName, params.NFDNamespace)
	if err != nil {
		// Already deleted or doesn't exist
		return nil
	}

	_, err = nfdBuilder.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete NFD instance: %w", err)
	}

	klog.V(params.NeuronLogLevel).Info("Successfully deleted NFD instance")

	return nil
}

// CreateNeuronNFDRule creates the NodeFeatureRule for Neuron device detection.
func CreateNeuronNFDRule(apiClient *clients.Settings, namespace string) error {
	klog.V(params.NeuronLogLevel).Info("Creating Neuron NodeFeatureRule")

	nfdRuleBuilder := nfd.NewNodeFeatureRuleBuilder(apiClient, NeuronNFDRuleName, namespace)
	if nfdRuleBuilder == nil {
		return fmt.Errorf("failed to create NodeFeatureRule builder")
	}

	if nfdRuleBuilder.Exists() {
		klog.V(params.NeuronLogLevel).Info("Neuron NodeFeatureRule already exists")

		return nil
	}

	// Use WithSimplePCIRule for clean PCI device detection
	nfdRuleBuilder = nfdRuleBuilder.WithSimplePCIRule(
		"neuron-device",
		map[string]string{params.NeuronNFDLabelKey: params.NeuronNFDLabelValue},
		[]string{params.PCIVendorID},
		params.DeviceIDs,
	)

	_, err := nfdRuleBuilder.Create()
	if err != nil {
		return fmt.Errorf("failed to create Neuron NFD rule: %w", err)
	}

	klog.V(params.NeuronLogLevel).Info("Successfully created Neuron NodeFeatureRule")

	return nil
}

// DeleteNeuronNFDRule deletes the Neuron NodeFeatureRule.
func DeleteNeuronNFDRule(apiClient *clients.Settings, namespace string) error {
	klog.V(params.NeuronLogLevel).Info("Deleting Neuron NodeFeatureRule")

	nfdRuleBuilder, err := nfd.PullFeatureRule(apiClient, NeuronNFDRuleName, namespace)
	if err != nil {
		// Already deleted or doesn't exist
		klog.V(params.NeuronLogLevel).Info("Neuron NodeFeatureRule doesn't exist, nothing to delete")

		return nil
	}

	_, err = nfdRuleBuilder.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete Neuron NFD rule: %w", err)
	}

	klog.V(params.NeuronLogLevel).Info("Successfully deleted Neuron NodeFeatureRule")

	return nil
}

// NFDRuleExists checks if the Neuron NFD rule exists.
func NFDRuleExists(apiClient *clients.Settings, namespace string) bool {
	ruleBuilder, err := nfd.PullFeatureRule(apiClient, NeuronNFDRuleName, namespace)

	return err == nil && ruleBuilder != nil
}

// CreateDeviceConfigFromEnv creates a DeviceConfig from environment configuration.
func CreateDeviceConfigFromEnv(
	apiClient *clients.Settings,
	driversImage, driverVersion, devicePluginImage,
	schedulerImage, schedulerExtensionImage, imageRepoSecretName string,
) error {
	builder := neuron.NewBuilder(
		apiClient,
		params.DefaultDeviceConfigName,
		params.NeuronNamespace,
		driversImage,
		driverVersion,
		devicePluginImage,
	).WithSelector(map[string]string{
		params.NeuronNFDLabelKey: params.NeuronNFDLabelValue,
	})

	if schedulerImage != "" && schedulerExtensionImage != "" {
		builder = builder.WithScheduler(schedulerImage, schedulerExtensionImage)
	}

	if imageRepoSecretName != "" {
		builder = builder.WithImageRepoSecret(imageRepoSecretName)
	}

	_, err := builder.Create()

	return err
}
