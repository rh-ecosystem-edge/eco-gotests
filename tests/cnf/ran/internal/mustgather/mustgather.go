// Package mustgather collects component-specific diagnostic data after failed RAN tests.
package mustgather

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2/types"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	mustGatherServiceAccountName = "must-gather-admin"
	gatherContainerName          = "gather"
	copyContainerName            = "copy"
	mustGatherOutputPath         = "/must-gather"
	mustGatherCommand            = "/usr/bin/gather"
	mustGatherPodTimeout         = 10 * time.Minute
)

// ImageProvider resolves the image for a must-gather run.
type ImageProvider func(client *clients.Settings) (string, error)

// StaticImage returns an image provider that always uses image.
func StaticImage(image string) ImageProvider {
	return func(_ *clients.Settings) (string, error) {
		return image, nil
	}
}

// CollectIfFailed collects a must-gather after a failed test and saves it in the failed-test report directory.
func CollectIfFailed(
	report types.SpecReport,
	testSuite string,
	client *clients.Settings,
	name string,
	imageProvider ImageProvider,
) {
	if !report.State.Is(types.SpecStateFailureStates) {
		return
	}

	if client == nil {
		klog.V(ranparam.LogLevel).Infof("Client is nil, skipping %s must-gather", name)

		return
	}

	dumpDir := RANConfig.GetDumpFailedTestReportLocation(testSuite)
	if dumpDir == "" {
		klog.V(ranparam.LogLevel).Infof("No dump directory configured, skipping %s must-gather", name)

		return
	}

	if imageProvider == nil {
		klog.V(ranparam.LogLevel).Infof("No image provider configured, skipping %s must-gather", name)

		return
	}

	image, err := imageProvider(client)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof(
			"Failed to get %s must-gather image: %v, skipping must-gather", name, err)

		return
	}

	if image == "" {
		klog.V(ranparam.LogLevel).Infof("Must-gather image is empty, skipping %s must-gather", name)

		return
	}

	klog.V(ranparam.LogLevel).Infof("Running %s must-gather with image: %s", name, image)

	testCaseReportFolderName := strings.ReplaceAll(report.FullText(), " ", "_")
	mustGatherDir := filepath.Join(dumpDir, testCaseReportFolderName)

	if err := os.MkdirAll(mustGatherDir, 0o755); err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to create must-gather directory: %v", err)

		return
	}

	tarballPath := filepath.Join(mustGatherDir, fmt.Sprintf("%s-must-gather.tar", name))

	if err := runMustGather(client, image, tarballPath, name); err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to run %s must-gather: %v", name, err)

		return
	}

	klog.V(ranparam.LogLevel).Infof(
		"%s must-gather completed successfully, output saved to: %s", name, tarballPath)
}

// runMustGather creates the required resources, runs the must-gather pod, and downloads its output.
func runMustGather(client *clients.Settings, image, tarballPath, name string) error {
	runID := time.Now().UnixNano()
	namespaceName, clusterRoleBindingName := resourceNames(name, runID)

	_, err := createMustGatherNamespace(client, namespaceName)
	if err != nil {
		return fmt.Errorf("failed to create must-gather namespace: %w", err)
	}

	defer func() {
		cleanupErr := cleanupMustGatherResources(client, namespaceName, clusterRoleBindingName)
		if cleanupErr != nil {
			klog.V(ranparam.LogLevel).Infof("Failed to cleanup must-gather resources: %v", cleanupErr)
		}
	}()

	_, err = serviceaccount.NewBuilder(client, mustGatherServiceAccountName, namespaceName).Create()
	if err != nil {
		return fmt.Errorf("failed to create service account: %w", err)
	}

	serviceAccountSubject := rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      mustGatherServiceAccountName,
		Namespace: namespaceName,
	}

	clusterRoleBindingBuilder := rbac.NewClusterRoleBindingBuilder(
		client, clusterRoleBindingName, "cluster-admin", serviceAccountSubject)

	_, err = clusterRoleBindingBuilder.Create()
	if err != nil {
		return fmt.Errorf("failed to create cluster role binding: %w", err)
	}

	podBuilder, err := createMustGatherPod(client, namespaceName, image, name)
	if err != nil {
		return fmt.Errorf("failed to create must-gather pod: %w", err)
	}

	if err := waitForGatherComplete(podBuilder); err != nil {
		return fmt.Errorf("failed waiting for must-gather to complete: %w", err)
	}

	if err := downloadMustGatherOutput(podBuilder, tarballPath); err != nil {
		return fmt.Errorf("failed to download must-gather output: %w", err)
	}

	return nil
}

// resourceNames returns unique names for a must-gather namespace and cluster role binding.
func resourceNames(name string, runID int64) (string, string) {
	return fmt.Sprintf("%s-must-gather-%d", name, runID), fmt.Sprintf("%s-must-gather-admin-%d", name, runID)
}

// createMustGatherNamespace creates a privileged namespace for the must-gather pod.
func createMustGatherNamespace(client *clients.Settings, name string) (*namespace.Builder, error) {
	namespaceBuilder := namespace.NewBuilder(client, name).
		WithLabel("openshift.io/run-level", "0").
		WithLabel("pod-security.kubernetes.io/enforce", "privileged").
		WithLabel("pod-security.kubernetes.io/audit", "privileged").
		WithLabel("pod-security.kubernetes.io/warn", "privileged").
		WithLabel("security.openshift.io/scc.podSecurityLabelSync", "false")

	return namespaceBuilder.Create()
}

// createMustGatherPod creates gather and copy containers based on the pod created by oc adm must-gather.
// See https://github.com/openshift/oc/blob/main/pkg/cli/admin/mustgather/mustgather.go.
func createMustGatherPod(client *clients.Settings, namespaceName, image, name string) (*pod.Builder, error) {
	gatherContainer, err := pod.NewContainerBuilder(gatherContainerName, image, []string{
		"/bin/bash", "-c", mustGatherCommand,
	}).
		WithSecurityCapabilities([]string{"ALL"}, true).
		WithVolumeMount(corev1.VolumeMount{
			Name:      "must-gather-output",
			MountPath: mustGatherOutputPath,
		}).
		GetContainerCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to build gather container: %w", err)
	}

	copyContainer, err := pod.NewContainerBuilder(copyContainerName, image, []string{
		"/bin/bash", "-c", "trap : TERM INT; sleep infinity & wait",
	}).
		WithSecurityCapabilities([]string{"ALL"}, true).
		WithVolumeMount(corev1.VolumeMount{
			Name:      "must-gather-output",
			MountPath: mustGatherOutputPath,
		}).
		GetContainerCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to build copy container: %w", err)
	}

	podBuilder := pod.NewBuilder(client, fmt.Sprintf("%s-must-gather", name), namespaceName, image)

	podBuilder.Definition.Spec.Containers = []corev1.Container{*gatherContainer, *copyContainer}
	podBuilder.Definition.Spec.ServiceAccountName = mustGatherServiceAccountName
	podBuilder.Definition.Spec.RestartPolicy = corev1.RestartPolicyNever
	podBuilder.Definition.Spec.PriorityClassName = "system-cluster-critical"
	podBuilder.Definition.Spec.TerminationGracePeriodSeconds = new(int64(0))
	podBuilder.Definition.Spec.NodeSelector = map[string]string{corev1.LabelOSStable: "linux"}
	podBuilder.Definition.Spec.Tolerations = []corev1.Toleration{{Operator: corev1.TolerationOpExists}}
	podBuilder.Definition.Spec.Volumes = []corev1.Volume{
		{
			Name: "must-gather-output",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	podBuilder, err = podBuilder.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create pod: %w", err)
	}

	return podBuilder, nil
}

// waitForGatherComplete waits for the gather container to terminate successfully.
func waitForGatherComplete(podBuilder *pod.Builder) error {
	klog.V(ranparam.LogLevel).Info("Waiting for must-gather to complete...")

	if err := podBuilder.WaitUntilRunning(mustGatherPodTimeout); err != nil {
		return fmt.Errorf("pod failed to start running: %w", err)
	}

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, mustGatherPodTimeout, true,
		func(ctx context.Context) (bool, error) {
			if !podBuilder.Exists() {
				return false, fmt.Errorf("pod no longer exists")
			}

			for _, containerStatus := range podBuilder.Object.Status.ContainerStatuses {
				if containerStatus.Name != gatherContainerName || containerStatus.State.Terminated == nil {
					continue
				}

				if containerStatus.State.Terminated.ExitCode != 0 {
					return true, fmt.Errorf(
						"gather container terminated with non-zero exit code %d: %s",
						containerStatus.State.Terminated.ExitCode,
						containerStatus.State.Terminated.Message,
					)
				}

				return true, nil
			}

			return false, nil
		})
}

// downloadMustGatherOutput copies the must-gather output from the pod to the local filesystem.
func downloadMustGatherOutput(podBuilder *pod.Builder, tarballPath string) error {
	klog.V(ranparam.LogLevel).Infof("Downloading must-gather output to %s", tarballPath)

	buffer, err := podBuilder.Copy(mustGatherOutputPath, copyContainerName, true)
	if err != nil {
		return fmt.Errorf("failed to copy from pod: %w", err)
	}

	if err := os.WriteFile(tarballPath, buffer.Bytes(), 0o644); err != nil {
		return fmt.Errorf("failed to write tarball: %w", err)
	}

	return nil
}

// cleanupMustGatherResources deletes the must-gather namespace and cluster role binding.
func cleanupMustGatherResources(client *clients.Settings, namespaceName, clusterRoleBindingName string) error {
	klog.V(ranparam.LogLevel).Infof("Cleaning up must-gather resources in namespace %s", namespaceName)

	clusterRoleBindingBuilder, err := rbac.PullClusterRoleBinding(client, clusterRoleBindingName)
	if err == nil {
		if err := clusterRoleBindingBuilder.Delete(); err != nil {
			return fmt.Errorf("failed to delete cluster role binding: %w", err)
		}
	}

	namespaceBuilder, err := namespace.Pull(client, namespaceName)
	if err == nil {
		if err := namespaceBuilder.DeleteAndWait(2 * time.Minute); err != nil {
			return fmt.Errorf("failed to delete namespace: %w", err)
		}
	}

	return nil
}
