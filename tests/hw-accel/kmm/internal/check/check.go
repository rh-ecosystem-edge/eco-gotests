package check

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/imagestream"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmminittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// IsKMMHub returns true if the test is running against KMM-HUB operator instead of KMM operator.
func IsKMMHub() bool {
	return strings.Contains(ModulesConfig.SubscriptionName, "hub")
}

// NodeLabel checks if label is present on the node.
func NodeLabel(apiClient *clients.Settings, moduleName, nsname string, nodeSelector map[string]string) (bool, error) {
	nodeBuilder, err := nodes.List(apiClient, metav1.ListOptions{LabelSelector: labels.Set(nodeSelector).String()})
	if err != nil {
		klog.V(kmmparams.KmmLogLevel).Infof("could not discover %v nodes", nodeSelector)
	}

	foundLabels := 0
	label := fmt.Sprintf(kmmparams.ModuleNodeLabelTemplate, nsname, moduleName)

	for _, node := range nodeBuilder {
		_, ok := node.Object.Labels[label]
		if ok {
			klog.V(kmmparams.KmmLogLevel).Infof("Found label %v that contains %v on node %v",
				label, moduleName, node.Object.Name)

			foundLabels++
			if foundLabels == len(nodeBuilder) {
				return true, nil
			}
		}
	}

	err = fmt.Errorf("not all nodes (%v) have the label '%s' ", len(nodeBuilder), label)

	return false, err
}

// ModuleLoaded verifies the module is loaded on the node.
func ModuleLoaded(apiClient *clients.Settings, modName string, timeout time.Duration) error {
	modName = strings.Replace(modName, "-", "_", 10)

	return runCommandOnTestPods(apiClient, []string{"lsmod"}, modName, timeout)
}

// ModuleLoadedOnNode verifies the module is loaded on a specific node.
func ModuleLoadedOnNode(apiClient *clients.Settings, modName string, timeout time.Duration, nodeName string) error {
	modName = strings.Replace(modName, "-", "_", 10)

	return runCommandOnTestPodsOnNode(apiClient, []string{"lsmod"}, modName, timeout, nodeName)
}

// ModuleExistsOnNode checks if a kernel module exists on the specified node using modinfo.
func ModuleExistsOnNode(apiClient *clients.Settings, moduleName, nodeName string) (bool, error) {
	pods, err := pod.List(apiClient, kmmparams.KmmOperatorNamespace, metav1.ListOptions{
		FieldSelector: "status.phase=Running",
		LabelSelector: kmmparams.KmmTestHelperLabelName,
	})
	if err != nil {
		return false, fmt.Errorf("failed to list helper pods: %w", err)
	}

	for _, helperPod := range pods {
		if helperPod.Object.Spec.NodeName != nodeName {
			continue
		}

		// Use modinfo to check if the module exists on the system
		command := []string{"chroot", "/host", "modinfo", moduleName}
		klog.V(kmmparams.KmmLogLevel).Infof("Checking if module %s exists on node %s", moduleName, nodeName)

		buff, err := helperPod.ExecCommand(command, "test")
		if err != nil {
			// modinfo returns non-zero if module doesn't exist
			klog.V(kmmparams.KmmLogLevel).Infof("Module %s does not exist on node %s: %v", moduleName, nodeName, err)

			return false, err
		}

		contents := buff.String()
		klog.V(kmmparams.KmmLogLevel).Infof("modinfo output for %s: %s", moduleName, contents)

		return true, nil
	}

	return false, fmt.Errorf("no helper pod found on node %s", nodeName)
}

// ModuleNotLoadedOnNode verifies a module is not loaded on a specific node.
func ModuleNotLoadedOnNode(apiClient *clients.Settings, modName string, timeout time.Duration, nodeName string) error {
	modName = strings.Replace(modName, "-", "_", 10)

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			pods, err := pod.List(apiClient, kmmparams.KmmOperatorNamespace, metav1.ListOptions{
				FieldSelector: "status.phase=Running",
				LabelSelector: kmmparams.KmmTestHelperLabelName,
			})
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("deployment list error: %s\n", err)

				return false, err
			}

			for _, iterPod := range pods {
				if iterPod.Object.Spec.NodeName != nodeName {
					continue
				}

				klog.V(kmmparams.KmmLogLevel).Infof("Checking module %s is NOT loaded on node %s via pod %s",
					modName, nodeName, iterPod.Object.Name)

				buff, err := iterPod.ExecCommand([]string{"lsmod"}, "test")
				if err != nil {
					return false, err
				}

				contents := buff.String()

				if !strings.Contains(contents, modName) {
					klog.V(kmmparams.KmmLogLevel).Infof("Module %s is NOT loaded on node %s (as expected)",
						modName, nodeName)

					return true, nil
				}

				klog.V(kmmparams.KmmLogLevel).Infof("Module %s is still loaded on node %s, waiting...",
					modName, nodeName)

				return false, nil
			}

			return false, fmt.Errorf("no helper pod found on node %s", nodeName)
		})
}

// Dmesg verifies that dmesg contains message.
func Dmesg(apiClient *clients.Settings, message string, timeout time.Duration) error {
	return runCommandOnTestPods(apiClient, []string{"dmesg"}, message, timeout)
}

// ModuleSigned verifies the module is signed.
func ModuleSigned(apiClient *clients.Settings, modName, message, nsname, image string) error {
	modulePath := fmt.Sprintf("modinfo /opt/lib/modules/*/%s.ko", modName)
	command := []string{"bash", "-c", modulePath}

	kernelVersion, err := get.KernelFullVersion(apiClient, GeneralConfig.WorkerLabelMap)
	if err != nil {
		return err
	}

	processedImage := strings.ReplaceAll(image, "$KERNEL_FULL_VERSION", kernelVersion)
	testPod := pod.NewBuilder(apiClient, "image-checker", nsname, processedImage)

	_, err = testPod.CreateAndWaitUntilRunning(2 * time.Minute)
	if err != nil {
		klog.V(kmmparams.KmmLogLevel).Infof("Could not create signing verification pod. Got error : %v", err)

		return err
	}

	klog.V(kmmparams.KmmLogLevel).Infof("\n\nPodName: %v\n\n", testPod.Object.Name)

	buff, err := testPod.ExecCommand(command, "test")
	if err != nil {
		return err
	}

	_, _ = testPod.Delete()

	contents := buff.String()
	klog.V(kmmparams.KmmLogLevel).Infof("%s contents: \n \t%v\n", command, contents)

	if strings.Contains(contents, message) {
		klog.V(kmmparams.KmmLogLevel).Infof("command '%s' output contains '%s'\n", command, message)

		return nil
	}

	err = fmt.Errorf("could not find signature in module")

	return err
}

// IntreeModuleLoaded makes sure the needed in-tree module is present on the nodes.
func IntreeModuleLoaded(apiClient *clients.Settings, module string, timeout time.Duration) error {
	return runCommandOnTestPods(apiClient, []string{"modprobe", module}, "", timeout)
}

// ImageStreamExistsForModule validates that an imagestream exists with the correct kernel tag
// This only runs when using OpenShift internal registry.
func ImageStreamExistsForModule(apiClient *clients.Settings, namespace, moduleName, kmodName, tag string) error {
	// Check if module uses internal registry
	module, err := kmm.Pull(apiClient, moduleName, namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(kmmparams.KmmLogLevel).Infof("Module %s not found in namespace %s, skipping imagestream check",
				moduleName, namespace)

			return nil
		}

		return fmt.Errorf("failed to pull module %s/%s: %w", namespace, moduleName, err)
	}

	usesInternalRegistry := false

	if module.Object != nil && module.Object.Spec.ModuleLoader != nil {
		container := module.Object.Spec.ModuleLoader.Container
		if len(container.KernelMappings) > 0 {
			for _, km := range container.KernelMappings {
				if strings.Contains(km.ContainerImage, "image-registry.openshift-image-registry.svc") {
					usesInternalRegistry = true

					break
				}
			}
		}
	}

	// Skip validation if not using internal registry
	if !usesInternalRegistry {
		klog.V(kmmparams.KmmLogLevel).Infof("Module %s not using internal registry, skipping imagestream check",
			moduleName)

		return nil
	}

	// ImageStream name is the same as kmod name
	imagestreamName := kmodName
	klog.V(kmmparams.KmmLogLevel).Infof("Checking ImageStream %s/%s for kernel tag %s",
		namespace, imagestreamName, tag)

	// Pull the specific ImageStream
	imgStreamBuilder, err := imagestream.Pull(apiClient, imagestreamName, namespace)
	if err != nil {
		return fmt.Errorf("failed to pull ImageStream %s/%s: %w", namespace, imagestreamName, err)
	}

	statusTags, err := imgStreamBuilder.GetStatusTags()
	if err != nil {
		return fmt.Errorf("failed to get status tags for ImageStream %s/%s: %w", namespace, imagestreamName, err)
	}

	// Check if kernel version exists in status tags
	for _, statusTag := range statusTags {
		if statusTag == tag {
			klog.V(kmmparams.KmmLogLevel).Infof("ImageStream %s/%s has kernel tag %s in status",
				namespace, imagestreamName, tag)

			return nil
		}
	}

	return fmt.Errorf("kernel tag %s not found in ImageStream %s/%s", tag, namespace, imagestreamName)
}

// ImageExists verifies that an image exists with a tag matching the cluster's kernel version.
func ImageExists(apiClient *clients.Settings, baseImage string,
	nodeSelector map[string]string) (string, error) {
	kernelVersion, err := get.KernelFullVersion(apiClient, nodeSelector)
	if err != nil {
		return "", fmt.Errorf("failed to get kernel version: %w", err)
	}

	klog.V(kmmparams.KmmLogLevel).Infof("Cluster kernel version: %s", kernelVersion)

	// Find any ready helper pod (don't care which node)
	helperPods, err := pod.List(apiClient, kmmparams.KmmOperatorNamespace, metav1.ListOptions{
		LabelSelector: kmmparams.KmmTestHelperLabelName,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list helper pods: %w", err)
	}

	var helperPod *pod.Builder

	for _, helperPodCandidate := range helperPods {
		if helperPodCandidate.Object.Status.Phase != corev1.PodRunning {
			continue
		}

		for _, cs := range helperPodCandidate.Object.Status.ContainerStatuses {
			if cs.Name == "test" && cs.Ready {
				helperPod = helperPodCandidate

				break
			}
		}

		if helperPod != nil {
			break
		}
	}

	if helperPod == nil {
		return "", fmt.Errorf("no ready helper pod found")
	}

	parts := strings.SplitN(baseImage, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid image format: %s", baseImage)
	}

	registry := parts[0]
	repository := parts[1]

	var apiURL string
	if registry == "quay.io" {
		apiURL = fmt.Sprintf("https://quay.io/api/v1/repository/%s/tag/?limit=100&filter_tag_name=like:%s",
			repository, kernelVersion)
	} else {
		apiURL = fmt.Sprintf("https://%s/v2/%s/tags/list", registry, repository)
	}

	curlCmd := []string{"curl", "-s", "-f", apiURL}
	klog.V(kmmparams.KmmLogLevel).Infof("Executing: %v", curlCmd)

	output, err := helperPod.ExecCommand(curlCmd, "test")
	if err != nil {
		errOutput := output.String()
		klog.V(kmmparams.KmmLogLevel).Infof("curl failed for %s: %v, output: %s",
			apiURL, err, errOutput)

		return "", fmt.Errorf("failed to list tags for %s: %w (output: %s)", baseImage, err, errOutput)
	}

	tagsOutput := output.String()
	klog.V(kmmparams.KmmLogLevel).Infof("Available tags for %s: %s", baseImage, tagsOutput)

	if !strings.Contains(tagsOutput, kernelVersion) {
		return "", fmt.Errorf("no image tag found for kernel version %s in %s", kernelVersion, baseImage)
	}

	klog.V(kmmparams.KmmLogLevel).Infof("Found image tag matching kernel version %s in %s", kernelVersion, baseImage)

	return fmt.Sprintf("%s (kernel: %s)", baseImage, kernelVersion), nil
}

func runCommandOnTestPods(apiClient *clients.Settings,
	command []string, message string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			pods, err := pod.List(apiClient, kmmparams.KmmOperatorNamespace, metav1.ListOptions{
				FieldSelector: "status.phase=Running",
				LabelSelector: kmmparams.KmmTestHelperLabelName,
			})
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("deployment list error: %s\n", err)

				return false, err
			}

			// using a map so that both ModuleLoaded and Dmesg calls don't interfere with the counter
			iter := 0

			for _, iterPod := range pods {
				klog.V(kmmparams.KmmLogLevel).Infof("\n\nPodName: %v\nCommand: %v\nExpect: %v\n\n",
					iterPod.Object.Name, command, message)

				buff, err := iterPod.ExecCommand(command, "test")
				if err != nil {
					return false, err
				}

				contents := buff.String()
				klog.V(kmmparams.KmmLogLevel).Infof("%s contents: \n \t%v\n", command, contents)

				if strings.Contains(contents, message) {
					klog.V(kmmparams.KmmLogLevel).Infof(
						"command '%s' contains '%s' in pod %s\n", command, message, iterPod.Object.Name)

					iter++

					if iter == len(pods) {
						return true, nil
					}
				}
			}

			return false, err
		})
}

func runCommandOnTestPodsOnNode(apiClient *clients.Settings,
	command []string, message string, timeout time.Duration, nodeName string) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			pods, err := pod.List(apiClient, kmmparams.KmmOperatorNamespace, metav1.ListOptions{
				FieldSelector: "status.phase=Running",
				LabelSelector: kmmparams.KmmTestHelperLabelName,
			})
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("deployment list error: %s\n", err)

				return false, err
			}

			for _, iterPod := range pods {
				// Skip pods not on the target node
				if iterPod.Object.Spec.NodeName != nodeName {
					continue
				}

				klog.V(kmmparams.KmmLogLevel).Infof("\n\nPodName: %v\nNode: %v\nCommand: %v\nExpect: %v\n\n",
					iterPod.Object.Name, nodeName, command, message)

				buff, err := iterPod.ExecCommand(command, "test")
				if err != nil {
					return false, err
				}

				contents := buff.String()
				klog.V(kmmparams.KmmLogLevel).Infof("%s contents: \n \t%v\n", command, contents)

				if strings.Contains(contents, message) {
					klog.V(kmmparams.KmmLogLevel).Infof(
						"command '%s' contains '%s' in pod %s on node %s\n",
						command, message, iterPod.Object.Name, nodeName)

					return true, nil
				}
			}

			return false, nil
		})
}
