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
	"k8s.io/apimachinery/pkg/runtime/schema"
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

// NFDRuleGVR defines the GroupVersionResource for NodeFeatureRule.
var NFDRuleGVR = schema.GroupVersionResource{
	Group:    "nfd.openshift.io",
	Version:  "v1alpha1",
	Resource: "nodefeaturerules",
}

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
		klog.V(params.NeuronLogLevel).Infof("Warning: NFD workers readiness check: %v", err)
	}

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

	ruleYAML := getNeuronNFDRuleYAML(namespace)
	nfdRuleBuilder := nfd.NewNodeFeatureRuleBuilderFromObjectString(apiClient, ruleYAML)

	if nfdRuleBuilder == nil {
		return fmt.Errorf("failed to create NodeFeatureRule builder")
	}

	if nfdRuleBuilder.Exists() {
		klog.V(params.NeuronLogLevel).Info("Neuron NodeFeatureRule already exists")

		return nil
	}

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

	// Use dynamic client since NodeFeatureRuleBuilder doesn't have Delete method
	err := apiClient.Resource(NFDRuleGVR).
		Namespace(namespace).
		Delete(context.Background(), NeuronNFDRuleName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete Neuron NFD rule: %w", err)
	}

	klog.V(params.NeuronLogLevel).Info("Successfully deleted Neuron NodeFeatureRule")

	return nil
}

// getNeuronNFDRuleYAML returns the YAML configuration for Neuron NodeFeatureRule.
func getNeuronNFDRuleYAML(namespace string) string {
	// Build device IDs JSON array
	deviceIDsJSON := "["

	for i, id := range params.DeviceIDs {
		if i > 0 {
			deviceIDsJSON += ", "
		}

		deviceIDsJSON += fmt.Sprintf("\"%s\"", id)
	}

	deviceIDsJSON += "]"

	// Match using matchAny with pci.device vendor and device IDs
	return fmt.Sprintf(`
[
    {
        "apiVersion": "nfd.openshift.io/v1alpha1",
        "kind": "NodeFeatureRule",
        "metadata": {
            "name": "%s",
            "namespace": "%s"
        },
        "spec": {
            "rules": [
                {
                    "name": "neuron-device",
                    "labels": {
                        "%s": "%s"
                    },
                    "matchAny": [
                        {
                            "matchFeatures": [
                                {
                                    "feature": "pci.device",
                                    "matchExpressions": {
                                        "vendor": {
                                            "op": "In",
                                            "value": ["%s"]
                                        },
                                        "device": {
                                            "op": "In",
                                            "value": %s
                                        }
                                    }
                                }
                            ]
                        }
                    ]
                }
            ]
        }
    }
]`, NeuronNFDRuleName, namespace, params.NeuronNFDLabelKey, params.NeuronNFDLabelValue,
		params.PCIVendorID, deviceIDsJSON)
}

// NFDRuleExists checks if the Neuron NFD rule exists.
func NFDRuleExists(apiClient *clients.Settings, namespace string) bool {
	_, err := apiClient.Resource(NFDRuleGVR).
		Namespace(namespace).
		Get(context.Background(), NeuronNFDRuleName, metav1.GetOptions{})

	return err == nil
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
