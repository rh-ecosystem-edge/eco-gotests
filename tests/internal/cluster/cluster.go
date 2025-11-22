package cluster

import (
	"regexp"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
)

// PullTestImageOnNodes pulls given image on range of relevant nodes based on nodeSelector.
func PullTestImageOnNodes(apiClient *clients.Settings, nodeSelector, image string, pullTimeout int) error {
	klog.V(90).Infof("Pulling image %s to nodes with the following label %v", image, nodeSelector)

	nodesList, err := nodes.List(
		apiClient,
		metav1.ListOptions{LabelSelector: labels.Set(map[string]string{nodeSelector: ""}).String()},
	)
	if err != nil {
		return err
	}

	for _, node := range nodesList {
		klog.V(90).Infof("Pulling image %s to node %s", image, node.Object.Name)
		podBuilder := pod.NewBuilder(
			apiClient, fmt.Sprintf("pullpod-%s", node.Object.Name), "default", image)

		err := podBuilder.PullImage(time.Duration(pullTimeout)*time.Second, []string{
			"/bin/sh", "-c", "echo image Pulled && exit 0"})
		if err != nil {
			return err
		}
	}

	return nil
}

// ExecCmd runc cmd on all nodes that match nodeSelector.
func ExecCmd(apiClient *clients.Settings, nodeSelector string, shellCmd string) error {
	klog.V(90).Infof("Executing cmd: %v on nodes based on label: %v using mcp pods", shellCmd, nodeSelector)

	nodeList, err := nodes.List(
		apiClient,
		metav1.ListOptions{LabelSelector: labels.Set(map[string]string{nodeSelector: ""}).String()},
	)
	if err != nil {
		return err
	}

	for _, node := range nodeList {
		listOptions := metav1.ListOptions{
			FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": node.Definition.Name}).String(),
			LabelSelector: labels.SelectorFromSet(labels.Set{"k8s-app": GeneralConfig.MCOConfigDaemonName}).String(),
		}

		mcPodList, err := pod.List(apiClient, GeneralConfig.MCONamespace, listOptions)
		if err != nil {
			return err
		}

		for _, mcPod := range mcPodList {
			err = mcPod.WaitUntilRunning(300 * time.Second)
			if err != nil {
				return err
			}

			cmdToExec := []string{"sh", "-c", fmt.Sprintf("nsenter --mount=/proc/1/ns/mnt -- sh -c '%s'", shellCmd)}

			klog.V(90).Infof("Exec cmd %v on pod %s", cmdToExec, mcPod.Definition.Name)

			buf, err := mcPod.ExecCommand(cmdToExec)
			if err != nil {
				return fmt.Errorf("%w\n%s", err, buf.String())
			}
		}
	}

	return nil
}

// ExecCmdWithStdout runs cmd on all selected nodes and returns their stdout.
//
//nolint:funlen
func ExecCmdWithStdout(
	apiClient *clients.Settings, shellCmd string, options ...metav1.ListOptions) (map[string]string, error) {
	klog.V(90).Infof("Executing command '%s' with stdout and options ('%v')", shellCmd, options)

	if GeneralConfig.MCOConfigDaemonName == "" {
		return nil, fmt.Errorf("error: mco config daemon pod name cannot be empty")
	}

	if GeneralConfig.MCONamespace == "" {
		return nil, fmt.Errorf("error: mco namespace cannot be empty")
	}

	logMessage := fmt.Sprintf("Executing cmd: %v on nodes", shellCmd)

	passedOptions := metav1.ListOptions{}

	if len(options) > 1 {
		klog.V(90).Infof("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(90).Info(logMessage)

	nodeList, err := nodes.List(
		apiClient,
		passedOptions,
	)
	if err != nil {
		return nil, err
	}

	klog.V(90).Infof("Found %d nodes matching selector", len(nodeList))

	outputMap := make(map[string]string)

	for _, node := range nodeList {
		listOptions := metav1.ListOptions{
			FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": node.Definition.Name}).String(),
			LabelSelector: labels.SelectorFromSet(labels.Set{"k8s-app": GeneralConfig.MCOConfigDaemonName}).String(),
		}

		mcPodList, err := pod.List(apiClient, GeneralConfig.MCONamespace, listOptions)
		if err != nil {
			return nil, err
		}

		for _, mcPod := range mcPodList {
			err = mcPod.WaitUntilRunning(300 * time.Second)
			if err != nil {
				return nil, err
			}

			hostnameCmd := []string{"sh", "-c", "nsenter --mount=/proc/1/ns/mnt -- sh -c 'printf $(hostname)'"}

			hostnameBuf, err := mcPod.ExecCommand(hostnameCmd)
			if err != nil {
				return nil, fmt.Errorf("failed gathering node hostname: %w", err)
			}

			cmdToExec := []string{"sh", "-c", fmt.Sprintf("nsenter --mount=/proc/1/ns/mnt -- sh -c '%s'", shellCmd)}

			klog.V(90).Infof("Exec cmd %v on pod %s", cmdToExec, mcPod.Definition.Name)

			commandBuf, err := mcPod.ExecCommand(cmdToExec)
			if err != nil {
				return nil, fmt.Errorf("failed executing command '%s' on node %s: %w", shellCmd, hostnameBuf.String(), err)
			}

			hostname := regexp.MustCompile(`\r`).ReplaceAllString(hostnameBuf.String(), "")
			output := regexp.MustCompile(`\r`).ReplaceAllString(commandBuf.String(), "")

			outputMap[hostname] = output
		}
	}

	return outputMap, nil
}

// ExecCmdWithRetries executes a command on the provided client on each node matching nodeSelector,
// retrying on internal errors
// "retries" times with a "interval" duration between retries, and ignores the stdout.
func ExecCmdWithRetries(client *clients.Settings, retries uint,
	interval time.Duration, nodeSelector, command string) error {
	klog.V(90).Infof("Executing command '%s' with %d retries and interval %v. Node Selector: %v",
		command, retries, interval, nodeSelector)

	retry := 1

	return wait.PollUntilContextTimeout(
		context.TODO(), interval, time.Duration(retries-1)*interval, true, func(ctx context.Context) (bool, error) {
			err := ExecCmd(client, nodeSelector, command)
			if isErrorExecuting(err) {
				klog.V(90).Infof("Error during command execution, retry %d (%d max): %v", retry, retries, err)

				retry++

				return false, nil
			}

			if err != nil {
				return false, err
			}

			return true, nil
		})
}

// ExecCmdWithStdoutWithRetries executes a command on the provided client,
// retrying on internal errors "retries" times with a "interval"
// duration between retries, and returns the stdout for each node.
func ExecCmdWithStdoutWithRetries(
	client *clients.Settings, retries uint, interval time.Duration,
	command string, options ...metav1.ListOptions) (map[string]string, error) {
	klog.V(90).Infof("Executing command with stdout '%s' with %d retries and interval %v. Options: %v",
		command, retries, interval, options)

	var (
		outputs map[string]string
		err     error
		retry   = 1
	)

	err = wait.PollUntilContextTimeout(
		context.TODO(), interval, time.Duration(retries-1)*interval, true, func(ctx context.Context) (bool, error) {
			outputs, err = ExecCmdWithStdout(client, command, options...)
			if isErrorExecuting(err) {
				klog.V(90).Infof("Error during command execution, retry %d (%d max): %v", retry, retries, err)

				retry++

				return false, nil
			}

			if err != nil {
				return false, err
			}

			return true, nil
		})

	return outputs, err
}

// ExecCommandOnSNOWithRetries executes a command on the provided single node client,
// retrying on internal errors "retries" times
// waits with a "interval" duration between retries, and returns the stdout.
func ExecCommandOnSNOWithRetries(client *clients.Settings, retries uint,
	interval time.Duration, command string) (string, error) {
	klog.V(90).Infof("Executing command on SNO '%s' with %d retries and interval %v", command, retries, interval)

	outputs, err := ExecCmdWithStdoutWithRetries(client, retries, interval, command)
	if err != nil {
		return "", err
	}

	if len(outputs) != 1 {
		return "", fmt.Errorf("expected results from one node, found %d nodes", len(outputs))
	}

	for _, output := range outputs {
		return output, nil
	}

	return "", fmt.Errorf("found unreachable code in ExecCommandOnSNO")
}

// WaitForRecover waits up to timeout for all pods in namespaces on a provided node to recover.
func WaitForRecover(client *clients.Settings, namespaces []string, timeout time.Duration) error {
	klog.V(90).Infof("Wait for cluster to recover for namespaces: %v timeout: %v", namespaces, timeout)

	err := waitForReachable(client, timeout)
	if err != nil {
		return err
	}

	err = pod.WaitForPodsInNamespacesHealthy(client, namespaces, timeout)
	if err != nil {
		return err
	}

	return nil
}

// SoftRebootSNO executes systemctl reboot on a node.
func SoftRebootSNO(apiClient *clients.Settings, retries uint, interval time.Duration) error {
	klog.V(90).Infof("Rebooting SNO node with %d retries interval %v", retries, interval)

	cmdToExec := "sudo systemctl reboot"

	_, err := ExecCommandOnSNOWithRetries(apiClient, retries, interval, cmdToExec)

	return err
}

// WaitForUnreachable waits up to timeout for the cluster to become unavailable
// by attempting to list nodes in the cluster.
func WaitForUnreachable(client *clients.Settings, timeout time.Duration) error {
	klog.V(90).Infof("Wait for cluster unreachable with timeout: %v", timeout)

	return wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := nodes.List(client, metav1.ListOptions{TimeoutSeconds: ptr.To[int64](3)})
			if err != nil {
				return true, nil
			}

			return false, nil
		})
}

// waitForReachable waits up to timeout for the cluster to become available by attempting to list nodes in the
// cluster.
func waitForReachable(client *clients.Settings, timeout time.Duration) error {
	klog.V(90).Infof("Wait for cluster reachable with timeout: %v", timeout)

	return wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := nodes.List(client, metav1.ListOptions{TimeoutSeconds: ptr.To[int64](3)})
			if err != nil {
				return false, nil
			}

			return true, nil
		})
}

// isErrorExecuting matches errors that contain the message "error executing command in container".
func isErrorExecuting(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(err.Error(), "error executing command in container") ||
		strings.Contains(err.Error(), "container not found")
}
