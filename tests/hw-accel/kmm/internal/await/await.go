package await

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
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
// required from KMM 2.0 so that NMC has time to unload the modules.
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
	timeout time.Duration, selector map[string]string) error {
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
				klog.V(kmmparams.KmmLogLevel).Infof("%v", node.Object.Labels)

				_, ok := node.Object.Labels[label]
				if ok {
					klog.V(kmmparams.KmmLogLevel).Infof("Found label %v that contains %v on node %v",
						label, moduleName, node.Object.Name)

					foundLabels++
					klog.V(kmmparams.KmmLogLevel).Infof("Number of nodes: %v, Number of nodes with '%v' label pods: %v\n",
						nodesForSelector, label, foundLabels)

					if foundLabels == len(nodeBuilder) {
						return true, nil
					}
				}
			}

			return false, err
		})
}
