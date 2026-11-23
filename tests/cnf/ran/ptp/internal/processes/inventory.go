package processes

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// inventoryProcesses are the linuxptp processes tracked by an inventory. A PtpConfig reload tears down and restarts
// every one of them on the affected node, regardless of which profile changed, so observing this set is enough to
// decide whether a reload has completed.
var inventoryProcesses = []PtpProcess{Ptp4l, Phc2sys, Ts2phc}

// inventoryCommand lists every process with its PID, age in seconds, and full command line. Age is read on the node
// rather than derived from a timestamp so that comparisons are between two durations, which cancels any constant
// clock offset between the node and the test runner. That matters here because clock correctness is what the suite
// is testing.
const inventoryCommand = "ps -eo pid=,etimes=,args="

// ProcessInfo describes one running linuxptp process.
type ProcessInfo struct {
	// PID is the process ID, which changes every time the daemon restarts the process.
	PID string
	// Age is how long the process has been running, as measured on the node.
	Age time.Duration
}

// NodeInventory maps a process key to its running process. The key is the executable name joined with the config
// index it was started with, for example "ptp4l.0" or "phc2sys.0". Unlike the PID, the key is stable across restarts,
// which makes it usable for comparing the set of processes before and after a reload.
type NodeInventory map[string]ProcessInfo

// Keys returns the sorted process keys in the inventory.
func (inventory NodeInventory) Keys() []string {
	return slices.Sorted(maps.Keys(inventory))
}

// Inventory maps node names to the linuxptp processes running on them.
type Inventory map[string]NodeInventory

// SnapshotInventory records the linuxptp processes running on each of the provided nodes. When nodeNames is empty it
// snapshots every node running a PTP daemon.
//
// Callers should take the snapshot at a point where the cluster is known to be converged, typically right after the
// baseline lock check in a BeforeEach. A snapshot taken while the daemon is midway through a reload records only the
// processes that happen to be up at that instant, which would make it useless as an expected set.
func SnapshotInventory(client *clients.Settings, nodeNames []string) (Inventory, error) {
	if len(nodeNames) == 0 {
		nodes, err := ptpdaemon.ListPtpDaemonNodes(client)
		if err != nil {
			return nil, fmt.Errorf("failed to list PTP daemon nodes for inventory: %w", err)
		}

		for _, node := range nodes {
			nodeNames = append(nodeNames, node.Definition.Name)
		}
	}

	inventory := make(Inventory, len(nodeNames))

	for _, nodeName := range nodeNames {
		nodeInventory, err := snapshotNode(client, nodeName)
		if err != nil {
			return nil, err
		}

		inventory[nodeName] = nodeInventory
	}

	return inventory, nil
}

// WaitForReloadConverged waits until every provided node is running exactly the processes recorded in expected and all
// of them were started after the given time. It polls every 10 seconds until the timeout.
//
// Both conditions are needed. Requiring the processes to be newer than the write proves the daemon has acted on it,
// which neither the process status metric nor the clock state metric can show while a reload is still queued.
// Requiring the key set to match proves the reload finished: the daemon stops every process before starting any, and
// it staggers the restarts by several seconds, so a check that only looked at the processes currently up would pass
// during that window while most of them were still missing.
func WaitForReloadConverged(
	client *clients.Settings,
	nodeNames []string,
	expected Inventory,
	since time.Time,
	timeout time.Duration) error {
	if len(nodeNames) == 0 {
		return nil
	}

	var lastErr error

	err := wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			lastErr = checkReloadConverged(client, nodeNames, expected, since)
			if lastErr != nil {
				klog.V(tsparams.LogLevel).Infof("Daemon reload not converged yet: %v", lastErr)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		// The error from the last check says which node and process blocked convergence, which is far more
		// useful than the bare timeout, so prefer it when there is one.
		if lastErr != nil {
			return fmt.Errorf("failed to wait for daemon reload to converge after %s: %w", timeout, lastErr)
		}

		return fmt.Errorf("failed to wait for daemon reload to converge after %s: %w", timeout, err)
	}

	return nil
}

// checkReloadConverged performs a single convergence check across the provided nodes. It returns nil when every node
// matches the expected process set and every process started after the given time.
func checkReloadConverged(
	client *clients.Settings, nodeNames []string, expected Inventory, since time.Time) error {
	// Elapsed is recomputed on every check so that it is compared against a process age read at roughly the same
	// moment. A process is newer than the write when it has been running for less time than has passed since.
	elapsed := time.Since(since)

	for _, nodeName := range nodeNames {
		expectedNode, ok := expected[nodeName]
		if !ok {
			return fmt.Errorf("no expected process inventory recorded for node %s", nodeName)
		}

		actualNode, err := snapshotNode(client, nodeName)
		if err != nil {
			return err
		}

		if !slices.Equal(expectedNode.Keys(), actualNode.Keys()) {
			return fmt.Errorf("node %s is running processes %v but expected %v",
				nodeName, actualNode.Keys(), expectedNode.Keys())
		}

		for key, info := range actualNode {
			if info.Age >= elapsed {
				return fmt.Errorf(
					"node %s process %s (pid %s) has been running for %s, which is longer than the %s since "+
						"the PtpConfig was written, so the daemon has not restarted it yet",
					nodeName, key, info.PID, info.Age, elapsed)
			}
		}
	}

	return nil
}

// snapshotNode records the linuxptp processes running on a single node.
func snapshotNode(client *clients.Settings, nodeName string) (NodeInventory, error) {
	output, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, inventoryCommand,
		ptpdaemon.WithRetries(3),
		ptpdaemon.WithRetryOnError(true),
		ptpdaemon.WithRetryOnEmptyOutput(true),
		ptpdaemon.WithRetryDelay(3*time.Second))
	if err != nil {
		return nil, fmt.Errorf("failed to list processes on node %s: %w", nodeName, err)
	}

	inventory := make(NodeInventory)

	for line := range strings.SplitSeq(output, "\n") {
		key, info, ok := parseProcessLine(line)
		if !ok {
			continue
		}

		inventory[key] = info
	}

	return inventory, nil
}

// parseProcessLine parses one line of [inventoryCommand] output into an inventory entry. It returns false when the
// line is malformed or is not one of the [inventoryProcesses].
func parseProcessLine(line string) (string, ProcessInfo, bool) {
	fields := strings.Fields(line)

	// A usable line is the PID, the age, and at least the executable.
	if len(fields) < 3 {
		return "", ProcessInfo{}, false
	}

	pid, ageSeconds, args := fields[0], fields[1], fields[2:]

	seconds, err := strconv.Atoi(ageSeconds)
	if err != nil {
		return "", ProcessInfo{}, false
	}

	process, ok := matchInventoryProcess(args)
	if !ok {
		return "", ProcessInfo{}, false
	}

	key := string(process)

	// The daemon runs one ptp4l per profile, so the executable name alone is not unique. The config index it was
	// started with is, and unlike the PID it survives a restart.
	if matches := ptpProcessRegex.FindStringSubmatch(strings.Join(args, " ")); matches != nil {
		key = fmt.Sprintf("%s.%s", process, matches[2])
	}

	return key, ProcessInfo{PID: pid, Age: time.Duration(seconds) * time.Second}, true
}

// matchInventoryProcess returns the tracked process the command line belongs to. The executable is located by scanning
// the arguments rather than taking the first one, because the daemon launches these processes through chrt.
func matchInventoryProcess(args []string) (PtpProcess, bool) {
	for _, arg := range args {
		// Only the executable itself is considered, so that a process referencing another process's config or
		// socket path is not mistaken for it.
		base := arg[strings.LastIndex(arg, "/")+1:]

		for _, process := range inventoryProcesses {
			if base == string(process) {
				return process, true
			}
		}
	}

	return "", false
}
