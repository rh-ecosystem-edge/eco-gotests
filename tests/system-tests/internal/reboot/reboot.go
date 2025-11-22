package reboot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	systemtestsscc "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/scc"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsinittools"
	systemtestsparams "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// HardRebootNode executes ipmitool chassis power cycle on a node.
func HardRebootNode(nodeName string, nsName string) error {
	err := setupHardRebootDeployment(nodeName, nsName)
	if err != nil {
		return err
	}

	ipmiPods, err := prepareRebootResources(nodeName, nsName)
	if err != nil {
		return err
	}

	err = executeRebootCommand(ipmiPods)
	if err != nil {
		return err
	}

	err = waitForNodeRebootCycle(nodeName)
	if err != nil {
		return err
	}

	err = waitForAPIServerReady()
	if err != nil {
		return err
	}

	err = cleanupRebootDeployment(nsName)
	if err != nil {
		return err
	}

	return nil
}

// setupHardRebootDeployment sets up the privileged deployment for hard reboot.
func setupHardRebootDeployment(nodeName, nsName string) error {
	err := systemtestsscc.AddPrivilegedSCCtoDefaultSA(nsName)
	if err != nil {
		return err
	}

	rmiCmdToExec := []string{"chroot", "/rootfs", "/bin/sh", "-c", "podman rmi -f " + SystemTestsTestConfig.IpmiToolImage}
	klog.V(90).Infof("Cleaning up any of the existing ipmitool images. Exec cmd %v", rmiCmdToExec)
	_, err = remote.ExecuteOnNodeWithDebugPod(rmiCmdToExec, nodeName)

	return err
}

// prepareRebootResources creates the deployment and gets necessary resources.
func prepareRebootResources(nodeName, nsName string) ([]*pod.Builder, error) {
	deployContainer := pod.NewContainerBuilder(systemtestsparams.HardRebootDeploymentName,
		SystemTestsTestConfig.IpmiToolImage, []string{"sleep", "86400"})

	trueVar := true
	deployContainer = deployContainer.WithSecurityContext(&corev1.SecurityContext{Privileged: &trueVar})
	deployContainer = deployContainer.WithImagePullPolicy(corev1.PullAlways)

	deployContainerCfg, err := deployContainer.GetContainerCfg()
	if err != nil {
		return nil, err
	}

	createDeploy := deployment.NewBuilder(APIClient, systemtestsparams.HardRebootDeploymentName, nsName,
		map[string]string{"test": "hardreboot"}, *deployContainerCfg)
	createDeploy = createDeploy.WithNodeSelector(map[string]string{"kubernetes.io/hostname": nodeName})

	_, err = createDeploy.CreateAndWaitUntilReady(300 * time.Second)
	if err != nil {
		return nil, err
	}

	listOptions := metav1.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}).String(),
		LabelSelector: labels.SelectorFromSet(labels.Set{"test": "hardreboot"}).String(),
	}

	ipmiPods, err := pod.List(APIClient, nsName, listOptions)
	if err != nil {
		return nil, err
	}

	return ipmiPods, nil
}

// executeRebootCommand executes the ipmitool power cycle command.
func executeRebootCommand(ipmiPods []*pod.Builder) error {
	cmdToExec := []string{"ipmitool", "chassis", "power", "cycle"}

	klog.V(90).Infof("Exec cmd %v on pod %s", cmdToExec, ipmiPods[0].Definition.Name)
	_, err := ipmiPods[0].ExecCommand(cmdToExec)

	return err
}

// waitForNodeRebootCycle waits for the node to go down and come back up.
func waitForNodeRebootCycle(nodeName string) error {
	// Check if this is a single node cluster
	allNodes, err := nodes.List(APIClient, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list nodes to determine cluster size: %w", err)
	}

	isSingleNodeCluster := len(allNodes) == 1
	klog.V(90).Infof("Cluster has %d nodes, single node cluster: %v", len(allNodes), isSingleNodeCluster)

	if !isSingleNodeCluster {
		err = waitForNodeNotReady(nodeName)
		if err != nil {
			klog.V(90).Infof("Warning: Node %s did not become NotReady within timeout, continuing...", nodeName)
		} else {
			klog.V(90).Infof("Node %s successfully went NotReady", nodeName)
		}
	} else {
		klog.V(90).Infof("Single node cluster detected - skipping NotReady check")
		// wait for one minute for the node to reboot
		time.Sleep(1 * time.Minute)
	}

	return waitForNodeReady(nodeName)
}

// waitForNodeNotReady waits for the node to become NotReady.
func waitForNodeNotReady(nodeName string) error {
	klog.V(90).Infof("Waiting for node %s to become NotReady after power cycle", nodeName)

	return wait.PollUntilContextTimeout(
		context.TODO(),
		5*time.Second,
		10*time.Minute,
		true,
		func(ctx context.Context) (bool, error) {
			node, nodeErr := nodes.Pull(APIClient, nodeName)
			if nodeErr != nil {
				klog.V(90).Infof("Error pulling node %s: %v", nodeName, nodeErr)

				return false, nil // Node might be unreachable, which is expected
			}

			for _, condition := range node.Object.Status.Conditions {
				if condition.Type == corev1.NodeReady {
					if condition.Status != corev1.ConditionTrue {
						klog.V(90).Infof("Node %s is NotReady: %s", nodeName, condition.Reason)

						return true, nil
					}
				}
			}

			return false, nil
		})
}

// waitForNodeReady waits for the node to become Ready.
func waitForNodeReady(nodeName string) error {
	klog.V(90).Infof("Waiting for node %s to become Ready again", nodeName)

	// Wait for node to become Ready (indicating it's back up)
	err := wait.PollUntilContextTimeout(
		context.TODO(),
		10*time.Second,
		25*time.Minute,
		true,
		func(ctx context.Context) (bool, error) {
			node, nodeErr := nodes.Pull(APIClient, nodeName)
			if nodeErr != nil {
				klog.V(90).Infof("Error pulling node %s (expected during reboot): %v", nodeName, nodeErr)

				return false, nil // Continue polling, API might be unavailable during reboot
			}

			for _, condition := range node.Object.Status.Conditions {
				if condition.Type == corev1.NodeReady {
					if condition.Status == corev1.ConditionTrue {
						klog.V(90).Infof("Node %s is Ready: %s", nodeName, condition.Reason)

						return true, nil
					}
				}
			}

			klog.V(90).Infof("Node %s is still not Ready", nodeName)

			return false, nil
		})
	if err != nil {
		return fmt.Errorf("node %s did not become Ready within timeout: %w", nodeName, err)
	}

	klog.V(90).Infof("Node %s successfully came back online", nodeName)

	return nil
}

// waitForAPIServerReady waits for the OpenShift API server to be available.
func waitForAPIServerReady() error {
	klog.V(90).Infof("Waiting for OpenShift API server to be available")

	openshiftAPIDeploy, err := deployment.Pull(APIClient, "apiserver", "openshift-apiserver")
	if err != nil {
		return fmt.Errorf("failed to get OpenShift API deployment: %w", err)
	}

	err = openshiftAPIDeploy.WaitUntilCondition("Available", 5*time.Minute)
	if err != nil {
		return fmt.Errorf("OpenShift API server not available after node reboot: %w", err)
	}

	return nil
}

// cleanupRebootDeployment removes the hard reboot deployment.
func cleanupRebootDeployment(nsName string) error {
	createDeploy, err := deployment.Pull(APIClient, systemtestsparams.HardRebootDeploymentName, nsName)
	if err != nil {
		return err
	}

	err = createDeploy.DeleteAndWait(2 * time.Minute)

	return err
}

// KernelCrashKdump triggers a kernel crash dump which generates a vmcore dump.
func KernelCrashKdump(nodeName string) error {
	cmdToExec := []string{"chroot", "/rootfs", "/bin/sh", "-c", "rm -rf /var/crash/*"}

	klog.V(90).Infof("Remove any existing crash dumps. Exec cmd %v", cmdToExec)

	_, err := remote.ExecuteOnNodeWithDebugPod(cmdToExec, nodeName)
	if err != nil {
		return err
	}

	cmdToExec = []string{"chroot", "/rootfs", "/bin/sh", "-c",
		"echo 1 > /proc/sys/kernel/sysrq && echo c | tee /proc/sysrq-trigger"}

	klog.V(90).Infof("Trigerring kernel crash. Exec cmd %v", cmdToExec)

	_, err = remote.ExecuteOnNodeWithDebugPodWithTimeout(cmdToExec, nodeName, 15*time.Second)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			klog.V(90).Infof("Context timeout exceeded while triggering kernel crash, assuming it was triggered")

			return nil // context timeout exceeded, so we can assume the kernel crash was triggered
		}

		klog.V(90).Infof("Failed to trigger kernel crash: %v", err)

		return err
	}

	return nil
}

// SoftRebootNode executes systemctl reboot on a node.
func SoftRebootNode(nodeName string) error {
	cmdToExec := []string{"chroot", "/rootfs", "systemctl", "reboot"}

	_, err := remote.ExecuteOnNodeWithDebugPod(cmdToExec, nodeName)
	if err != nil {
		return err
	}

	return nil
}
