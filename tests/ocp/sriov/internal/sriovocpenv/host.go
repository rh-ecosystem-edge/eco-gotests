package sriovocpenv

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

func execOnSriovConfigDaemon(nodeName, command string) error {
	klog.V(90).Infof("Running command %q on sriov-network-config-daemon on node %s", command, nodeName)

	pods, err := pod.List(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace, metav1.ListOptions{
		LabelSelector: "app=sriov-network-config-daemon",
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
	if err != nil {
		return fmt.Errorf("failed to list config daemon pods on node %s: %w", nodeName, err)
	}

	if len(pods) == 0 {
		return fmt.Errorf("failed to find config daemon pod on node %s", nodeName)
	}

	output, err := pods[0].ExecCommand([]string{"bash", "-c", command})
	if err != nil {
		return fmt.Errorf("failed to execute command on node %s: %s %w",
			nodeName, output.String(), err)
	}

	return nil
}

// RunCommandOnHostNetworkPod runs a command on a privileged host-network pod scheduled on the given node.
func RunCommandOnHostNetworkPod(nodeName, nsName, command string) (string, error) {
	klog.V(90).Infof("Running command %s on the host network pod on node %s",
		command, nodeName)

	testPod, err := pod.NewBuilder(APIClient, "hostnetworkpod", nsName, SriovOcpConfig.OcpSriovTestContainer).
		DefineOnNode(nodeName).WithPrivilegedFlag().WithHostNetwork().CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	if err != nil {
		return "", err
	}

	defer func() {
		_, deleteErr := testPod.DeleteAndWait(tsparams.DefaultTimeout)
		if deleteErr != nil {
			klog.V(90).Infof("failed to delete hostnetwork pod %s: %v", testPod.Definition.Name, deleteErr)
		}
	}()

	output, err := testPod.ExecCommand([]string{"/bin/bash", "-c", command})
	if err != nil {
		return "", err
	}

	return output.String(), nil
}
