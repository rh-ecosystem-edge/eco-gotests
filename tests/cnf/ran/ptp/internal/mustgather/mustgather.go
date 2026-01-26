package mustgather

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2/types"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

const (
	// mustGatherAnnotation is the annotation on CSVs that contains the must-gather image.
	mustGatherAnnotation = "operators.openshift.io/must-gather-image"
	// mustGatherNamespacePrefix is the prefix for the temporary must-gather namespace.
	mustGatherNamespacePrefix = "ptp-must-gather-"
	// mustGatherServiceAccountName is the name of the service account used by the must-gather pod.
	mustGatherServiceAccountName = "must-gather-admin"
	// mustGatherCRBPrefix is the prefix for the cluster role binding for the must-gather pod.
	mustGatherCRBPrefix = "ptp-must-gather-admin-"
	// gatherContainerName is the name of the container that runs the gather script.
	gatherContainerName = "gather"
	// copyContainerName is the name of the container used for copying files.
	copyContainerName = "copy"
	// mustGatherOutputPath is the path where must-gather output is stored in the pod.
	mustGatherOutputPath = "/must-gather"
	// mustGatherCommand is the default command to run in the must-gather container.
	mustGatherCommand = "/usr/bin/gather"
	// mustGatherPodTimeout is the timeout for waiting for the must-gather pod to complete.
	mustGatherPodTimeout = 10 * time.Minute
	// mustGatherPodName is the name of the must-gather pod.
	mustGatherPodName = "ptp-must-gather"
)

var majorMinorVersionRegex = regexp.MustCompile(`(\d+)\.(\d+)`)

// MustGatherIfFailed runs the PTP must-gather if the test has failed, saving the output to the report directory.
func MustGatherIfFailed(
	report types.SpecReport,
	testSuite string,
	client *clients.Settings,
) {
	if !report.State.Is(types.SpecStateFailureStates) {
		return
	}

	if client == nil {
		klog.V(ranparam.LogLevel).Info("Client is nil, skipping PTP must-gather")

		return
	}

	dumpDir := RANConfig.GetDumpFailedTestReportLocation(testSuite)
	if dumpDir == "" {
		klog.V(ranparam.LogLevel).Info("No dump directory configured, skipping PTP must-gather")

		return
	}

	image, err := getMustGatherImage(client)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to get must-gather image: %v, skipping PTP must-gather", err)

		return
	}

	klog.V(ranparam.LogLevel).Infof("Running PTP must-gather with image: %s", image)

	// Create the test case specific directory for the must-gather output
	tcReportFolderName := strings.ReplaceAll(report.FullText(), " ", "_")
	mustGatherDir := filepath.Join(dumpDir, tcReportFolderName)

	if err := os.MkdirAll(mustGatherDir, 0755); err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to create must-gather directory: %v", err)

		return
	}

	tarballPath := filepath.Join(mustGatherDir, "ptp-must-gather.tar")

	if err := runMustGather(client, image, tarballPath); err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to run PTP must-gather: %v", err)

		return
	}

	klog.V(ranparam.LogLevel).Infof("PTP must-gather completed successfully, output saved to: %s", tarballPath)
}

// getMustGatherImage retrieves the must-gather image, trying in order:
//
//  1. The image set in the RANConfig
//  2. The PTP operator CSV annotation
//  3. The fallback image from registry.redhat.io corresponding to the current OCP version
func getMustGatherImage(client *clients.Settings) (string, error) {
	if RANConfig.PtpMustGatherImage != "" {
		return RANConfig.PtpMustGatherImage, nil
	}

	csvList, err := olm.ListClusterServiceVersionWithNamePattern(
		client, "ptp-operator", ranparam.PtpOperatorNamespace)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to list CSVs: %v, using fallback image", err)

		return getPtpMustGatherImageForVersion(RANConfig.Spoke1OCPVersion)
	}

	for _, csv := range csvList {
		if csv.Object == nil {
			continue
		}

		annotations := csv.Object.GetAnnotations()
		if image, ok := annotations[mustGatherAnnotation]; ok && image != "" {
			klog.V(ranparam.LogLevel).Infof("Found must-gather image in CSV annotation: %s", image)

			return image, nil
		}
	}

	klog.V(ranparam.LogLevel).Info("No must-gather annotation found on PTP operator CSV, using fallback image")

	return getPtpMustGatherImageForVersion(RANConfig.Spoke1OCPVersion)
}

// getPtpMustGatherImageForVersion retrieves the must-gather image for a given OCP version. For versions at least 4.16,
// the image is from registry.redhat.io/openshift4/ptp-must-gather-rhel9. For older versions, the image is from
// registry.redhat.io/openshift4/ptp-must-gather-rhel8. The tag is based on the major and minor version of the OCP
// version.
func getPtpMustGatherImageForVersion(ocpVersion string) (string, error) {
	matches := majorMinorVersionRegex.FindStringSubmatch(ocpVersion)
	if len(matches) < 3 {
		return "", fmt.Errorf("invalid version format: %q", ocpVersion)
	}

	major, minor := matches[1], matches[2]

	atLeast416, err := version.IsVersionStringInRange(matches[0], "4.16", "")
	if err != nil {
		return "", fmt.Errorf("failed to check if version is at least 4.16: %w", err)
	}

	if atLeast416 {
		return fmt.Sprintf("registry.redhat.io/openshift4/ptp-must-gather-rhel9:v%s.%s", major, minor), nil
	}

	return fmt.Sprintf("registry.redhat.io/openshift4/ptp-must-gather-rhel8:v%s.%s", major, minor), nil
}

// runMustGather creates the necessary resources, runs the must-gather pod, and downloads the output.
func runMustGather(client *clients.Settings, image, tarballPath string) error {
	runID := time.Now().UnixNano()
	nsName := fmt.Sprintf("%s%d", mustGatherNamespacePrefix, runID)
	crbName := fmt.Sprintf("%s%d", mustGatherCRBPrefix, runID)

	_, err := createMustGatherNamespace(client, nsName)
	if err != nil {
		return fmt.Errorf("failed to create must-gather namespace: %w", err)
	}

	// Ensure cleanup happens regardless of success or failure.
	defer func() {
		cleanupErr := cleanupMustGatherResources(client, nsName, crbName)
		if cleanupErr != nil {
			klog.V(ranparam.LogLevel).Infof("Failed to cleanup must-gather resources: %v", cleanupErr)
		}
	}()

	_, err = serviceaccount.NewBuilder(client, mustGatherServiceAccountName, nsName).Create()
	if err != nil {
		return fmt.Errorf("failed to create service account: %w", err)
	}

	saSubject := rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      mustGatherServiceAccountName,
		Namespace: nsName,
	}

	crbBuilder := rbac.NewClusterRoleBindingBuilder(client, crbName, "cluster-admin", saSubject)

	_, err = crbBuilder.Create()
	if err != nil {
		return fmt.Errorf("failed to create cluster role binding: %w", err)
	}

	podBuilder, err := createMustGatherPod(client, nsName, image)
	if err != nil {
		return fmt.Errorf("failed to create must-gather pod: %w", err)
	}

	err = waitForGatherComplete(podBuilder)
	if err != nil {
		return fmt.Errorf("failed waiting for must-gather to complete: %w", err)
	}

	err = downloadMustGatherOutput(podBuilder, tarballPath)
	if err != nil {
		return fmt.Errorf("failed to download must-gather output: %w", err)
	}

	return nil
}

// createMustGatherNamespace creates a privileged namespace for the must-gather pod.
func createMustGatherNamespace(client *clients.Settings, name string) (*namespace.Builder, error) {
	nsBuilder := namespace.NewBuilder(client, name).
		WithLabel("openshift.io/run-level", "0").
		WithLabel("pod-security.kubernetes.io/enforce", "privileged").
		WithLabel("pod-security.kubernetes.io/audit", "privileged").
		WithLabel("pod-security.kubernetes.io/warn", "privileged").
		WithLabel("security.openshift.io/scc.podSecurityLabelSync", "false")

	return nsBuilder.Create()
}

// createMustGatherPod creates the must-gather pod with gather and copy containers. The specifics are taken from the pod
// that the `oc adm must-gather` command creates.
//
// See https://github.com/openshift/oc/blob/main/pkg/cli/admin/mustgather/mustgather.go.
func createMustGatherPod(client *clients.Settings, nsName, image string) (*pod.Builder, error) {
	// The gather container runs the must-gather command. It will complete once the must-gather command has
	// finished.
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

	// Once the must-gather container has finished, the copy container allows the pod to continue running for us to
	// copy the must-gather output. It runs an infinite sleep command so the pod stays running.
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

	podBuilder := pod.NewBuilder(client, mustGatherPodName, nsName, image)

	podBuilder.Definition.Spec.Containers = []corev1.Container{*gatherContainer, *copyContainer}
	podBuilder.Definition.Spec.ServiceAccountName = mustGatherServiceAccountName
	podBuilder.Definition.Spec.RestartPolicy = corev1.RestartPolicyNever
	podBuilder.Definition.Spec.PriorityClassName = "system-cluster-critical"
	podBuilder.Definition.Spec.TerminationGracePeriodSeconds = ptr.To[int64](0)
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

// waitForGatherComplete waits for the gather container to complete.
func waitForGatherComplete(podBuilder *pod.Builder) error {
	klog.V(ranparam.LogLevel).Info("Waiting for must-gather to complete...")

	err := podBuilder.WaitUntilRunning(mustGatherPodTimeout)
	if err != nil {
		return fmt.Errorf("pod failed to start running: %w", err)
	}

	// Poll until the gather container has terminated. We cannot use the pod condition since the copy container will
	// stay running.
	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, mustGatherPodTimeout, true,
		func(ctx context.Context) (bool, error) {
			if !podBuilder.Exists() {
				return false, fmt.Errorf("pod no longer exists")
			}

			for _, containerStatus := range podBuilder.Object.Status.ContainerStatuses {
				if containerStatus.Name == gatherContainerName {
					if containerStatus.State.Terminated != nil {
						if containerStatus.State.Terminated.ExitCode != 0 {
							return true, fmt.Errorf(
								"gather container terminated with non-zero exit code %d: %s",
								containerStatus.State.Terminated.ExitCode,
								containerStatus.State.Terminated.Message,
							)
						}

						return true, nil
					}
				}
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

	err = os.WriteFile(tarballPath, buffer.Bytes(), 0644)
	if err != nil {
		return fmt.Errorf("failed to write tarball: %w", err)
	}

	return nil
}

// cleanupMustGatherResources deletes the must-gather namespace and cluster role binding.
func cleanupMustGatherResources(client *clients.Settings, nsName, crbName string) error {
	klog.V(ranparam.LogLevel).Infof("Cleaning up must-gather resources in namespace %s", nsName)

	crbBuilder, err := rbac.PullClusterRoleBinding(client, crbName)
	if err == nil {
		err = crbBuilder.Delete()
		if err != nil {
			return fmt.Errorf("failed to delete cluster role binding: %w", err)
		}
	}

	nsBuilder, err := namespace.Pull(client, nsName)
	if err == nil {
		err = nsBuilder.DeleteAndWait(2 * time.Minute)
		if err != nil {
			return fmt.Errorf("failed to delete namespace: %w", err)
		}
	}

	return nil
}
