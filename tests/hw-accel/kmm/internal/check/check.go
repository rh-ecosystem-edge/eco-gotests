package check

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/imagestream"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
)

// NodeLabel checks if label is present on the node.
func NodeLabel(apiClient *clients.Settings, moduleName, nsname string, nodeSelector map[string]string) (bool, error) {
	nodeBuilder, err := nodes.List(apiClient, metav1.ListOptions{LabelSelector: labels.Set(nodeSelector).String()})

	if err != nil {
		glog.V(kmmparams.KmmLogLevel).Infof("could not discover %v nodes", nodeSelector)
	}

	foundLabels := 0
	label := fmt.Sprintf(kmmparams.ModuleNodeLabelTemplate, nsname, moduleName)

	for _, node := range nodeBuilder {
		_, ok := node.Object.Labels[label]
		if ok {
			glog.V(kmmparams.KmmLogLevel).Infof("Found label %v that contains %v on node %v",
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
		glog.V(kmmparams.KmmLogLevel).Infof("Could not create signing verification pod. Got error : %v", err)

		return err
	}

	glog.V(kmmparams.KmmLogLevel).Infof("\n\nPodName: %v\n\n", testPod.Object.Name)

	buff, err := testPod.ExecCommand(command, "test")

	if err != nil {
		return err
	}

	_, _ = testPod.Delete()

	contents := buff.String()
	glog.V(kmmparams.KmmLogLevel).Infof("%s contents: \n \t%v\n", command, contents)

	if strings.Contains(contents, message) {
		glog.V(kmmparams.KmmLogLevel).Infof("command '%s' output contains '%s'\n", command, message)

		return nil
	}

	err = fmt.Errorf("could not find signature in module")

	return err
}

// IntreeModuleLoaded makes sure the needed in-tree module is present on the nodes.
func IntreeModuleLoaded(apiClient *clients.Settings, module string, timeout time.Duration) error {
	return runCommandOnTestPods(apiClient, []string{"modprobe", module}, "", timeout)
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
				glog.V(kmmparams.KmmLogLevel).Infof("deployment list error: %s\n", err)

				return false, err
			}

			// using a map so that both ModuleLoaded and Dmesg calls don't interfere with the counter
			iter := 0

			for _, iterPod := range pods {
				glog.V(kmmparams.KmmLogLevel).Infof("\n\nPodName: %v\nCommand: %v\nExpect: %v\n\n",
					iterPod.Object.Name, command, message)

				buff, err := iterPod.ExecCommand(command, "test")

				if err != nil {
					return false, err
				}

				contents := buff.String()
				glog.V(kmmparams.KmmLogLevel).Infof("%s contents: \n \t%v\n", command, contents)

				if strings.Contains(contents, message) {
					glog.V(kmmparams.KmmLogLevel).Infof(
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

// ImageStreamExistsForModule validates that an imagestream exists with the correct kernel tag
// This only runs when using OpenShift internal registry.
func ImageStreamExistsForModule(apiClient *clients.Settings, namespace, moduleName, kmodName, tag string) error {
	// Check if module uses internal registry
	module, err := kmm.Pull(apiClient, moduleName, namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			glog.V(kmmparams.KmmLogLevel).Infof("Module %s not found in namespace %s, skipping imagestream check",
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
		glog.V(kmmparams.KmmLogLevel).Infof("Module %s not using internal registry, skipping imagestream check",
			moduleName)

		return nil
	}

	// ImageStream name is the same as kmod name
	imagestreamName := kmodName
	glog.V(kmmparams.KmmLogLevel).Infof("Checking ImageStream %s/%s for kernel tag %s",
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
			glog.V(kmmparams.KmmLogLevel).Infof("ImageStream %s/%s has kernel tag %s in status",
				namespace, imagestreamName, tag)

			return nil
		}
	}

	return fmt.Errorf("kernel tag %s not found in ImageStream %s/%s", tag, namespace, imagestreamName)
}
