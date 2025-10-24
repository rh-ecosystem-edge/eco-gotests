package reboot

import (
	"context"
	"errors"
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	systemtestsscc "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/scc"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsinittools"
	systemtestsparams "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
)

// HardRebootNode executes ipmitool chassis power cycle on a node.
func HardRebootNode(nodeName string, nsName string) error {
	err := systemtestsscc.AddPrivilegedSCCtoDefaultSA(nsName)
	if err != nil {
		return err
	}

	rmiCmdToExec := []string{"chroot", "/rootfs", "/bin/sh", "-c", "podman rmi -f " + SystemTestsTestConfig.IpmiToolImage}
	glog.V(90).Infof("Cleaning up any of the existing ipmitool images. Exec cmd %v", rmiCmdToExec)
	_, err = remote.ExecuteOnNodeWithDebugPod(rmiCmdToExec, nodeName)

	if err != nil {
		return err
	}

	deployContainer := pod.NewContainerBuilder(systemtestsparams.HardRebootDeploymentName,
		SystemTestsTestConfig.IpmiToolImage, []string{"sleep", "86400"})

	trueVar := true
	deployContainer = deployContainer.WithSecurityContext(&corev1.SecurityContext{Privileged: &trueVar})
	deployContainer = deployContainer.WithImagePullPolicy(corev1.PullAlways)

	deployContainerCfg, err := deployContainer.GetContainerCfg()
	if err != nil {
		return err
	}

	createDeploy := deployment.NewBuilder(APIClient, systemtestsparams.HardRebootDeploymentName, nsName,
		map[string]string{"test": "hardreboot"}, *deployContainerCfg)
	createDeploy = createDeploy.WithNodeSelector(map[string]string{"kubernetes.io/hostname": nodeName})

	_, err = createDeploy.CreateAndWaitUntilReady(300 * time.Second)
	if err != nil {
		return err
	}

	listOptions := metav1.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}).String(),
		LabelSelector: labels.SelectorFromSet(labels.Set{"test": "hardreboot"}).String(),
	}
	ipmiPods, err := pod.List(APIClient, nsName, listOptions)

	if err != nil {
		return err
	}

	// pull openshift apiserver deployment object to wait for after the node reboot.
	openshiftAPIDeploy, err := deployment.Pull(APIClient, "apiserver", "openshift-apiserver")

	if err != nil {
		return err
	}

	cmdToExec := []string{"ipmitool", "chassis", "power", "cycle"}

	glog.V(90).Infof("Exec cmd %v on pod %s", cmdToExec, ipmiPods[0].Definition.Name)
	_, err = ipmiPods[0].ExecCommand(cmdToExec)

	if err != nil {
		return err
	}

	// wait for one minute for the node to reboot
	time.Sleep(1 * time.Minute)

	// wait for the openshift apiserver deployment to be available
	err = openshiftAPIDeploy.WaitUntilCondition("Available", 5*time.Minute)

	if err != nil {
		return err
	}

	err = createDeploy.DeleteAndWait(2 * time.Minute)

	if err != nil {
		return err
	}

	return nil
}

// KernelCrashKdump triggers a kernel crash dump which generates a vmcore dump.
func KernelCrashKdump(nodeName string) error {
	cmdToExec := []string{"chroot", "/rootfs", "/bin/sh", "-c", "rm -rf /var/crash/*"}

	glog.V(90).Infof("Remove any existing crash dumps. Exec cmd %v", cmdToExec)
	_, err := remote.ExecuteOnNodeWithDebugPod(cmdToExec, nodeName)

	if err != nil {
		return err
	}

	cmdToExec = []string{"chroot", "/rootfs", "/bin/sh", "-c",
		"echo 1 > /proc/sys/kernel/sysrq && echo c | tee /proc/sysrq-trigger"}

	glog.V(90).Infof("Trigerring kernel crash. Exec cmd %v", cmdToExec)
	_, err = remote.ExecuteOnNodeWithDebugPodWithTimeout(cmdToExec, nodeName, 15*time.Second)

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			glog.V(90).Infof("Context timeout exceeded while triggering kernel crash, assuming it was triggered")

			return nil // context timeout exceeded, so we can assume the kernel crash was triggered
		}

		glog.V(90).Infof("Failed to trigger kernel crash: %v", err)

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
