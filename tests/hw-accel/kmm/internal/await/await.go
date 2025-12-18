package await

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

var buildPod = make(map[string]string)

// BuildPodCompleted awaits kmm build pods to finish build.
func BuildPodCompleted(apiClient *clients.Settings, nsname string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			if buildPod[nsname] == "" {
				// Search across all pod phases to catch build pods that may complete quickly
				pods, err := pod.List(apiClient, nsname, metav1.ListOptions{})
				if err != nil {
					klog.V(kmmparams.KmmLogLevel).Infof("build list error: %s", err)
				}

				for _, podObj := range pods {
					if strings.Contains(podObj.Object.Name, "-build") {
						buildPod[nsname] = podObj.Object.Name
						klog.V(kmmparams.KmmLogLevel).Infof("Build pod '%s' found\n", podObj.Object.Name)
					}
				}
			}

			if buildPod[nsname] != "" {
				fieldSelector := fmt.Sprintf("metadata.name=%s", buildPod[nsname])
				pods, _ := pod.List(apiClient, nsname, metav1.ListOptions{FieldSelector: fieldSelector})

				if len(pods) == 0 {
					klog.V(kmmparams.KmmLogLevel).Infof("BuildPod %s no longer in namespace", buildPod)
					buildPod[nsname] = ""

					return true, nil
				}

				for _, podObj := range pods {
					if strings.Contains(string(podObj.Object.Status.Phase), "Failed") {
						err = fmt.Errorf("BuildPod %s has failed", podObj.Object.Name)
						klog.V(kmmparams.KmmLogLevel).Info(err)

						buildPod[nsname] = ""

						return false, err
					}

					if strings.Contains(string(podObj.Object.Status.Phase), "Succeeded") {
						klog.V(kmmparams.KmmLogLevel).Infof("BuildPod %s is in phase Succeeded",
							podObj.Object.Name)

						buildPod[nsname] = ""

						return true, nil
					}
				}
			}

			return false, err
		})
}

// ModuleDeployment awaits module to de deployed.
func ModuleDeployment(apiClient *clients.Settings, moduleName, nsname string,
	timeout time.Duration, selector map[string]string) error {
	label := fmt.Sprintf(kmmparams.ModuleNodeLabelTemplate, nsname, moduleName)

	return deploymentPerLabel(apiClient, moduleName, label, timeout, selector)
}

// ModuleVersionDeployment awaits module with version to be deployed.
func ModuleVersionDeployment(apiClient *clients.Settings, moduleName, nsName string,
	timeout time.Duration, selector map[string]string, labelValue string) error {
	label := fmt.Sprintf(kmmparams.ModuleVersionNodeLabelTemplate, nsName, moduleName)

	return deploymentPerLabel(apiClient, moduleName, label, timeout, selector, labelValue)
}

// DeviceDriverDeployment awaits device driver pods to de deployed.
func DeviceDriverDeployment(apiClient *clients.Settings, moduleName, nsname string,
	timeout time.Duration, selector map[string]string) error {
	label := fmt.Sprintf(kmmparams.DevicePluginNodeLabelTemplate, nsname, moduleName)

	return deploymentPerLabel(apiClient, moduleName, label, timeout, selector)
}

// ModuleUndeployed awaits module pods to be undeployed.
func ModuleUndeployed(apiClient *clients.Settings, nsName string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			pods, err := pod.List(apiClient, nsName, metav1.ListOptions{})
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("pod list error: %s\n", err)

				return false, err
			}

			klog.V(kmmparams.KmmLogLevel).Infof("current number of pods: %v\n", len(pods))

			return len(pods) == 0, nil
		})
}

// ModuleObjectDeleted awaits module object to be deleted.
func ModuleObjectDeleted(apiClient *clients.Settings, moduleName, nsName string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := kmm.Pull(apiClient, moduleName, nsName)
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("error while pulling the module; most likely it is deleted")
			}

			return err != nil, nil
		})
}

// BootModuleConfigObjectDeleted awaits BootModuleConfig object to be deleted.
func BootModuleConfigObjectDeleted(apiClient *clients.Settings, bmcName, nsName string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := kmm.PullBootModuleConfig(apiClient, bmcName, nsName)
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("error while pulling the BootModuleConfig; most likely it is deleted")
			}

			return err != nil, nil
		})
}

// PreflightStageDone awaits preflightvalidationocp to be in stage Done.
func PreflightStageDone(apiClient *clients.Settings, preflight, module, nsname string,
	timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			pre, err := kmm.PullPreflightValidationOCP(apiClient, preflight,
				nsname)
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("error pulling preflightvalidationocp")
			}

			preflightValidationOCP, err := pre.Get()
			if err != nil {
				return false, err
			}

			// Search for the module in the new Modules array structure
			for _, moduleStatus := range preflightValidationOCP.Status.Modules {
				if moduleStatus.Name == module && moduleStatus.Namespace == nsname {
					status := moduleStatus.VerificationStage
					klog.V(kmmparams.KmmLogLevel).Infof("Stage: %s", status)

					return status == "Done", nil
				}
			}

			klog.V(kmmparams.KmmLogLevel).Infof("module %s not found in preflight validation status", module)

			return false, nil
		})
}

func deploymentPerLabel(apiClient *clients.Settings, moduleName, label string,
	timeout time.Duration, selector map[string]string, expectedLabelValue ...string) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			nodeBuilder, err := nodes.List(apiClient, metav1.ListOptions{LabelSelector: labels.Set(selector).String()})
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("could not discover %v nodes", selector)
			}

			nodesForSelector, err := get.NumberOfNodesForSelector(apiClient, selector)
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("nodes list error: %s", err)

				return false, err
			}

			foundLabels := 0

			for _, node := range nodeBuilder {
				klog.V(kmmparams.KmmLogLevel).Infof("Existing labels: %v", node.Object.Labels)

				value, ok := node.Object.Labels[label]
				if ok {
					klog.V(kmmparams.KmmLogLevel).Infof("Found label %v that contains %v on node %v",
						label, moduleName, node.Object.Name)

					if len(expectedLabelValue) > 0 {
						klog.V(kmmparams.KmmLogLevel).Infof("Checking label value is: %s", expectedLabelValue[0])
						klog.V(kmmparams.KmmLogLevel).Infof("Current node label value is: %s", value)

						if expectedLabelValue[0] == value {
							klog.V(kmmparams.KmmLogLevel).Infof("Label value: %s matches the expected value: %s",
								node.Object.Labels[label],
								expectedLabelValue[0],
							)
						} else {
							return false, fmt.Errorf("label value '%s' does not match the expected value: '%s'",
								node.Object.Labels[label], expectedLabelValue[0])
						}
					}

					foundLabels++
					klog.V(kmmparams.KmmLogLevel).Infof("Number of nodes: %v, Number of nodes with '%v' label: %v\n",
						nodesForSelector, label, foundLabels)

					if foundLabels == len(nodeBuilder) {
						return true, nil
					}
				}
			}

			return false, err
		})
}

// MachineConfigCreated awaits MachineConfig to be created.
func MachineConfigCreated(apiClient *clients.Settings, mcName string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			mcBuilder, err := mco.PullMachineConfig(apiClient, mcName)
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("MachineConfig %s not found yet: %v", mcName, err)

				return false, nil
			}

			if mcBuilder != nil && mcBuilder.Exists() {
				klog.V(kmmparams.KmmLogLevel).Infof("MachineConfig %s created", mcName)

				return true, nil
			}

			return false, nil
		})
}

// NodeDesiredConfigChange waits for MCO to render a new config for the node
// and for the node to be ready for manual reboot.
// This function handles three scenarios:
// 1. Node already has a pending config change (desiredConfig != currentConfig) - proceeds immediately
// 2. MCO needs to render a new config - waits for desiredConfig to change
// 3. Node is ready for reboot - state is "Done" with pending config
func NodeDesiredConfigChange(
	apiClient *clients.Settings,
	nodeName string,
	timeout time.Duration,
) error {
	node, err := nodes.Pull(apiClient, nodeName)
	if err != nil {
		return fmt.Errorf("failed to pull node %s: %w", nodeName, err)
	}

	initialCurrentConfig := node.Object.Annotations["machineconfiguration.openshift.io/currentConfig"]
	initialDesiredConfig := node.Object.Annotations["machineconfiguration.openshift.io/desiredConfig"]
	initialState := node.Object.Annotations["machineconfiguration.openshift.io/state"]

	klog.V(kmmparams.KmmLogLevel).Infof(
		"Node %s initial state - currentConfig: %s, desiredConfig: %s, state: %s",
		nodeName, initialCurrentConfig, initialDesiredConfig, initialState)

	// Check if node already has a pending config change and is ready for reboot
	if initialDesiredConfig != initialCurrentConfig && initialState == "Done" {
		klog.V(kmmparams.KmmLogLevel).Infof(
			"Node %s already has pending config change and is ready for reboot", nodeName)

		return nil
	}

	// Wait for node to be ready for manual reboot:
	// - desiredConfig differs from currentConfig (new config is pending)
	// - state is "Done" (MCO finished processing, waiting for reboot)
	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			updatedNode, err := nodes.Pull(apiClient, nodeName)
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("Error pulling node %s: %v", nodeName, err)

				return false, nil
			}

			currentCfg := updatedNode.Object.Annotations["machineconfiguration.openshift.io/currentConfig"]
			desiredCfg := updatedNode.Object.Annotations["machineconfiguration.openshift.io/desiredConfig"]
			nodeState := updatedNode.Object.Annotations["machineconfiguration.openshift.io/state"]

			klog.V(kmmparams.KmmLogLevel).Infof(
				"Node %s - currentConfig: %s, desiredConfig: %s, state: %s",
				nodeName, currentCfg, desiredCfg, nodeState)

			// Ready for manual reboot when:
			// - desiredConfig differs from currentConfig (new config is pending)
			// - state is "Done" (MCO finished processing, waiting for reboot)
			if desiredCfg != currentCfg && nodeState == "Done" {
				klog.V(kmmparams.KmmLogLevel).Infof(
					"Node %s is ready for manual reboot (new config pending, state: Done)", nodeName)

				return true, nil
			}

			// Log what we're waiting for
			if desiredCfg == currentCfg {
				klog.V(kmmparams.KmmLogLevel).Infof(
					"Waiting for MCO to render new config (desiredConfig == currentConfig)")
			} else if nodeState != "Done" {
				klog.V(kmmparams.KmmLogLevel).Infof(
					"Config pending but state is %s (waiting for Done)", nodeState)
			}

			return false, nil
		})
}

// NodeConfigApplied waits for the node to have applied the new config after reboot.
func NodeConfigApplied(
	apiClient *clients.Settings,
	nodeName string,
	timeout time.Duration,
) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			updatedNode, err := nodes.Pull(apiClient, nodeName)
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("Error pulling node %s: %v", nodeName, err)

				return false, nil
			}

			currentCfg := updatedNode.Object.Annotations["machineconfiguration.openshift.io/currentConfig"]
			desiredCfg := updatedNode.Object.Annotations["machineconfiguration.openshift.io/desiredConfig"]
			nodeState := updatedNode.Object.Annotations["machineconfiguration.openshift.io/state"]

			klog.V(kmmparams.KmmLogLevel).Infof(
				"Node %s - currentConfig: %s, desiredConfig: %s, state: %s",
				nodeName, currentCfg, desiredCfg, nodeState)

			if currentCfg == desiredCfg && nodeState == "Done" {
				klog.V(kmmparams.KmmLogLevel).Infof(
					"Node %s has applied new config successfully", nodeName)

				return true, nil
			}

			return false, nil
		})
}
