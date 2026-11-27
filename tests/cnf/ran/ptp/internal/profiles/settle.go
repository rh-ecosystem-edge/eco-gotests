package profiles

import (
	"fmt"
	"slices"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/processes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// reloadConvergeTimeout bounds how long to wait for the daemon to act on a PtpConfig write and finish restarting its
// processes. The daemon has been observed to take 60 to 80 seconds to notice a write and a further 30 to 40 seconds
// to bring every process back, so this leaves substantial headroom.
const reloadConvergeTimeout = 5 * time.Minute

// Baseline is the converged state of the cluster at the start of a spec. It is captured by [CaptureBaseline] in a
// BeforeEach and consumed by [RestoreAndSettle] in the matching AfterEach.
type Baseline struct {
	// Taken is when the baseline was captured, used to decide which PtpConfigs were written during the spec.
	Taken time.Time
	// Inventory is the set of linuxptp processes that were running when the cluster was last known to be healthy.
	Inventory processes.Inventory
}

// CaptureBaseline records the converged state of the cluster so that [RestoreAndSettle] can later tell both whether a
// PtpConfig was written during the spec and whether the daemon has finished acting on it.
//
// This must be called once the cluster is known to be healthy, which in practice means after the BeforeEach lock
// check. A baseline captured while the daemon is midway through a reload records only the processes that happen to be
// up at that moment, and would then be an unreachable target for the AfterEach.
func CaptureBaseline(client *clients.Settings) (*Baseline, error) {
	inventory, err := processes.SnapshotInventory(client, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to snapshot process inventory for baseline: %w", err)
	}

	return &Baseline{Taken: time.Now(), Inventory: inventory}, nil
}

// DaemonConfigWrites describes the configuration objects written during a spec and when the most recent of those
// writes happened.
type DaemonConfigWrites struct {
	// PtpConfigs are the PtpConfig objects that were written.
	PtpConfigs []runtimeclient.ObjectKey
	// HardwareProfiles are the RelatedPtpProfileName values of the HardwareConfig objects that were written. On
	// GNR-D the holdover settings live in a HardwareConfig rather than in the PtpConfig plugin section, and that
	// CR is reconciled by a separate controller on its own poll, so it has to be tracked alongside PtpConfigs.
	HardwareProfiles []string
	// Last is the most recent write time across both kinds of object.
	Last time.Time
}

// Any reports whether any configuration object was written.
func (writes *DaemonConfigWrites) Any() bool {
	return len(writes.PtpConfigs) > 0 || len(writes.HardwareProfiles) > 0
}

// DetectDaemonConfigWrites reports which PtpConfigs and HardwareConfigs were written since the given time and when the
// most recent of those writes happened. It reads the write times from metadata.managedFields, so it sees every write
// regardless of which client made it.
//
// Deriving this from the cluster rather than tracking the suite's own writes is deliberate. A test that modifies and
// then restores a config itself leaves nothing for a spec-level restore to detect, yet the daemon still has that write
// queued, and that is precisely the case that was slipping through.
func DetectDaemonConfigWrites(client *clients.Settings, since time.Time) (*DaemonConfigWrites, error) {
	// managedFields timestamps have a resolution of one second, so the comparison is made against a truncated
	// baseline. Erring towards reporting a write is safe: the convergence check then confirms the daemon has
	// already acted on it, which for a write that predates the baseline it will have.
	cutoff := since.Truncate(time.Second)
	writes := &DaemonConfigWrites{}

	ptpConfigList, err := ptp.ListPtpConfigs(client)
	if err != nil {
		return nil, fmt.Errorf("failed to list PtpConfigs to detect writes: %w", err)
	}

	for _, ptpConfig := range ptpConfigList {
		writeTime := latestManagedFieldsTime(ptpConfig.Definition.ManagedFields)
		if writeTime.Before(cutoff) {
			continue
		}

		writes.PtpConfigs = append(writes.PtpConfigs, runtimeclient.ObjectKey{
			Name:      ptpConfig.Definition.Name,
			Namespace: ptpConfig.Definition.Namespace,
		})

		if writeTime.After(writes.Last) {
			writes.Last = writeTime
		}
	}

	// HardwareConfig is only present on 4.22+ clusters with the CRD installed, so a failure to list it is not
	// fatal. This mirrors how GetNodeInfoMap treats the same CRD.
	hwConfigList, err := ptp.ListHardwareConfigs(client)
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to list HardwareConfigs to detect writes, skipping them: %v", err)

		return writes, nil
	}

	for _, hwConfig := range hwConfigList {
		writeTime := latestManagedFieldsTime(hwConfig.Definition.ManagedFields)
		if writeTime.Before(cutoff) {
			continue
		}

		if name := hwConfig.Definition.Spec.RelatedPtpProfileName; name != "" {
			writes.HardwareProfiles = append(writes.HardwareProfiles, name)
		}

		if writeTime.After(writes.Last) {
			writes.Last = writeTime
		}
	}

	return writes, nil
}

// NodesForWrites returns the names of the nodes affected by the written configuration objects. The result is sorted so
// that callers get a deterministic ordering.
//
// A reload is node-global: the daemon stops and restarts every linuxptp process on a node regardless of which profile
// changed. It is not cluster-wide though, so only the nodes a written config actually applies to will restart, and
// waiting on the others would simply time out.
func NodesForWrites(client *clients.Settings, writes *DaemonConfigWrites) ([]string, error) {
	if writes == nil || !writes.Any() {
		return nil, nil
	}

	nodeInfoMap, err := GetNodeInfoMap(client)
	if err != nil {
		return nil, fmt.Errorf("failed to get node info map while resolving nodes for writes: %w", err)
	}

	writtenConfigs := make(map[runtimeclient.ObjectKey]struct{}, len(writes.PtpConfigs))
	for _, config := range writes.PtpConfigs {
		writtenConfigs[config] = struct{}{}
	}

	writtenProfiles := make(map[string]struct{}, len(writes.HardwareProfiles))
	for _, profile := range writes.HardwareProfiles {
		writtenProfiles[profile] = struct{}{}
	}

	var nodeNames []string

	for nodeName, nodeInfo := range nodeInfoMap {
		for _, profileInfo := range nodeInfo.Profiles {
			_, byConfig := writtenConfigs[profileInfo.Reference.ConfigReference]
			_, byProfile := writtenProfiles[profileInfo.Reference.ProfileName]

			if byConfig || byProfile {
				nodeNames = append(nodeNames, nodeName)

				break
			}
		}
	}

	slices.Sort(nodeNames)

	return nodeNames, nil
}

// latestManagedFieldsTime returns the most recent time any field manager wrote the object. It returns the zero time
// when the object carries no managedFields entries with a timestamp.
func latestManagedFieldsTime(managedFields []metav1.ManagedFieldsEntry) time.Time {
	var latest time.Time

	for _, entry := range managedFields {
		if entry.Time == nil {
			continue
		}

		if entry.Time.Time.After(latest) {
			latest = entry.Time.Time
		}
	}

	return latest
}

// RestoreAndSettle restores the saved PtpConfigs and then blocks until the cluster is back at the healthy baseline. It
// is the cleanup counterpart to [SavePtpConfigs] and [CaptureBaseline], and is intended to be the whole body of an
// AfterEach for any suite that modifies PtpConfigs.
//
// Writing a PtpConfig only queues work. The daemon notices the change on a later poll and only then restarts ptp4l and
// phc2sys, a minute or more after the write returns. Neither the process status metric nor the clock state metric can
// see that pending reload, because the daemon is itself the exporter and keeps publishing healthy values for the
// processes it has not torn down yet. A spec that returns in that window hands the next one a daemon that is about to
// restart every process.
//
// So convergence is established by observing the processes directly: every linuxptp process on the affected nodes
// must match the baseline set and must have started after the most recent write. Only then are the metric gates
// applied, to confirm the restarted processes are actually healthy.
func RestoreAndSettle(
	client *clients.Settings,
	prometheusAPI prometheusv1.API,
	savedPtpConfigs []*ptp.PtpConfigBuilder,
	baseline *Baseline) error {
	_, err := RestorePtpConfigs(client, savedPtpConfigs)
	if err != nil {
		return fmt.Errorf("failed to restore PtpConfigs: %w", err)
	}

	// Settling happens after the restore so that the restore's own write, if it made one, is accounted for.
	return Settle(client, prometheusAPI, baseline)
}

// Settle blocks until the daemon has acted on every configuration write made during the spec and the cluster is back
// at the healthy baseline. It is for suites that perform their own cleanup and so have nothing for
// [RestoreAndSettle] to restore, but that still leave a reload queued behind them.
//
// Both the GNR-D HardwareConfig path and the PtpConfig path are covered, because both restart the daemon's processes
// and both are reconciled on a poll rather than synchronously.
func Settle(client *clients.Settings, prometheusAPI prometheusv1.API, baseline *Baseline) error {
	if baseline == nil {
		return fmt.Errorf("cannot settle without a baseline captured by CaptureBaseline")
	}

	writes, err := DetectDaemonConfigWrites(client, baseline.Taken)
	if err != nil {
		return fmt.Errorf("failed to detect daemon config writes: %w", err)
	}

	switch {
	case !writes.Any():
		klog.V(tsparams.LogLevel).Info("No PtpConfig or HardwareConfig writes during this spec, nothing to settle")
	case writes.Last.IsZero():
		// Without a write time there is nothing to compare process ages against. Falling back to the metric
		// gates alone matches the behaviour from before this check existed, so it degrades rather than hangs.
		klog.V(tsparams.LogLevel).Info(
			"Detected config writes but no managedFields timestamp, skipping the convergence check")
	default:
		nodeNames, err := NodesForWrites(client, writes)
		if err != nil {
			return fmt.Errorf("failed to resolve nodes for written configs: %w", err)
		}

		klog.V(tsparams.LogLevel).Infof(
			"Waiting for the daemon to converge on %d PtpConfig(s) and %d HardwareConfig profile(s) "+
				"last written at %s, on nodes %v",
			len(writes.PtpConfigs), len(writes.HardwareProfiles), writes.Last, nodeNames)

		err = processes.WaitForReloadConverged(
			client, nodeNames, baseline.Inventory, writes.Last, reloadConvergeTimeout)
		if err != nil {
			return fmt.Errorf("failed to converge after config writes: %w", err)
		}
	}

	err = metrics.EnsureClocksAreLocked(prometheusAPI)
	if err != nil {
		return fmt.Errorf("failed to ensure clocks are locked while settling: %w", err)
	}

	return nil
}
