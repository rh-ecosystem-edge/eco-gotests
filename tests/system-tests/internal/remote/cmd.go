package remote

import (
	"context"
	"fmt"
	"time"

	ssh "github.com/povsister/scp"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsinittools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// ExecuteOnNodeWithDebugPod executes a command on a node.
func ExecuteOnNodeWithDebugPod(cmdToExec []string, nodeName string) (string, error) {
	listOptions := metav1.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}).String(),
		LabelSelector: labels.SelectorFromSet(labels.Set{"k8s-app": SystemTestsTestConfig.MCOConfigDaemonName}).String(),
	}

	mcPodList, err := pod.List(APIClient, SystemTestsTestConfig.MCONamespace, listOptions)
	if err != nil {
		return "", err
	}

	klog.V(90).Infof("Exec cmd %v on pod %s", cmdToExec, mcPodList[0].Definition.Name)

	buf, err := mcPodList[0].ExecCommand(cmdToExec)
	if err != nil {
		return "", fmt.Errorf("%w\n%s", err, buf.String())
	}

	return buf.String(), err
}

// ExecuteOnNodeWithDebugPodWithTimeout executes a command on a node with a timeout.
func ExecuteOnNodeWithDebugPodWithTimeout(cmdToExec []string, nodeName string, timeout time.Duration) (string, error) {
	listOptions := metav1.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}).String(),
		LabelSelector: labels.SelectorFromSet(labels.Set{"k8s-app": SystemTestsTestConfig.MCOConfigDaemonName}).String(),
	}

	mcPodList, err := pod.List(APIClient, SystemTestsTestConfig.MCONamespace, listOptions)
	if err != nil {
		return "", err
	}

	klog.V(90).Infof("Exec cmd %v on pod %s", cmdToExec, mcPodList[0].Definition.Name)

	buf, err := mcPodList[0].ExecCommandWithTimeout(cmdToExec, timeout)
	if err != nil {
		klog.V(90).Infof("Failed to execute command on node %s: %v",
			nodeName, err)

		return "", err
	}

	klog.V(90).Infof("Command executed successfully on node %s", nodeName)

	return buf.String(), nil
}

// ExecuteOnNodeWithPrivilegedDebugPod executes command on the specific node using privileged debug pod.
func ExecuteOnNodeWithPrivilegedDebugPod(apiClient *clients.Settings,
	nodeName, imageName string, cmd []string) (string, error) {
	const (
		debugPodLabel = "system-test-privileged-debug"
		debugPodName  = "st-privileged-debug"
	)

	debugPod := pod.NewBuilder(
		apiClient,
		debugPodName,
		SystemTestsTestConfig.MCONamespace,
		imageName)

	klog.V(90).Infof("Check if %q pod exists", debugPodName)

	podSelector := fmt.Sprintf("%s=%s", debugPodLabel, nodeName)

	err := wait.PollUntilContextTimeout(context.TODO(), time.Second, time.Minute, true,
		func(context.Context) (bool, error) {
			oldPods, err := pod.List(apiClient, SystemTestsTestConfig.MCONamespace,
				metav1.ListOptions{LabelSelector: podSelector})
			if err != nil {
				klog.V(90).Infof("Error listing pods: %v", err)

				return false, nil
			}

			for _, _pod := range oldPods {
				klog.V(90).Infof("Deleting pod %q in %q namespace",
					_pod.Definition.Name, _pod.Definition.Namespace)

				_, delErr := _pod.DeleteAndWait(15 * time.Second)
				if delErr != nil {
					klog.V(90).Infof("Failed to delete pod %q in %q namespace: %v",
						_pod.Definition.Name, _pod.Definition.Namespace, delErr)

					return false, nil
				}
			}

			return true, nil
		})
	if err != nil {
		klog.V(90).Infof("Failed to assert if previous %q pod exists", debugPodName)

		return "", fmt.Errorf("failed to assert if previous %q pod exists", debugPodName)
	}

	debugPod, err = debugPod.WithPrivilegedFlag().
		WithHostNetwork().
		WithLabel(debugPodLabel, nodeName).
		WithNodeSelector(map[string]string{"kubernetes.io/hostname": nodeName}).
		CreateAndWaitUntilRunning(1 * time.Minute)
	if err != nil {
		return "", err
	}

	buf, err := debugPod.ExecCommand(cmd)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// ExecCmdOnHost executes specific cmd on remote host.
func ExecCmdOnHost(remoteHostname, remoteHostUsername, remoteHostPass, cmd string) (string, error) {
	if remoteHostname == "" {
		klog.V(100).Info("The remoteHostname is empty")

		return "", fmt.Errorf("the remoteHostname could not be empty")
	}

	if remoteHostUsername == "" {
		klog.V(100).Info("The remoteHostUsername is empty")

		return "", fmt.Errorf("the remoteHostUsername could not be empty")
	}

	if remoteHostPass == "" {
		klog.V(100).Info("The remoteHostPass is empty")

		return "", fmt.Errorf("the remoteHostPass could not be empty")
	}

	klog.V(100).Info("Build a SSH config from username/password")

	sshConf := ssh.NewSSHConfigFromPassword(remoteHostUsername, remoteHostPass)

	klog.V(100).Infof("Dial SSH to the host %s", remoteHostname)

	scpClient, err := ssh.NewClient(remoteHostname, sshConf, &ssh.ClientOption{})
	if err != nil {
		klog.V(100).Infof("Failed to build new ssh client due to: %v", err)

		return "", fmt.Errorf("failed to build new ssh client due to: %w", err)
	}

	ss, _ := scpClient.NewSession()
	defer ss.Close()

	out, err := ss.CombinedOutput(cmd)
	if err != nil {
		klog.V(100).Infof("Failed to run cmd %s on the host %s due to: %v",
			cmd, remoteHostname, err)

		return "", fmt.Errorf("failed to run cmd %s on the host %s due to: %w", cmd, remoteHostname, err)
	}

	return string(out), nil
}
