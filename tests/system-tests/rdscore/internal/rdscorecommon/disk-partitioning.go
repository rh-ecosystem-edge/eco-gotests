package rdscorecommon

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

func runCmdOnWorkerNodes(ctx context.Context, cmd []string, description string) map[string]string {
	By(fmt.Sprintf("Listing worker nodes for %s", description))

	nodeList, err := nodes.List(APIClient, RDSCoreConfig.WorkerLabelListOption)
	Expect(err).ToNot(HaveOccurred(), "Failed to list worker nodes")
	Expect(nodeList).ToNot(BeEmpty(), "No worker nodes found")

	results := make(map[string]string)

	for _, node := range nodeList {
		nodeName := node.Definition.Name

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Running %q on node %s", description, nodeName)

		var output string

		err := wait.PollUntilContextTimeout(ctx, 3*time.Second, time.Minute, true,
			func(context.Context) (bool, error) {
				out, execErr := remote.ExecuteOnNodeWithDebugPod(cmd, nodeName)
				if execErr != nil {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
						"Failed to run %q on node %s: %v", description, nodeName, execErr)

					return false, nil
				}

				output = out

				return true, nil
			})
		Expect(err).ToNot(HaveOccurred(), "Failed to run %q on node %s", description, nodeName)

		results[nodeName] = strings.TrimSpace(output)
	}

	return results
}

// VerifyContainersMountPointOnWorkers verifies /var/lib/containers is a separate mount point on worker nodes.
func VerifyContainersMountPointOnWorkers(ctx SpecContext) {
	cmd := []string{"chroot", "/rootfs", "/bin/sh", "-c",
		fmt.Sprintf("findmnt -n %s", rdscoreparams.ContainersMountPoint)}

	outputs := runCmdOnWorkerNodes(ctx, cmd, "verify containers mount point")

	for nodeName, output := range outputs {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Node %s: findmnt %s = %q", nodeName, rdscoreparams.ContainersMountPoint, output)
		Expect(output).ToNot(BeEmpty(),
			"%s is not a separate mount point on node %s", rdscoreparams.ContainersMountPoint, nodeName)
	}
}

// VerifyContainersXFSFilesystemOnWorkers verifies the filesystem type on /var/lib/containers is XFS on worker nodes.
func VerifyContainersXFSFilesystemOnWorkers(ctx SpecContext) {
	cmd := []string{"chroot", "/rootfs", "/bin/sh", "-c",
		fmt.Sprintf("findmnt -n -o FSTYPE %s", rdscoreparams.ContainersMountPoint)}

	outputs := runCmdOnWorkerNodes(ctx, cmd, "verify containers filesystem type")

	for nodeName, fsType := range outputs {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Node %s: %s fstype = %q", nodeName, rdscoreparams.ContainersMountPoint, fsType)
		Expect(fsType).To(Equal(rdscoreparams.ContainersFSType),
			"Expected %s filesystem on node %s, got %q", rdscoreparams.ContainersFSType, nodeName, fsType)
	}
}

// VerifyContainersPartitionSourceOnWorkers verifies the mount source contains the containers partition label.
func VerifyContainersPartitionSourceOnWorkers(ctx SpecContext) {
	cmd := []string{"chroot", "/rootfs", "/bin/sh", "-c",
		fmt.Sprintf("findmnt -n -o SOURCE %s", rdscoreparams.ContainersMountPoint)}

	outputs := runCmdOnWorkerNodes(ctx, cmd, "verify containers mount source")

	for nodeName, source := range outputs {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Node %s: %s source = %q", nodeName, rdscoreparams.ContainersMountPoint, source)
		Expect(source).To(ContainSubstring(rdscoreparams.ContainersPartLabel),
			"Expected source containing %q on node %s, got %q",
			rdscoreparams.ContainersPartLabel, nodeName, source)
	}
}

// VerifyContainersMountUnitOnWorkers verifies the var-lib-containers systemd mount unit is active on worker nodes.
func VerifyContainersMountUnitOnWorkers(ctx SpecContext) {
	cmd := []string{"chroot", "/rootfs", "/bin/sh", "-c",
		fmt.Sprintf("systemctl is-active %s", rdscoreparams.ContainersMountUnit)}

	outputs := runCmdOnWorkerNodes(ctx, cmd, "verify containers mount unit")

	for nodeName, status := range outputs {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Node %s: %s status = %q", nodeName, rdscoreparams.ContainersMountUnit, status)
		Expect(status).To(Equal("active"),
			"Expected %s to be active on node %s, got %q", rdscoreparams.ContainersMountUnit, nodeName, status)
	}
}
