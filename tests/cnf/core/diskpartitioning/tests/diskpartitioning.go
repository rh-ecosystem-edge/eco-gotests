package tests

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/diskpartitioning/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/internal/coreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var workerListOptions = metav1.ListOptions{
	LabelSelector: tsparams.WorkerNodeLabel,
}

var _ = Describe("Disk Partitioning", Label(tsparams.LabelDiskPartitioningTestCases), func() {
	It("should have /var/lib/containers mounted on worker nodes",
		reportxml.ID("TODO-1"), func() {
			By("Checking that /var/lib/containers is a separate mount point on worker nodes")

			outputs, err := cluster.ExecCmdWithStdoutWithRetries(
				coreinittools.APIClient, 3, 10*time.Second,
				fmt.Sprintf("findmnt -n %s", tsparams.ContainersMountPoint),
				workerListOptions,
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to run findmnt on worker nodes")
			Expect(outputs).ToNot(BeEmpty(), "No worker nodes returned output")

			for host, output := range outputs {
				output = strings.TrimSpace(output)
				klog.V(tsparams.LogLevel).Infof("Node %s: findmnt %s = %q", host, tsparams.ContainersMountPoint, output)
				Expect(output).ToNot(BeEmpty(),
					"%s is not a separate mount point on node %s", tsparams.ContainersMountPoint, host)
			}
		})

	It("should have an XFS filesystem on /var/lib/containers on worker nodes",
		reportxml.ID("TODO-2"), func() {
			By("Checking the filesystem type of /var/lib/containers on worker nodes")

			fsTypes, err := cluster.ExecCmdWithStdoutWithRetries(
				coreinittools.APIClient, 3, 10*time.Second,
				fmt.Sprintf("findmnt -n -o FSTYPE %s", tsparams.ContainersMountPoint),
				workerListOptions,
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to check filesystem type on worker nodes")
			Expect(fsTypes).ToNot(BeEmpty(), "No worker nodes returned output")

			for host, fsType := range fsTypes {
				fsType = strings.TrimSpace(fsType)
				klog.V(tsparams.LogLevel).Infof("Node %s: %s fstype = %q", host, tsparams.ContainersMountPoint, fsType)
				Expect(fsType).To(Equal(tsparams.ContainersFSType),
					"Expected %s filesystem on node %s, got %q", tsparams.ContainersFSType, host, fsType)
			}
		})

	It("should have /var/lib/containers sourced from the containers partition on worker nodes",
		reportxml.ID("TODO-3"), func() {
			By("Checking the mount source of /var/lib/containers on worker nodes")

			sources, err := cluster.ExecCmdWithStdoutWithRetries(
				coreinittools.APIClient, 3, 10*time.Second,
				fmt.Sprintf("findmnt -n -o SOURCE %s", tsparams.ContainersMountPoint),
				workerListOptions,
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to check mount source on worker nodes")
			Expect(sources).ToNot(BeEmpty(), "No worker nodes returned output")

			for host, source := range sources {
				source = strings.TrimSpace(source)
				klog.V(tsparams.LogLevel).Infof("Node %s: %s source = %q", host, tsparams.ContainersMountPoint, source)
				Expect(source).To(ContainSubstring(tsparams.ContainersPartLabel),
					"Expected source containing %q on node %s, got %q",
					tsparams.ContainersPartLabel, host, source)
			}
		})

	It("should have the var-lib-containers systemd mount unit active on worker nodes",
		reportxml.ID("TODO-4"), func() {
			By("Checking the systemd mount unit status on worker nodes")

			statuses, err := cluster.ExecCmdWithStdoutWithRetries(
				coreinittools.APIClient, 3, 10*time.Second,
				fmt.Sprintf("systemctl is-active %s", tsparams.ContainersMountUnit),
				workerListOptions,
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to check systemd mount unit status on worker nodes")
			Expect(statuses).ToNot(BeEmpty(), "No worker nodes returned output")

			for host, status := range statuses {
				status = strings.TrimSpace(status)
				klog.V(tsparams.LogLevel).Infof("Node %s: %s status = %q", host, tsparams.ContainersMountUnit, status)
				Expect(status).To(Equal("active"),
					"Expected %s to be active on node %s, got %q", tsparams.ContainersMountUnit, host, status)
			}
		})
})
