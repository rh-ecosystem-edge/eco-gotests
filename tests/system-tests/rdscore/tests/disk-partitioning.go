package rds_core_system_test

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscorecommon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

var _ = Describe(
	"Disk Partitioning",
	Ordered,
	ContinueOnFailure,
	Label(rdscoreparams.LabelDiskPartitioning), func() {
		It("Verify /var/lib/containers is a separate mount point on worker nodes",
			Label("disk-partitioning-mount-point"),
			reportxml.ID("TODO-1"),
			rdscorecommon.VerifyContainersMountPointOnWorkers)

		It("Verify /var/lib/containers has XFS filesystem on worker nodes",
			Label("disk-partitioning-xfs"),
			reportxml.ID("TODO-2"),
			rdscorecommon.VerifyContainersXFSFilesystemOnWorkers)

		It("Verify /var/lib/containers mount source contains containers partition label on worker nodes",
			Label("disk-partitioning-source"),
			reportxml.ID("TODO-3"),
			rdscorecommon.VerifyContainersPartitionSourceOnWorkers)

		It("Verify var-lib-containers systemd mount unit is active on worker nodes",
			Label("disk-partitioning-mount-unit"),
			reportxml.ID("TODO-4"),
			rdscorecommon.VerifyContainersMountUnitOnWorkers)
	})
