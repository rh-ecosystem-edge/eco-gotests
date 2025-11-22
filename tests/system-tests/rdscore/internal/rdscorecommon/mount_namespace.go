package rdscorecommon

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

//nolint:funlen
func mountNamespaceEncapsulation(nodeLabel string) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Check encapsulation for nodes %s", nodeLabel)

	var (
		nodeList []*nodes.Builder
		err      error
		ctx      SpecContext
	)

	By("Retrieve nodes list")

	Eventually(func() bool {
		nodeList, err = nodes.List(
			APIClient,
			metav1.ListOptions{LabelSelector: nodeLabel},
		)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list nodes: %v", err)

			return false
		}

		return len(nodeList) > 0
	}).WithContext(ctx).WithTimeout(1*time.Minute).WithPolling(5*time.Second).Should(BeTrue(),
		fmt.Sprintf("Failed to find pods matching label: %q", nodeLabel))

	for _, node := range nodeList {
		By("Open debug pod and check the systemd, kubelet and CRI-O mount namespaces")

		nodeName := node.Definition.Name

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Check the systemd mount namespace on node %s", nodeName)

		systemdMountNsCmd := []string{"chroot", "/rootfs", "/bin/sh", "-c", "readlink /proc/1/ns/mnt"}

		systemdMountNsOutput, err := remote.ExecuteOnNodeWithDebugPod(systemdMountNsCmd, nodeName)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to execute %s cmd on the node %s due to %v",
			systemdMountNsCmd, nodeName, err))

		systemdMountNs := strings.Split(systemdMountNsOutput, ":")[1]

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Check the kubelet mount namespace on node %s", nodeName)

		kubeletMountNsCmd := []string{"chroot", "/rootfs", "/bin/sh", "-c", "readlink /proc/$(pgrep kubelet)/ns/mnt"}

		var kubeletMountNs string

		err = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, time.Minute, true,
			func(context.Context) (bool, error) {
				kubeletMountNsOutput, err := remote.ExecuteOnNodeWithDebugPod(kubeletMountNsCmd, nodeName)
				if err != nil {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to run command %s on node %s due to %v",
						kubeletMountNsCmd, nodeName, err)

					return false, nil
				}

				if len(strings.Split(kubeletMountNsOutput, ":")) < 2 {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Malformed output: %q", kubeletMountNsOutput)

					return false, nil
				}

				kubeletMountNs = strings.Split(kubeletMountNsOutput, ":")[1]

				return true, nil
			})
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to check kubelet mount namespace")

			Fail(fmt.Sprintf("Failed to check kubelet mount namespace: %v", err))
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Check the CRI-O mount namespace on node %s", nodeName)

		crioMountNsCmd := []string{"chroot", "/rootfs", "/bin/sh", "-c", "readlink /proc/$(pgrep crio)/ns/mnt"}

		var crioMountNs string

		err = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, time.Minute, true,
			func(context.Context) (bool, error) {
				crioMountNsOutput, err := remote.ExecuteOnNodeWithDebugPod(crioMountNsCmd, nodeName)
				if err != nil {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to execute %s cmd on the node %s due to %v",
						crioMountNsCmd, nodeName, err)

					return false, nil
				}

				if len(strings.Split(crioMountNsOutput, ":")) < 2 {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Malformed output: %q", crioMountNsOutput)

					return false, nil
				}

				crioMountNs = strings.Split(crioMountNsOutput, ":")[1]

				return true, nil
			})
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to check CRI-O mount namespace")

			Fail(fmt.Sprintf("Failed to check CRI-O mount namespace: %v", err))
		}

		By("Check that encapsulation is in effect")

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Checking if kubelet and CRI-O are in the same mount namespace")

		Expect(kubeletMountNs == crioMountNs).To(Equal(true),
			fmt.Sprintf("General mount namespace failure; kubelet and CRI-O have to be in the same mount namespace;"+
				"kubelet mount namespace: %s; CRI-O mount namespace: %s", kubeletMountNs, crioMountNs))

		Expect(systemdMountNs != crioMountNs).To(Equal(true),
			fmt.Sprintf("Encapsulation is not in effect; systemd have to be in a different mount namespace to "+
				"kubelet and CRI-O; systemd mount namespace: %s; kubelet mount namespace: %s; CRI-O mount namespace: %s",
				systemdMountNs, kubeletMountNs, crioMountNs))

		By("Inspecting encapsulated namespaces")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Check the systemd mount namespace on node %s", nodeName)

		inspectingCmd := []string{"chroot", "/rootfs", "/bin/sh", "-c", "findmnt -n -oPROPAGATION /run/kubens"}

		out, err := remote.ExecuteOnNodeWithDebugPod(inspectingCmd, nodeName)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to execute %s cmd on the node %s due to %v",
			inspectingCmd, nodeName, err))
		Expect(strings.Contains(out, "private,slave")).To(BeTrue(),
			fmt.Sprintf("propagation of the directory does not contain a namespace mount pin for the node %s; %s",
				nodeName, out))
	}
}

// VerifyMountNamespaceOnControlPlane check mount namespace on Control Plane nodes.
func VerifyMountNamespaceOnControlPlane(ctx SpecContext) {
	mountNamespaceEncapsulation(RDSCoreConfig.KDumpCPNodeLabel)
}

// VerifyMountNamespaceOnWorkerMCP check mount namespace service on nodes in "Worker" MCP.
func VerifyMountNamespaceOnWorkerMCP(ctx SpecContext) {
	mountNamespaceEncapsulation(RDSCoreConfig.KDumpWorkerMCPNodeLabel)
}

// VerifyMountNamespaceOnCNFMCP check mount namespace service on nodes in "CNF" MCP.
func VerifyMountNamespaceOnCNFMCP(ctx SpecContext) {
	mountNamespaceEncapsulation(RDSCoreConfig.KDumpCNFMCPNodeLabel)
}
